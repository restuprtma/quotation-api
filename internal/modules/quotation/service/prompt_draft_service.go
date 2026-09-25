package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

const PromptSolutionType = "custom_software"

var (
	ErrPromptInvalidRequest    = errors.New("invalid prompt-first request")
	ErrPromptInsufficientInfo  = errors.New("insufficient information")
	ErrPromptInvalidExtraction = errors.New("invalid meeting extraction")
	ErrPromptClientNotFound    = errors.New("client is not available for company")
	ErrPromptPersistence       = errors.New("prompt draft persistence failed")
)

type MeetingIdentity struct {
	ProjectName  string
	Organization string
	PIC          string
}

type PromptDraftError struct {
	Code string
	err  error
}

func (e *PromptDraftError) Error() string    { return e.Code }
func (e *PromptDraftError) Unwrap() error    { return e.err }
func (e *PromptDraftError) SafeCode() string { return e.Code }

type promptDraftRepository interface {
	FindByIdempotencyKey(context.Context, uuid.UUID, string) (*domain.QuotationRevision, error)
	GetClientNameForCompany(context.Context, uuid.UUID, uuid.UUID) (string, error)
	NextQuotationNumber(context.Context, uuid.UUID) (string, error)
	CreateDraftTransaction(context.Context, repository.CreateDraftParams) (*repository.CreateDraftResult, error)
	CompletePromptDraft(context.Context, repository.CompletePromptDraftParams) (*repository.PromptDraftSaveResult, error)
	LoadPromptDraft(context.Context, uuid.UUID, uuid.UUID) (*repository.PromptDraftRecord, error)
}

type promptCatalogResolver interface {
	ResolveByCode(context.Context, string, string, time.Time) (*domain.RateCard, []domain.RateCardItem, error)
}

type promptMeetingExtractor interface {
	Extract(context.Context, uuid.UUID, dto.MeetingExtractionRequest, CatalogContext) (domain.PromptQuotationState, error)
}

type promptDedicatedCalculator interface {
	CalculateWithProfile(context.Context, string, string, domain.DedicatedSquadInput, ResolvedQuotationProfile) (domain.DedicatedSquadCalculation, error)
}

type PromptDraftService struct {
	genRepo    promptDraftRepository
	catalog    promptCatalogResolver
	extractor  promptMeetingExtractor
	calculator promptDedicatedCalculator
	renderer   DedicatedSquadHTMLRenderer
	sanitizer  HTMLSanitizer
}

func NewPromptDraftService(
	genRepo promptDraftRepository,
	catalog promptCatalogResolver,
	extractor promptMeetingExtractor,
	calculator promptDedicatedCalculator,
	renderer DedicatedSquadHTMLRenderer,
	sanitizer HTMLSanitizer,
) *PromptDraftService {
	if renderer == nil {
		renderer = NewDedicatedSquadHTMLRenderer()
	}
	if sanitizer == nil {
		sanitizer = NewHTMLSanitizer()
	}
	return &PromptDraftService{genRepo: genRepo, catalog: catalog, extractor: extractor, calculator: calculator, renderer: renderer, sanitizer: sanitizer}
}

