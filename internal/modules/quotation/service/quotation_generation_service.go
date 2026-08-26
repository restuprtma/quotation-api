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
	DefaultAttemptTimeout  = 45 * time.Second
	SafetyEnvelopeFileName = "quotation_safety_v1.txt"
	SafetyEnvelopeVersion  = "quotation_safety_v1"
	// DefaultLegalTemplateCode selects the company's standard legal template.
	// Kept here (not inline) so a future request field can override it.
	DefaultLegalTemplateCode = "quotation_standard"

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
)

// GenerationError is the ONLY error type returned by Generate. Error() prints
// just the code, so even accidental logging of the error stays safe.
type GenerationError struct {
	Code    string
	message string
}

func (e *GenerationError) Error() string { return e.Code }

func newGenerationError(code, safeMessage string) *GenerationError {
	return &GenerationError{Code: code, message: safeMessage}
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
	genRepo        *repository.GenerationRepository
	providerSource ProviderSource
	validator      AllocationValidator
	allocator      RateAllocator
	renderer       HTMLRenderer
	sanitizer      HTMLSanitizer
	domValidator   DOMValidator
	attemptTimeout time.Duration
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
		genRepo:        genRepo,
		providerSource: source,
		validator:      NewAllocationValidator(),
		allocator:      NewRateAllocator(),
		renderer:       NewHTMLRenderer(),
		sanitizer:      NewHTMLSanitizer(),
		domValidator:   NewDOMValidator(),
		attemptTimeout: DefaultAttemptTimeout,
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
		CreatedBy:             createdBy,
	})
	if err != nil {
		return uuid.Nil, newGenerationError(ErrCodeGenerationFailed, "failed to create draft")
	}

	provider, provName, model, perr := s.providerSource.ProviderForCompany(ctx, companyID)
	if perr != nil {
		var ge *GenerationError
		if errors.As(perr, &ge) {
			return draft.QuotationID, ge
		}
		return draft.QuotationID, newGenerationError(ErrCodeGenerationFailed, "AI provider is unavailable")
	}

	enriched, gerr := s.optimizeBudget(ctx, provider, req, cfg)
	if gerr == nil {
		gerr = s.composeAndPersist(ctx, provider, req, cfg, enriched, draft.RevisionID, provName, model)
	}
	if gerr != nil {
		return draft.QuotationID, gerr
	}
	return draft.QuotationID, nil
}

