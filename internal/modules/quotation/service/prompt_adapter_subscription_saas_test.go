package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestBuildPromptSubscriptionSaaSInputMapsFactsAndTrustedMoney(t *testing.T) {
	state := validPromptSubscriptionSaaSState()

	input, issues := BuildPromptSubscriptionSaaSInput(&state, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))

	assert.Empty(t, blockingSubscriptionIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, "Venturo Print", input.ProductName)
	assert.Equal(t, domain.SubscriptionBillingQuarterly, input.BillingCycle)
	assert.Equal(t, int32(12), input.InitialTermMonths)
	require.Len(t, input.PricePeriods, 1)
	assert.Equal(t, int64(1_250_000), input.PricePeriods[0].MonthlyRate)
	require.Len(t, input.OneTimeItems, 1)
	assert.Equal(t, int64(2_500_000), input.OneTimeItems[0].Amount)
	require.Len(t, input.FeatureGroups, 1)
	assert.Equal(t, []string{"Order management", "Invoice export"}, input.FeatureGroups[0].Features)
	require.NotNil(t, input.IncomeSharing)
	assert.Equal(t, int32(5_000), input.IncomeSharing.VenturoShareBPS)
	assert.Equal(t, int32(5_000), input.IncomeSharing.PartnerShareBPS)
}

func TestCalculatePromptSubscriptionSaaSUsesCalculatorAndBlocksUnknownPrice(t *testing.T) {
	t.Run("uses calculator and renderer inputs", func(t *testing.T) {
		state := validPromptSubscriptionSaaSState()
		_, calc, _, issues, err := CalculatePromptSubscriptionSaaS(&state, time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.Empty(t, blockingSubscriptionIssues(issues), "unexpected issues: %+v", issues)
		assert.Equal(t, int64(15_000_000), calc.RecurringTotal)
		assert.Equal(t, int64(2_500_000), calc.OneTimeTotal)
		assert.Equal(t, int64(17_500_000), calc.TotalContract)
	})

	t.Run("unknown monthly rate has no total", func(t *testing.T) {
		state := validPromptSubscriptionSaaSState()
		serviceInput := mustPromptSubscriptionSaaSServiceInput(t, state.ServiceInput)
		serviceInput.PricePeriods[0].MonthlyRate.Amount = nil
		state.ServiceInput = mustPromptSubscriptionSaaSJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptSubscriptionSaaS(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalContract)
		assert.True(t, hasSubscriptionIssue(issues, "subscription.monthly_rate.unavailable"), "%+v", issues)
	})

	t.Run("meeting price is never a trusted rate", func(t *testing.T) {
		state := validPromptSubscriptionSaaSState()
		serviceInput := mustPromptSubscriptionSaaSServiceInput(t, state.ServiceInput)
		serviceInput.PricePeriods[0].MonthlyRate.Source = domain.SourceMeeting
		state.ServiceInput = mustPromptSubscriptionSaaSJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptSubscriptionSaaS(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalContract)
		assert.True(t, hasSubscriptionIssue(issues, "subscription.monthly_rate.untrusted_source"), "%+v", issues)
	})
}

func TestPromptSubscriptionSaaSRejectsInvalidCoverageAndPaymentMismatch(t *testing.T) {
	for name, testCase := range map[string]struct {
		mutate    func(*PromptSubscriptionSaaSServiceInput)
		issueCode string
	}{
		"overlapping price periods": {mutate: func(in *PromptSubscriptionSaaSServiceInput) {
			in.PricePeriods = append(in.PricePeriods, PromptSubscriptionPricePeriod{
				Label: factString("Year two", domain.SourceUser), StartMonthOffset: 6, DurationMonths: 6,
				MonthlyRate: moneyInput(1_500_000, domain.SourceUser, ""),
			})
			in.InitialTermMonths = factInt(18, domain.SourceUser)
		}, issueCode: "subscription.input.invalid"},
		"gapped price periods": {mutate: func(in *PromptSubscriptionSaaSServiceInput) {
			in.PricePeriods = append(in.PricePeriods, PromptSubscriptionPricePeriod{
				Label: factString("Year two", domain.SourceUser), StartMonthOffset: 13, DurationMonths: 6,
				MonthlyRate: moneyInput(1_500_000, domain.SourceUser, ""),
			})
			in.InitialTermMonths = factInt(18, domain.SourceUser)
		}, issueCode: "subscription.input.invalid"},
		"payment mismatch": {mutate: func(in *PromptSubscriptionSaaSServiceInput) {
			in.PaymentSchedule[3].Nominal = moneyInput(3_749_999, domain.SourceUser, "")
		}, issueCode: "subscription.payment.total_mismatch"},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptSubscriptionSaaSState()
			serviceInput := mustPromptSubscriptionSaaSServiceInput(t, state.ServiceInput)
			testCase.mutate(&serviceInput)
			state.ServiceInput = mustPromptSubscriptionSaaSJSON(t, serviceInput)

			_, _, _, issues, err := CalculatePromptSubscriptionSaaS(&state, time.Now().UTC())
			require.NoError(t, err)
			assert.True(t, hasSubscriptionIssue(issues, testCase.issueCode), "%+v", issues)
		})
	}
}

func blockingSubscriptionIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	blocking := make([]domain.DraftIssue, 0)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func hasSubscriptionIssue(issues []domain.DraftIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func validPromptSubscriptionSaaSState() domain.PromptQuotationState {
	input := PromptSubscriptionSaaSServiceInput{
		Kind:              PromptSubscriptionSaaSServiceInputKind,
		ProductName:       factString("Venturo Print", domain.SourceMeeting),
		SectionLayout:     factString(string(domain.SubscriptionLayoutSubscriptionDevelopment), domain.SourceSuggestion),
		BillingCycle:      factString(string(domain.SubscriptionBillingQuarterly), domain.SourceMeeting),
		TaxTreatment:      factString(string(domain.SubscriptionTaxNotStated), domain.SourceSuggestion),
		InitialTermMonths: factInt(12, domain.SourceMeeting),
		AutoRenewal:       factBool(false, domain.SourceSuggestion),
		PricePeriods: []PromptSubscriptionPricePeriod{{
			Label: factString("Subscription 12 bulan", domain.SourceMeeting), StartMonthOffset: 0, DurationMonths: 12,
			MonthlyRate: moneyInput(1_250_000, domain.SourceCatalog, "SAAS-PRINT-2026"),
		}},
		OneTimeItems: []PromptSubscriptionOneTimeItem{{Name: factString("Setup & onboarding", domain.SourceMeeting), Amount: moneyInput(2_500_000, domain.SourceUser, "")}},
		PaymentSchedule: []PromptSubscriptionPaymentRow{
			{WorkPeriod: factString("Q1", domain.SourceMeeting), StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: factString("Saat kontrak", domain.SourceMeeting), DueDateLabel: factString("H+14", domain.SourceMeeting), Nominal: moneyInput(6_250_000, domain.SourceUser, "")},
			{WorkPeriod: factString("Q2", domain.SourceMeeting), StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: factString("Bulan 4", domain.SourceMeeting), DueDateLabel: factString("H+14", domain.SourceMeeting), Nominal: moneyInput(3_750_000, domain.SourceUser, "")},
			{WorkPeriod: factString("Q3", domain.SourceMeeting), StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: factString("Bulan 7", domain.SourceMeeting), DueDateLabel: factString("H+14", domain.SourceMeeting), Nominal: moneyInput(3_750_000, domain.SourceUser, "")},
			{WorkPeriod: factString("Q4", domain.SourceMeeting), StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: factString("Bulan 10", domain.SourceMeeting), DueDateLabel: factString("H+14", domain.SourceMeeting), Nominal: moneyInput(3_750_000, domain.SourceUser, "")},
		},
		FeatureGroups:    []PromptSubscriptionFeatureGroup{{Name: factString("Core", domain.SourceMeeting), Features: []domain.Fact[string]{factString("Order management", domain.SourceMeeting), factString("Invoice export", domain.SourceMeeting)}}},
		RightsClauseCode: factString("", domain.SourceSuggestion),
		IsPartnership:    factBool(true, domain.SourceMeeting),
		IncomeSharing:    &PromptSubscriptionIncomeSharing{VenturoShareBPS: factInt(5_000, domain.SourceMeeting), PartnerShareBPS: factInt(5_000, domain.SourceMeeting), EligibleRevenueDescription: factString("penjualan dari mitra", domain.SourceMeeting), SettlementBusinessDays: factInt(30, domain.SourceMeeting), ProhibitUnilateralDiscount: factBool(true, domain.SourceMeeting)},
	}
	raw, err := EncodePromptSubscriptionSaaSServiceInput(input)
	if err != nil {
		panic(err)
	}
	return domain.PromptQuotationState{SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "Subscription Venturo Print 12 bulan dengan setup dan bagi hasil mitra.", ClientName: ptrFact("Nadia", domain.SourceUser), CommercialScheme: ptrFact(domain.CommercialSchemeSubscriptionSaaS, domain.SourceUser), ProjectName: ptrFact("Venturo Print", domain.SourceUser), ServiceInput: raw}
}

func factString(value string, source domain.ValueSource) domain.Fact[string] {
	return domain.Fact[string]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}
func factInt(value int, source domain.ValueSource) domain.Fact[int] {
	return domain.Fact[int]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}
func factBool(value bool, source domain.ValueSource) domain.Fact[bool] {
	return domain.Fact[bool]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}
func ptrFact(value string, source domain.ValueSource) *domain.Fact[string] {
	fact := factString(value, source)
	return &fact
}
func moneyInput(value int64, source domain.ValueSource, catalogRef string) domain.MoneyInput {
	amount := value
	money := domain.MoneyInput{Amount: &amount, Source: source}
	if catalogRef != "" {
		money.CatalogRef = &catalogRef
	}
	return money
}

func mustPromptSubscriptionSaaSServiceInput(t *testing.T, raw []byte) PromptSubscriptionSaaSServiceInput {
	t.Helper()
	input, err := DecodePromptSubscriptionSaaSServiceInput(raw)
	require.NoError(t, err)
	return input
}
func mustPromptSubscriptionSaaSJSON(t *testing.T, input PromptSubscriptionSaaSServiceInput) []byte {
	t.Helper()
	raw, err := EncodePromptSubscriptionSaaSServiceInput(input)
	require.NoError(t, err)
	return raw
}
func mustReadPromptFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "prompt_first", name))
	require.NoError(t, err)
	return raw
}

func TestPromptSubscriptionSaaSServiceInputFixturesAreCompleteAndIncomplete(t *testing.T) {
	for _, name := range []string{"subscription-saas-normal.json", "subscription-saas-incomplete.json"} {
		raw := mustReadPromptFixture(t, name)
		input, err := DecodePromptSubscriptionSaaSServiceInput(raw)
		require.NoError(t, err)
		if strings.Contains(name, "incomplete") {
			assert.Nil(t, input.PricePeriods[0].MonthlyRate.Amount)
		} else {
			assert.NotNil(t, input.PricePeriods[0].MonthlyRate.Amount)
		}
	}
}
