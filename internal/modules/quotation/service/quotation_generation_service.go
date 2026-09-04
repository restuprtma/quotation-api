package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/pkg/logger"
)

// Pipeline-wide constants. MaxProposalAttempts bounds the budget optimization
// loop; DefaultAttemptTimeout bounds EVERY individual AI provider call so a
// generate request can never hang indefinitely.
const (
	MaxProposalAttempts    = 3
	DefaultAttemptTimeout  = 180 * time.Second
	SafetyEnvelopeFileName = "quotation_safety_v1.txt"
	SafetyEnvelopeVersion  = "quotation_safety_v1"
	// DefaultLegalTemplateCode selects the company's standard legal template.
	// MUST stay identical to the code seeded by SettingsService.
	// EnsureDefaultAIAssets so tenants using default assets work out of the box.
	DefaultLegalTemplateCode = "QUOTATION_TERMS"

	dedicatedSquadScheme = "dedicated_squad"
)

// Safe error codes surfaced to callers. Values match the roadmap error list;
// messages are STATIC — they never embed prompts, provider payloads, keys,
// budget figures, or rate card internals.
const (
	ErrCodeSettingsIncomplete  = "settings_incomplete"
	ErrCodeInvalidClient       = "invalid_client"
	ErrCodeInvalidRateCard     = "invalid_rate_card"
	ErrCodeRateItemMissing     = "rate_item_missing"
	ErrCodeBudgetUnfeasible    = "budget_unfeasible"
	ErrCodeAllocationInvalid   = "allocation_invalid"
	ErrCodeProviderInvalidCred = "provider_invalid_credential"
	ErrCodeProviderRateLimited = "provider_rate_limited"
	ErrCodeProviderTimeout     = "provider_timeout"
	ErrCodeUnsafeHTML          = "unsafe_html"
	ErrCodeGenerationFailed    = "generation_failed"

	// Session 5 regeneration-specific safe codes.
	ErrCodeRegenerateRateLimited = "regenerate_rate_limited"
	ErrCodeRevisionConflict      = "revision_conflict"
)

// GenerationError is the ONLY error type returned by Generate. Error() prints
// just the code, so even accidental logging of the error stays safe.
type GenerationError struct {
	Code string
	// Summary is only populated for budget_unfeasible; it carries the last
	// attempt's allocation composition total and the budget maximum/excess so
	// the client sees WHY instead of a bare code. Stable role names only.
	Summary string
	message string
}

func (e *GenerationError) Error() string { return e.Code }

func newGenerationError(code, safeMessage string) *GenerationError {
	return &GenerationError{Code: code, message: safeMessage}
}

func newGenerationErrorWithSummary(code, safeMessage, summary string) *GenerationError {
	return &GenerationError{Code: code, message: safeMessage, Summary: summary}
}

// ---------------------------------------------------------------------------
// Provider source — the single seam between orchestration and Fase 5
// credential machinery.
// ---------------------------------------------------------------------------

// ProviderSource resolves the tenant's AI provider right before use. The
// default implementation decrypts the stored credential PER REQUEST via
// InstantiateProvider; nothing derived from credentials is ever cached.
type ProviderSource interface {
	ProviderForCompany(ctx context.Context, companyID uuid.UUID) (AIProvider, string /*providerName*/, string /*model*/, error)
}

// DBCredentialProviderSource implements ProviderSource against the active
// ai_credentials row of the company.
type DBCredentialProviderSource struct {
	credentialRepo *repository.CredentialRepository
	factory        *ProviderFactory
	encryptionKey  string
}

func NewDBCredentialProviderSource(credRepo *repository.CredentialRepository, factory *ProviderFactory, encryptionKey string) *DBCredentialProviderSource {
	return &DBCredentialProviderSource{credentialRepo: credRepo, factory: factory, encryptionKey: encryptionKey}
}

func (s *DBCredentialProviderSource) ProviderForCompany(ctx context.Context, companyID uuid.UUID) (AIProvider, string, string, error) {
	cred, err := s.credentialRepo.GetActiveCredential(ctx, companyID.String())
	if err != nil {
		if errors.Is(err, repository.ErrCredentialNotFound) {
			return nil, "", "", newGenerationError(ErrCodeSettingsIncomplete, "AI provider is not configured")
		}
		return nil, "", "", newGenerationError(ErrCodeGenerationFailed, "failed to load AI provider configuration")
	}

	// Request-scoped decryption happens inside InstantiateProvider. The key
	// lives only within this call frame — never cached anywhere.
	provider, err := InstantiateProvider(ctx, s.factory, cred, s.encryptionKey)
	if err != nil {
		return nil, "", "", mapProviderError(err)
	}
	return provider, cred.Provider, cred.Model, nil
}

// mapProviderError converts provider-layer failures into safe codes. It is
// the single classification point for every AI call failure.
func mapProviderError(err error) *GenerationError {
	logger.Error("AI Provider failed", logger.Err(err))
	var perr *ProviderError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return newGenerationError(ErrCodeProviderTimeout, "AI provider did not respond in time")
	case errors.Is(err, context.Canceled):
		return newGenerationError(ErrCodeGenerationFailed, "generation canceled")
	case errors.As(err, &perr):
		switch perr.Code {
		case ErrCredentialNotConfigured, ErrInvalidCredential:
			return newGenerationError(ErrCodeProviderInvalidCred, "AI provider credential is invalid")
		case ErrUnsupportedModel:
			return newGenerationError(ErrCodeSettingsIncomplete, "AI model configuration is not supported")
		case ErrRateLimited:
			return newGenerationError(ErrCodeProviderRateLimited, "AI provider is rate limited")
		case ErrTimeout:
			return newGenerationError(ErrCodeProviderTimeout, "AI provider timed out")
		default:
			return newGenerationError(ErrCodeGenerationFailed, "AI provider is unavailable")
		}
	default:
		return newGenerationError(ErrCodeGenerationFailed, "generation failed")
	}
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type QuotationGenerationService struct {
	genRepo           *repository.GenerationRepository
	providerSource    ProviderSource
	validator         AllocationValidator
	allocator         RateAllocator
	renderer          HTMLRenderer
	sanitizer         HTMLSanitizer
	domValidator      DOMValidator
	attemptTimeout    time.Duration
	dedicatedRenderer DedicatedSquadHTMLRenderer
}

type GenerationOption func(*QuotationGenerationService)

// WithAttemptTimeout overrides the per-AI-call deadline (tests use a short
// value; production keeps DefaultAttemptTimeout).
func WithAttemptTimeout(d time.Duration) GenerationOption {
	return func(s *QuotationGenerationService) { s.attemptTimeout = d }
}