func (s *PromptDraftService) CreateFromMeeting(ctx context.Context, companyID uuid.UUID, userID *uuid.UUID, req dto.CreateFromMeetingRequest) (*dto.MeetingDraftResponse, error) {
	if s == nil || s.genRepo == nil || companyID == uuid.Nil {
		return nil, promptError("invalid_request", ErrPromptInvalidRequest)
	}
	notes := strings.TrimSpace(req.MeetingNotes)
	if notes == "" || len([]rune(notes)) > domain.MaxPromptMeetingNotes {
		return nil, promptError("invalid_request", ErrPromptInvalidRequest)
	}
	if req.ProjectName != nil && utf8.RuneCountInString(strings.TrimSpace(*req.ProjectName)) > 200 {
		return nil, promptError("invalid_request", ErrPromptInvalidRequest)
	}
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if _, err := uuid.Parse(idempotencyKey); err != nil {
		return nil, promptError("invalid_request", ErrPromptInvalidRequest)
	}

	if existing, err := s.genRepo.FindByIdempotencyKey(ctx, companyID, idempotencyKey); err != nil {
		return nil, promptError("generation_failed", ErrPromptPersistence)
	} else if existing != nil {
		return s.loadResponse(ctx, companyID, existing.QuotationID, true)
	}

	clientName, err := s.resolveClient(ctx, companyID, req)
	if err != nil {
		return nil, err
	}
	ident := normalizeMeetingIdentity(req, clientName)

	catalogContext, card, items, catalogErr := s.resolveCatalog(ctx, companyID)
	if catalogErr != nil && !errors.Is(catalogErr, ErrPromptCatalogEmpty) {
		return nil, promptError("commercial_input_missing", catalogErr)
	}

	if s.extractor == nil {
		return nil, promptError("generation_failed", ErrPromptInvalidExtraction)
	}
	state, err := s.extractor.Extract(ctx, companyID, dto.MeetingExtractionRequest{MeetingNotes: notes}, catalogContext)
	if err != nil {
		return nil, promptError(promptExtractionErrorCode(err), ErrPromptInvalidExtraction)
	}
	if ident.Organization != "" {
		state.ClientCompany = userFact(ident.Organization)
	}
	if ident.PIC != "" {
		state.PICName = optionalUserFact(ident.PIC)
		state.ClientName = optionalUserFact(ident.PIC) // compatibility for old snapshots/renderers
	}
	if ident.ProjectName != "" {
		state.ProjectName = userFact(ident.ProjectName)
	}
	// The extractor's readiness was computed before request-level identity
	// overrides. Recompute it only after those overrides so validation never
	// rejects a now-complete state because of a stale readiness value.
	state.Readiness = ""
	state.Issues = state.ComputeIssues()
	if catalogErr != nil {
		state.Issues = append(state.Issues, domain.DraftIssue{Code: "commercial_input_missing", Field: "rate_card", Message: "the default rate card is not available", Severity: domain.IssueBlocking})
	}
	state.Readiness = readinessForIssues(state.Issues)

	quotationNumber, err := s.genRepo.NextQuotationNumber(ctx, companyID)
	if err != nil {
		return nil, promptError("generation_failed", ErrPromptPersistence)
	}
	inputJSON, _ := json.Marshal(req)
	draft, err := s.genRepo.CreateDraftTransaction(ctx, repository.CreateDraftParams{
		CompanyID: companyID, ClientID: req.ClientID, ClientName: ident.Organization,
		QuotationNumber: quotationNumber, Subject: promptSubject(state),
		SolutionType: PromptSolutionType, CommercialScheme: domain.CommercialSchemeDedicatedSquad,
		IdempotencyKey: idempotencyKey, GenerationInput: inputJSON,
		RateCardSnapshot: snapshotRateCard(card, items), LegalTemplateSnapshot: []byte("{}"),
		SkillSnapshot: []byte("{}"), SafetyEnvelopeVersion: SafetyEnvelopeVersion, CreatedBy: userID,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, promptError("client_not_found", ErrPromptClientNotFound)
		}
		return nil, promptError("generation_failed", ErrPromptPersistence)
	}
	if draft.IdempotentReplay {
		return s.loadResponse(ctx, companyID, draft.QuotationID, true)
	}

	contentHTML := renderIncompletePromptDraft(state)
	var calculation json.RawMessage
	var readinessCalculation *domain.DedicatedSquadCalculation
	var profileSnapshot, moduleSnapshot, timelineSnapshot json.RawMessage = []byte("null"), []byte("[]"), []byte("[]")
	if catalogErr == nil && state.Readiness != domain.ReadinessIncomplete && s.calculator != nil {
		anchor := time.Now().UTC()
		input, issues := BuildDedicatedInput(&state, items, &anchor)
		var calc domain.DedicatedSquadCalculation
		var profile ResolvedQuotationProfile
		var calcErr error
		if !hasBlockingIssue(issues) {
			profile, calcErr = ResolveDedicatedSquadProfile(input)
			if calcErr == nil {
				calc, calcErr = s.calculator.CalculateWithProfile(ctx, companyID.String(), card.ID.String(), input, profile)
			}
		}
		state.Issues = append(state.Issues, issues...)
		if calcErr != nil {
			state.Issues = append(state.Issues, domain.DraftIssue{Code: "commercial_input_missing", Field: "calculation", Message: "the quotation could not be calculated from the official rate card", Severity: domain.IssueBlocking})
		} else if !hasBlockingIssue(state.Issues) {
			state.Readiness = readinessForIssues(state.Issues)
			calcBytes, _ := json.Marshal(calc)
			calculation = calcBytes
			readinessCalculation = &calc
			profileBytes, _ := json.Marshal(profile)
			profileSnapshot = profileBytes
			modules := promptModules(&state)
			moduleSnapshot, _ = json.Marshal(modules)
			rules, rulesErr := EvaluateDedicatedSquadRulesWithProfile(input, profile, calc.DevelopmentSubtotal)
			if rulesErr == nil {
				timeline, timelineErr := GenerateTimeline(input, rules, modules)
				if timelineErr == nil {
					timelineSnapshot, _ = json.Marshal(timeline)
					rendered, renderErr := s.renderer.Render(DedicatedSquadDocumentInput{
						Metadata:    QuotationMetadata{QuotationNumber: quotationNumber, Subject: promptSubject(state), ClientName: promptPICName(state), ClientCompanyName: valueOfFact(state.ClientCompany), IssueDate: time.Now().Format("2 January 2006")},
						Calculation: calc, Profile: profile, Rules: rules, Timeline: timeline,
					})
					if renderErr == nil {
						contentHTML = renderPromptPriceableHTML(rendered, &state)
					}
				}
			}
		}
	}
	readiness := EvaluatePromptReadiness(&state, readinessCalculation, PromptPaymentReadiness{
		Rows: calculationPayments(readinessCalculation),
	})
	state.Issues = readiness.Issues
	state.Readiness = readiness.Readiness
	sanitized, sanitizeErr := s.sanitizer.Sanitize(contentHTML)
	if sanitizeErr != nil || strings.TrimSpace(sanitized) == "" {
		return nil, promptError("unsafe_html", ErrPromptPersistence)
	}
	contentHTML = sanitized
	stateJSON, _ := json.Marshal(state)
	sectionsJSON, _ := json.Marshal(promptSectionHashes(contentHTML))
	hash := sha256.Sum256([]byte(contentHTML))
	hashString := hex.EncodeToString(hash[:])
	if _, err := s.genRepo.CompletePromptDraft(ctx, repository.CompletePromptDraftParams{
		CompanyID: companyID, QuotationID: draft.QuotationID, RevisionID: draft.RevisionID,
		ContentHTML: contentHTML, PromptState: stateJSON, PromptSectionHashes: sectionsJSON,
		RateCardSnapshot: snapshotRateCard(card, items), LegalTemplateSnapshot: []byte("{}"), SkillSnapshot: []byte("{}"),
		ResolvedProfileSnapshot: profileSnapshot, CalculationSnapshot: calculation,
		ModuleSnapshot: moduleSnapshot, TimelineSnapshot: timelineSnapshot, ContentHash: &hashString,
	}); err != nil {
		return nil, promptError("generation_failed", ErrPromptPersistence)
	}
	return s.loadResponse(ctx, companyID, draft.QuotationID, false)
}

