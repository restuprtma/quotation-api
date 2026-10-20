package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type promptRepoStub struct {
	companySeen        uuid.UUID
	clientName         string
	nextNumber         string
	draft              *repository.CreateDraftResult
	record             *repository.PromptDraftRecord
	createParams       repository.CreateDraftParams
	claim              *repository.PromptDraftClaim
	failureCode        string
	completeErr        error
	nextNumberCalls    int
	createDraftCalls   int
	completeDraftCalls int
}

func (r *promptRepoStub) FindByIdempotencyKey(context.Context, uuid.UUID, string) (*domain.QuotationRevision, error) {
	if r.record == nil {
		return nil, nil
	}
	return &domain.QuotationRevision{QuotationID: r.record.QuotationID}, nil
}
func (r *promptRepoStub) GetClientNameForCompany(_ context.Context, companyID, _ uuid.UUID) (string, error) {
	r.companySeen = companyID
	return r.clientName, nil
}
func (r *promptRepoStub) NextQuotationNumber(context.Context, uuid.UUID) (string, error) {
	r.nextNumberCalls++
	if r.nextNumber == "" {
		r.nextNumber = "Q-2026-001"
	}
	return r.nextNumber, nil
}
func (r *promptRepoStub) CreateDraftTransaction(_ context.Context, params repository.CreateDraftParams) (*repository.CreateDraftResult, error) {
	r.createDraftCalls++
	r.createParams = params
	if r.draft == nil {
		r.draft = &repository.CreateDraftResult{QuotationID: uuid.New(), RevisionID: uuid.New()}
	}
	return r.draft, nil
}
func (r *promptRepoStub) ClaimPromptDraft(_ context.Context, _ uuid.UUID, quotationID uuid.UUID) (*repository.PromptDraftClaim, error) {
	if r.record != nil && r.record.GenerationStatus == repository.GenerationStatusCompleted {
		return nil, repository.ErrGenerationAlreadyCompleted
	}
	if r.claim == nil {
		rateSnapshot := r.createParams.RateCardSnapshot
		if string(rateSnapshot) == "null" || len(rateSnapshot) == 0 {
			rateSnapshot = []byte(`{"card":{"id":"11111111-1111-1111-1111-111111111111","code":"VENTURO-STD-2026","name":"Default","currency":"IDR"},"items":[{"id":"22222222-2222-2222-2222-222222222222","role":"frontend_programmer","display_name":"Frontend Programmer","seniority":"intermediate","term_type":"long_term"}]}`)
		}
		r.claim = &repository.PromptDraftClaim{QuotationID: quotationID, RevisionID: r.draft.RevisionID, QuotationNumber: r.nextNumber, GenerationInput: r.createParams.GenerationInput, RateCardSnapshot: rateSnapshot}
	}
	return r.claim, nil
}
func (r *promptRepoStub) FailPromptDraft(_ context.Context, _ uuid.UUID, _, _ uuid.UUID, code string) error {
	r.failureCode = code
	return nil
}
func (r *promptRepoStub) CompletePromptDraft(_ context.Context, params repository.CompletePromptDraftParams) (*repository.PromptDraftSaveResult, error) {
	r.completeDraftCalls++
	if r.completeErr != nil {
		return nil, r.completeErr
	}
	r.record = &repository.PromptDraftRecord{QuotationID: params.QuotationID, RevisionID: params.RevisionID, QuotationNumber: r.nextNumber, GenerationStatus: repository.GenerationStatusCompleted, ContentHTML: params.ContentHTML, ContentVersion: 1, StateVersion: 1, PromptState: json.RawMessage(params.PromptState), Calculation: params.CalculationSnapshot, RateCardSnapshot: params.RateCardSnapshot}
	return &repository.PromptDraftSaveResult{RevisionID: params.RevisionID, ContentVersion: 1, StateVersion: 1}, nil
}

type rejectingPromptSanitizer struct{}

func (rejectingPromptSanitizer) Sanitize(string) (string, error) {
	return "", errors.New("unsafe html")
}
func (r *promptRepoStub) LoadPromptDraft(context.Context, uuid.UUID, uuid.UUID) (*repository.PromptDraftRecord, error) {
	return r.record, nil
}

type promptExtractorStub struct{ state domain.PromptQuotationState }

func (e promptExtractorStub) Extract(context.Context, uuid.UUID, dto.MeetingExtractionRequest, CatalogContext) (domain.PromptQuotationState, error) {
	return e.state, nil
}

type countingPromptExtractor struct {
	state domain.PromptQuotationState
	err   error
	calls int
}

func (e *countingPromptExtractor) Extract(context.Context, uuid.UUID, dto.MeetingExtractionRequest, CatalogContext) (domain.PromptQuotationState, error) {
	e.calls++
	if e.err != nil {
		return domain.PromptQuotationState{}, e.err
	}
	return e.state, nil
}

type promptExtractionErrorStub struct{ err error }