func NewQuotationGenerationService(
	genRepo *repository.GenerationRepository,
	source ProviderSource,
	opts ...GenerationOption,
) *QuotationGenerationService {
	svc := &QuotationGenerationService{
		genRepo:           genRepo,
		providerSource:    source,
		validator:         NewAllocationValidator(),
		allocator:         NewRateAllocator(),
		renderer:          NewHTMLRenderer(),
		sanitizer:         NewHTMLSanitizer(),
		domValidator:      NewDOMValidator(),
		dedicatedRenderer: NewDedicatedSquadHTMLRenderer(),
		attemptTimeout:    DefaultAttemptTimeout,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Generate runs the full AI-first quotation pipeline for one request and
// returns the committed quotation ID. Any failure after draft creation leaves
// the revision pending/retryable and returns a safe GenerationError.
func (s *QuotationGenerationService) Generate(ctx context.Context, companyID uuid.UUID, createdBy *uuid.UUID, req dto.GenerateQuotationRequest) (uuid.UUID, error) {
	if err := validateGenerateInput(req); err != nil {
		return uuid.Nil, err
	}

	idemKey := strings.TrimSpace(derefString(req.IdempotencyKey))
	if _, err := uuid.Parse(idemKey); err != nil {
		return uuid.Nil, newGenerationError(ErrCodeGenerationFailed, "idempotency_key must be a UUID")
	}

	existing, err := s.genRepo.FindByIdempotencyKey(ctx, companyID, idemKey)
	if err != nil {
		return uuid.Nil, newGenerationError(ErrCodeGenerationFailed, "idempotency lookup failed")
	}
	if existing != nil {
		return existing.QuotationID, nil
	}

	cfg, gerr := s.loadConfiguration(ctx, companyID, req)
	if gerr != nil {
		return uuid.Nil, gerr
	}

	inputJSON, jerr := json.Marshal(req)
	if jerr != nil {
		return uuid.Nil, newGenerationError(ErrCodeGenerationFailed, "failed to serialize generation input")
	}

	// Identifiers are committed BEFORE any provider interaction so failures
	// remain retryable and idempotent.
	draft, err := s.genRepo.CreateDraftTransaction(ctx, repository.CreateDraftParams{
		CompanyID:             companyID,
		ClientID:              req.ClientID,
		ClientName:            strings.TrimSpace(req.ClientName),
		QuotationNumber:       cfg.quotationNumber,
		Subject:               strings.TrimSpace(req.Subject),
		SolutionType:          req.SolutionType,
		CommercialScheme:      req.CommercialScheme,
		IdempotencyKey:        idemKey,
		GenerationInput:       inputJSON,
		BudgetMaximum:         &req.BudgetMaximum,
		RateCardSnapshot:      cfg.rateCardSnapshot,
		LegalTemplateSnapshot: cfg.legalSnapshot,
		SkillSnapshot:         cfg.skillSnapshot,
		SafetyEnvelopeVersion: SafetyEnvelopeVersion,
		CreatedBy:             createdBy,
	})
	if err != nil {
		return uuid.Nil, newGenerationError(ErrCodeGenerationFailed, "failed to create draft")
	}

	if req.DedicatedSquad != nil {
		gerr := s.generateDedicated(ctx, companyID, req, cfg, draft.RevisionID)
		if gerr != nil {
			var ge *GenerationError
			reason := "dedicated squad generation failed"
			if errors.As(gerr, &ge) {
				reason = ge.message
			}
			_ = s.genRepo.UpdateGenerationFailure(ctx, s.genRepo.Pool(), companyID, draft.RevisionID, reason)
			return draft.QuotationID, gerr
		}
		return draft.QuotationID, nil
	}

	provider, provName, model, perr := s.providerSource.ProviderForCompany(ctx, companyID)
	if perr != nil {
		var ge *GenerationError
		if errors.As(perr, &ge) {
			_ = s.genRepo.UpdateGenerationFailure(ctx, s.genRepo.Pool(), companyID, draft.RevisionID, ge.message)
			return draft.QuotationID, ge
		}
		fallbackErr := newGenerationError(ErrCodeGenerationFailed, "AI provider is unavailable")
		_ = s.genRepo.UpdateGenerationFailure(ctx, s.genRepo.Pool(), companyID, draft.RevisionID, fallbackErr.message)
		return draft.QuotationID, fallbackErr
	}

	enriched, gerr := s.optimizeBudget(ctx, provider, req, cfg)
	if gerr == nil {
		gerr = s.composeAndPersist(ctx, provider, req, cfg, enriched, draft.RevisionID, provName, model)
	}
	if gerr != nil {
		var ge *GenerationError
		if errors.As(gerr, &ge) {
			reason := ge.message
			if ge.Code == ErrCodeBudgetUnfeasible && ge.Summary != "" {
				reason = ge.Summary
			}
			_ = s.genRepo.UpdateGenerationFailure(ctx, s.genRepo.Pool(), companyID, draft.RevisionID, reason)
		} else {
			_ = s.genRepo.UpdateGenerationFailure(ctx, s.genRepo.Pool(), companyID, draft.RevisionID, gerr.Error())
		}
		return draft.QuotationID, gerr
	}
	return draft.QuotationID, nil
}

func (s *QuotationGenerationService) generateDedicated(ctx context.Context, companyID uuid.UUID, req dto.GenerateQuotationRequest, cfg *generationConfig, revisionID uuid.UUID) error {
	input := *req.DedicatedSquad
	calculator := NewDedicatedSquadCalculator(rateSnapshotLookup{items: cfg.rateCardItems})
	calculation, err := calculator.Calculate(ctx, companyID.String(), req.RateCardID.String(), input)
	if err != nil {
		if errors.Is(err, dto.ErrMissingRateItem) {
			return newGenerationError(ErrCodeRateItemMissing, "rate item is missing for dedicated squad")
		}
		return newGenerationError(ErrCodeAllocationInvalid, "dedicated squad calculation is invalid")
	}
	rules, err := EvaluateDedicatedSquadRules(input, calculation.DevelopmentSubtotal)
	if err != nil {
		return newGenerationError(ErrCodeAllocationInvalid, "dedicated squad rules are invalid")
	}

	modules, providerName, model, gerr := s.extractDedicatedModules(ctx, companyID, input)
	if gerr != nil {
		return gerr
	}
	timeline, err := GenerateTimeline(input, rules, modules)
	if err != nil {
		return newGenerationError(ErrCodeAllocationInvalid, "dedicated squad timeline is invalid")
	}

	metadata := QuotationMetadata{
		QuotationNumber: cfg.quotationNumber, Subject: strings.TrimSpace(req.Subject),
		ClientName: cfg.clientName, ClientCompanyName: cfg.clientCompanyName,
		IssueDate: indonesianDate(*input.PaymentAnchorDate),
	}
	return s.renderAndPersistDedicated(ctx, repository.CompleteDedicatedGenerationParams{
		CompanyID: companyID, RevisionID: revisionID,
		CalculationSnapshot: mustJSON(calculation), ModuleSnapshot: mustJSON(modules), TimelineSnapshot: mustJSON(timeline),
		AiProvider: providerName, AiModel: model,
	}, DedicatedSquadDocumentInput{Metadata: metadata, Input: input, Calculation: calculation, Rules: rules, Timeline: timeline})
}

func (s *QuotationGenerationService) regenerateDedicated(ctx context.Context, companyID uuid.UUID, req dto.GenerateQuotationRequest, draft *repository.DraftRevision) error {
	var calculation domain.DedicatedSquadCalculation
	if len(draft.CalculationSnapshot) == 0 || string(draft.CalculationSnapshot) == "null" || json.Unmarshal(draft.CalculationSnapshot, &calculation) != nil {
		return newGenerationError(ErrCodeGenerationFailed, "stored calculation snapshot is unreadable")
	}
	input := *req.DedicatedSquad
	rules, err := EvaluateDedicatedSquadRules(input, calculation.DevelopmentSubtotal)
	if err != nil {
		return newGenerationError(ErrCodeAllocationInvalid, "stored dedicated squad rules are invalid")
	}
	modules, providerName, model, gerr := s.extractDedicatedModules(ctx, companyID, input)
	if gerr != nil {
		return gerr
	}
	timeline, err := GenerateTimeline(input, rules, modules)
	if err != nil {
		return newGenerationError(ErrCodeAllocationInvalid, "dedicated squad timeline is invalid")
	}
	metadata := QuotationMetadata{
		QuotationNumber: draft.QuotationNumber, Subject: strings.TrimSpace(req.Subject),
		ClientName: strings.TrimSpace(req.ClientName), ClientCompanyName: strings.TrimSpace(req.ClientCompanyName),
		IssueDate: indonesianDate(*input.PaymentAnchorDate),
	}
	return s.renderAndPersistDedicated(ctx, repository.CompleteDedicatedGenerationParams{
		CompanyID: companyID, RevisionID: draft.RevisionID, Regenerated: true,
		CalculationSnapshot: append(json.RawMessage(nil), draft.CalculationSnapshot...),
		ModuleSnapshot:      mustJSON(modules), TimelineSnapshot: mustJSON(timeline),
		AiProvider: providerName, AiModel: model,
	}, DedicatedSquadDocumentInput{Metadata: metadata, Input: input, Calculation: calculation, Rules: rules, Timeline: timeline})
}

func (s *QuotationGenerationService) extractDedicatedModules(ctx context.Context, companyID uuid.UUID, input domain.DedicatedSquadInput) ([]domain.Module, string, string, error) {
	if input.Model == domain.ModelSprint {
		return []domain.Module{}, "", "", nil
	}
	provider, providerName, model, err := s.providerSource.ProviderForCompany(ctx, companyID)
	if err != nil {
		var ge *GenerationError
		if errors.As(err, &ge) {
			return nil, "", "", ge
		}
		return nil, "", "", newGenerationError(ErrCodeGenerationFailed, "AI provider is unavailable")
	}
	attemptCtx, cancel := context.WithTimeout(ctx, s.attemptTimeout)
	defer cancel()
	modules, err := (ModuleExtractor{}).Extract(attemptCtx, provider, input.Requirement)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			return nil, "", "", newGenerationError(ErrCodeProviderTimeout, "AI provider did not respond in time")
		}
		var providerErr *ProviderError
		if errors.As(err, &providerErr) {
			return nil, "", "", mapProviderError(err)
		}
		return nil, "", "", newGenerationError(ErrCodeGenerationFailed, "module extraction failed")
	}
	return modules, providerName, model, nil
}