func calculationPayments(calculation *domain.DedicatedSquadCalculation) []domain.PaymentRow {
	if calculation == nil {
		return nil
	}
	return calculation.Payments
}

func (s *PromptDraftService) resolveClient(ctx context.Context, companyID uuid.UUID, req dto.CreateFromMeetingRequest) (string, error) {
	if req.ClientID != nil && *req.ClientID != uuid.Nil {
		name, err := s.genRepo.GetClientNameForCompany(ctx, companyID, *req.ClientID)
		if err != nil {
			return "", promptError("generation_failed", ErrPromptClientNotFound)
		}
		if strings.TrimSpace(name) == "" {
			return "", promptError("client_not_found", ErrPromptClientNotFound)
		}
		return name, nil
	}
	if req.ClientCompanyName != nil && strings.TrimSpace(*req.ClientCompanyName) != "" {
		return strings.TrimSpace(*req.ClientCompanyName), nil
	}
	if req.ClientName != nil && strings.TrimSpace(*req.ClientName) != "" {
		return strings.TrimSpace(*req.ClientName), nil
	}
	return "", promptError("insufficient_information", ErrPromptInsufficientInfo)
}

func (s *PromptDraftService) resolveCatalog(ctx context.Context, companyID uuid.UUID) (CatalogContext, *domain.RateCard, []domain.RateCardItem, error) {
	if s.catalog == nil {
		return CatalogContext{}, nil, nil, ErrPromptCatalogEmpty
	}
	card, items, err := s.catalog.ResolveByCode(ctx, companyID.String(), domain.DefaultPromptRateCardCode, time.Now().UTC())
	if err != nil {
		return CatalogContext{RateCardCode: domain.DefaultPromptRateCardCode}, nil, nil, err
	}
	return CatalogContext{RateCardCode: card.Code, RateCardName: card.Name, Currency: card.Currency, Items: items}, card, items, nil
}

