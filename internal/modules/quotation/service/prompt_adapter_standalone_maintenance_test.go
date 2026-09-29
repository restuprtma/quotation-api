package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestCalculatePromptStandaloneMaintenanceRetainerComplete(t *testing.T) {
	state := validPromptRetainerState()

	input, calculation, profile, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingStandaloneMaintenanceIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.StandaloneMaintenanceRetainer, calculation.Mode)
	require.Len(t, calculation.Retainer, 2)
	assert.Equal(t, int64(16_050_000), calculation.Retainer[0].LineTotal)
	assert.Equal(t, int64(7_035_000), calculation.Retainer[1].LineTotal)
	assert.Equal(t, int64(23_085_000), calculation.Subtotal)
	assert.Equal(t, int64(0), calculation.DiscountAmount)
	assert.Equal(t, int64(23_085_000), calculation.TotalInvestment)
	assert.Equal(t, int64(23_085_000), calculation.PaymentTotal)
	assert.Len(t, input.RetainerItems, 2)
	assert.Len(t, input.DefectGroups, 1)
	assert.Equal(t, domain.StandaloneMaintenanceRetainer, profile.Mode)
}

func TestCalculatePromptStandaloneMaintenanceRetainerMissingOrUntrustedRate(t *testing.T) {
	for name, mutate := range map[string]func(*PromptStandaloneMaintenanceServiceInput){
		"missing retainer monthly rate": func(input *PromptStandaloneMaintenanceServiceInput) {
			input.RetainerItems[0].MonthlyRate.Amount = nil
			input.RetainerItems[0].MonthlyRate.Source = domain.SourceMeeting
		},
		"meeting rate is untrusted": func(input *PromptStandaloneMaintenanceServiceInput) {
			input.RetainerItems[0].MonthlyRate.Source = domain.SourceMeeting
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptRetainerState()
			serviceInput := mustPromptStandaloneMaintenanceServiceInput(t, state.ServiceInput)
			mutate(&serviceInput)
			state.ServiceInput = mustPromptStandaloneMaintenanceJSON(t, serviceInput)

			_, calculation, _, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Now().UTC())

			require.NoError(t, err)
			assert.Zero(t, calculation.TotalInvestment)
			if strings.Contains(name, "missing") {
				assert.True(t, hasStandaloneMaintenanceIssue(issues, "standalone_maintenance.retainer_monthly_rate.unavailable"), "%+v", issues)
			} else {
				assert.True(t, hasStandaloneMaintenanceIssue(issues, "standalone_maintenance.retainer_monthly_rate.untrusted_source"), "%+v", issues)
			}
		})
	}
}

func TestCalculatePromptStandaloneMaintenanceHourlyTasksWithHoursNeverYieldsPriceWithoutExplicitQuotedTotal(t *testing.T) {
	// Gate check: hours alone never yield a price; quoted hourly total is not derived from display rate.
	state := validPromptHourlyState()
	serviceInput := mustPromptStandaloneMaintenanceServiceInput(t, state.ServiceInput)
	// Missing quoted line total on hourly item
	serviceInput.HourlyCostItems[0].QuotedLineTotal.Amount = nil
	serviceInput.HourlyCostItems[0].QuotedLineTotal.Source = domain.SourceSuggestion
	state.ServiceInput = mustPromptStandaloneMaintenanceJSON(t, serviceInput)

	_, calculation, _, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Now().UTC())

	require.NoError(t, err)
	assert.Zero(t, calculation.TotalInvestment)
	assert.True(t, hasStandaloneMaintenanceIssue(issues, "standalone_maintenance.hourly_quoted_total.unavailable"), "%+v", issues)
}