func (e promptExtractionErrorStub) Extract(context.Context, uuid.UUID, dto.MeetingExtractionRequest, CatalogContext) (domain.PromptQuotationState, error) {
	return domain.PromptQuotationState{}, e.err
}

type promptCatalogStub struct {
	card  *domain.RateCard
	items []domain.RateCardItem
}

type promptCalculatorStub struct{}

func (promptCalculatorStub) CalculateWithProfile(context.Context, string, string, domain.DedicatedSquadInput, ResolvedQuotationProfile) (domain.DedicatedSquadCalculation, error) {
	line := domain.SquadLineCalculation{SquadLineInput: domain.SquadLineInput{Role: "programmer", Seniority: "intermediate", HeadcountQuarters: 8, PeriodQuarters: 12}, AmountPerUnit: 10_000_000, LineTotal: 30_000_000}
	return domain.DedicatedSquadCalculation{Model: domain.ModelSOWSmall, Development: []domain.SquadLineCalculation{line}, DevelopmentSubtotal: 30_000_000, Total: 30_000_000, TotalInvestment: 30_000_000, Payments: []domain.PaymentRow{{Nominal: 15_000_000}, {Nominal: 15_000_000}}}, nil
}

type promptRendererStub struct{}

func (promptRendererStub) Render(DedicatedSquadDocumentInput) (string, error) {
	return `<article><p>deterministic quotation</p></article>`, nil
}

func (c promptCatalogStub) ResolveByCode(context.Context, string, string, time.Time) (*domain.RateCard, []domain.RateCardItem, error) {
	return c.card, c.items, nil
}

func (c promptCatalogStub) ResolveDefaultCard(context.Context, string, time.Time) (*domain.RateCard, []domain.RateCardItem, error) {
	return c.card, c.items, nil
}

func TestPromptDraftService_UsesCompanyScopedClientLookup(t *testing.T) {
	companyID := uuid.New()
	repo := &promptRepoStub{clientName: "PT Arunika"}
	state := incompletePromptTestState()
	svc := NewPromptDraftService(repo, nil, promptExtractorStub{state: state}, nil, nil, nil)
	clientID := uuid.New()

	_, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
		ClientID:       &clientID,
		MeetingNotes:   state.MeetingNotes,
		IdempotencyKey: uuid.New().String(),
	})
	require.NoError(t, err)
	require.Equal(t, companyID, repo.companySeen)
}

func TestPromptDraftService_LoadResponseJSONSnapshotRedactsPersistedMeetingNotes(t *testing.T) {
	companyID, quotationID := uuid.New(), uuid.New()
	repo := &promptRepoStub{record: &repository.PromptDraftRecord{
		QuotationID: quotationID, RevisionID: uuid.New(), QuotationNumber: "Q-2026-001",
		PromptState: json.RawMessage(`{"schema_version":"prompt_quotation_v1","meeting_notes":"CANARY-PERSISTED-MEETING-NOTES-MUST-NOT-LEAK","scope":[],"team":[],"issues":[],"readiness":"incomplete"}`),
	}}
	svc := NewPromptDraftService(repo, nil, nil, nil, nil, nil)

	response, err := svc.loadResponse(context.Background(), companyID, quotationID, false)
	require.NoError(t, err)
	body, err := json.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "meeting_notes")
	assert.NotContains(t, string(body), "CANARY-PERSISTED-MEETING-NOTES-MUST-NOT-LEAK")
}

func TestCreateMeetingDraft_PersistsWithoutCallingExtractor(t *testing.T) {
	companyID := uuid.New()
	repo := &promptRepoStub{clientName: "PT Arunika"}
	extractor := &countingPromptExtractor{err: errors.New("must not be called")}
	svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)
	req := dto.CreateFromMeetingRequest{ClientName: stringPtr("PT Arunika"), MeetingNotes: "Membutuhkan aplikasi inventori.", IdempotencyKey: uuid.New().String()}

	pending, err := svc.CreateMeetingDraft(context.Background(), companyID, nil, req)
	require.NoError(t, err)
	assert.Equal(t, 1, repo.createDraftCalls)
	assert.Zero(t, extractor.calls)
	assert.Equal(t, repository.GenerationStatusPending, pending.GenerationStatus)
}

func TestCreateMeetingDraft_ResolvesDynamicDefaultCard(t *testing.T) {
	companyID := uuid.New()
	repo := &promptRepoStub{clientName: "PT Arunika"}
	customCard := &domain.RateCard{
		ID:        uuid.New(),
		CompanyID: companyID,
		Code:      "CUSTOM-STD-2026",
		Name:      "Custom Tenant Default",
		Currency:  "IDR",
		IsDefault: true,
		IsActive:  true,
	}
	customItems := []domain.RateCardItem{
		{ID: uuid.New(), CompanyID: companyID, RateCardID: customCard.ID, Role: "frontend_programmer", Seniority: "intermediate", TermType: "long_term"},
	}
	catalog := promptCatalogStub{card: customCard, items: customItems}
	svc := NewPromptDraftService(repo, catalog, nil, nil, nil, nil)
	req := dto.CreateFromMeetingRequest{
		ClientName:     stringPtr("PT Arunika"),
		MeetingNotes:   "Aplikasi mobile inventory.",
		IdempotencyKey: uuid.New().String(),
	}

	pending, err := svc.CreateMeetingDraft(context.Background(), companyID, nil, req)
	require.NoError(t, err)
	require.NotNil(t, pending)
	require.NotNil(t, repo.createParams.RateCardSnapshot)
	assert.Contains(t, string(repo.createParams.RateCardSnapshot), "CUSTOM-STD-2026")
}