func (s *QuotationGenerationService) renderAndPersistDedicated(ctx context.Context, params repository.CompleteDedicatedGenerationParams, document DedicatedSquadDocumentInput) error {
	rawHTML, err := s.dedicatedRenderer.Render(document)
	if err != nil {
		return newGenerationError(ErrCodeUnsafeHTML, "dedicated document failed to render")
	}
	sanitized, err := s.sanitizer.Sanitize(rawHTML)
	if err != nil || strings.TrimSpace(sanitized) == "" {
		return newGenerationError(ErrCodeUnsafeHTML, "dedicated document sanitization failed")
	}
	if !strings.Contains(sanitized, `class="quotation-document"`) || !strings.Contains(sanitized, `data-block-type="commercial"`) {
		return newGenerationError(ErrCodeUnsafeHTML, "dedicated document integrity validation failed")
	}
	hash := sha256.Sum256([]byte(sanitized))
	hashString := hex.EncodeToString(hash[:])
	params.ContentHTML, params.ContentHash = sanitized, &hashString

	tx, err := s.genRepo.Pool().Begin(ctx)
	if err != nil {
		return newGenerationError(ErrCodeGenerationFailed, "failed to begin dedicated generation completion")
	}
	defer tx.Rollback(ctx)
	if err := s.genRepo.CompleteDedicatedGeneration(ctx, tx, params); err != nil {
		return newGenerationError(ErrCodeGenerationFailed, "failed to persist dedicated generation result")
	}
	if err := tx.Commit(ctx); err != nil {
		return newGenerationError(ErrCodeGenerationFailed, "failed to commit dedicated generation result")
	}
	return nil
}

type rateSnapshotLookup struct{ items []domain.RateCardItem }

func (l rateSnapshotLookup) ResolveItem(_ context.Context, _, _, role, seniority, termType string) (*domain.RateCardItem, error) {
	var found *domain.RateCardItem
	for i := range l.items {
		item := &l.items[i]
		if strings.EqualFold(item.Role, role) && strings.EqualFold(item.Seniority, seniority) && item.TermType == termType {
			if found != nil {
				return nil, fmt.Errorf("duplicate rate item for %s|%s|%s", role, seniority, termType)
			}
			copy := *item
			found = &copy
		}
	}
	if found == nil {
		return nil, fmt.Errorf("%w: %s|%s|%s", dto.ErrMissingRateItem, role, seniority, termType)
	}
	return found, nil
}

// composeAndPersist performs the post-allocation half of the initial
// generation pipeline: composition (render → narrative → assemble → sanitize
// → DOM validation) followed by persisting the result.
func (s *QuotationGenerationService) composeAndPersist(
	ctx context.Context,
	provider AIProvider,
	req dto.GenerateQuotationRequest,
	cfg *generationConfig,
	enriched dto.EnrichedAllocation,
	revisionID uuid.UUID,
	provName, model string,
) error {
	sanitized, gerr := s.runComposition(ctx, provider, req, cfg, enriched)
	if gerr != nil {
		return gerr
	}

	sum := sha256.Sum256([]byte(sanitized))
	hash := hex.EncodeToString(sum[:])
	if uerr := s.genRepo.UpdateGenerationResult(ctx, s.genRepo.Pool(), cfg.companyID, revisionID, repository.UpdateGenerationResultParams{
		ContentHTML:           sanitized,
		AllocationSnapshot:    mustJSON(enriched),
		RateCardSnapshot:      cfg.rateCardSnapshot,
		LegalTemplateSnapshot: cfg.legalSnapshot,
		SkillSnapshot:         cfg.skillSnapshot,
		AiProvider:            provName,
		AiModel:               model,
		ContentHash:           &hash,
		SafetyEnvelopeVersion: SafetyEnvelopeVersion,
	}); uerr != nil {
		return newGenerationError(ErrCodeGenerationFailed, "failed to persist generation result")
	}
	return nil
}

// runComposition renders the deterministic critical blocks, composes the AI
// narrative, assembles, sanitizes and validates DOM integrity. It returns the
// final sanitized HTML or a safe GenerationError. Nothing is persisted here.
func (s *QuotationGenerationService) runComposition(
	ctx context.Context,
	provider AIProvider,
	req dto.GenerateQuotationRequest,
	cfg *generationConfig,
	enriched dto.EnrichedAllocation,
) (string, error) {
	critical, rerr := s.renderer.RenderCriticalBlocks(CriticalBlocksInput{
		Metadata: QuotationMetadata{
			QuotationNumber:   cfg.quotationNumber,
			Subject:           strings.TrimSpace(req.Subject),
			ClientName:        cfg.clientName,
			ClientCompanyName: cfg.clientCompanyName,
			IssueDate:         indonesianDate(time.Now()),
		},
		Allocation: enriched,
		Payment:    req.PaymentTerms,
		LegalHTML:  cfg.legal.ContentHTML,
	})
	if rerr != nil {
		return "", newGenerationError(ErrCodeUnsafeHTML, "critical blocks failed to render")
	}

	narrative, gerr := s.composeNarrative(ctx, provider, req, cfg, enriched)
	if gerr != nil {
		return "", gerr
	}

	assembled, aerr := s.renderer.Assemble(critical, narrative)
	if aerr != nil {
		return "", newGenerationError(ErrCodeUnsafeHTML, "document assembly rejected")
	}

	sanitized, serr := s.sanitizer.Sanitize(assembled)
	if serr != nil {
		return "", newGenerationError(ErrCodeUnsafeHTML, "document sanitization failed")
	}

	if verr := s.domValidator.ValidateIntegrity(sanitized, buildIntegrityExpectations(req, cfg, enriched)); verr != nil {
		return "", newGenerationError(ErrCodeUnsafeHTML, "document integrity validation failed")
	}
	return sanitized, nil
}

// ---------------------------------------------------------------------------
// Session 5: atomic regeneration
// ---------------------------------------------------------------------------

