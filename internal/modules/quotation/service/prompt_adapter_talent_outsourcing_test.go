package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestBuildPromptTalentOutsourcingManpower(t *testing.T) {
	state := validPromptTalentManpowerState()
	input, issues := BuildPromptTalentOutsourcingInput(&state, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))

	assert.Empty(t, blockingTalentIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.TalentModeManpowerSupply, input.EngagementMode)
	require.Len(t, input.CostGroups, 2)
	assert.Equal(t, domain.TalentCategoryPersonnel, input.CostGroups[0].Category)
	assert.Equal(t, domain.TalentPeriodWorkday, input.CostGroups[0].Items[0].PeriodUnit)
	assert.Equal(t, domain.TalentCategoryNonPersonnel, input.CostGroups[1].Category)
	assert.Equal(t, domain.TalentPeriodMonth, input.CostGroups[1].Items[0].PeriodUnit)
}

func TestBuildPromptTalentOutsourcingManagedSquad(t *testing.T) {
	state := validPromptTalentManagedState()
	input, issues := BuildPromptTalentOutsourcingInput(&state, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))

	assert.Empty(t, blockingTalentIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.TalentModeManagedSquad, input.EngagementMode)
	require.Len(t, input.PreparationItems, 1)
	assert.Equal(t, int64(10_000_000), input.PreparationItems[0].QuotedLineTotal)
	require.Len(t, input.SquadItems, 1)
	assert.Equal(t, int64(20_000_000), input.SquadItems[0].MonthlyRate)
}