func TestProcessMeetingDraft_TimeoutMarksSameDraftFailed(t *testing.T) {
	companyID := uuid.New()
	repo := &promptRepoStub{clientName: "PT Arunika"}
	extractor := &countingPromptExtractor{err: newGenerationError(ErrCodeProviderTimeout, "AI provider timed out")}
	svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)
	req := dto.CreateFromMeetingRequest{ClientName: stringPtr("PT Arunika"), MeetingNotes: "Membutuhkan aplikasi inventori.", IdempotencyKey: uuid.New().String()}

	pending, err := svc.CreateMeetingDraft(context.Background(), companyID, nil, req)
	require.NoError(t, err)
	_, err = svc.ProcessMeetingDraft(context.Background(), companyID, pending.QuotationID)
	require.Error(t, err)
	assert.Equal(t, ErrCodeProviderTimeout, repo.failureCode)
	require.NotNil(t, repo.claim)
	assert.Equal(t, pending.QuotationID, repo.claim.QuotationID)
}

func TestProcessMeetingDraft_InvalidStoredInputDoesNotCallProvider(t *testing.T) {
	companyID, quotationID, revisionID := uuid.New(), uuid.New(), uuid.New()
	repo := &promptRepoStub{claim: &repository.PromptDraftClaim{
		QuotationID: quotationID, RevisionID: revisionID, QuotationNumber: "Q-2026-001",
		GenerationInput: json.RawMessage(`{"meeting_notes":7}`),
	}}
	extractor := &countingPromptExtractor{}
	svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)

	_, err := svc.ProcessMeetingDraft(context.Background(), companyID, quotationID)
	require.Error(t, err)
	assert.Zero(t, extractor.calls)
	assert.Equal(t, "invalid_extraction.malformed_json", repo.failureCode)
	var extractionErr *MeetingExtractionError
	require.ErrorAs(t, err, &extractionErr)
	assert.Equal(t, "stored_input_decode", extractionErr.Stage)
	var typeErr *json.UnmarshalTypeError
	require.ErrorAs(t, extractionErr.Cause, &typeErr)
	assert.Equal(t, "meeting_notes", typeErr.Field)
}

func TestProcessMeetingDraft_RetryCompletesOriginalDraft(t *testing.T) {
	companyID := uuid.New()
	repo := &promptRepoStub{clientName: "PT Arunika"}
	extractor := &countingPromptExtractor{err: newGenerationError(ErrCodeProviderTimeout, "AI provider timed out")}
	svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)
	req := dto.CreateFromMeetingRequest{ClientName: stringPtr("PT Arunika"), MeetingNotes: incompletePromptTestState().MeetingNotes, IdempotencyKey: uuid.New().String()}

	pending, err := svc.CreateMeetingDraft(context.Background(), companyID, nil, req)
	require.NoError(t, err)
	_, err = svc.ProcessMeetingDraft(context.Background(), companyID, pending.QuotationID)
	require.Error(t, err)
	extractor.err = nil
	extractor.state = incompletePromptTestState()
	result, err := svc.ProcessMeetingDraft(context.Background(), companyID, pending.QuotationID)
	require.NoError(t, err)
	assert.Equal(t, pending.QuotationID, result.QuotationID)
	assert.Equal(t, 1, repo.createDraftCalls)
	assert.Equal(t, 2, extractor.calls)
}

func TestProcessMeetingDraft_PersistsPostClaimFailuresOnce(t *testing.T) {
	companyID := uuid.New()
	request := func() dto.CreateFromMeetingRequest {
		return dto.CreateFromMeetingRequest{ClientName: stringPtr("PT Arunika"), MeetingNotes: incompletePromptTestState().MeetingNotes, IdempotencyKey: uuid.New().String()}
	}

	tests := []struct {
		name      string
		extractor *countingPromptExtractor
		sanitizer HTMLSanitizer
		complete  error
		wantCode  string
	}{
		{name: "invalid extraction", extractor: &countingPromptExtractor{err: &MeetingExtractionError{Reason: "malformed_json"}}, wantCode: "invalid_extraction.malformed_json"},
		{name: "canceled provider request", extractor: &countingPromptExtractor{err: context.Canceled}, wantCode: "invalid_extraction"},
		{name: "sanitizer rejection", extractor: &countingPromptExtractor{state: incompletePromptTestState()}, sanitizer: rejectingPromptSanitizer{}, wantCode: "unsafe_html"},
		{name: "completion persistence", extractor: &countingPromptExtractor{state: incompletePromptTestState()}, complete: errors.New("write failed"), wantCode: "generation_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &promptRepoStub{clientName: "PT Arunika", completeErr: tt.complete}
			svc := NewPromptDraftService(repo, nil, tt.extractor, nil, nil, tt.sanitizer)
			pending, err := svc.CreateMeetingDraft(context.Background(), companyID, nil, request())
			require.NoError(t, err)
			_, err = svc.ProcessMeetingDraft(context.Background(), companyID, pending.QuotationID)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, repo.failureCode)
			assert.Equal(t, 1, repo.createDraftCalls)
		})
	}
}