// Regenerate re-runs the full pipeline for an existing active draft using its
// stored generation_input plus the tenant's CURRENT skill/rate/legal
// configuration, then swaps the artefacts atomically:
//
//   - failure anywhere BEFORE the final update (provider timeout, invalid
//     allocation, unsafe HTML, persistence error) leaves the stored
//     content_html byte-identical and content_version unchanged;
//   - success replaces content_html + snapshots and increments
//     content_version in ONE statement; no revision row is ever created;
//   - published/superseded revisions are rejected with revision_conflict;
//   - cost-control consumes one regeneration slot per attempt before any AI
//     spend, bounded to MaxRegeneratesPerHour per hour per draft by an atomic
//     gated DB update (regenerate_rate_limited when exhausted).
func (s *QuotationGenerationService) Regenerate(ctx context.Context, companyID, quotationID uuid.UUID) error {
	draft, err := s.genRepo.LoadDraftRevisionForRegeneration(ctx, companyID, quotationID)
	if err != nil {
		if errors.Is(err, repository.ErrRevisionConflict) {
			return newGenerationError(ErrCodeRevisionConflict, "regenerate is only allowed on active drafts")
		}
		// ErrRevisionNotFound passes through untouched so the HTTP layer can
		// answer 404 without leaking resource existence across tenants.
		return err
	}

	var req dto.GenerateQuotationRequest
	if jerr := json.Unmarshal(draft.GenerationInput, &req); jerr != nil {
		return newGenerationError(ErrCodeGenerationFailed, "stored generation input is unreadable")
	}

	if _, qerr := s.genRepo.ConsumeRegenerateQuota(ctx, s.genRepo.Pool(), companyID, draft.RevisionID); qerr != nil {
		if errors.Is(qerr, pgx.ErrNoRows) {
			return newGenerationError(ErrCodeRegenerateRateLimited, "regenerate limit reached for this draft")
		}
		return newGenerationError(ErrCodeGenerationFailed, "failed to update regenerate counter")
	}
	if req.DedicatedSquad != nil {
		return s.regenerateDedicated(ctx, companyID, req, draft)
	}

	cfg, gerr := s.loadConfiguration(ctx, companyID, req)
	if gerr != nil {
		return gerr
	}
	// Reuse the committed quotation number; deriving a fresh sequence number
	// here would both desynchronise the rendered metadata block and burn the
	// numbering series without creating anything.
	cfg.quotationNumber = draft.QuotationNumber

	provider, provName, model, perr := s.providerSource.ProviderForCompany(ctx, companyID)
	if perr != nil {
		return perr
	}

	enriched, gerr := s.optimizeBudget(ctx, provider, req, cfg)
	if gerr == nil {
		gerr = s.composeAndPersistRegenerated(ctx, provider, req, cfg, enriched, draft.RevisionID, provName, model)
	}
	return gerr
}

// composeAndPersistRegenerated runs the shared composition pipeline then
// persists with UpdateRegeneratedResult whose single UPDATE also increments
// content_version — the swap is therefore atomic with respect to readers and
// cannot leave a half-updated revision behind.
func (s *QuotationGenerationService) composeAndPersistRegenerated(
	ctx context.Context,
	provider AIProvider,
	req dto.GenerateQuotationRequest,
	cfg *generationConfig,
	enriched dto.EnrichedAllocation,
	revisionID uuid.UUID,
	provName, model string,
) error {
	sanitized, gerr := s.runComposition(ctx, provider, req, cfg, enriched)
	if gerr != nil {
		return gerr
	}

	sum := sha256.Sum256([]byte(sanitized))
	hash := hex.EncodeToString(sum[:])
	if uerr := s.genRepo.UpdateRegeneratedResult(ctx, s.genRepo.Pool(), cfg.companyID, revisionID, repository.UpdateGenerationResultParams{
		ContentHTML:           sanitized,
		AllocationSnapshot:    mustJSON(enriched),
		RateCardSnapshot:      cfg.rateCardSnapshot,
		LegalTemplateSnapshot: cfg.legalSnapshot,
		SkillSnapshot:         cfg.skillSnapshot,
		AiProvider:            provName,
		AiModel:               model,
		ContentHash:           &hash,
		SafetyEnvelopeVersion: SafetyEnvelopeVersion,
	}); uerr != nil {
		if errors.Is(uerr, repository.ErrRevisionConflict) {
			// The revision stopped being an active draft while the pipeline
			// was running — never overwrite published/superseded content.
			return newGenerationError(ErrCodeRevisionConflict, "revision is no longer an active draft")
		}
		return newGenerationError(ErrCodeGenerationFailed, "failed to persist regenerated result")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Phase 7 Session 1: Save Draft Contract
// ---------------------------------------------------------------------------

// SaveDraft persists user edits to a draft quotation's HTML document, performing
// allowlist sanitization, independent DOM verification of legal & commercial blocks
// against stored snapshots, and atomic optimistic locking via content_version.
func (s *QuotationGenerationService) SaveDraft(
	ctx context.Context,
	companyID, quotationID uuid.UUID,
	userID *uuid.UUID,
	req dto.SaveDraftRequest,
) (*dto.SaveDraftResponse, error) {
	if req.ContentVersion < 1 {
		return nil, newGenerationError(ErrCodeRevisionConflict, "content_version must be positive")
	}
	trimmedHTML := strings.TrimSpace(req.ContentHTML)
	if trimmedHTML == "" {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "content_html cannot be empty")
	}

	draft, err := s.genRepo.LoadDraftRevisionForSave(ctx, companyID, quotationID)
	if err != nil {
		if errors.Is(err, repository.ErrRevisionConflict) {
			return nil, newGenerationError(ErrCodeRevisionConflict, "save draft is only allowed on active drafts")
		}
		return nil, err
	}
	if req.RevisionID != uuid.Nil && req.RevisionID != draft.RevisionID {
		return nil, newGenerationError(ErrCodeRevisionConflict, "revision_id mismatch")
	}
	if draft.ContentVersion != req.ContentVersion {
		return nil, newGenerationError(ErrCodeRevisionConflict, "stale content_version")
	}

	// 1. Sanitize HTML via allowlist
	sanitized, serr := s.sanitizer.Sanitize(trimmedHTML)
	if serr != nil {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "document sanitization failed")
	}

	// 2. Extract snapshot expectations for comparisons
	var enriched dto.EnrichedAllocation
	if err := json.Unmarshal(draft.AllocationSnapshot, &enriched); err != nil ||
		enriched.TotalInvestment <= 0 || strings.TrimSpace(enriched.Currency) == "" || len(enriched.Resources) == 0 {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "allocation snapshot is invalid")
	}
	for _, resource := range enriched.Resources {
		if strings.TrimSpace(resource.Role) == "" || strings.TrimSpace(resource.Seniority) == "" ||
			resource.EmployeeCount <= 0 || resource.PeriodMonths <= 0 || resource.AmountPerMonth <= 0 || resource.LineTotal <= 0 {
			return nil, newGenerationError(ErrCodeUnsafeHTML, "allocation snapshot resource is invalid")
		}
	}
	var generationInput dto.GenerateQuotationRequest
	if err := json.Unmarshal(draft.GenerationInput, &generationInput); err != nil || len(generationInput.PaymentTerms.Items) == 0 {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "generation input snapshot is invalid")
	}
	percentageTotal := int64(0)
	for _, item := range generationInput.PaymentTerms.Items {
		if item.Percentage <= 0 || item.Percentage > 100 {
			return nil, newGenerationError(ErrCodeUnsafeHTML, "generation input payment terms are invalid")
		}
		percentageTotal += item.Percentage
	}
	if percentageTotal != 100 {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "generation input payment terms are invalid")
	}
	totalInvestment := enriched.TotalInvestment

	// 3. Validate safety (forbidden terms, budget leak)
	if verr := s.domValidator.ValidateDocumentSafety(sanitized, draft.BudgetMaximum, totalInvestment); verr != nil {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "document safety validation failed")
	}

	// 4. Check Legal Block modification
	var legalSnap struct {
		Code        string `json:"code"`
		ContentHTML string `json:"content_html"`
	}
	if err := json.Unmarshal(draft.LegalTemplateSnapshot, &legalSnap); err != nil {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "legal template snapshot is invalid")
	}
	expectedLegalHTML := strings.TrimSpace(legalSnap.ContentHTML)
	if expectedLegalHTML == "" {
		if strings.TrimSpace(legalSnap.Code) == "" {
			return nil, newGenerationError(ErrCodeUnsafeHTML, "legal template snapshot is incomplete")
		}
		tmpl, terr := s.genRepo.GetActiveLegalTemplateForCompany(ctx, companyID, legalSnap.Code)
		if terr != nil || strings.TrimSpace(tmpl.ContentHTML) == "" {
			return nil, newGenerationError(ErrCodeUnsafeHTML, "legal template snapshot cannot be resolved")
		}
		expectedLegalHTML = tmpl.ContentHTML
	}

	legalModified, lerr := s.domValidator.CheckLegalModified(sanitized, expectedLegalHTML)
	if lerr != nil {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "failed to parse legal block")
	}

	// 5. Check Commercial Block modification
	var expectations IntegrityExpectations
	if totalInvestment > 0 {
		expectations = IntegrityExpectations{
			Currency:        enriched.Currency,
			TotalInvestment: enriched.TotalInvestment,
			Lines:           make([]ExpectedLine, 0, len(enriched.Resources)),
		}
		for _, res := range enriched.Resources {
			expectations.Lines = append(expectations.Lines, ExpectedLine{
				Role:           res.Role,
				Seniority:      res.Seniority,
				EmployeeCount:  res.EmployeeCount,
				PeriodMonths:   res.PeriodMonths,
				AmountPerMonth: res.AmountPerMonth,
				LineTotal:      res.LineTotal,
			})
		}
		for _, item := range generationInput.PaymentTerms.Items {
			expectations.PaymentItems = append(expectations.PaymentItems, ExpectedPayment{Percentage: item.Percentage})
		}
	}
	commercialModified, cerr := s.domValidator.CheckCommercialModified(sanitized, expectations)
	if cerr != nil {
		return nil, newGenerationError(ErrCodeUnsafeHTML, "failed to parse commercial block")
	}

	// 6. Compute hash
	sum := sha256.Sum256([]byte(sanitized))
	hash := hex.EncodeToString(sum[:])
	now := time.Now().UTC()

	// 7. Atomic update in repository with optimistic locking
	res, uerr := s.genRepo.UpdateDraft(ctx, s.genRepo.Pool(), companyID, quotationID, repository.UpdateDraftParams{
		RevisionID:                draft.RevisionID,
		ExpectedVersion:           req.ContentVersion,
		ContentHTML:               sanitized,
		ContentHash:               &hash,
		LegalContentModified:      legalModified,
		LegalModifiedBy:           userID,
		LegalModifiedAt:           &now,
		CommercialContentModified: commercialModified,
		CommercialModifiedBy:      userID,
		CommercialModifiedAt:      &now,
	})
	if uerr != nil {
		if errors.Is(uerr, repository.ErrRevisionConflict) {
			return nil, newGenerationError(ErrCodeRevisionConflict, "stale content_version")
		}
		return nil, newGenerationError(ErrCodeGenerationFailed, "failed to persist draft update")
	}

	return &dto.SaveDraftResponse{
		QuotationID:               quotationID,
		RevisionID:                res.RevisionID,
		ContentVersion:            res.ContentVersion,
		ContentHTML:               res.ContentHTML,
		LegalContentModified:      res.LegalContentModified,
		CommercialContentModified: res.CommercialContentModified,
		UpdatedAt:                 res.UpdatedAt,
	}, nil
}

