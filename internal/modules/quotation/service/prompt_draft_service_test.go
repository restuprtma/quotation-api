package service

import (
	"context"
	"encoding/json"
	"errors"
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
func (r *promptRepoStub) CreateDraftTransaction(_ context.Context, _ repository.CreateDraftParams) (*repository.CreateDraftResult, error) {
	r.createDraftCalls++
	if r.draft == nil {
		r.draft = &repository.CreateDraftResult{QuotationID: uuid.New(), RevisionID: uuid.New()}
	}
	return r.draft, nil
}
func (r *promptRepoStub) CompletePromptDraft(_ context.Context, params repository.CompletePromptDraftParams) (*repository.PromptDraftSaveResult, error) {
	r.completeDraftCalls++
	r.record = &repository.PromptDraftRecord{QuotationID: params.QuotationID, RevisionID: params.RevisionID, QuotationNumber: r.nextNumber, ContentHTML: params.ContentHTML, ContentVersion: 1, StateVersion: 1, PromptState: json.RawMessage(params.PromptState), Calculation: params.CalculationSnapshot}
	return &repository.PromptDraftSaveResult{RevisionID: params.RevisionID, ContentVersion: 1, StateVersion: 1}, nil
}
func (r *promptRepoStub) LoadPromptDraft(context.Context, uuid.UUID, uuid.UUID) (*repository.PromptDraftRecord, error) {
	return r.record, nil
}

type promptExtractorStub struct{ state domain.PromptQuotationState }

func (e promptExtractorStub) Extract(context.Context, uuid.UUID, dto.MeetingExtractionRequest, CatalogContext) (domain.PromptQuotationState, error) {
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

func TestPromptDraftService_RejectsNoBusinessSignalBeforePersistence(t *testing.T) {
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
	assert.Zero(t, repo.nextNumberCalls)
	assert.Zero(t, repo.createDraftCalls)
	assert.Zero(t, repo.completeDraftCalls)
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
			err: &MeetingExtractionError{Reason: "unsupported_evidence"},
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