func TestPromptDraftService_IncompleteDraftHasNoFictitiousCalculation(t *testing.T) {
	repo := &promptRepoStub{clientName: "PT Arunika"}
	state := incompletePromptTestState()
	svc := NewPromptDraftService(repo, nil, promptExtractorStub{state: state}, nil, nil, nil)

	result, err := svc.CreateFromMeeting(context.Background(), uuid.New(), nil, dto.CreateFromMeetingRequest{
		ClientName:     stringPtr("PT Arunika"),
		MeetingNotes:   state.MeetingNotes,
		IdempotencyKey: uuid.New().String(),
	})
	require.NoError(t, err)
	require.Equal(t, domain.ReadinessIncomplete, result.Readiness)
	require.Empty(t, result.Calculation)
	require.Contains(t, result.ContentHTML, "information-missing")
}

func TestPromptDraftService_NoBusinessSignalFailsPersistedDraft(t *testing.T) {
	repo := &promptRepoStub{clientName: "PT Arunika"}
	svc := NewPromptDraftService(repo, nil, promptExtractorStub{state: domain.PromptQuotationState{}}, nil, nil, nil)

	_, err := svc.CreateFromMeeting(context.Background(), uuid.New(), nil, dto.CreateFromMeetingRequest{
		ClientName:     stringPtr("PT Arunika"),
		MeetingNotes:   "Catatan hanya berisi identitas.",
		IdempotencyKey: uuid.New().String(),
	})

	var safe *PromptDraftError
	require.ErrorAs(t, err, &safe)
	assert.Equal(t, "meeting_notes_no_business_signal", safe.SafeCode())
	assert.Equal(t, 1, repo.nextNumberCalls)
	assert.Equal(t, 1, repo.createDraftCalls)
	assert.Zero(t, repo.completeDraftCalls)
	assert.Equal(t, "meeting_notes_no_business_signal", repo.failureCode)
}

func TestPromptDraftService_IdempotencyReplayLoadsSameDraft(t *testing.T) {
	repo := &promptRepoStub{clientName: "PT Arunika"}
	state := incompletePromptTestState()
	svc := NewPromptDraftService(repo, nil, promptExtractorStub{state: state}, nil, nil, nil)
	req := dto.CreateFromMeetingRequest{ClientName: stringPtr("PT Arunika"), MeetingNotes: state.MeetingNotes, IdempotencyKey: uuid.New().String()}

	first, err := svc.CreateFromMeeting(context.Background(), uuid.New(), nil, req)
	require.NoError(t, err)
	second, err := svc.CreateFromMeeting(context.Background(), uuid.New(), nil, req)
	require.NoError(t, err)
	require.Equal(t, first.QuotationID, second.QuotationID)
	require.True(t, second.IdempotentReplay)
}

func TestPromptDraftService_PriceableDraftIncludesBackendCalculation(t *testing.T) {
	cardID, itemID := uuid.New(), uuid.New()
	card := &domain.RateCard{ID: cardID, Code: domain.DefaultPromptRateCardCode, Name: "Venturo Standard", Currency: "IDR"}
	// A meeting may ask for a "programmer", but the resolved item must remain
	// an existing catalog role; this fixture must not invent a Programmer rate.
	item := domain.RateCardItem{ID: itemID, RateCardID: cardID, Role: "frontend_programmer", DisplayName: "Frontend Programmer", Seniority: "intermediate", TermType: "long_term", AmountPerMonth: 10_000_000}
	state := completePromptTestState(itemID.String())
	repo := &promptRepoStub{clientName: "PT Arunika"}
	svc := NewPromptDraftService(repo, promptCatalogStub{card: card, items: []domain.RateCardItem{item}}, promptExtractorStub{state: state}, promptCalculatorStub{}, promptRendererStub{}, nil)

	result, err := svc.CreateFromMeeting(context.Background(), uuid.New(), nil, dto.CreateFromMeetingRequest{
		ClientName: stringPtr("PT Arunika"), MeetingNotes: state.MeetingNotes, IdempotencyKey: uuid.New().String(),
	})
	require.NoError(t, err)
	require.Equal(t, domain.ReadinessPriceable, result.Readiness)
	require.NotEmpty(t, result.Calculation)
	require.Contains(t, result.ContentHTML, "data-quotation-section=\"commercial\"")
	require.Equal(t, []dto.PromptCatalogRole{{RateItemID: itemID, Role: item.Role, DisplayName: item.DisplayName, Seniority: item.Seniority, TermType: item.TermType}}, result.CatalogRoles)
}

