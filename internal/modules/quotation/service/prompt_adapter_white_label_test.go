package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestCalculatePromptWhiteLabelLicenseOnlyUsesExistingCalculator(t *testing.T) {
	state := validPromptWhiteLabelState()

	input, calculation, profile, issues, err := CalculatePromptWhiteLabel(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingWhiteLabelIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, int64(35_000_000), calculation.LicenseSubtotal)
	assert.Zero(t, calculation.MaintenanceHostingSubtotal)
	assert.Equal(t, int64(35_000_000), calculation.TotalInvestment)
	assert.Equal(t, int64(35_000_000), calculation.LicensePaymentTotal)
	assert.False(t, profile.ShowMaintenanceHosting)
	assert.Len(t, input.LicenseItems, 1)
}

func TestPromptWhiteLabelMapsRecurringComponentsNarrativeAndSeparatePayments(t *testing.T) {
	state := validPromptWhiteLabelState()
	serviceInput := mustPromptWhiteLabelServiceInput(t, state.ServiceInput)
	serviceInput.MaintenanceItems = []PromptWhiteLabelMaintenanceItem{{
		Name:         whiteLabelFactString("Maintenance programmer", domain.SourceMeeting),
		MonthlyRate:  whiteLabelMoney(16_170_000, domain.SourceCatalog, "WL-MAINT-2026"),
		FTEQuarters:  whiteLabelFactInt(2, domain.SourceMeeting),
		PeriodMonths: whiteLabelFactInt(3, domain.SourceMeeting),
	}}
	serviceInput.HostingItems = []PromptWhiteLabelHostingItem{{
		Name:         whiteLabelFactString("Managed hosting", domain.SourceMeeting),
		MonthlyRate:  whiteLabelMoney(350_000, domain.SourceUser, ""),
		Quantity:     whiteLabelFactInt(1, domain.SourceMeeting),
		PeriodMonths: whiteLabelFactInt(3, domain.SourceMeeting),
	}}
	serviceInput.ModuleGroups = []PromptWhiteLabelModuleGroup{{
		Name:     whiteLabelFactString("Inventory", domain.SourceMeeting),
		Features: []domain.Fact[string]{whiteLabelFactString("Stock movement", domain.SourceMeeting)},
	}}
	serviceInput.RightsStatements = []domain.Fact[string]{whiteLabelFactString("Client may rebrand the licensed product.", domain.SourceMeeting)}
	serviceInput.PaymentSchedule = append(serviceInput.PaymentSchedule, PromptWhiteLabelPaymentRow{
		WorkPeriod:       whiteLabelFactString("Recurring services", domain.SourceMeeting),
		Terms:            whiteLabelFactString("Monthly", domain.SourceMeeting),
		InvoiceDateLabel: whiteLabelFactString("Month 1", domain.SourceMeeting),
		DueDateLabel:     whiteLabelFactString("H+14", domain.SourceMeeting),
		Component:        domain.WhiteLabelPaymentMaintenanceHosting,
		Nominal:          whiteLabelMoney(25_305_000, domain.SourceUser, ""),
	})
	state.ServiceInput = mustPromptWhiteLabelJSON(t, serviceInput)

	input, calculation, _, issues, err := CalculatePromptWhiteLabel(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingWhiteLabelIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, int32(2), input.MaintenanceItems[0].FTEQuarters, "adapter must preserve quarter-FTE units")
	assert.Equal(t, int64(24_255_000), calculation.MaintenanceSubtotal)
	assert.Equal(t, int64(1_050_000), calculation.HostingSubtotal)
	assert.Equal(t, int64(25_305_000), calculation.MaintenanceHostingSubtotal)
	assert.Equal(t, calculation.LicenseSubtotal, calculation.LicensePaymentTotal)
	assert.Equal(t, calculation.MaintenanceHostingSubtotal, calculation.MaintenanceHostingPaymentTotal)

	adapter := NewPromptWhiteLabelAdapter(nil)
	html, err := adapter.Render(QuotationMetadata{QuotationNumber: "Q-WL-001", Subject: "White label", ClientName: "Nadia"}, PromptWhiteLabelSnapshot{Input: input, Calculation: calculation}, "<p>Legal</p>")
	require.NoError(t, err)
	assert.Contains(t, html, "Inventory")
	assert.Contains(t, html, "Stock movement")
	assert.Contains(t, html, "Client may rebrand the licensed product.")
}