// ---------------------------------------------------------------------------
// Configuration loading
// ---------------------------------------------------------------------------

type generationConfig struct {
	companyID         uuid.UUID
	skill             *domain.Skill
	rateCard          *domain.RateCard
	rateCardItems     []domain.RateCardItem
	legal             *domain.LegalTemplate
	clientName        string
	clientCompanyName string
	quotationNumber   string
	termType          string
	rateCardSnapshot  json.RawMessage
	legalSnapshot     json.RawMessage
	skillSnapshot     json.RawMessage
}

func (s *QuotationGenerationService) loadConfiguration(ctx context.Context, companyID uuid.UUID, req dto.GenerateQuotationRequest) (*generationConfig, error) {
	cfg := &generationConfig{companyID: companyID}

	skill, err := s.genRepo.GetSkillForGeneration(ctx, companyID, req.SkillID)
	if err != nil {
		if errors.Is(err, repository.ErrSkillNotFound) {
			return nil, newGenerationError(ErrCodeSettingsIncomplete, "AI skill is not configured")
		}
		return nil, newGenerationError(ErrCodeGenerationFailed, "failed to load AI skill")
	}
	cfg.skill = skill

	card, items, err := s.genRepo.GetRateCardWithItems(ctx, companyID, req.RateCardID)
	if err != nil {
		if errors.Is(err, repository.ErrRateCardNotFound) {
			return nil, newGenerationError(ErrCodeInvalidRateCard, "rate card is not available")
		}
		return nil, newGenerationError(ErrCodeGenerationFailed, "failed to load rate card")
	}
	if len(items) == 0 {
		return nil, newGenerationError(ErrCodeInvalidRateCard, "rate card has no rate items")
	}
	cfg.rateCard = card
	cfg.rateCardItems = items

	legal, err := s.genRepo.GetActiveLegalTemplateForCompany(ctx, companyID, DefaultLegalTemplateCode)
	if err != nil {
		if errors.Is(err, repository.ErrLegalTemplateNotFound) {
			return nil, newGenerationError(ErrCodeSettingsIncomplete, "legal template is not configured")
		}
		return nil, newGenerationError(ErrCodeGenerationFailed, "failed to load legal template")
	}
	cfg.legal = legal

	var clientName string
	var clientCompanyName string
	if req.ClientID != nil && *req.ClientID != uuid.Nil {
		var cerr error
		resolvedName, cerr := s.genRepo.GetClientName(ctx, *req.ClientID)
		if cerr != nil {
			logger.Error("Failed to resolve client name", logger.Err(cerr))
		} else {
			clientCompanyName = resolvedName
		}
	}
	if req.ClientName != "" {
		clientName = strings.TrimSpace(req.ClientName)
	} else if clientCompanyName != "" {
		clientName = clientCompanyName
	}
	if req.ClientCompanyName != "" {
		clientCompanyName = strings.TrimSpace(req.ClientCompanyName)
	}
	cfg.clientName = clientName
	cfg.clientCompanyName = clientCompanyName

	number, err := s.genRepo.NextQuotationNumber(ctx, companyID)
	if err != nil {
		return nil, newGenerationError(ErrCodeGenerationFailed, "failed to derive quotation number")
	}
	cfg.quotationNumber = number
	cfg.termType = s.allocator.DetermineTermType(req.ProjectDurationMonths)

	cfg.rateCardSnapshot = mustJSON(rateCardSnapshotView{
		ID:                    card.ID,
		Code:                  card.Code,
		Name:                  card.Name,
		Market:                card.Market,
		ClientTier:            card.ClientTier,
		Currency:              card.Currency,
		ShortTermSurchargePct: card.ShortTermSurchargePct,
		EnterpriseMarkupPct:   card.EnterpriseMarkupPct,
		Items:                 rateCardItemViews(items),
	})
	cfg.legalSnapshot = mustJSON(legalSnapshotView{
		ID:          legal.ID,
		Code:        legal.Code,
		Version:     legal.Version,
		ContentHTML: legal.ContentHTML,
	})
	cfg.skillSnapshot = mustJSON(skillSnapshotView{
		ID:      skill.ID,
		Command: skill.Command,
		Name:    skill.Name,
	})

	return cfg, nil
}

// ---------------------------------------------------------------------------
// Budget optimization loop
// ---------------------------------------------------------------------------

type retryFeedback struct {
	AttemptNumber      int   `json:"attempt_number"`
	MaximumAttempts    int   `json:"maximum_attempts"`
	BudgetExcessAmount int64 `json:"budget_excess_amount"`
}