func TestCalculatePromptStandaloneMaintenanceHourlyWithExplicitQuotedTotal(t *testing.T) {
	state := validPromptHourlyState()

	input, calculation, profile, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingStandaloneMaintenanceIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.StandaloneMaintenanceHourlyTask, calculation.Mode)
	require.Len(t, calculation.Hourly, 2)
	assert.Equal(t, int64(730_000), calculation.Hourly[0].LineTotal)
	assert.Equal(t, int64(140_000), calculation.Hourly[1].LineTotal)
	assert.Equal(t, int64(870_000), calculation.Subtotal)
	assert.Equal(t, int64(870_000), calculation.TotalInvestment)
	assert.Equal(t, int64(870_000), calculation.PaymentTotal)
	assert.Len(t, input.HourlyCostItems, 2)
	assert.Len(t, input.HourlyTasks, 2)
	assert.Equal(t, domain.StandaloneMaintenanceHourlyTask, profile.Mode)

	adapter := NewPromptStandaloneMaintenanceAdapter(nil)
	html, err := adapter.Render(
		QuotationMetadata{QuotationNumber: "Q-MNT-001", Subject: "Support Portal", ClientName: "Nadia"},
		PromptStandaloneMaintenanceSnapshot{Input: input, Calculation: calculation, Profile: profile},
		"<p>Legal template terms</p>",
	)
	require.NoError(t, err)
	assert.Contains(t, html, "Perbaikan crash login")
	assert.Contains(t, html, "Regression testing")
	assert.Contains(t, html, "Source code dan modul merupakan hak milik bersama.")
}

func TestCalculatePromptStandaloneMaintenanceDefectsAndPriorities(t *testing.T) {
	state := validPromptRetainerState()
	serviceInput := mustPromptStandaloneMaintenanceServiceInput(t, state.ServiceInput)
	serviceInput.DefectGroups = []PromptMaintenanceDefectGroup{{
		Title: maintenanceFactString("1. Detail Pasien", domain.SourceMeeting),
		Items: []PromptMaintenanceDefectItem{
			{
				ItemNumber:  maintenanceFactString("1.1", domain.SourceMeeting),
				Priority:    maintenanceFactString("P0", domain.SourceMeeting),
				Function:    maintenanceFactString("Melihat riwayat medis pasien di luar sesi konsultasi.", domain.SourceMeeting),
				ScreenFlow:  maintenanceFactString("Pasien > Data Pasien > Medical History.", domain.SourceMeeting),
				Existing:    maintenanceFactString("Hanya mencantumkan catatan registrasi awal.", domain.SourceMeeting),
				Improvement: maintenanceFactString("Mencantumkan seluruh riwayat SOAP.", domain.SourceMeeting),
				Reference:   maintenanceFactPtr("Ref 1.01", domain.SourceMeeting),
			},
			{
				ItemNumber:  maintenanceFactString("1.2", domain.SourceMeeting),
				Priority:    maintenanceFactString("P1", domain.SourceMeeting),
				Function:    maintenanceFactString("Export PDF riwayat resep", domain.SourceMeeting),
				ScreenFlow:  maintenanceFactString("Pasien > Resep > Export.", domain.SourceMeeting),
				Existing:    maintenanceFactString("Error saat download PDF.", domain.SourceMeeting),
				Improvement: maintenanceFactString("Generate PDF background worker.", domain.SourceMeeting),
			},
		},
	}}
	state.ServiceInput = mustPromptStandaloneMaintenanceJSON(t, serviceInput)

	input, calculation, profile, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingStandaloneMaintenanceIssues(issues), "unexpected issues: %+v", issues)
	require.Len(t, input.DefectGroups, 1)
	require.Len(t, input.DefectGroups[0].Items, 2)
	assert.Equal(t, domain.StandalonePriorityP0, input.DefectGroups[0].Items[0].Priority)
	assert.Equal(t, domain.StandalonePriorityP1, input.DefectGroups[0].Items[1].Priority)
	assert.Equal(t, "Ref 1.01", input.DefectGroups[0].Items[0].Reference)
	assert.Empty(t, input.DefectGroups[0].Items[1].Reference)

	adapter := NewPromptStandaloneMaintenanceAdapter(nil)
	html, err := adapter.Render(
		QuotationMetadata{QuotationNumber: "Q-RET-001", Subject: "Retainer Nex", ClientName: "Nadia"},
		PromptStandaloneMaintenanceSnapshot{Input: input, Calculation: calculation, Profile: profile},
		"<p>Legal</p>",
	)
	require.NoError(t, err)
	assert.Contains(t, html, "1. Detail Pasien")
	assert.Contains(t, html, "P0")
	assert.Contains(t, html, "P1")
	assert.Contains(t, html, "Mencantumkan seluruh riwayat SOAP.")
	assert.Contains(t, html, "Ref 1.01")
}