func TestPromptDraftService_ResolvesUnresolvedRoleFromCatalogWithMultipleTerms(t *testing.T) {
	cardID, longID, shortID := uuid.New(), uuid.New(), uuid.New()
	card := &domain.RateCard{ID: cardID, Code: domain.DefaultPromptRateCardCode, Name: "Venturo Standard", Currency: "IDR"}
	items := []domain.RateCardItem{
		{ID: longID, RateCardID: cardID, Role: "frontend_programmer", DisplayName: "Frontend Programmer", Seniority: "intermediate", TermType: "long_term", AmountPerMonth: 10_000_000},
		{ID: shortID, RateCardID: cardID, Role: "frontend_programmer", DisplayName: "Frontend Programmer", Seniority: "intermediate", TermType: "short_term", AmountPerMonth: 14_000_000},
	}
	// State has line with ResolvedRateItemID = nil (as would happen before disambiguation)
	state := completePromptTestState("")
	state.Team[0].RequestedRole = "frontend_programmer"
	state.Team[0].ResolvedRateItemID = nil
	repo := &promptRepoStub{clientName: "PT Arunika"}
	svc := NewPromptDraftService(repo, promptCatalogStub{card: card, items: items}, promptExtractorStub{state: state}, promptCalculatorStub{}, promptRendererStub{}, nil)

	result, err := svc.CreateFromMeeting(context.Background(), uuid.New(), nil, dto.CreateFromMeetingRequest{
		ClientName: stringPtr("PT Arunika"), MeetingNotes: state.MeetingNotes, IdempotencyKey: uuid.New().String(),
	})
	require.NoError(t, err)
	require.Equal(t, domain.ReadinessPriceable, result.Readiness)
	require.NotEmpty(t, result.Calculation)
	require.Equal(t, longID.String(), *result.PromptState.Team[0].ResolvedRateItemID)
}

func TestPromptFirstSanitizerPreservesSectionMarkers(t *testing.T) {
	html, err := NewHTMLSanitizer().Sanitize(`<article class="quotation-document"><section data-quotation-section="commercial" data-section-version="1" data-managed="true">cost</section></article>`)
	require.NoError(t, err)
	require.Contains(t, html, `data-quotation-section="commercial"`)
	require.Contains(t, html, `data-section-version="1"`)
	require.Contains(t, html, `data-managed="true"`)
}

func incompletePromptTestState() domain.PromptQuotationState {
	return domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		MeetingNotes:  "Bu Nadia dari PT Arunika membahas aplikasi inventory.",
		Scope:         []domain.Fact[string]{{Value: "Stok gudang", Source: domain.SourceMeeting, Evidence: "Stok gudang"}},
	}
}

func completePromptTestState(itemID string) domain.PromptQuotationState {
	confirmed := func(value, evidence string) *domain.Fact[string] {
		return &domain.Fact[string]{Value: value, Source: domain.SourceMeeting, Evidence: evidence, Confirmed: true}
	}
	duration := 3
	return domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		MeetingNotes:  "Bu Nadia dari PT Arunika butuh aplikasi inventory selama tiga bulan dengan satu programmer intermediate.",
		ClientName:    confirmed("Nadia", "Bu Nadia"), ClientCompany: confirmed("PT Arunika", "PT Arunika"),
		ProjectName: confirmed("Arunika Inventory", "aplikasi inventory"), CommercialScheme: confirmed(domain.CommercialSchemeDedicatedSquad, "dedicated squad"),
		Scope:          []domain.Fact[string]{{Value: "Stok gudang", Source: domain.SourceMeeting, Evidence: "Stok gudang", Confirmed: true}},
		DurationMonths: &domain.Fact[int]{Value: duration, Source: domain.SourceMeeting, Evidence: "tiga bulan", Confirmed: true},
		Team:           []domain.PromptTeamLine{{RequestedRole: "programmer", ResolvedRateItemID: &itemID, Seniority: "intermediate", HeadcountQuarters: 8, PeriodQuarters: 12, Source: domain.SourceMeeting, Evidence: "programmer intermediate", Confirmed: true}},
		PaymentPattern: &domain.Fact[string]{Value: "50:50", Source: domain.SourceMeeting, Evidence: "50:50", Confirmed: false},
	}
}

func stringPtr(value string) *string { return &value }