// optimizeBudget drives the allocation stage: propose -> validate -> enrich,
// feeding the excess amount back to the provider on overbudget proposals.
// Validation failures are fatal; only budget overruns trigger a retry, and
// mandatory scope coverage is re-enforced by the validator on EVERY attempt.
func (s *QuotationGenerationService) optimizeBudget(ctx context.Context, provider AIProvider, req dto.GenerateQuotationRequest, cfg *generationConfig) (dto.EnrichedAllocation, error) {
	envelope := GetSafetyEnvelope(SafetyEnvelopeFileName)
	var feedback *retryFeedback
	// The client already sees the allocation lines and rate card figures in
	// rendered documents, so a summary of the LAST aborted attempt is carried
	// alongside budget_unfeasible (see GenerationError doc).
	var lastSummary string

	for attempt := 1; attempt <= MaxProposalAttempts; attempt++ {
		text, gerr := s.callProvider(ctx, provider, buildAllocationPrompt(envelope, cfg.skill.ContentInstruction, buildAllocationPayload(req, cfg, feedback)))
		if gerr != nil {
			return dto.EnrichedAllocation{}, gerr
		}

		proposal, derr := s.validator.DecodeAllocationJSON(extractJSONObject(text))
		if derr != nil {
			return dto.EnrichedAllocation{}, newGenerationError(ErrCodeAllocationInvalid, "allocation proposal is invalid")
		}
		if verr := s.validator.ValidateProposal(req, proposal); verr != nil {
			return dto.EnrichedAllocation{}, newGenerationError(ErrCodeAllocationInvalid, "allocation proposal is invalid")
		}

		enriched, aerr := s.allocator.CalculateRaw(proposal, cfg.rateCardItems, req.ProjectDurationMonths, cfg.rateCard.Currency)
		if aerr != nil {
			if errors.Is(aerr, dto.ErrMissingRateItem) {
				return dto.EnrichedAllocation{}, newGenerationError(ErrCodeRateItemMissing, "rate item is missing for the proposed allocation")
			}
			return dto.EnrichedAllocation{}, newGenerationError(ErrCodeGenerationFailed, "allocation enrichment failed")
		}

		finalTotal := enriched.TotalInvestment
		lastSummary = buildBudgetSummary(enriched, finalTotal, req.BudgetMaximum)

		if req.BudgetMaximum > 0 && finalTotal > req.BudgetMaximum {
			if attempt == MaxProposalAttempts {
				return dto.EnrichedAllocation{}, newGenerationErrorWithSummary(ErrCodeBudgetUnfeasible, "budget cannot accommodate a viable allocation", lastSummary)
			}
			excess, xerr := checkedAdd(finalTotal, -req.BudgetMaximum)
			if xerr != nil {
				return dto.EnrichedAllocation{}, newGenerationError(ErrCodeGenerationFailed, "budget excess calculation failed")
			}
			feedback = &retryFeedback{
				AttemptNumber:      attempt + 1,
				MaximumAttempts:    MaxProposalAttempts,
				BudgetExcessAmount: excess,
			}
			continue
		}

		if req.BudgetMaximum > 0 {
			utilizationNumerator, uerr := checkedMul(finalTotal, 100)
			if uerr == nil {
				enriched.UtilizationPercent = utilizationNumerator / req.BudgetMaximum
			}
		}
		return enriched, nil
	}
	// Defensive: unreachable while every iteration either returns or continues.
	return dto.EnrichedAllocation{}, newGenerationErrorWithSummary(ErrCodeBudgetUnfeasible, "budget cannot accommodate a viable allocation", lastSummary)
}

// buildBudgetSummary renders the last attempt's allocation so budget_unfeasible
// answers WHY: stable role/seniority names, durations in months derived from
// the stored quarter units, and amounts formatted exactly like the rendered
// document. It only contains rate-card-derived figures the client already sees,
// never prompts, provider payloads, or keys.
func buildBudgetSummary(enriched dto.EnrichedAllocation, finalTotal, budgetMaximum int64) string {
	if len(enriched.Resources) == 0 {
		return ""
	}
	rows := make([]string, 0, len(enriched.Resources))
	for _, res := range enriched.Resources {
		dur := res.PeriodQuarters
		if dur <= 0 && res.PeriodMonths > 0 {
			dur = res.PeriodMonths * 4
		}
		rows = append(rows, fmt.Sprintf("%d %s %s %s", res.EmployeeCount, res.Role, res.Seniority, quarterDurationLabel(dur)))
	}
	excess := finalTotal - budgetMaximum
	return fmt.Sprintf(
		"Setelah %d percobaan, alokasi minimum layak (%s) total %s, masih %s di atas batas %s.",
		MaxProposalAttempts,
		strings.Join(rows, ", "),
		formatCurrency(finalTotal, enriched.Currency),
		formatCurrency(excess, enriched.Currency),
		formatCurrency(budgetMaximum, enriched.Currency),
	)
}

// quarterDurationLabel converts integer quarter-months to an Indonesian month
// label (4 quarters = 1 bln), keeping integer-only money/time math.
func quarterDurationLabel(quarters int64) string {
	fractional := map[int64]string{1: "25", 2: "5", 3: "75"}
	whole := quarters / 4
	rem := quarters % 4
	if rem == 0 {
		return fmt.Sprintf("%d bln", whole)
	}
	return fmt.Sprintf("%d,%s bln", whole, fractional[rem])
}

func buildAllocationPayload(req dto.GenerateQuotationRequest, cfg *generationConfig, feedback *retryFeedback) allocationPromptPayload {
	allowedRoles := getAvailableRolesForTerm(cfg.rateCardItems, cfg.termType)
	allowedSeniorities := getAvailableSenioritiesForTerm(cfg.rateCardItems, cfg.termType)

	return allocationPromptPayload{
		GenerationInput: allocationPromptInput{
			Subject:                   strings.TrimSpace(req.Subject),
			SolutionType:              req.SolutionType,
			CommercialScheme:          req.CommercialScheme,
			RateCardCode:              req.RateCardCode,
			ProjectDurationMonths:     req.ProjectDurationMonths,
			Summary:                   req.Summary,
			MandatoryScope:            req.MandatoryScope,
			OptionalScope:             req.OptionalScope,
			AllowedStandardActivities: req.AllowedStandardActivities,
			AdditionalNotes:           req.AdditionalNotes,
		},
		TermType:           cfg.termType,
		AllowedRoles:       allowedRoles,
		AllowedSeniorities: allowedSeniorities,
		RetryFeedback:      feedback,
	}
}

func getAvailableRolesForTerm(items []domain.RateCardItem, termType string) []string {
	var roles []string
	for _, item := range items {
		if item.TermType == termType && containsString(validRoles, item.Role) && !containsString(roles, item.Role) {
			roles = append(roles, item.Role)
		}
	}
	if len(roles) == 0 {
		return append([]string(nil), validRoles...)
	}
	return roles
}

func getAvailableSenioritiesForTerm(items []domain.RateCardItem, termType string) []string {
	var seniorities []string
	for _, item := range items {
		if item.TermType == termType && containsString(validSeniorities, item.Seniority) && !containsString(seniorities, item.Seniority) {
			seniorities = append(seniorities, item.Seniority)
		}
	}
	if len(seniorities) == 0 {
		return append([]string(nil), validSeniorities...)
	}
	return seniorities
}

func buildAllocationPrompt(envelope, instruction string, payload allocationPromptPayload) string {
	payloadJSON := mustJSON(payload)
	return strings.Join([]string{
		envelope,
		"",
		"# Instruction",
		instruction,
		"",
		"# Task",
		allocationTaskSchema,
		"",
		"# Payload",
		string(payloadJSON),
	}, "\n")
}