func TestCalculatePromptStandaloneMaintenanceDiscountAppliedAfterInitialPrice(t *testing.T) {
	state := validPromptRetainerState()
	bps := int32(500) // 5%
	state.AdditionalDiscountBPS = &bps

	// Subtotal is 23_085_000. 5% discount is 1_154_250. Total investment is 21_930_750.
	serviceInput := mustPromptStandaloneMaintenanceServiceInput(t, state.ServiceInput)
	serviceInput.PaymentSchedule = []PromptMaintenancePaymentRow{{
		WorkPeriod:       maintenanceFactString("13 Juli - 13 Agustus 2026", domain.SourceMeeting),
		Terms:            maintenanceFactString("Maintenance", domain.SourceMeeting),
		InvoiceDateLabel: maintenanceFactString("13 Agustus 2026", domain.SourceMeeting),
		DueDateLabel:     maintenanceFactString("27 Agustus 2026", domain.SourceMeeting),
		Nominal:          maintenanceMoney(21_930_750, domain.SourceUser, ""),
	}}
	state.ServiceInput = mustPromptStandaloneMaintenanceJSON(t, serviceInput)

	input, calculation, profile, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingStandaloneMaintenanceIssues(issues), "unexpected issues: %+v", issues)
	assert.True(t, input.ShowDiscountRow)
	assert.Equal(t, int64(1_154_250), input.DiscountAmount)
	assert.Equal(t, int64(23_085_000), calculation.Subtotal)
	assert.Equal(t, int64(1_154_250), calculation.DiscountAmount)
	assert.Equal(t, int64(21_930_750), calculation.TotalInvestment)
	assert.Equal(t, int64(21_930_750), calculation.PaymentTotal)
	assert.True(t, profile.ShowDiscountRow)
}

func TestCalculatePromptStandaloneMaintenancePaymentMismatch(t *testing.T) {
	state := validPromptRetainerState()
	serviceInput := mustPromptStandaloneMaintenanceServiceInput(t, state.ServiceInput)
	serviceInput.PaymentSchedule[0].Nominal = maintenanceMoney(23_084_999, domain.SourceUser, "")
	state.ServiceInput = mustPromptStandaloneMaintenanceJSON(t, serviceInput)

	_, _, _, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Now().UTC())

	require.NoError(t, err)
	assert.True(t, hasStandaloneMaintenanceIssue(issues, "standalone_maintenance.payment.total_mismatch"), "%+v", issues)
}

func TestCalculatePromptStandaloneMaintenanceQuarterFTEWholeRupiah(t *testing.T) {
	state := validPromptRetainerState()
	serviceInput := mustPromptStandaloneMaintenanceServiceInput(t, state.ServiceInput)
	// Monthly rate 10_001 with 1 FTE quarter = 10_001 / 4 = not divisible by 4
	serviceInput.RetainerItems[0].MonthlyRate = maintenanceMoney(10_001, domain.SourceUser, "")
	serviceInput.RetainerItems[0].FTEQuarters = maintenanceFactInt(1, domain.SourceUser)
	serviceInput.RetainerItems[0].PeriodMonths = maintenanceFactInt(1, domain.SourceUser)
	state.ServiceInput = mustPromptStandaloneMaintenanceJSON(t, serviceInput)

	_, _, _, issues, err := CalculatePromptStandaloneMaintenance(&state, time.Now().UTC())

	require.NoError(t, err)
	assert.True(t, hasStandaloneMaintenanceIssue(issues, "standalone_maintenance.input.invalid"), "%+v", issues)
}

func TestPromptServiceAdapterRegistryExposesStandaloneMaintenance(t *testing.T) {
	registry := NewPromptServiceAdapterRegistry(nil, nil, nil, nil)

	require.NotNil(t, registry.SubscriptionSaaS())
	require.NotNil(t, registry.WhiteLabel())
	require.NotNil(t, registry.StandaloneMaintenance())
	assert.Equal(t, domain.CommercialSchemeStandaloneMaintenance, registry.StandaloneMaintenance().Scheme())
}

