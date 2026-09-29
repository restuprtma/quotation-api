package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestEvaluatePromptReadiness(t *testing.T) {
	readyState := func() *domain.PromptQuotationState {
		return &domain.PromptQuotationState{
			SchemaVersion: domain.SchemaPromptQuotationV1,
			MeetingNotes:  "Nadia meminta aplikasi inventory.",
			ClientName:    &domain.Fact[string]{Value: "Nadia", Source: domain.SourceUser, Confirmed: true},
			ClientCompany: &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
			PICName:       &domain.Fact[string]{Value: "Nadia", Source: domain.SourceUser, Confirmed: true},
			CommercialScheme: &domain.Fact[string]{
				Value: "dedicated_squad", Source: domain.SourceUser, Confirmed: true,
			},
			ProjectName:    &domain.Fact[string]{Value: "Arunika Inventory", Source: domain.SourceUser, Confirmed: true},
			Scope:          []domain.Fact[string]{{Value: "Inventory management", Source: domain.SourceUser, Confirmed: true}},
			DurationMonths: &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true},
			Team: []domain.PromptTeamLine{{
				RequestedRole: "programmer", ResolvedRateItemID: stringPtr("rate-programmer"), Seniority: "intermediate",
				HeadcountQuarters: 4, PeriodQuarters: 12, Source: domain.SourceUser, Confirmed: true,
			}},
			PaymentPattern: &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
		}
	}
	calculation := func() *domain.DedicatedSquadCalculation {
		return &domain.DedicatedSquadCalculation{TotalInvestment: 10_000_000}
	}
	payment := func() PromptPaymentReadiness {
		return PromptPaymentReadiness{Rows: []domain.PaymentRow{{Nominal: 5_000_000}, {Nominal: 5_000_000}}, DiscountPolicyVerified: true}
	}

	tests := []struct {
		name      string
		mutate    func(*domain.PromptQuotationState, *domain.DedicatedSquadCalculation, *PromptPaymentReadiness)
		readiness string
		issueCode string
	}{
		{
			name:      "ready for final when all commercial inputs reconcile",
			readiness: domain.ReadinessReadyForFinal,
		},
		{
			name: "unresolved role is incomplete",
			mutate: func(state *domain.PromptQuotationState, _ *domain.DedicatedSquadCalculation, _ *PromptPaymentReadiness) {
				state.Team[0].ResolvedRateItemID = nil
			},
			readiness: domain.ReadinessIncomplete,
			issueCode: "team.rate_unresolved",
		},
		{
			name: "unknown price is incomplete",
			mutate: func(_ *domain.PromptQuotationState, calculation *domain.DedicatedSquadCalculation, _ *PromptPaymentReadiness) {
				*calculation = domain.DedicatedSquadCalculation{}
			},
			readiness: domain.ReadinessIncomplete,
			issueCode: "price.unavailable",
		},
		{
			name: "unconfirmed suggestion remains priceable",
			mutate: func(state *domain.PromptQuotationState, _ *domain.DedicatedSquadCalculation, _ *PromptPaymentReadiness) {
				state.ProjectName = &domain.Fact[string]{Value: "Arunika Inventory", Source: domain.SourceSuggestion, Confirmed: false}
			},
			readiness: domain.ReadinessPriceable,
			issueCode: "project_name.unconfirmed",
		},
		{
			name: "payment mismatch is incomplete",
			mutate: func(_ *domain.PromptQuotationState, _ *domain.DedicatedSquadCalculation, payment *PromptPaymentReadiness) {
				payment.Rows[1].Nominal = 4_999_999
			},
			readiness: domain.ReadinessIncomplete,
			issueCode: "payment.total_mismatch",
		},
		{
			name: "discount without verified policy is incomplete",
			mutate: func(state *domain.PromptQuotationState, _ *domain.DedicatedSquadCalculation, payment *PromptPaymentReadiness) {
				state.AdditionalDiscountBPS = int32Ptr(500)
				payment.DiscountPolicyVerified = false
			},
			readiness: domain.ReadinessIncomplete,
			issueCode: "discount.policy_unverified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := readyState()
			calc := calculation()
			payments := payment()
			if tt.mutate != nil {
				tt.mutate(state, calc, &payments)
			}

			result := EvaluatePromptReadiness(state, calc, payments)

			assert.Equal(t, tt.readiness, result.Readiness)
			if tt.issueCode != "" {
				assert.Contains(t, issueCodes(result.Issues), tt.issueCode)
			}
		})
	}
}

func issueCodes(issues []domain.DraftIssue) []string {
	codes := make([]string, 0, len(issues))
	for _, issue := range issues {
		codes = append(codes, issue.Code)
	}
	return codes
}
