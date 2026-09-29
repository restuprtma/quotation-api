package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type promptRevisionRepoStub struct {
	record *repository.PromptDraftRecord
	params repository.UpdatePromptDraftCASParams
	err    error
}

func (r *promptRevisionRepoStub) LoadPromptDraft(context.Context, uuid.UUID, uuid.UUID) (*repository.PromptDraftRecord, error) {
	return r.record, nil
}
func (r *promptRevisionRepoStub) UpdatePromptDraftCAS(_ context.Context, params repository.UpdatePromptDraftCASParams) (*repository.PromptDraftSaveResult, error) {
	r.params = params
	if r.err != nil {
		return nil, r.err
	}
	return &repository.PromptDraftSaveResult{RevisionID: params.RevisionID, ContentVersion: params.ExpectedContentVersion + 1, StateVersion: params.ExpectedStateVersion + 1}, nil
}

func TestPromptRevisionService_AppliesAdditionalDiscountAndRecalculatesPayments(t *testing.T) {
	repo := promptRevisionTestRepo(t)
	svc := NewPromptRevisionService(repo, NewHTMLSanitizer(), FixedDiscountPolicy{MaxBPS: 1_000, Floor: 1})

	result, err := svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, dto.PromptPatchRequest{
		RevisionID: repo.record.RevisionID, ContentVersion: repo.record.ContentVersion, StateVersion: repo.record.StateVersion,
		Operations: []dto.PromptPatchOperation{{Type: dto.PromptPatchSetDiscountBPS, DiscountBPS: int32Ptr(500)}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(30_000_000), result.Calculation.TotalBeforeDiscount)
	require.Equal(t, int64(1_500_000), result.Calculation.AdditionalDiscount)
	require.Equal(t, int64(28_500_000), result.Calculation.TotalInvestment)
	require.Equal(t, int64(0), result.Calculation.Discount, "historical preparation discount is not repurposed")
	require.Len(t, result.Calculation.Payments, 2)
	require.Equal(t, int64(14_250_000), result.Calculation.Payments[0].Nominal)
	require.Equal(t, int64(14_250_000), result.Calculation.Payments[1].Nominal)
	require.Contains(t, repo.params.ContentHTML, `BA edited scope`)
	require.Contains(t, repo.params.ContentHTML, `BA edited timeline`)
	require.Contains(t, repo.params.ContentHTML, `legal copy`)
	require.Contains(t, repo.params.ContentHTML, `additional_discount`)
	require.Len(t, result.CatalogRoles, 1)
	require.Equal(t, "frontend_programmer", result.CatalogRoles[0].Role)
}

func TestPromptRevisionService_RejectsDiscountLimitAndFloorAndStaleVersions(t *testing.T) {
	repo := promptRevisionTestRepo(t)
	svc := NewPromptRevisionService(repo, NewHTMLSanitizer(), FixedDiscountPolicy{MaxBPS: 500, Floor: 28_600_000})
	req := dto.PromptPatchRequest{RevisionID: repo.record.RevisionID, ContentVersion: repo.record.ContentVersion, StateVersion: repo.record.StateVersion, Operations: []dto.PromptPatchOperation{{Type: dto.PromptPatchSetDiscountBPS, DiscountBPS: int32Ptr(600)}}}
	_, err := svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, req)
	require.Equal(t, "discount_above_max", safePromptRevisionErrorCode(err))

	req.Operations[0].DiscountBPS = int32Ptr(500)
	_, err = svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, req)
	require.Equal(t, "discount_below_floor", safePromptRevisionErrorCode(err))

	req.Operations = []dto.PromptPatchOperation{{Type: dto.PromptPatchSetPaymentPattern, PaymentPattern: stringPtr("40:40:20")}}
	req.ContentVersion--
	_, err = svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, req)
	require.Equal(t, "revision_conflict", safePromptRevisionErrorCode(err))
	req.ContentVersion++

	repo.err = repository.ErrRevisionConflict
	_, err = svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, req)
	require.Equal(t, "revision_conflict", safePromptRevisionErrorCode(err))
}

func TestPromptRevisionService_RejectsArbitraryCommercialFieldsAndUnverifiedPolicy(t *testing.T) {
	repo := promptRevisionTestRepo(t)
	svc := NewPromptRevisionService(repo, NewHTMLSanitizer(), nil)
	req := dto.PromptPatchRequest{RevisionID: repo.record.RevisionID, ContentVersion: repo.record.ContentVersion, StateVersion: repo.record.StateVersion, Operations: []dto.PromptPatchOperation{{Type: "set_scope"}}}
	_, err := svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, req)
	require.Equal(t, "invalid_patch", safePromptRevisionErrorCode(err))

	req.Operations = []dto.PromptPatchOperation{{Type: dto.PromptPatchSetDiscountBPS, DiscountBPS: int32Ptr(500)}}
	_, err = svc.ApplyCommercialPatch(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, req)
	require.Equal(t, "discount_policy_unverified", safePromptRevisionErrorCode(err))
}