func TestCalculatePromptTalentOutsourcingUsesCalculator(t *testing.T) {
	t.Run("workday manpower supply", func(t *testing.T) {
		state := validPromptTalentManpowerState()
		_, calc, _, issues, err := CalculatePromptTalentOutsourcing(&state, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.Empty(t, blockingTalentIssues(issues), "unexpected issues: %+v", issues)

		// personnel: unit_rate 15M, qty 2, count 10 (workdays).
		// Existing calculator divides by 20. Total: (15M * 2 * 10) / 20 = 15M
		assert.Equal(t, int64(15_000_000), calc.PersonnelSubtotal)
		// non-personnel: unit_rate 2M, qty 1, count 1 (months).
		assert.Equal(t, int64(2_000_000), calc.NonPersonnelSubtotal)
	})

	t.Run("monthly managed squad", func(t *testing.T) {
		state := validPromptTalentManagedState()
		_, calc, _, issues, err := CalculatePromptTalentOutsourcing(&state, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.Empty(t, blockingTalentIssues(issues), "unexpected issues: %+v", issues)

		assert.Equal(t, int64(10_000_000), calc.PreparationSubtotal)
		// 1 headcount, 6 months, 20M monthly = 120M
		assert.Equal(t, int64(120_000_000), calc.SquadSubtotal)
	})
}

func TestCalculatePromptTalentOutsourcingBlocksMissingAmounts(t *testing.T) {
	t.Run("missing nonpersonnel price", func(t *testing.T) {
		state := validPromptTalentManpowerState()
		serviceInput := mustPromptTalentServiceInput(t, state.ServiceInput)
		serviceInput.CostGroups[1].Items[0].UnitRate.Amount = nil
		state.ServiceInput = mustPromptTalentJSON(t, serviceInput)

		_, _, _, issues, err := CalculatePromptTalentOutsourcing(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, hasTalentIssue(issues, "talent_outsourcing.cost_item.unit_rate.unavailable"), "%+v", issues)
	})

	t.Run("missing preparation price", func(t *testing.T) {
		state := validPromptTalentManagedState()
		serviceInput := mustPromptTalentServiceInput(t, state.ServiceInput)
		serviceInput.PreparationItems[0].QuotedLineTotal.Amount = nil
		state.ServiceInput = mustPromptTalentJSON(t, serviceInput)

		_, _, _, issues, err := CalculatePromptTalentOutsourcing(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, hasTalentIssue(issues, "talent_outsourcing.preparation_item.quoted_line_total.unavailable"), "%+v", issues)
	})

	t.Run("unknown tax policy", func(t *testing.T) {
		state := validPromptTalentManpowerState()
		serviceInput := mustPromptTalentServiceInput(t, state.ServiceInput)
		serviceInput.TaxTreatment = factString("unknown_tax", domain.SourceSuggestion)
		state.ServiceInput = mustPromptTalentJSON(t, serviceInput)

		_, _, _, issues, err := CalculatePromptTalentOutsourcing(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, hasTalentIssue(issues, "talent_outsourcing.tax_policy.unknown"), "%+v", issues)
	})
}

func validPromptTalentManpowerState() domain.PromptQuotationState {
	input := PromptTalentOutsourcingServiceInput{
		Kind:           PromptTalentOutsourcingServiceInputKind,
		EngagementMode: factString(string(domain.TalentModeManpowerSupply), domain.SourceMeeting),
		DocumentNumber: factString("DOC-001", domain.SourceSuggestion),
		ServiceName:    factString("Manpower Service", domain.SourceMeeting),
		TaxTreatment:   factString(string(domain.TalentTaxBPJSDPP12), domain.SourceSuggestion),
		CostGroups: []PromptTalentCostGroup{
			{
				Category: domain.TalentCategoryPersonnel,
				Title:    factString("Tim Inti", domain.SourceSuggestion),
				Items: []PromptTalentCostItem{
					{
						Name:         factString("Senior Engineer", domain.SourceMeeting),
						Quantity:     factInt(2, domain.SourceMeeting),
						QuantityUnit: factString("Orang", domain.SourceMeeting),
						PeriodCount:  factInt(10, domain.SourceMeeting),
						PeriodUnit:   domain.TalentPeriodWorkday,
						UnitRate:     moneyInput(15_000_000, domain.SourceCatalog, "VENTURO-STD-2026"),
					},
				},
			},
			{
				Category: domain.TalentCategoryNonPersonnel,
				Title:    factString("Lain-lain", domain.SourceSuggestion),
				Items: []PromptTalentCostItem{
					{
						Name:         factString("Transport", domain.SourceMeeting),
						Quantity:     factInt(1, domain.SourceMeeting),
						QuantityUnit: factString("LS", domain.SourceMeeting),
						PeriodCount:  factInt(1, domain.SourceMeeting),
						PeriodUnit:   domain.TalentPeriodMonth,
						UnitRate:     moneyInput(2_000_000, domain.SourceUser, ""),
					},
				},
			},
		},
		ScopeBlocks: []PromptTalentScopeBlock{
			{Title: factString("Lingkup", domain.SourceMeeting), Description: factString("Deskripsi pekerjaan", domain.SourceMeeting), Timeline: factString("Q1 2026", domain.SourceMeeting)},
		},
		PaymentSchedule: []PromptTalentPaymentRow{
			{WorkPeriod: factString("Bulan 1", domain.SourceMeeting), Terms: factString("100%", domain.SourceMeeting), InvoiceDateLabel: factString("Akhir", domain.SourceMeeting), DueDateLabel: factString("Net 14", domain.SourceMeeting), Nominal: moneyInput(18_870_000, domain.SourceUser, "")}, // Total + Tax
		},
		PaymentDueDays:  factInt(14, domain.SourceMeeting),
		LimitationCodes: []domain.TalentLimitationCode{domain.TalentLimitOutOfPocket},
	}
	raw, err := EncodePromptTalentOutsourcingServiceInput(input)
	if err != nil {
		panic(err)
	}
	return domain.PromptQuotationState{SchemaVersion: domain.SchemaPromptQuotationV1, ClientName: ptrFact("Nadia", domain.SourceUser), CommercialScheme: ptrFact(domain.CommercialSchemeTalentOutsourcing, domain.SourceUser), ServiceInput: raw}
}

func validPromptTalentManagedState() domain.PromptQuotationState {
	input := PromptTalentOutsourcingServiceInput{
		Kind:           PromptTalentOutsourcingServiceInputKind,
		EngagementMode: factString(string(domain.TalentModeManagedSquad), domain.SourceMeeting),
		DocumentNumber: factString("DOC-002", domain.SourceSuggestion),
		ServiceName:    factString("Managed Squad Service", domain.SourceMeeting),
		TaxTreatment:   factString(string(domain.TalentTaxExclusive), domain.SourceSuggestion),
		PreparationItems: []PromptTalentPreparationItem{
			{
				Name:            factString("Onboarding", domain.SourceMeeting),
				Headcount:       factInt(1, domain.SourceMeeting),
				QuotedLineTotal: moneyInput(10_000_000, domain.SourceUser, ""),
			},
		},
		SquadItems: []PromptTalentSquadItem{
			{
				Role:         factString("Lead", domain.SourceMeeting),
				MonthlyRate:  moneyInput(20_000_000, domain.SourceCatalog, "VENTURO-STD-2026"),
				Headcount:    factInt(1, domain.SourceMeeting),
				PeriodMonths: factInt(6, domain.SourceMeeting),
			},
		},
		ScopeBlocks: []PromptTalentScopeBlock{
			{Title: factString("Lingkup", domain.SourceMeeting), Description: factString("Deskripsi pekerjaan", domain.SourceMeeting), Timeline: factString("H2 2026", domain.SourceMeeting)},
		},
		PaymentSchedule: []PromptTalentPaymentRow{
			{WorkPeriod: factString("Bulan 1-6", domain.SourceMeeting), Terms: factString("100%", domain.SourceMeeting), InvoiceDateLabel: factString("Bulanan", domain.SourceMeeting), DueDateLabel: factString("Net 14", domain.SourceMeeting), Nominal: moneyInput(130_000_000, domain.SourceUser, "")},
		},
		PaymentDueDays:  factInt(14, domain.SourceMeeting),
		LimitationCodes: []domain.TalentLimitationCode{domain.TalentLimitScopeChange},
	}
	raw, err := EncodePromptTalentOutsourcingServiceInput(input)
	if err != nil {
		panic(err)
	}
	return domain.PromptQuotationState{SchemaVersion: domain.SchemaPromptQuotationV1, ClientName: ptrFact("Nadia", domain.SourceUser), CommercialScheme: ptrFact(domain.CommercialSchemeTalentOutsourcing, domain.SourceUser), ServiceInput: raw}
}

func blockingTalentIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	blocking := make([]domain.DraftIssue, 0)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func hasTalentIssue(issues []domain.DraftIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func mustPromptTalentServiceInput(t *testing.T, raw []byte) PromptTalentOutsourcingServiceInput {
	t.Helper()
	input, err := DecodePromptTalentOutsourcingServiceInput(raw)
	require.NoError(t, err)
	return input
}
func mustPromptTalentJSON(t *testing.T, input PromptTalentOutsourcingServiceInput) []byte {
	t.Helper()
	raw, err := EncodePromptTalentOutsourcingServiceInput(input)
	require.NoError(t, err)
	return raw
}