// composeAndPersist performs the post-allocation half of the pipeline:
// critical rendering, narrative composition, assembly, sanitization, DOM
// integrity validation and result persistence.
func (s *QuotationGenerationService) composeAndPersist(
	ctx context.Context,
	provider AIProvider,
	req dto.GenerateQuotationRequest,
	cfg *generationConfig,
	enriched dto.EnrichedAllocation,
	revisionID uuid.UUID,
	provName, model string,
) error {
	critical, rerr := s.renderer.RenderCriticalBlocks(CriticalBlocksInput{
		Metadata: QuotationMetadata{
			QuotationNumber: cfg.quotationNumber,
			Subject:         strings.TrimSpace(req.Subject),
			ClientName:      cfg.clientName,
			IssueDate:       indonesianDate(time.Now()),
		},
		Allocation: enriched,
		Payment:    req.PaymentTerms,
		LegalHTML:  cfg.legal.ContentHTML,
	})
	if rerr != nil {
		return newGenerationError(ErrCodeUnsafeHTML, "critical blocks failed to render")
	}

	narrative, gerr := s.composeNarrative(ctx, provider, req, cfg, enriched)
	if gerr != nil {
		return gerr
	}

	assembled, aerr := s.renderer.Assemble(critical, narrative)
	if aerr != nil {
		return newGenerationError(ErrCodeUnsafeHTML, "document assembly rejected")
	}

	sanitized, serr := s.sanitizer.Sanitize(assembled)
	if serr != nil {
		return newGenerationError(ErrCodeUnsafeHTML, "document sanitization failed")
	}

	if verr := s.domValidator.ValidateIntegrity(sanitized, buildIntegrityExpectations(req, cfg, enriched)); verr != nil {
		return newGenerationError(ErrCodeUnsafeHTML, "document integrity validation failed")
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
	}); uerr != nil {
		return newGenerationError(ErrCodeGenerationFailed, "failed to persist generation result")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Configuration loading
// ---------------------------------------------------------------------------

type generationConfig struct {
	companyID        uuid.UUID
	skill            *domain.Skill
	rateCard         *domain.RateCard
	rateCardItems    []domain.RateCardItem
	legal            *domain.LegalTemplate
	clientName       string
	quotationNumber  string
	termType         string
	rateCardSnapshot json.RawMessage
	legalSnapshot    json.RawMessage
	skillSnapshot    json.RawMessage
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

	clientName, cerr := s.genRepo.GetClientName(ctx, req.ClientID)
	if cerr != nil {
		// Display-only field; a lookup outage must not block generation.
		logger.Error("Failed to resolve client name", logger.Err(cerr))
		clientName = ""
	}
	cfg.clientName = clientName

	number, err := s.genRepo.NextQuotationNumber(ctx, companyID)
	if err != nil {
		return nil, newGenerationError(ErrCodeGenerationFailed, "failed to derive quotation number")
	}
	cfg.quotationNumber = number
	cfg.termType = s.allocator.DetermineTermType(req.ProjectDurationMonths)

	cfg.rateCardSnapshot = mustJSON(rateCardSnapshotView{
		ID:       card.ID,
		Code:     card.Code,
		Name:     card.Name,
		Market:   card.Market,
		Currency: card.Currency,
		Items:    rateCardItemViews(items),
	})
	cfg.legalSnapshot = mustJSON(legalSnapshotView{
		ID:      legal.ID,
		Code:    legal.Code,
		Version: legal.Version,
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

		enriched, aerr := s.allocator.Calculate(proposal, cfg.rateCardItems, req.ProjectDurationMonths, req.BudgetMaximum, cfg.rateCard.Currency)
		if aerr == nil {
			return enriched, nil
		}

		var exceeded *dto.BudgetExceededError
		if errors.As(aerr, &exceeded) {
			if attempt == MaxProposalAttempts {
				return dto.EnrichedAllocation{}, newGenerationError(ErrCodeBudgetUnfeasible, "budget cannot accommodate a viable allocation")
			}
			feedback = &retryFeedback{
				AttemptNumber:      attempt + 1,
				MaximumAttempts:    MaxProposalAttempts,
				BudgetExcessAmount: exceeded.Excess,
			}
			continue
		}
		if errors.Is(aerr, dto.ErrMissingRateItem) {
			return dto.EnrichedAllocation{}, newGenerationError(ErrCodeRateItemMissing, "rate item is missing for the proposed allocation")
		}
		return dto.EnrichedAllocation{}, newGenerationError(ErrCodeGenerationFailed, "allocation enrichment failed")
	}
	// Defensive: unreachable while every iteration either returns or continues.
	return dto.EnrichedAllocation{}, newGenerationError(ErrCodeBudgetUnfeasible, "budget cannot accommodate a viable allocation")
}

func buildAllocationPayload(req dto.GenerateQuotationRequest, cfg *generationConfig, feedback *retryFeedback) allocationPromptPayload {
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
		AllowedRoles:       append([]string(nil), validRoles...),
		AllowedSeniorities: append([]string(nil), validSeniorities...),
		RetryFeedback:      feedback,
	}
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
{"resources":[{"role":"<one of allowed_roles>","seniority":"<one of allowed_seniorities>","employee_count":<integer >= 1>,"period_months":<integer >= 1>,"scope_references":["<exact scope strings from generation_input>"],"reason":"<short reason>"}],"selected_optional_scope":["<subset of optional_scope>"],"standard_activities":["<subset of allowed_standard_activities>"]}
Rules: every mandatory_scope item must appear in at least one resource scope_references; never invent roles, seniorities or business features; NEVER output rates, amounts, prices, discounts or totals.`

// ---------------------------------------------------------------------------
// Narrative composition
// ---------------------------------------------------------------------------

var narrativeOutputContract = []string{
	"Output ONLY an HTML fragment, no markdown fences, no explanations.",
	"Allowed tags: p, strong, em, ul, ol, li, h1, h2, h3, br.",
	"Never output attributes, data-* markers, tables, script, style, links or images.",
	"Never mention prices, totals, budgets, payment terms or legal clauses.",
	"Use Bahasa Indonesia formal.",
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

const narrativeTaskSchema = `Compose the narrative section of a business quotation: an opening paragraph, a "Ruang Lingkup dan Timeline" heading followed by one list item per scope_items entry, and a closing courtesy paragraph.`

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
	Role           string `json:"role"`
	Seniority      string `json:"seniority"`
	TermType       string `json:"term_type"`
	AmountPerMonth int64  `json:"amount_per_month"`
}

type rateCardSnapshotView struct {
	ID       uuid.UUID              `json:"id"`
	Code     string                 `json:"code"`
	Name     string                 `json:"name"`
	Market   string                 `json:"market"`
	Currency string                 `json:"currency"`
	Items    []rateCardItemSnapshot `json:"items"`
}

type legalSnapshotView struct {
	ID      uuid.UUID `json:"id"`
	Code    string    `json:"code"`
	Version int       `json:"version"`
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
			Role:           item.Role,
			Seniority:      item.Seniority,
			TermType:       item.TermType,
			AmountPerMonth: item.AmountPerMonth,
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
			Role:      res.Role,
			Seniority: res.Seniority,
			LineTotal: res.LineTotal,
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

func indonesianDate(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), indonesianMonths[t.Month()-1], t.Year())
}