func TestPromptWhiteLabelBlocksMissingOrUntrustedPricesWithoutInventingAmounts(t *testing.T) {
	for name, mutate := range map[string]func(*PromptWhiteLabelServiceInput){
		"missing license unit rate": func(input *PromptWhiteLabelServiceInput) {
			input.LicenseItems[0].UnitRate.Amount = nil
			input.LicenseItems[0].UnitRate.Source = domain.SourceMeeting
		},
		"meeting rate is untrusted": func(input *PromptWhiteLabelServiceInput) {
			input.LicenseItems[0].UnitRate.Source = domain.SourceMeeting
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptWhiteLabelState()
			serviceInput := mustPromptWhiteLabelServiceInput(t, state.ServiceInput)
			mutate(&serviceInput)
			state.ServiceInput = mustPromptWhiteLabelJSON(t, serviceInput)

			_, calculation, _, issues, err := CalculatePromptWhiteLabel(&state, time.Now().UTC())

			require.NoError(t, err)
			assert.Zero(t, calculation.TotalInvestment)
			if strings.Contains(name, "missing") {
				assert.True(t, hasWhiteLabelIssue(issues, "white_label.license_unit_rate.unavailable"), "%+v", issues)
			} else {
				assert.True(t, hasWhiteLabelIssue(issues, "white_label.license_unit_rate.untrusted_source"), "%+v", issues)
			}
		})
	}
}

func TestPromptWhiteLabelReportsWholeRupiahAndComponentPaymentFailures(t *testing.T) {
	for name, testCase := range map[string]struct {
		mutate    func(*PromptWhiteLabelServiceInput)
		issueCode string
	}{
		"quarter FTE must produce whole rupiah": {mutate: func(input *PromptWhiteLabelServiceInput) {
			input.MaintenanceItems = []PromptWhiteLabelMaintenanceItem{{
				Name: whiteLabelFactString("Fractional support", domain.SourceUser), MonthlyRate: whiteLabelMoney(10_001, domain.SourceUser, ""),
				FTEQuarters: whiteLabelFactInt(1, domain.SourceUser), PeriodMonths: whiteLabelFactInt(1, domain.SourceUser),
			}}
			input.PaymentSchedule = append(input.PaymentSchedule, PromptWhiteLabelPaymentRow{
				WorkPeriod: whiteLabelFactString("Support", domain.SourceUser), Terms: whiteLabelFactString("Monthly", domain.SourceUser),
				InvoiceDateLabel: whiteLabelFactString("Month 1", domain.SourceUser), DueDateLabel: whiteLabelFactString("H+14", domain.SourceUser),
				Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: whiteLabelMoney(2_500, domain.SourceUser, ""),
			})
		}, issueCode: "white_label.input.invalid"},
		"maintenance hosting payment mismatch": {mutate: func(input *PromptWhiteLabelServiceInput) {
			input.HostingItems = []PromptWhiteLabelHostingItem{{
				Name: whiteLabelFactString("Hosting", domain.SourceUser), MonthlyRate: whiteLabelMoney(350_000, domain.SourceUser, ""),
				Quantity: whiteLabelFactInt(1, domain.SourceUser), PeriodMonths: whiteLabelFactInt(3, domain.SourceUser),
			}}
			input.PaymentSchedule = append(input.PaymentSchedule, PromptWhiteLabelPaymentRow{
				WorkPeriod: whiteLabelFactString("Hosting", domain.SourceUser), Terms: whiteLabelFactString("Monthly", domain.SourceUser),
				InvoiceDateLabel: whiteLabelFactString("Month 1", domain.SourceUser), DueDateLabel: whiteLabelFactString("H+14", domain.SourceUser),
				Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: whiteLabelMoney(1_049_999, domain.SourceUser, ""),
			})
		}, issueCode: "white_label.payment.maintenance_hosting_mismatch"},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptWhiteLabelState()
			serviceInput := mustPromptWhiteLabelServiceInput(t, state.ServiceInput)
			testCase.mutate(&serviceInput)
			state.ServiceInput = mustPromptWhiteLabelJSON(t, serviceInput)

			_, _, _, issues, err := CalculatePromptWhiteLabel(&state, time.Now().UTC())

			require.NoError(t, err)
			assert.True(t, hasWhiteLabelIssue(issues, testCase.issueCode), "%+v", issues)
		})
	}
}