func TestNormalizeMeetingIdentityCompatibility(t *testing.T) {
	req := func(projectName, company, pic string, clientName *string, clientID *uuid.UUID) dto.CreateFromMeetingRequest {
		r := dto.CreateFromMeetingRequest{
			MeetingNotes:   "notes",
			IdempotencyKey: uuid.New().String(),
		}
		if projectName != "" {
			r.ProjectName = &projectName
		}
		if company != "" {
			r.ClientCompanyName = &company
		}
		if pic != "" {
			r.PICName = &pic
		}
		if clientName != nil {
			r.ClientName = clientName
		}
		if clientID != nil {
			r.ClientID = clientID
		}
		return r
	}
	legacyReq := func(company, pic string) dto.CreateFromMeetingRequest {
		return dto.CreateFromMeetingRequest{
			ClientCompanyName: &company,
			ClientName:        &pic,
			MeetingNotes:      "notes",
			IdempotencyKey:    uuid.New().String(),
		}
	}
	legacyOnly := func(legacyClient string) dto.CreateFromMeetingRequest {
		return dto.CreateFromMeetingRequest{
			ClientName:     &legacyClient,
			MeetingNotes:   "notes",
			IdempotencyKey: uuid.New().String(),
		}
	}
	reqWithID := func(projectName, companyOrClient, pic string) dto.CreateFromMeetingRequest {
		id := uuid.New()
		return dto.CreateFromMeetingRequest{
			ClientID:          &id,
			ProjectName:       &projectName,
			ClientCompanyName: &companyOrClient,
			PICName:           &pic,
			MeetingNotes:      "notes",
			IdempotencyKey:    uuid.New().String(),
		}
	}
	identity := func(projectName, org, pic string) MeetingIdentity {
		return MeetingIdentity{
			ProjectName:  projectName,
			Organization: org,
			PIC:          pic,
		}
	}

	tests := []struct {
		name     string
		req      dto.CreateFromMeetingRequest
		resolved string
		want     MeetingIdentity
	}{
		{"canonical", req("Project A", "PT Acme", "Rani", nil, nil), "", identity("Project A", "PT Acme", "Rani")},
		{"legacy pair", legacyReq("PT Acme", "Rani"), "", identity("", "PT Acme", "Rani")},
		{"legacy client only", legacyOnly("PT Acme"), "", identity("", "PT Acme", "PT Acme")},
		{"selected registry wins", reqWithID("Project A", "stale", "Rani"), "PT Canonical", identity("Project A", "PT Canonical", "Rani")},
		{"legacy client and pic without company", dto.CreateFromMeetingRequest{ClientName: stringPtr("PT Legacy"), PICName: stringPtr("Rani"), MeetingNotes: "notes", IdempotencyKey: uuid.New().String()}, "", identity("", "PT Legacy", "Rani")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeMeetingIdentity(tt.req, tt.resolved)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPromptDraftService_PreservesSafeExtractionReason(t *testing.T) {
	companyID := uuid.New()
	repo := &promptRepoStub{clientName: "PT Arunika"}

	t.Run("allowlisted reason preserved", func(t *testing.T) {
		extractor := promptExtractionErrorStub{
			err: &MeetingExtractionError{Reason: "unsupported_evidence", Field: "duration_months"},
		}
		svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)

		_, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
			ClientName:     stringPtr("PT Arunika"),
			MeetingNotes:   "Catatan meeting untuk quotation",
			IdempotencyKey: uuid.New().String(),
		})
		require.Error(t, err)

		var safe interface{ SafeCode() string }
		require.ErrorAs(t, err, &safe)
		assert.Equal(t, "invalid_extraction.unsupported_evidence.duration_months", safe.SafeCode())
	})

	t.Run("unsafe evidence field falls back to generic evidence code", func(t *testing.T) {
		extractor := promptExtractionErrorStub{
			err: &MeetingExtractionError{Reason: "unsupported_evidence", Field: "scope[0].evidence: secret"},
		}
		svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)

		_, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
			ClientName:     stringPtr("PT Arunika"),
			MeetingNotes:   "Catatan meeting untuk quotation",
			IdempotencyKey: uuid.New().String(),
		})
		require.Error(t, err)

		var safe interface{ SafeCode() string }
		require.ErrorAs(t, err, &safe)
		assert.Equal(t, "invalid_extraction.unsupported_evidence", safe.SafeCode())
	})

	t.Run("unrecognized reason falls back to invalid_extraction", func(t *testing.T) {
		extractor := promptExtractionErrorStub{
			err: &MeetingExtractionError{Reason: "provider response: secret"},
		}
		svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)

		_, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
			ClientName:     stringPtr("PT Arunika"),
			MeetingNotes:   "Catatan meeting untuk quotation",
			IdempotencyKey: uuid.New().String(),
		})
		require.Error(t, err)

		var safe interface{ SafeCode() string }
		require.ErrorAs(t, err, &safe)
		assert.Equal(t, "invalid_extraction", safe.SafeCode())
	})

	t.Run("generic non-extraction error falls back to invalid_extraction", func(t *testing.T) {
		extractor := promptExtractionErrorStub{
			err: errors.New("database connection failed"),
		}
		svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)

		_, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
			ClientName:     stringPtr("PT Arunika"),
			MeetingNotes:   "Catatan meeting untuk quotation",
			IdempotencyKey: uuid.New().String(),
		})
		require.Error(t, err)

		var safe interface{ SafeCode() string }
		require.ErrorAs(t, err, &safe)
		assert.Equal(t, "invalid_extraction", safe.SafeCode())
	})

	t.Run("provider safe code is preserved", func(t *testing.T) {
		extractor := promptExtractionErrorStub{
			err: newGenerationError(ErrCodeProviderTimeout, "AI provider timed out"),
		}
		svc := NewPromptDraftService(repo, nil, extractor, nil, nil, nil)

		_, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
			ClientName:     stringPtr("PT Arunika"),
			MeetingNotes:   "Catatan meeting untuk quotation",
			IdempotencyKey: uuid.New().String(),
		})
		require.Error(t, err)

		var safe interface{ SafeCode() string }
		require.ErrorAs(t, err, &safe)
		assert.Equal(t, ErrCodeProviderTimeout, safe.SafeCode())
	})
}