func validPromptRetainerState() domain.PromptQuotationState {
	input := PromptStandaloneMaintenanceServiceInput{
		Kind:         PromptStandaloneMaintenanceServiceInputKind,
		SystemName:   maintenanceFactString("Aplikasi Nex Health", domain.SourceMeeting),
		Mode:         maintenanceFactString(string(domain.StandaloneMaintenanceRetainer), domain.SourceMeeting),
		TaxTreatment: maintenanceFactString(string(domain.StandaloneTaxExclusive), domain.SourceMeeting),
		RetainerItems: []PromptMaintenanceRetainerItem{
			{
				Name:         maintenanceFactString("Senior Web Programmer", domain.SourceMeeting),
				MonthlyRate:  maintenanceMoney(16_050_000, domain.SourceCatalog, "VENTURO-STD-2026-PROG-SR"),
				FTEQuarters:  maintenanceFactInt(4, domain.SourceMeeting),
				PeriodMonths: maintenanceFactInt(1, domain.SourceMeeting),
			},
			{
				Name:         maintenanceFactString("Senior Quality Assurance", domain.SourceMeeting),
				MonthlyRate:  maintenanceMoney(14_070_000, domain.SourceCatalog, "VENTURO-STD-2026-QA-SR"),
				FTEQuarters:  maintenanceFactInt(2, domain.SourceMeeting),
				PeriodMonths: maintenanceFactInt(1, domain.SourceMeeting),
			},
		},
		DefectGroups: []PromptMaintenanceDefectGroup{{
			Title: maintenanceFactString("1. Detail Pasien", domain.SourceMeeting),
			Items: []PromptMaintenanceDefectItem{{
				ItemNumber:  maintenanceFactString("1.1", domain.SourceMeeting),
				Priority:    maintenanceFactString("P0", domain.SourceMeeting),
				Function:    maintenanceFactString("Melihat riwayat medis pasien di luar sesi konsultasi.", domain.SourceMeeting),
				ScreenFlow:  maintenanceFactString("Pasien > Data Pasien dan Riwayat > Lihat Detail Pasien > Medical History.", domain.SourceMeeting),
				Existing:    maintenanceFactString("Hanya mencantumkan catatan registrasi awal.", domain.SourceMeeting),
				Improvement: maintenanceFactString("Mencantumkan seluruh riwayat SOAP (Dokter, Terapis, dan Perawat).", domain.SourceMeeting),
				Reference:   maintenanceFactPtr("Ref 1.01", domain.SourceMeeting),
			}},
		}},
		PaymentSchedule: []PromptMaintenancePaymentRow{{
			WorkPeriod:       maintenanceFactString("13 Juli - 13 Agustus 2026", domain.SourceMeeting),
			Terms:            maintenanceFactString("Maintenance", domain.SourceMeeting),
			InvoiceDateLabel: maintenanceFactString("13 Agustus 2026", domain.SourceMeeting),
			DueDateLabel:     maintenanceFactString("27 Agustus 2026", domain.SourceMeeting),
			Nominal:          maintenanceMoney(23_085_000, domain.SourceUser, ""),
		}},
	}
	raw, err := EncodePromptStandaloneMaintenanceServiceInput(input)
	if err != nil {
		panic(err)
	}
	return domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Retainer maintenance untuk Aplikasi Nex Health selama 1 bulan.",
		ClientName:       maintenanceFactPtr("Nadia", domain.SourceUser),
		ProjectName:      maintenanceFactPtr("Aplikasi Nex Health", domain.SourceUser),
		CommercialScheme: maintenanceFactPtr(domain.CommercialSchemeStandaloneMaintenance, domain.SourceUser),
		ServiceInput:     raw,
	}
}