func TestPromptServiceAdapterRegistryExposesWhiteLabelWithoutReplacingSaaS(t *testing.T) {
	registry := NewPromptServiceAdapterRegistry(nil, nil, nil, nil)

	require.NotNil(t, registry.SubscriptionSaaS())
	require.NotNil(t, registry.WhiteLabel())
	assert.Equal(t, domain.CommercialSchemeWhiteLabel, registry.WhiteLabel().Scheme())
}

func validPromptWhiteLabelState() domain.PromptQuotationState {
	input := PromptWhiteLabelServiceInput{
		Kind:         PromptWhiteLabelServiceInputKind,
		ProductName:  whiteLabelFactString("POS Mini Market", domain.SourceMeeting),
		TaxTreatment: whiteLabelFactString(string(domain.WhiteLabelTaxNotStated), domain.SourceSuggestion),
		LicenseItems: []PromptWhiteLabelLicenseItem{{
			Name: whiteLabelFactString("POS Mini Market License", domain.SourceMeeting), UnitRate: whiteLabelMoney(35_000_000, domain.SourceCatalog, "WL-POS-2026"),
			Quantity: whiteLabelFactInt(1, domain.SourceMeeting),
		}},
		PaymentSchedule: []PromptWhiteLabelPaymentRow{{
			WorkPeriod: whiteLabelFactString("License purchase", domain.SourceMeeting), Terms: whiteLabelFactString("Settlement", domain.SourceMeeting),
			InvoiceDateLabel: whiteLabelFactString("On contract", domain.SourceMeeting), DueDateLabel: whiteLabelFactString("H+14", domain.SourceMeeting),
			Component: domain.WhiteLabelPaymentLicense, Nominal: whiteLabelMoney(35_000_000, domain.SourceUser, ""),
		}},
		LimitationCodes: []domain.Fact[string]{whiteLabelFactString("out_of_pocket", domain.SourceSuggestion)},
	}
	raw, err := EncodePromptWhiteLabelServiceInput(input)
	if err != nil {
		panic(err)
	}
	return domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "Nadia needs a POS Mini Market white-label license with rebranding rights.",
		ClientName: whiteLabelFactPtr("Nadia", domain.SourceUser), ProjectName: whiteLabelFactPtr("POS Mini Market", domain.SourceUser),
		CommercialScheme: whiteLabelFactPtr(domain.CommercialSchemeWhiteLabel, domain.SourceUser), ServiceInput: raw,
	}
}

func whiteLabelFactString(value string, source domain.ValueSource) domain.Fact[string] {
	return domain.Fact[string]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}

func whiteLabelFactInt(value int, source domain.ValueSource) domain.Fact[int] {
	return domain.Fact[int]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}

func whiteLabelFactPtr(value string, source domain.ValueSource) *domain.Fact[string] {
	fact := whiteLabelFactString(value, source)
	return &fact
}

func whiteLabelMoney(value int64, source domain.ValueSource, catalogRef string) domain.MoneyInput {
	amount := value
	money := domain.MoneyInput{Amount: &amount, Source: source}
	if catalogRef != "" {
		money.CatalogRef = &catalogRef
	}
	return money
}

func mustPromptWhiteLabelServiceInput(t *testing.T, raw []byte) PromptWhiteLabelServiceInput {
	t.Helper()
	input, err := DecodePromptWhiteLabelServiceInput(raw)
	require.NoError(t, err)
	return input
}

func mustPromptWhiteLabelJSON(t *testing.T, input PromptWhiteLabelServiceInput) []byte {
	t.Helper()
	raw, err := EncodePromptWhiteLabelServiceInput(input)
	require.NoError(t, err)
	return raw
}

func blockingWhiteLabelIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	blocking := make([]domain.DraftIssue, 0)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func hasWhiteLabelIssue(issues []domain.DraftIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