const allocationTaskSchema = `Return exactly ONE JSON object and nothing else. Schema:
{"resources":[{"role":"<one of allowed_roles>","seniority":"<one of allowed_seniorities>","employee_count":<integer >= 1>,"period_quarters":<integer >= 1>,"scope_references":["<exact scope strings from generation_input>"],"reason":"<short reason>"}],"selected_optional_scope":["<subset of optional_scope>"],"standard_activities":["<subset of allowed_standard_activities>"]}
Rules: every mandatory_scope item must appear in at least one resource scope_references; never invent roles, seniorities or business features; NEVER output rates, amounts, prices, discounts or totals.
period_quarters rules: period_quarters is an INTEGER count of quarter-months THAT role is actively engaged, not the total project length. 1 quarter-month = 0.25 bulan, so 2 = 0.5 month, 3 = 0.75 month, 4 = one full month, 5 = 1.25 months. It must be between 1 and project_duration_months * 4 (compute the cap; e.g. a 6-month project caps at 24 quarters, a 2-month project at 8 quarters). Allocate each role only for the phases where its work actually happens, following a realistic software delivery lifecycle: business_analyst is front-loaded (requirements & design, first part of the project only, never the whole duration); ui_ux_designer is front-loaded on design and tapers off once development is underway; programmer is the core build role and usually the ONLY role legitimately active across most of the project duration; quality_assurance is back-loaded (testing and stabilization near the end); devops is concentrated around initial setup and final release/hardening. Use the reason field to justify every chosen period_quarters (which phase, roughly how many months). Every role assigned the full cap must explicitly justify it.
HARD CONSTRAINT - Do NOT default every role to the full project duration: a non-programmer role carrying the full duration is idle capacity the client pays for. The server sums your proposed lines into a raw total and checks it against budget_maximum; a proposal above budget is rejected as budget_unfeasible and there are at most 3 attempts, so never spend an attempt on a proposal whose untrimmed non-programmer durations are an obvious cost driver. Assigning full duration to a non-programmer role while the proposal is at or above budget_maximum is a ladder violation and wastes the attempt.
LOW-BUDGET MANDATORY PRIORITY LADDER - when the budget is tight relative to team size (especially project_duration_months <= 2), you MUST apply the four levels strictly in order, escalating only when the previous level demonstrably cannot bring the raw total under budget_maximum. Short projects are where budgets are tightest, so structural phasing applies MORE here, never less:
LEVEL 1 - TRIM NON-CORE DURATIONS to their real phase footprint, expressed in quarter units. Reference pattern for a 2-month project (cap 8 quarters): ui_ux_designer 1-2 quarters (design up front, execution belongs to programmers), business_analyst 2 quarters (requirements & design up front), devops 1 quarter (setup at start, deploy at end), quality_assurance 1-2 quarters (testing at the end), programmer at or near the full 8 quarters - the only role that may span the whole timeline. Scale the same way for 3 months: support roles stay at 1-4 quarters each.
LEVEL 2 - CONSOLIDATE OVERLAPPING ROLES: drop a resource row entirely and expand the surviving row's scope_references to cover its scope. Never invent merged role names - every role must still come from allowed_roles. Allowed consolidation: business_analyst absorbing ui_ux_designer work (both are front-loaded product/design work). Never consolidate a programmer row with a non-programmer row, and never merge quality_assurance with devops.
LEVEL 3 - REDUCE SCOPE, NOT WORK QUALITY: exclude optional_scope items and drop standard_activities that are not essential, trimming the affected rows' scope_references accordingly. Keep the quality of every retained role's work intact.
LEVEL 4 - LAST RESORT: if levels 1-3 combined still leave the raw total above budget_maximum, still return your most aggressively reduced proposal (minimum viable team, non-core roles at 1-2 quarters, core programmer dominant) and state in the surviving reason fields exactly what was already stripped (e.g. "minimum viable team: support roles at 0.25 month of a 2-month project"). Never return a proposal where every role carries the full project duration.`

// ---------------------------------------------------------------------------
// Narrative composition
// ---------------------------------------------------------------------------

var narrativeOutputContract = []string{
	"Output ONLY an HTML fragment, no markdown fences, no explanations.",
	"You MUST wrap the opening paragraph in <section data-block-type=\"opening\">.</section>",
	"The opening paragraph MUST begin with: 'Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek. Dokumen ini memuat ruang lingkup pekerjaan, output, timeline, dan rincian biaya sebagai acuan kesepakatan bersama kedua belah pihak.'",
	"You MUST wrap the scope section in <section data-block-type=\"scope\">.</section>",
	"The scope section MUST have heading <h2>Lingkup Pekerjaan &amp; Timeline</h2>.",
	"For each scope_items entry, output a <strong> title followed by a <ul> with 2-4 <li> items describing specific technical deliverables or activities for that scope area. Write in Bahasa Indonesia formal with English technical terms inline.",
	"You MUST wrap the closing paragraph in <section data-block-type=\"closing\">.</section>",
	"Allowed tags: section, p, strong, em, ul, ol, li, h1, h2, h3, br.",
	"Never output tables, script, style, links or images.",
	"Never mention prices, totals, budgets, payment terms or legal clauses.",
	"Use Bahasa Indonesia formal with English IT/software engineering terms.",
}

func (s *QuotationGenerationService) composeNarrative(ctx context.Context, provider AIProvider, req dto.GenerateQuotationRequest, cfg *generationConfig, enriched dto.EnrichedAllocation) (string, error) {
	envelope := GetSafetyEnvelope(SafetyEnvelopeFileName)

	scopeItems := make([]string, 0, len(req.MandatoryScope)+len(enriched.SelectedOptionalScope)+len(enriched.StandardActivities))
	scopeItems = append(scopeItems, req.MandatoryScope...)
	scopeItems = append(scopeItems, enriched.SelectedOptionalScope...)
	scopeItems = append(scopeItems, enriched.StandardActivities...)

	payloadJSON := mustJSON(narrativePromptPayload{
		QuotationNumber:       cfg.quotationNumber,
		Subject:               strings.TrimSpace(req.Subject),
		ClientName:            cfg.clientName,
		SolutionType:          req.SolutionType,
		ProjectDurationMonths: req.ProjectDurationMonths,
		Summary:               req.Summary,
		ScopeItems:            scopeItems,
		AdditionalNotes:       req.AdditionalNotes,
		OutputContract:        narrativeOutputContract,
	})

	prompt := strings.Join([]string{
		envelope,
		"",
		"# Instruction",
		cfg.skill.ContentInstruction,
		"",
		"# Task",
		narrativeTaskSchema,
		"",
		"# Payload",
		string(payloadJSON),
	}, "\n")

	text, gerr := s.callProvider(ctx, provider, prompt)
	if gerr != nil {
		return "", gerr
	}
	narrative := strings.TrimSpace(stripCodeFences(text))
	if narrative == "" {
		return "", newGenerationError(ErrCodeUnsafeHTML, "narrative is empty")
	}
	return narrative, nil
}

const narrativeTaskSchema = `Compose the narrative section of a Venturo Pro Solusindo business quotation document:
1. An opening paragraph that affirms the company's readiness with 100+ IT professionals to support the project.
2. A "Lingkup Pekerjaan & Timeline" section: for each scope_items entry, write a descriptive subsection with the scope name in <strong> tags, followed by a <ul> list of 2-4 specific technical activities or deliverables. Group related scope items by project phase (e.g., Requirement Analysis & System Design, Development, SIT/UAT & Go-Live) when the scope naturally fits those phases.
3. A brief closing courtesy paragraph.
The tone is formal Indonesian (Bahasa Indonesia) with English IT terminology inline.`

type allocationPromptInput struct {
	Subject                   string   `json:"subject"`
	SolutionType              string   `json:"solution_type"`
	CommercialScheme          string   `json:"commercial_scheme"`
	RateCardCode              string   `json:"rate_card_code"`
	ProjectDurationMonths     int64    `json:"project_duration_months"`
	Summary                   string   `json:"summary"`
	MandatoryScope            []string `json:"mandatory_scope"`
	OptionalScope             []string `json:"optional_scope"`
	AllowedStandardActivities []string `json:"allowed_standard_activities"`
	AdditionalNotes           string   `json:"additional_notes"`
}

type allocationPromptPayload struct {
	GenerationInput    allocationPromptInput `json:"generation_input"`
	TermType           string                `json:"term_type"`
	AllowedRoles       []string              `json:"allowed_roles"`
	AllowedSeniorities []string              `json:"allowed_seniorities"`
	RetryFeedback      *retryFeedback        `json:"retry_feedback,omitempty"`
}

type narrativePromptPayload struct {
	QuotationNumber       string   `json:"quotation_number"`
	Subject               string   `json:"subject"`
	ClientName            string   `json:"client_name"`
	SolutionType          string   `json:"solution_type"`
	ProjectDurationMonths int64    `json:"project_duration_months"`
	Summary               string   `json:"summary"`
	ScopeItems            []string `json:"scope_items"`
	AdditionalNotes       string   `json:"additional_notes"`
	OutputContract        []string `json:"output_contract"`
}