func TestPromptRevisionService_PaymentRemainderBelongsToLastRow(t *testing.T) {
	parts := SplitPayment(101, PaymentPattern404020)
	require.Len(t, parts, 3)
	require.Equal(t, int64(40), parts[0].Amount)
	require.Equal(t, int64(40), parts[1].Amount)
	require.Equal(t, int64(21), parts[2].Amount)
}

func TestRecalculatePromptPayments_MonthlyUsesExistingDuration(t *testing.T) {
	calculation := domain.DedicatedSquadCalculation{TotalInvestment: 101, Payments: make([]domain.PaymentRow, 4)}
	pattern := confirmedPromptFact("monthly")
	require.NoError(t, recalculatePromptPayments(&calculation, pattern))
	require.Equal(t, int64(25), calculation.Payments[0].Nominal)
	require.Equal(t, int64(25), calculation.Payments[1].Nominal)
	require.Equal(t, int64(25), calculation.Payments[2].Nominal)
	require.Equal(t, int64(26), calculation.Payments[3].Nominal)
}

func promptRevisionTestRepo(t *testing.T) *promptRevisionRepoStub {
	t.Helper()
	companyID, quotationID, revisionID, itemID, cardID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	duration := 3
	state := domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "Dedicated frontend team for three months.",
		ClientName: confirmedPromptFact("Nadia"), ClientCompany: confirmedPromptFact("PT Arunika"), ProjectName: confirmedPromptFact("Arunika Inventory"), CommercialScheme: confirmedPromptFact(domain.CommercialSchemeDedicatedSquad),
		Scope: []domain.Fact[string]{{Value: "BA edited scope", Source: domain.SourceUser, Confirmed: true}}, DurationMonths: &domain.Fact[int]{Value: duration, Source: domain.SourceUser, Confirmed: true},
		Team:           []domain.PromptTeamLine{{RequestedRole: "frontend programmer", ResolvedRateItemID: stringPtr(itemID.String()), Seniority: "intermediate", HeadcountQuarters: 4, PeriodQuarters: 12, Source: domain.SourceUser, Confirmed: true}},
		PaymentPattern: confirmedPromptFact("50:50"), Readiness: domain.ReadinessPriceable,
	}
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)
	rateJSON, err := json.Marshal(struct {
		Card  domain.RateCard       `json:"card"`
		Items []domain.RateCardItem `json:"items"`
	}{Card: domain.RateCard{ID: cardID, Code: domain.DefaultPromptRateCardCode}, Items: []domain.RateCardItem{{ID: itemID, RateCardID: cardID, Role: "frontend_programmer", Seniority: "intermediate", TermType: "long_term", AmountPerMonth: 10_000_000}}})
	require.NoError(t, err)
	calcJSON, err := json.Marshal(domain.DedicatedSquadCalculation{Model: domain.ModelSOWSmall, Total: 30_000_000, TotalBeforeDiscount: 30_000_000, TotalInvestment: 30_000_000, Payments: []domain.PaymentRow{{InvoiceDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Nominal: 15_000_000}, {InvoiceDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Nominal: 15_000_000}}})
	require.NoError(t, err)
	return &promptRevisionRepoStub{record: &repository.PromptDraftRecord{CompanyID: companyID, QuotationID: quotationID, RevisionID: revisionID, ContentVersion: 7, StateVersion: 4, PromptState: stateJSON, Calculation: calcJSON, RateCardSnapshot: rateJSON, ContentHTML: `<article class="quotation-document"><section data-quotation-section="scope" data-section-version="1"><p>BA edited scope</p></section><section data-quotation-section="timeline" data-section-version="1"><p>BA edited timeline</p></section><section data-quotation-section="commercial" data-section-version="1" data-managed="true"><p>old commercial</p></section><section data-quotation-section="payment" data-section-version="1" data-managed="true"><p>old payment</p></section><section data-quotation-section="legal"><p>legal copy</p></section></article>`}}
}

func confirmedPromptFact(value string) *domain.Fact[string] {
	return &domain.Fact[string]{Value: value, Source: domain.SourceUser, Confirmed: true}
}
func int32Ptr(value int32) *int32 { return &value }