func TestPromptDraftService_CompleteIncompleteDraftScopeOnly(t *testing.T) {
	companyID := uuid.New()
	notes := "Klien membutuhkan modul stok gudang."
	scopeState := domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		MeetingNotes:  notes,
		Scope: []domain.Fact[string]{
			{Value: "modul stok gudang", Source: domain.SourceMeeting, Evidence: "stok gudang", Confirmed: false},
		},
		Team:      []domain.PromptTeamLine{},
		Issues:    []domain.DraftIssue{},
		Readiness: domain.ReadinessIncomplete,
	}

	repo := &promptRepoStub{clientName: "PT Stok Mas"}
	extractor := promptExtractorStub{state: scopeState}
	svc := NewPromptDraftService(repo, nil, extractor, promptCalculatorStub{}, promptRendererStub{}, NewHTMLSanitizer())

	resp, err := svc.CreateFromMeeting(context.Background(), companyID, nil, dto.CreateFromMeetingRequest{
		ClientCompanyName: stringPtr("PT Stok Mas"),
		MeetingNotes:      notes,
		IdempotencyKey:    uuid.New().String(),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, domain.ReadinessIncomplete, resp.Readiness)
	assert.Equal(t, repo.draft.QuotationID, resp.QuotationID)
	assert.Equal(t, 1, repo.completeDraftCalls)
	assert.Nil(t, repo.record.Calculation)
}

func TestRenderPromptPriceableHTML_NoNestedArticle(t *testing.T) {
	renderer := NewDedicatedSquadHTMLRenderer()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	meta := QuotationMetadata{
		QuotationNumber:   "QUO-2026-0001",
		Subject:           "Penawaran Dedicated Squad",
		ClientName:        "Budi",
		ClientCompanyName: "PT Arunika",
		IssueDate:         "23 September 2026",
	}
	calc := domain.DedicatedSquadCalculation{
		Model: domain.ModelSOWSmall,
		Development: []domain.SquadLineCalculation{
			{SquadLineInput: domain.SquadLineInput{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 12}, AmountPerUnit: 15_000_000, LineTotal: 45_000_000},
		},
		Payments: []domain.PaymentRow{
			{WorkPeriod: "Termin 1", Terms: "DP 50%", InvoiceDate: now, DueDate: now.AddDate(0, 0, 14), Nominal: 22_500_000},
			{WorkPeriod: "Termin 2", Terms: "Pelunasan 50%", InvoiceDate: now.AddDate(0, 1, 0), DueDate: now.AddDate(0, 1, 14), Nominal: 22_500_000},
		},
		Total:           45_000_000,
		TotalInvestment: 45_000_000,
	}
	profile := ResolvedQuotationProfile{
		DocumentOptions: ResolvedDocumentOptions{LetterPrefix: "VTR"},
	}
	rules := DedicatedSquadRules{}
	timeline := []domain.TimelineRow{
		{Description: "Fase 1", Timeline: "Bulan 1", Deliverables: []string{"Setup backend"}},
	}

	rendered, err := renderer.Render(DedicatedSquadDocumentInput{
		Metadata:    meta,
		Calculation: calc,
		Profile:     profile,
		Rules:       rules,
		Timeline:    timeline,
	})
	require.NoError(t, err)

	state := &domain.PromptQuotationState{
		Scope: []domain.Fact[string]{
			{Value: "Setup database dan API", Source: domain.SourceMeeting, Confirmed: true},
		},
	}

	html := renderPromptPriceableHTML(rendered, state)

	// Verify exactly one article tag exists in document
	require.Equal(t, 1, strings.Count(html, "<article"), "expected exactly 1 <article> tag")
	require.Equal(t, 1, strings.Count(html, "</article>"), "expected exactly 1 </article> tag")
	require.Contains(t, html, `class="quotation-document prompt-first"`)
	require.Contains(t, html, `data-quotation-section="commercial"`)
	require.Contains(t, html, `data-quotation-section="payment"`)
	require.Contains(t, html, `data-quotation-section="scope"`)

	// Verify commercial section does NOT contain an inner <article>
	commIdx := strings.Index(html, `data-quotation-section="commercial"`)
	require.True(t, commIdx != -1)
	commEndIdx := strings.Index(html[commIdx:], "</section>")
	require.True(t, commEndIdx != -1)
	commercialSection := html[commIdx : commIdx+commEndIdx]
	require.NotContains(t, commercialSection, "<article", "commercial section must not contain nested <article>")

	// Verify sanitizer preserves the prompt-first class
	sanitized, err := NewHTMLSanitizer().Sanitize(html)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(sanitized, "<article"))
	require.Contains(t, sanitized, `class="quotation-document prompt-first"`)
}