func (s *PromptDraftService) loadResponse(ctx context.Context, companyID, quotationID uuid.UUID, replay bool) (*dto.MeetingDraftResponse, error) {
	record, err := s.genRepo.LoadPromptDraft(ctx, companyID, quotationID)
	if err != nil {
		return nil, promptError("generation_failed", ErrPromptPersistence)
	}
	var state domain.PromptQuotationState
	if err := json.Unmarshal(record.PromptState, &state); err != nil {
		return nil, promptError("generation_failed", ErrPromptPersistence)
	}
	result := &dto.MeetingDraftResponse{QuotationID: record.QuotationID, RevisionID: record.RevisionID, QuotationNumber: record.QuotationNumber, ContentHTML: record.ContentHTML, ContentVersion: record.ContentVersion, StateVersion: record.StateVersion, PromptState: state, Issues: state.Issues, Readiness: state.Readiness, IdempotentReplay: replay}
	if len(record.Calculation) > 0 && string(record.Calculation) != "null" && state.Readiness != domain.ReadinessIncomplete {
		result.Calculation = record.Calculation
	}
	return result, nil
}

func promptError(code string, err error) error { return &PromptDraftError{Code: code, err: err} }

func promptExtractionErrorCode(err error) string {
	var extractionErr *MeetingExtractionError
	if !errors.As(err, &extractionErr) {
		return "invalid_extraction"
	}
	switch extractionErr.Reason {
	case "malformed_json", "trailing_content", "missing_required_field",
		"unknown_field", "schema_version", "unsupported_evidence", "state_validation":
		return "invalid_extraction." + extractionErr.Reason
	default:
		return "invalid_extraction"
	}
}

func normalizeMeetingIdentity(req dto.CreateFromMeetingRequest, resolvedOrganizationName string) MeetingIdentity {
	var ident MeetingIdentity

	if req.ProjectName != nil {
		ident.ProjectName = strings.TrimSpace(*req.ProjectName)
	}

	if strings.TrimSpace(resolvedOrganizationName) != "" {
		ident.Organization = strings.TrimSpace(resolvedOrganizationName)
	} else if req.ClientCompanyName != nil && strings.TrimSpace(*req.ClientCompanyName) != "" {
		ident.Organization = strings.TrimSpace(*req.ClientCompanyName)
	} else if req.ClientName != nil && strings.TrimSpace(*req.ClientName) != "" {
		ident.Organization = strings.TrimSpace(*req.ClientName)
	}

	if req.PICName != nil && strings.TrimSpace(*req.PICName) != "" {
		ident.PIC = strings.TrimSpace(*req.PICName)
	} else if req.ClientName != nil && strings.TrimSpace(*req.ClientName) != "" {
		ident.PIC = strings.TrimSpace(*req.ClientName)
	}

	return ident
}

func userFact(value string) *domain.Fact[string] {
	return &domain.Fact[string]{Value: value, Source: domain.SourceUser, Confirmed: true}
}

func optionalUserFact(value string) *domain.Fact[string] {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &domain.Fact[string]{Value: strings.TrimSpace(value), Source: domain.SourceUser, Confirmed: true}
}