func validPromptHourlyState() domain.PromptQuotationState {
	input := PromptStandaloneMaintenanceServiceInput{
		Kind:         PromptStandaloneMaintenanceServiceInputKind,
		SystemName:   maintenanceFactString("Portal Berita Liputan6", domain.SourceMeeting),
		Mode:         maintenanceFactString(string(domain.StandaloneMaintenanceHourlyTask), domain.SourceMeeting),
		TaxTreatment: maintenanceFactString(string(domain.StandaloneTaxNotStated), domain.SourceSuggestion),
		HourlyCostItems: []PromptMaintenanceHourlyCostItem{
			{
				Name:               maintenanceFactString("Senior Mobile Programmer", domain.SourceMeeting),
				DisplayMonthlyRate: maintenanceMoney(16_050_000, domain.SourceCatalog, "VENTURO-STD-2026-MOB-SR"),
				Headcount:          maintenanceFactInt(1, domain.SourceMeeting),
				HoursHalfUnits:     maintenanceFactInt(16, domain.SourceMeeting),
				QuotedLineTotal:    maintenanceMoney(730_000, domain.SourceUser, ""),
			},
			{
				Name:               maintenanceFactString("Senior QA", domain.SourceMeeting),
				DisplayMonthlyRate: maintenanceMoney(12_500_000, domain.SourceCatalog, "VENTURO-STD-2026-QA-SR"),
				Headcount:          maintenanceFactInt(1, domain.SourceMeeting),
				HoursHalfUnits:     maintenanceFactInt(4, domain.SourceMeeting),
				QuotedLineTotal:    maintenanceMoney(140_000, domain.SourceUser, ""),
			},
		},
		HourlyTasks: []PromptMaintenanceHourlyTask{
			{Description: maintenanceFactString("Perbaikan crash login", domain.SourceMeeting), Platform: maintenanceFactString("Android", domain.SourceMeeting), HoursHalfUnits: maintenanceFactInt(16, domain.SourceMeeting)},
			{Description: maintenanceFactString("Regression testing", domain.SourceMeeting), Platform: maintenanceFactString("QA Test", domain.SourceMeeting), HoursHalfUnits: maintenanceFactInt(4, domain.SourceMeeting)},
		},
		PaymentSchedule: []PromptMaintenancePaymentRow{{
			WorkPeriod:       maintenanceFactString("Support Juli 2026", domain.SourceMeeting),
			Terms:            maintenanceFactString("Pelunasan", domain.SourceMeeting),
			InvoiceDateLabel: maintenanceFactString("31 Juli 2026", domain.SourceMeeting),
			DueDateLabel:     maintenanceFactString("14 Agustus 2026", domain.SourceMeeting),
			Nominal:          maintenanceMoney(870_000, domain.SourceUser, ""),
		}},
		RightsStatements: []domain.Fact[string]{maintenanceFactString("Source code dan modul merupakan hak milik bersama.", domain.SourceMeeting)},
	}
	raw, err := EncodePromptStandaloneMaintenanceServiceInput(input)
	if err != nil {
		panic(err)
	}
	return domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Hourly maintenance untuk Portal Berita Liputan6.",
		ClientName:       maintenanceFactPtr("Nadia", domain.SourceUser),
		ProjectName:      maintenanceFactPtr("Portal Berita Liputan6", domain.SourceUser),
		CommercialScheme: maintenanceFactPtr(domain.CommercialSchemeStandaloneMaintenance, domain.SourceUser),
		ServiceInput:     raw,
	}
}

func maintenanceFactString(value string, source domain.ValueSource) domain.Fact[string] {
	return domain.Fact[string]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}

func maintenanceFactInt(value int, source domain.ValueSource) domain.Fact[int] {
	return domain.Fact[int]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}

func maintenanceFactPtr(value string, source domain.ValueSource) *domain.Fact[string] {
	fact := maintenanceFactString(value, source)
	return &fact
}

func maintenanceMoney(value int64, source domain.ValueSource, catalogRef string) domain.MoneyInput {
	amount := value
	money := domain.MoneyInput{Amount: &amount, Source: source}
	if catalogRef != "" {
		money.CatalogRef = &catalogRef
	}
	return money
}

func mustPromptStandaloneMaintenanceServiceInput(t *testing.T, raw []byte) PromptStandaloneMaintenanceServiceInput {
	t.Helper()
	input, err := DecodePromptStandaloneMaintenanceServiceInput(raw)
	require.NoError(t, err)
	return input
}

func mustPromptStandaloneMaintenanceJSON(t *testing.T, input PromptStandaloneMaintenanceServiceInput) []byte {
	t.Helper()
	raw, err := EncodePromptStandaloneMaintenanceServiceInput(input)
	require.NoError(t, err)
	return raw
}

func blockingStandaloneMaintenanceIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	blocking := make([]domain.DraftIssue, 0)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func hasStandaloneMaintenanceIssue(issues []domain.DraftIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