// ---------------------------------------------------------------------------
// AI call wrapper — every provider interaction goes through here so each
// attempt carries its own hard deadline.
// ---------------------------------------------------------------------------

func (s *QuotationGenerationService) callProvider(ctx context.Context, provider AIProvider, prompt string) (string, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, s.attemptTimeout)
	defer cancel()
	result, err := provider.Generate(attemptCtx, GenerateRequest{Prompt: prompt})
	if err != nil {
		return "", mapProviderError(err)
	}
	return result.Text, nil
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

type rateCardItemSnapshot struct {
	Role                  string `json:"role"`
	DisplayName           string `json:"display_name,omitempty"`
	Seniority             string `json:"seniority"`
	Discipline            string `json:"discipline,omitempty"`
	TermType              string `json:"term_type"`
	AmountPerMonth        int64  `json:"amount_per_month"`
	BaseSalary            int64  `json:"base_salary,omitempty"`
	TechLeadFee           int64  `json:"tech_lead_fee,omitempty"`
	AiRndFee              int64  `json:"ai_rnd_fee,omitempty"`
	WorkspaceFee          int64  `json:"workspace_fee,omitempty"`
	ManagementFee         int64  `json:"management_fee,omitempty"`
	EducationRequirement  string `json:"education_requirement,omitempty"`
	ExperienceRequirement string `json:"experience_requirement,omitempty"`
	PublishedRate         int64  `json:"published_rate,omitempty"`
	MaxDiscountPct        int    `json:"max_discount_pct,omitempty"`
	FloorRate             int64  `json:"floor_rate,omitempty"`
	KickoffFee            int64  `json:"kickoff_fee,omitempty"`
	RecruitmentFee        int64  `json:"recruitment_fee,omitempty"`
	OnboardingFee         int64  `json:"onboarding_fee,omitempty"`
}

type rateCardSnapshotView struct {
	ID                    uuid.UUID              `json:"id"`
	Code                  string                 `json:"code"`
	Name                  string                 `json:"name"`
	Market                string                 `json:"market"`
	ClientTier            string                 `json:"client_tier,omitempty"`
	Currency              string                 `json:"currency"`
	ShortTermSurchargePct int                    `json:"short_term_surcharge_pct,omitempty"`
	EnterpriseMarkupPct   int                    `json:"enterprise_markup_pct,omitempty"`
	Items                 []rateCardItemSnapshot `json:"items"`
}

type legalSnapshotView struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	Version     int       `json:"version"`
	ContentHTML string    `json:"content_html,omitempty"`
}

type skillSnapshotView struct {
	ID      uuid.UUID `json:"id"`
	Command string    `json:"command"`
	Name    string    `json:"name"`
}

func rateCardItemViews(items []domain.RateCardItem) []rateCardItemSnapshot {
	views := make([]rateCardItemSnapshot, 0, len(items))
	for _, item := range items {
		views = append(views, rateCardItemSnapshot{
			Role:                  item.Role,
			DisplayName:           item.DisplayName,
			Seniority:             item.Seniority,
			Discipline:            item.Discipline,
			TermType:              item.TermType,
			AmountPerMonth:        item.AmountPerMonth,
			BaseSalary:            item.BaseSalary,
			TechLeadFee:           item.TechLeadFee,
			AiRndFee:              item.AiRndFee,
			WorkspaceFee:          item.WorkspaceFee,
			ManagementFee:         item.ManagementFee,
			EducationRequirement:  item.EducationRequirement,
			ExperienceRequirement: item.ExperienceRequirement,
			PublishedRate:         item.PublishedRate,
			MaxDiscountPct:        item.MaxDiscountPct,
			FloorRate:             item.FloorRate,
			KickoffFee:            item.KickoffFee,
			RecruitmentFee:        item.RecruitmentFee,
			OnboardingFee:         item.OnboardingFee,
		})
	}
	return views
}

func buildIntegrityExpectations(req dto.GenerateQuotationRequest, cfg *generationConfig, enriched dto.EnrichedAllocation) IntegrityExpectations {
	expectations := IntegrityExpectations{
		Currency:        enriched.Currency,
		TotalInvestment: enriched.TotalInvestment,
		Lines:           make([]ExpectedLine, 0, len(enriched.Resources)),
		LegalHTML:       cfg.legal.ContentHTML,
		BudgetMaximum:   &req.BudgetMaximum,
	}
	for _, res := range enriched.Resources {
		expectations.Lines = append(expectations.Lines, ExpectedLine{
			Role:           res.Role,
			Seniority:      res.Seniority,
			EmployeeCount:  res.EmployeeCount,
			PeriodQuarters: res.PeriodQuarters,
			PeriodMonths:   res.PeriodMonths,
			AmountPerMonth: res.AmountPerMonth,
			LineTotal:      res.LineTotal,
		})
	}
	for _, item := range req.PaymentTerms.Items {
		expectations.PaymentItems = append(expectations.PaymentItems, ExpectedPayment{Percentage: item.Percentage})
	}
	return expectations
}

// validateGenerateInput rejects structurally impossible requests BEFORE any
// persistence or AI spend.
func validateGenerateInput(req dto.GenerateQuotationRequest) error {
	if req.CommercialScheme != dedicatedSquadScheme {
		return newGenerationError(ErrCodeGenerationFailed, "only dedicated_squad commercial scheme is supported")
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" || utf8.RuneCountInString(subject) > 200 {
		return newGenerationError(ErrCodeGenerationFailed, "subject must be between 1 and 200 characters")
	}
	if req.DedicatedSquad != nil {
		if req.RateCardID == uuid.Nil {
			return newGenerationError(ErrCodeInvalidRateCard, "rate_card_id is required")
		}
		if err := req.DedicatedSquad.Validate(); err != nil {
			return newGenerationError(ErrCodeAllocationInvalid, "dedicated_squad input is invalid")
		}
		return nil
	}
	if req.ProjectDurationMonths < 1 || req.ProjectDurationMonths > 24 {
		return newGenerationError(ErrCodeGenerationFailed, "project_duration_months must be between 1 and 24")
	}
	if req.BudgetMaximum < 1 {
		return newGenerationError(ErrCodeGenerationFailed, "budget_maximum must be positive")
	}
	if len(req.MandatoryScope) == 0 {
		return newGenerationError(ErrCodeGenerationFailed, "mandatory_scope must not be empty")
	}
	for _, scope := range req.MandatoryScope {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > 200 {
			return newGenerationError(ErrCodeGenerationFailed, "mandatory_scope entries must be non-empty and at most 200 characters")
		}
	}
	if err := validatePaymentTerms(req.PaymentTerms); err != nil {
		return newGenerationError(ErrCodeGenerationFailed, "payment terms are invalid")
	}
	return nil
}

// extractJSONObject isolates the outermost JSON object from a provider reply,
// tolerating markdown code fences around it.
func extractJSONObject(raw string) []byte {
	trimmed := stripCodeFences(strings.TrimSpace(raw))
	start := strings.IndexByte(trimmed, '{')
	end := strings.LastIndexByte(trimmed, '}')
	if start < 0 || end <= start {
		return []byte(trimmed)
	}
	return []byte(trimmed[start : end+1])
}

func stripCodeFences(raw string) string {
	lines := strings.Split(raw, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		lines = lines[1:]
		if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
			lines = lines[:n-1]
		}
	}
	return strings.Join(lines, "\n")
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		// Views contain only plain scalars/slices; marshalling cannot fail.
		panic(fmt.Sprintf("marshal immutable view: %v", err))
	}
	return data
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var indonesianMonths = []string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// wibZone pins quotation issue dates to Indonesia Western Time so the
// rendered date never shifts with the server host timezone.
var wibZone = time.FixedZone("WIB", 7*3600)

func indonesianDate(t time.Time) string {
	local := t.In(wibZone)
	return fmt.Sprintf("%d %s %d", local.Day(), indonesianMonths[local.Month()-1], local.Year())
}