func promptPICName(state domain.PromptQuotationState) string {
	if state.PICName != nil && strings.TrimSpace(state.PICName.Value) != "" {
		return strings.TrimSpace(state.PICName.Value)
	}
	if state.ClientName != nil && strings.TrimSpace(state.ClientName.Value) != "" {
		return strings.TrimSpace(state.ClientName.Value)
	}
	return ""
}

func promptSubject(state domain.PromptQuotationState) string {
	if state.ProjectName != nil && strings.TrimSpace(state.ProjectName.Value) != "" {
		return strings.TrimSpace(state.ProjectName.Value)
	}
	return "Quotation from meeting notes"
}

func valueOfFact(fact *domain.Fact[string]) string {
	if fact == nil {
		return ""
	}
	return fact.Value
}

func hasBlockingIssue(issues []domain.DraftIssue) bool {
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			return true
		}
	}
	return false
}

func readinessForIssues(issues []domain.DraftIssue) string {
	if hasBlockingIssue(issues) {
		return domain.ReadinessIncomplete
	}
	if len(issues) > 0 {
		return domain.ReadinessPriceable
	}
	return domain.ReadinessReadyForFinal
}

func snapshotRateCard(card *domain.RateCard, items []domain.RateCardItem) json.RawMessage {
	if card == nil {
		return []byte("null")
	}
	return mustJSON(struct {
		Card  domain.RateCard       `json:"card"`
		Items []domain.RateCardItem `json:"items"`
	}{Card: *card, Items: items})
}

func promptModules(state *domain.PromptQuotationState) []domain.Module {
	modules := make([]domain.Module, 0, len(state.Scope))
	for _, item := range state.Scope {
		if strings.TrimSpace(item.Value) != "" {
			modules = append(modules, domain.Module{Name: strings.TrimSpace(item.Value)})
		}
	}
	if len(modules) == 0 {
		modules = append(modules, domain.Module{Name: "Scope hasil meeting"})
	}
	return modules
}

func renderIncompletePromptDraft(state domain.PromptQuotationState) string {
	var b strings.Builder
	b.WriteString(`<article class="quotation-document prompt-first">`)
	b.WriteString(`<section data-quotation-section="scope" data-section-version="1"><h2>Lingkup Pekerjaan</h2><ul>`)
	for _, item := range state.Scope {
		b.WriteString(`<li>` + html.EscapeString(item.Value) + `</li>`)
	}
	b.WriteString(`</ul></section>`)
	b.WriteString(`<section data-quotation-section="commercial" data-section-version="1" data-managed="true"><p>Informasi biaya belum lengkap dan belum dihitung.</p></section>`)
	b.WriteString(`<section data-quotation-section="payment" data-section-version="1" data-managed="true"><p>Termin pembayaran menunggu informasi yang belum dikonfirmasi.</p></section>`)
	b.WriteString(`<section data-quotation-section="information-missing" data-section-version="1"><ul>`)
	for _, issue := range state.Issues {
		b.WriteString(`<li data-issue-code="` + html.EscapeString(issue.Code) + `">` + html.EscapeString(issue.Message) + `</li>`)
	}
	b.WriteString(`</ul></section></article>`)
	return b.String()
}

func renderPromptPriceableHTML(rendered string, state *domain.PromptQuotationState) string {
	return `<article class="quotation-document prompt-first"><section data-quotation-section="scope" data-section-version="1"><h2>Lingkup Pekerjaan</h2><ul>` + scopeHTML(state.Scope) + `</ul></section><section data-quotation-section="commercial" data-section-version="1" data-managed="true">` + rendered + `</section><section data-quotation-section="payment" data-section-version="1" data-managed="true"><p>Rincian termin mengikuti hasil kalkulasi backend.</p></section></article>`
}

func scopeHTML(scope []domain.Fact[string]) string {
	var b strings.Builder
	for _, item := range scope {
		b.WriteString(`<li>` + html.EscapeString(item.Value) + `</li>`)
	}
	return b.String()
}

func promptSectionHashes(content string) map[string]string {
	result := make(map[string]string, 3)
	for _, section := range []string{"scope", "commercial", "payment"} {
		value := content
		if rendered, err := promptSectionMarkup(content, section); err == nil {
			value = rendered
		}
		hash := sha256.Sum256([]byte(value))
		result[section] = hex.EncodeToString(hash[:])
	}
	return result
}