func TestHTMLSanitizer_AllowsPromptFirstClass(t *testing.T) {
	input := `<article class="quotation-document prompt-first"><section data-quotation-section="scope" data-section-version="1"><p>Scope item</p></section></article>`
	sanitized, err := NewHTMLSanitizer().Sanitize(input)
	require.NoError(t, err)
	require.Contains(t, sanitized, `class="quotation-document prompt-first"`)
}

func TestRenderIncompletePromptDraft_ContainsOfficialShell(t *testing.T) {
	state := domain.PromptQuotationState{
		ProjectName:   &domain.Fact[string]{Value: "Sistem HR Terintegrasi", Source: domain.SourceMeeting},
		ClientCompany: &domain.Fact[string]{Value: "PT Solusi Digital", Source: domain.SourceMeeting},
		ClientName:    &domain.Fact[string]{Value: "Bpk. Rahmat", Source: domain.SourceMeeting},
		Scope: []domain.Fact[string]{
			{Value: "Modul Kepegawaian", Source: domain.SourceMeeting},
			{Value: "Modul Payroll", Source: domain.SourceMeeting},
		},
		Issues: []domain.DraftIssue{
			{Code: "team.rate_unresolved", Message: "Role PM belum terdaftar di katalog", Severity: domain.IssueBlocking},
		},
	}
	meta := QuotationMetadata{
		QuotationNumber:   "QUO-2026-9999",
		Subject:           "Penawaran Sistem HR Terintegrasi",
		ClientName:        "Bpk. Rahmat",
		ClientCompanyName: "PT Solusi Digital",
		IssueDate:         "20 Oktober 2026",
	}

	html := renderIncompletePromptDraft(state, meta)

	// Article root
	require.Equal(t, 1, strings.Count(html, "<article"))
	require.Equal(t, 1, strings.Count(html, "</article>"))
	require.Contains(t, html, `class="quotation-document prompt-first"`)

	// Header / Kop
	require.Contains(t, html, `data-block-type="metadata"`)
	require.Contains(t, html, `data-role="quotation-brand"`)
	require.Contains(t, html, `/logo/venturo-logo.png`)
	require.Contains(t, html, "PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD")
	require.Contains(t, html, "QUO-2026-9999")
	require.Contains(t, html, "PT Solusi Digital")

	// Opening
	require.Contains(t, html, `data-block-type="opening"`)
	require.Contains(t, html, DedicatedSquadOpeningParagraph)

	// Scope
	require.Contains(t, html, `data-quotation-section="scope"`)
	require.Contains(t, html, "Modul Kepegawaian")
	require.Contains(t, html, "Modul Payroll")

	// Commercial placeholder
	require.Contains(t, html, `data-quotation-section="commercial"`)
	require.Contains(t, html, `data-managed="true"`)
	require.Contains(t, html, "Rincian biaya sedang menunggu konfirmasi data tim dan durasi final di panel samping.")

	// Payment placeholder
	require.Contains(t, html, `data-quotation-section="payment"`)
	require.Contains(t, html, `data-managed="true"`)
	require.Contains(t, html, "Jadwal termin pembayaran akan dihitung otomatis setelah nilai investasi terkonfirmasi.")

	// Missing information / issues
	require.Contains(t, html, `data-quotation-section="information-missing"`)
	require.Contains(t, html, "team.rate_unresolved")

	// Legal
	require.Contains(t, html, `data-block-type="legal"`)
	require.Contains(t, html, DedicatedBindingClause)

	// Signatures
	require.Contains(t, html, `data-block-type="signature"`)
	require.Contains(t, html, "Yulianto Frandi")
	require.Contains(t, html, "Bpk. Rahmat")

	// Sanitizer verification
	sanitized, err := NewHTMLSanitizer().Sanitize(html)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(sanitized, "<article"))
	require.Contains(t, sanitized, `class="quotation-document prompt-first"`)
	require.Contains(t, sanitized, `data-block-type="metadata"`)
	require.Contains(t, sanitized, `data-block-type="legal"`)
	require.Contains(t, sanitized, `data-block-type="signature"`)
}
