package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validPromptCustomDevelopmentKopiStudio24State() domain.PromptQuotationState {
	input := PromptCustomDevelopmentServiceInput{
		Kind:                 PromptCustomDevelopmentServiceInputKind,
		SystemName:           customDevFactString("Sistem Kopi Studio24", domain.SourceMeeting),
		DevelopmentTableMode: customDevFactString(string(domain.CustomDevelopmentMonthlyPeriod), domain.SourceMeeting),
		TaxTreatment:         customDevFactString(string(domain.CustomDevelopmentTaxExclusive), domain.SourceMeeting),
		DevelopmentGroups: []PromptCustomDevelopmentCostGroup{
			{
				Title: customDevFactString("Fase 1: Backend & Core Engine", domain.SourceMeeting),
				Items: []PromptCustomDevelopmentCostItem{
					{
						Name:            customDevFactString("Senior Backend Developer", domain.SourceMeeting),
						DisplayRate:     customDevMoneyInput(15_000_000, domain.SourceCatalog, "rate-sr-be"),
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("1", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(15_000_000, domain.SourceCatalog, "rate-sr-be"),
					},
					{
						Name:            customDevFactString("Frontend Developer", domain.SourceMeeting),
						DisplayRate:     customDevMoneyInput(5_062_500, domain.SourceCatalog, "rate-fe"),
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("1", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(5_062_500, domain.SourceCatalog, "rate-fe"),
					},
				},
			},
		},
		DiscountLabel: customDevFactPtr("DISCOUNT", domain.SourceUser),
		MonthlyItems: []PromptCustomDevelopmentMonthlyItem{
			{
				Name:          customDevFactString("Venturo Cloud Mini", domain.SourceMeeting),
				Specification: customDevFactPtr("Cloud Server Standard", domain.SourceMeeting),
				UnitRate:      customDevMoneyInput(800_000, domain.SourceCatalog, "cloud-mini"),
				Quantity:      customDevFactInt(1, domain.SourceMeeting),
			},
			{
				Name:          customDevFactString("Support", domain.SourceMeeting),
				Specification: customDevFactPtr("4 Jam Maintenance Support", domain.SourceMeeting),
				UnitRate:      customDevMoneyInput(93_000, domain.SourceCatalog, "support-hr"),
				Quantity:      customDevFactInt(4, domain.SourceMeeting),
			},
			{
				Name:          customDevFactString("WhatsApp Connect", domain.SourceMeeting),
				Specification: customDevFactPtr("Integrasi Notifikasi WA", domain.SourceMeeting),
				UnitRate:      customDevMoneyInput(100_000, domain.SourceCatalog, "wa-connect"),
				Quantity:      customDevFactInt(1, domain.SourceMeeting),
			},
		},
		MonthlyNotes: []domain.Fact[string]{
			customDevFactString("Biaya API Gemini ditandai out-of-pocket.", domain.SourceMeeting),
		},
		ModuleGroups: []PromptCustomDevelopmentModuleGroup{
			{
				Name: customDevFactString("Modul Transaksi & Kasir", domain.SourceMeeting),
				Features: []domain.Fact[string]{
					customDevFactString("Pencatatan order dan invoice", domain.SourceMeeting),
					customDevFactString("Integrasi payment gateway", domain.SourceMeeting),
				},
			},
		},
		TimelineRows: []PromptCustomDevelopmentTimelineRow{
			{
				Description: customDevFactString("Fase 1: Pengembangan dan Testing", domain.SourceMeeting),
				Timeline:    customDevFactString("4 Minggu", domain.SourceMeeting),
			},
		},
		PaymentSchedule: []PromptCustomDevelopmentPaymentRow{
			{
				WorkPeriod:       customDevFactString("Down Payment 50%", domain.SourceMeeting),
				Terms:            customDevFactString("Termin 1", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Saat tanda tangan kontrak", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          customDevMoneyInput(9_000_000, domain.SourceCatalog, ""),
			},
			{
				WorkPeriod:       customDevFactString("Pelunasan 50%", domain.SourceMeeting),
				Terms:            customDevFactString("Termin 2", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Setelah serah terima", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          customDevMoneyInput(9_000_000, domain.SourceCatalog, ""),
			},
			{
				WorkPeriod:       customDevFactString("Bulan 1", domain.SourceMeeting),
				Terms:            customDevFactString("Layanan Bulanan Cloud & Support", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Awal bulan berjalan", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentMonthly,
				Nominal:          customDevMoneyInput(1_272_000, domain.SourceCatalog, ""),
			},
		},
		LimitationCodes: []domain.Fact[string]{
			customDevFactString("out_of_pocket", domain.SourceMeeting),
			customDevFactString("overtime", domain.SourceMeeting),
			customDevFactString("third_party", domain.SourceMeeting),
			customDevFactString("scope_change", domain.SourceMeeting),
		},
	}

	raw, err := EncodePromptCustomDevelopmentServiceInput(input)
	if err != nil {
		panic(err)
	}

	bps := 1028 // (20_062_500 * 1028) / 10000 = 2_062_425; or explicit discount
	_ = bps

	return domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Pengembangan Sistem Kopi Studio24 dengan Backend & Core Engine, cloud hosting dan support bulanan.",
		ClientName:       customDevFactPtr("Nadia", domain.SourceMeeting),
		ClientCompany:    customDevFactPtr("Kopi Studio24", domain.SourceMeeting),
		CommercialScheme: customDevFactPtr(domain.CommercialSchemeCustomDevelopment, domain.SourceMeeting),
		ProjectName:      customDevFactPtr("Sistem Kopi Studio24", domain.SourceMeeting),
		ServiceInput:     raw,
	}
}

func validPromptCustomDevelopmentEcoflowState() domain.PromptQuotationState {
	input := PromptCustomDevelopmentServiceInput{
		Kind:                 PromptCustomDevelopmentServiceInputKind,
		SystemName:           customDevFactString("Company Profile Ecoflow", domain.SourceMeeting),
		DevelopmentTableMode: customDevFactString(string(domain.CustomDevelopmentFixedDays), domain.SourceMeeting),
		TaxTreatment:         customDevFactString(string(domain.CustomDevelopmentTaxExclusive), domain.SourceMeeting),
		DevelopmentGroups: []PromptCustomDevelopmentCostGroup{
			{
				Title: customDevFactString("Website 1: Landing Page & Katalog", domain.SourceMeeting),
				Items: []PromptCustomDevelopmentCostItem{
					{
						Name:            customDevFactString("Frontend & Backend Developer", domain.SourceMeeting),
						DisplayRate:     domain.MoneyInput{Source: domain.SourceSuggestion},
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("15 Hari", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(15_000_000, domain.SourceCatalog, ""),
					},
					{
						Name:            customDevFactString("QA Tester (Included)", domain.SourceMeeting),
						DisplayRate:     domain.MoneyInput{Source: domain.SourceSuggestion},
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("15 Hari", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingIncluded,
						QuotedLineTotal: domain.MoneyInput{Source: domain.SourceSuggestion}, // intentional nil amount
					},
				},
			},
			{
				Title: customDevFactString("Website 2: Portal Klien & Admin", domain.SourceMeeting),
				Items: []PromptCustomDevelopmentCostItem{
					{
						Name:            customDevFactString("Fullstack Developer", domain.SourceMeeting),
						DisplayRate:     domain.MoneyInput{Source: domain.SourceSuggestion},
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("10 Hari", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(10_000_000, domain.SourceCatalog, ""),
					},
				},
			},
		},
		DiscountLabel: customDevFactPtr("DISCOUNT DEVELOPMENT", domain.SourceUser),
		ModuleGroups: []PromptCustomDevelopmentModuleGroup{
			{
				Name: customDevFactString("Fitur Landing Page", domain.SourceMeeting),
				Features: []domain.Fact[string]{
					customDevFactString("Katalog Produk", domain.SourceMeeting),
					customDevFactString("Kontak dan Form Inquiry", domain.SourceMeeting),
				},
			},
		},
		TimelineRows: []PromptCustomDevelopmentTimelineRow{
			{
				Description: customDevFactString("Pengerjaan Website 1 dan 2", domain.SourceMeeting),
				Timeline:    customDevFactString("25 Hari Kerja", domain.SourceMeeting),
			},
		},
		PaymentSchedule: []PromptCustomDevelopmentPaymentRow{
			{
				WorkPeriod:       customDevFactString("Termin 1 (DP 50%)", domain.SourceMeeting),
				Terms:            customDevFactString("Down Payment", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Saat tanda tangan kontrak", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          customDevMoneyInput(7_500_000, domain.SourceCatalog, ""),
			},
			{
				WorkPeriod:       customDevFactString("Termin 2 (Pelunasan 50%)", domain.SourceMeeting),
				Terms:            customDevFactString("Pelunasan", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Setelah live deployment", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          customDevMoneyInput(7_500_000, domain.SourceCatalog, ""),
			},
		},
		LimitationCodes: []domain.Fact[string]{
			customDevFactString("out_of_pocket", domain.SourceMeeting),
			customDevFactString("overtime", domain.SourceMeeting),
			customDevFactString("third_party", domain.SourceMeeting),
			customDevFactString("scope_change", domain.SourceMeeting),
		},
	}

	raw, err := EncodePromptCustomDevelopmentServiceInput(input)
	if err != nil {
		panic(err)
	}

	discountBps := int32(4000) // 40% of 25M = 10M discount -> 15M total investment

	return domain.PromptQuotationState{
		SchemaVersion:         domain.SchemaPromptQuotationV1,
		MeetingNotes:          "Pembuatan Company Profile Ecoflow Website 1 Landing Page dan Website 2 Portal Klien.",
		ClientName:            customDevFactPtr("Budi", domain.SourceMeeting),
		ClientCompany:         customDevFactPtr("PT Ecoflow", domain.SourceMeeting),
		CommercialScheme:      customDevFactPtr(domain.CommercialSchemeCustomDevelopment, domain.SourceMeeting),
		ProjectName:           customDevFactPtr("Company Profile Ecoflow", domain.SourceMeeting),
		AdditionalDiscountBPS: &discountBps,
		ServiceInput:          raw,
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func customDevFactString(value string, source domain.ValueSource) domain.Fact[string] {
	return domain.Fact[string]{Value: value, Source: source, Evidence: value, Confirmed: true}
}

func customDevFactPtr(value string, source domain.ValueSource) *domain.Fact[string] {
	f := customDevFactString(value, source)
	return &f
}

func customDevFactInt(value int, source domain.ValueSource) domain.Fact[int] {
	return domain.Fact[int]{Value: value, Source: source, Evidence: "notes", Confirmed: true}
}

func customDevMoneyInput(amount int64, source domain.ValueSource, catalogRef string) domain.MoneyInput {
	m := domain.MoneyInput{
		Amount: &amount,
		Source: source,
	}
	if source == domain.SourceCatalog {
		if catalogRef == "" {
			catalogRef = "VENTURO-STD-2026-CATALOG-REF"
		}
		m.CatalogRef = &catalogRef
	} else if catalogRef != "" {
		m.CatalogRef = &catalogRef
	}
	return m
}

func mustPromptCustomDevelopmentServiceInput(t *testing.T, raw []byte) PromptCustomDevelopmentServiceInput {
	t.Helper()
	input, err := DecodePromptCustomDevelopmentServiceInput(raw)
	require.NoError(t, err)
	return input
}

func mustPromptCustomDevelopmentJSON(t *testing.T, in PromptCustomDevelopmentServiceInput) []byte {
	t.Helper()
	raw, err := EncodePromptCustomDevelopmentServiceInput(in)
	require.NoError(t, err)
	return raw
}

func blockingCustomDevIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	var blocking []domain.DraftIssue
	for _, iss := range issues {
		if iss.Severity == domain.IssueBlocking {
			blocking = append(blocking, iss)
		}
	}
	return blocking
}

func hasCustomDevIssue(issues []domain.DraftIssue, code string) bool {
	for _, iss := range issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}

func TestCalculatePromptCustomDevelopment_KopiStudio24(t *testing.T) {
	state := validPromptCustomDevelopmentKopiStudio24State()
	// Set payments to match subtotal 20_062_500 (no discount)
	serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
	serviceInput.PaymentSchedule[0].Nominal = customDevMoneyInput(10_031_250, domain.SourceCatalog, "")
	serviceInput.PaymentSchedule[1].Nominal = customDevMoneyInput(10_031_250, domain.SourceCatalog, "")
	state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

	input, calc, profile, issues, err := CalculatePromptCustomDevelopment(&state, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, blockingCustomDevIssues(issues), "unexpected issues: %+v", issues)

	assert.Equal(t, int64(20_062_500), calc.DevelopmentSubtotal)
	assert.Equal(t, int64(0), calc.DiscountAmount)
	assert.Equal(t, int64(20_062_500), calc.TotalInvestment)
	assert.Equal(t, int64(1_272_000), calc.MonthlyTotal)
	assert.Equal(t, int64(20_062_500), calc.DevelopmentPaymentTotal)
	assert.Equal(t, int32(1), calc.MonthlyPaymentRows)

	require.Len(t, calc.DevelopmentGroups, 1)
	require.Len(t, calc.DevelopmentGroups[0].Items, 2)
	assert.Equal(t, ptrInt64(15_000_000), calc.DevelopmentGroups[0].Items[0].LineTotal)
	assert.Equal(t, ptrInt64(5_062_500), calc.DevelopmentGroups[0].Items[1].LineTotal)

	require.Len(t, calc.MonthlyLines, 3)
	assert.Equal(t, int64(800_000), calc.MonthlyLines[0].LineTotal)
	assert.Equal(t, int64(372_000), calc.MonthlyLines[1].LineTotal)
	assert.Equal(t, int64(100_000), calc.MonthlyLines[2].LineTotal)

	assert.Equal(t, domain.CustomDevelopmentMonthlyPeriod, profile.TableMode)
	assert.True(t, profile.ShowRateColumn)
	assert.True(t, profile.ShowMonthly)
	assert.Equal(t, "Rate (Monthly)", profile.RateHeader)
	assert.Equal(t, "Period (Month)", profile.PeriodHeader)

	// Render HTML verification
	adapter := NewPromptCustomDevelopmentAdapter(nil)
	html, err := adapter.Render(
		QuotationMetadata{QuotationNumber: "Q-DEV-001", Subject: "Sistem Kopi Studio24", ClientName: "Nadia"},
		PromptCustomDevelopmentSnapshot{Input: input, Calculation: calc, Profile: profile},
		"<p>Syarat legal baku.</p>",
	)
	require.NoError(t, err)
	assert.Contains(t, html, "Sistem Kopi Studio24")
	assert.Contains(t, html, "Senior Backend Developer")
	assert.Contains(t, html, "Venturo Cloud Mini")
	assert.Contains(t, html, "Modul Transaksi &amp; Kasir")
	assert.Contains(t, html, "Rp 20.062.500")
	assert.Contains(t, html, "Rp 1.272.000")
}

func TestCalculatePromptCustomDevelopment_AssessmentOnline(t *testing.T) {
	input := PromptCustomDevelopmentServiceInput{
		Kind:                 PromptCustomDevelopmentServiceInputKind,
		SystemName:           customDevFactString("Assessment Online - Tes Kognitif", domain.SourceMeeting),
		DevelopmentTableMode: customDevFactString(string(domain.CustomDevelopmentMonthlyPeriod), domain.SourceMeeting),
		TaxTreatment:         customDevFactString(string(domain.CustomDevelopmentTaxExclusive), domain.SourceMeeting),
		DevelopmentGroups: []PromptCustomDevelopmentCostGroup{
			{
				Title: customDevFactString("Pengembangan Sistem Assessment", domain.SourceMeeting),
				Items: []PromptCustomDevelopmentCostItem{
					{
						Name:            customDevFactString("Developer", domain.SourceMeeting),
						DisplayRate:     customDevMoneyInput(6_685_000, domain.SourceCatalog, ""),
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("1", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(6_685_000, domain.SourceCatalog, ""),
					},
					{
						Name:            customDevFactString("Quality Assurance", domain.SourceMeeting),
						DisplayRate:     customDevMoneyInput(2_005_500, domain.SourceCatalog, ""),
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("1", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(2_005_500, domain.SourceCatalog, ""),
					},
					{
						Name:            customDevFactString("Hosting & Domain", domain.SourceMeeting),
						DisplayRate:     customDevMoneyInput(200_000, domain.SourceCatalog, ""),
						Headcount:       customDevFactInt(1, domain.SourceMeeting),
						PeriodLabel:     customDevFactString("12", domain.SourceMeeting),
						PricingStatus:   CustomDevelopmentPricingExplicit,
						QuotedLineTotal: customDevMoneyInput(2_400_000, domain.SourceCatalog, ""),
					},
				},
			},
		},
		DiscountLabel: customDevFactPtr("DISCOUNT", domain.SourceUser),
		ModuleGroups: []PromptCustomDevelopmentModuleGroup{
			{
				Name: customDevFactString("Modul Tes Kognitif", domain.SourceMeeting),
				Features: []domain.Fact[string]{
					customDevFactString("Modul Soal dan Jawaban", domain.SourceMeeting),
					customDevFactString("Scoring Otomatis", domain.SourceMeeting),
				},
			},
		},
		TimelineRows: []PromptCustomDevelopmentTimelineRow{
			{
				Description: customDevFactString("Pengembangan & UAT", domain.SourceMeeting),
				Timeline:    customDevFactString("2 Bulan", domain.SourceMeeting),
			},
		},
		PaymentSchedule: []PromptCustomDevelopmentPaymentRow{
			{
				WorkPeriod:       customDevFactString("Termin 1 (DP 50%)", domain.SourceMeeting),
				Terms:            customDevFactString("Down Payment", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Saat kontrak disepakati", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          customDevMoneyInput(5_545_250, domain.SourceCatalog, ""),
			},
			{
				WorkPeriod:       customDevFactString("Termin 2 (Pelunasan 50%)", domain.SourceMeeting),
				Terms:            customDevFactString("Pelunasan", domain.SourceMeeting),
				InvoiceDateLabel: customDevFactString("Setelah UAT selesai", domain.SourceMeeting),
				DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          customDevMoneyInput(5_545_250, domain.SourceCatalog, ""),
			},
		},
		LimitationCodes: []domain.Fact[string]{
			customDevFactString("out_of_pocket", domain.SourceMeeting),
			customDevFactString("overtime", domain.SourceMeeting),
			customDevFactString("third_party", domain.SourceMeeting),
			customDevFactString("scope_change", domain.SourceMeeting),
		},
	}
	raw, err := EncodePromptCustomDevelopmentServiceInput(input)
	require.NoError(t, err)

	state := domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Pengembangan Assessment Online Tes Kognitif dengan modul soal, scoring otomatis, hosting & domain.",
		ClientName:       customDevFactPtr("Pak Hendra", domain.SourceMeeting),
		ClientCompany:    customDevFactPtr("PT Assessment", domain.SourceMeeting),
		CommercialScheme: customDevFactPtr(domain.CommercialSchemeCustomDevelopment, domain.SourceMeeting),
		ProjectName:      customDevFactPtr("Assessment Online - Tes Kognitif", domain.SourceMeeting),
		ServiceInput:     raw,
	}

	_, calc, profile, issues, err := CalculatePromptCustomDevelopment(&state, time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, blockingCustomDevIssues(issues), "%+v", issues)

	assert.Equal(t, int64(11_090_500), calc.DevelopmentSubtotal)
	assert.Equal(t, int64(0), calc.DiscountAmount)
	assert.Equal(t, int64(11_090_500), calc.TotalInvestment)
	assert.Equal(t, int64(0), calc.MonthlyTotal)
	assert.Equal(t, int64(11_090_500), calc.DevelopmentPaymentTotal)
	assert.Empty(t, calc.MonthlyLines)
	assert.False(t, profile.ShowMonthly)
}

func TestCalculatePromptCustomDevelopment_Ecoflow_IncludedLinesNilTotals(t *testing.T) {
	state := validPromptCustomDevelopmentEcoflowState()

	input, calc, profile, issues, err := CalculatePromptCustomDevelopment(&state, time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, blockingCustomDevIssues(issues), "unexpected issues: %+v", issues)

	// Subtotal = 15M + 10M = 25M (included QA tester line is nil and does NOT add to subtotal)
	assert.Equal(t, int64(25_000_000), calc.DevelopmentSubtotal)
	assert.Equal(t, int64(10_000_000), calc.DiscountAmount)
	assert.Equal(t, int64(15_000_000), calc.TotalInvestment)
	assert.Equal(t, int64(0), calc.MonthlyTotal)
	assert.Equal(t, int64(15_000_000), calc.DevelopmentPaymentTotal)
	assert.Equal(t, int32(0), calc.MonthlyPaymentRows)

	// Check included line is nil in calc and input
	require.Len(t, calc.DevelopmentGroups, 2)
	require.Len(t, calc.DevelopmentGroups[0].Items, 2)
	assert.Equal(t, "Frontend & Backend Developer", calc.DevelopmentGroups[0].Items[0].Name)
	assert.Equal(t, ptrInt64(15_000_000), calc.DevelopmentGroups[0].Items[0].LineTotal)
	assert.Equal(t, "QA Tester (Included)", calc.DevelopmentGroups[0].Items[1].Name)
	assert.Nil(t, calc.DevelopmentGroups[0].Items[1].LineTotal)
	assert.Nil(t, input.DevelopmentGroups[0].Items[1].QuotedLineTotal)

	// Profile checks for fixed_days
	assert.Equal(t, domain.CustomDevelopmentFixedDays, profile.TableMode)
	assert.False(t, profile.ShowRateColumn)
	assert.Equal(t, "Period (Days)", profile.PeriodHeader)
	assert.True(t, profile.ShowDiscountRow)

	// Render test
	adapter := NewPromptCustomDevelopmentAdapter(nil)
	html, err := adapter.Render(
		QuotationMetadata{QuotationNumber: "Q-ECO-001", Subject: "Company Profile Ecoflow", ClientName: "Budi"},
		PromptCustomDevelopmentSnapshot{Input: input, Calculation: calc, Profile: profile},
		"<p>Syarat dan Ketentuan Legal</p>",
	)
	require.NoError(t, err)
	assert.Contains(t, html, "Company Profile Ecoflow")
	assert.Contains(t, html, "Website 1: Landing Page &amp; Katalog")
	assert.Contains(t, html, "QA Tester (Included)")
	assert.Contains(t, html, "DISCOUNT DEVELOPMENT")
	assert.Contains(t, html, "Rp 15.000.000")
}

func TestCalculatePromptCustomDevelopment_MissingQuoteBlocksTotal(t *testing.T) {
	// Gate check: missing quoted line total (and not confirmed included) blocks price calculation
	t.Run("unknown pricing status blocks total", func(t *testing.T) {
		state := validPromptCustomDevelopmentEcoflowState()
		serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
		// Set first item to unknown pricing status with missing quoted total
		serviceInput.DevelopmentGroups[0].Items[0].PricingStatus = CustomDevelopmentPricingUnknown
		serviceInput.DevelopmentGroups[0].Items[0].QuotedLineTotal.Amount = nil
		state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalInvestment)
		assert.Zero(t, calc.DevelopmentSubtotal)
		assert.True(t, hasCustomDevIssue(issues, "custom_development.quoted_line_total.unavailable"), "%+v", issues)
	})

	t.Run("never compute development total from display rate and period", func(t *testing.T) {
		state := validPromptCustomDevelopmentEcoflowState()
		serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
		// Supply display rate and period label, but no quoted line total and explicit status without amount
		displayRate := int64(15_000_000)
		serviceInput.DevelopmentGroups[0].Items[0].DisplayRate = domain.MoneyInput{
			Amount: &displayRate,
			Source: domain.SourceCatalog,
		}
		serviceInput.DevelopmentGroups[0].Items[0].PeriodLabel = customDevFactString("1 Bulan", domain.SourceMeeting)
		serviceInput.DevelopmentGroups[0].Items[0].PricingStatus = CustomDevelopmentPricingExplicit
		serviceInput.DevelopmentGroups[0].Items[0].QuotedLineTotal.Amount = nil
		state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalInvestment)
		assert.Zero(t, calc.DevelopmentSubtotal)
		assert.True(t, hasCustomDevIssue(issues, "custom_development.quoted_line_total.unavailable"), "%+v", issues)
	})
}

func TestCalculatePromptCustomDevelopment_UntrustedRateOrMoney(t *testing.T) {
	for name, mutate := range map[string]func(*PromptCustomDevelopmentServiceInput){
		"untrusted quoted line total from meeting": func(input *PromptCustomDevelopmentServiceInput) {
			input.DevelopmentGroups[0].Items[0].QuotedLineTotal.Source = domain.SourceMeeting
		},
		"untrusted display rate from meeting": func(input *PromptCustomDevelopmentServiceInput) {
			displayRate := int64(15_000_000)
			input.DevelopmentGroups[0].Items[0].DisplayRate = domain.MoneyInput{
				Amount: &displayRate,
				Source: domain.SourceMeeting,
			}
		},
		"untrusted monthly unit rate from meeting": func(input *PromptCustomDevelopmentServiceInput) {
			input.MonthlyItems = []PromptCustomDevelopmentMonthlyItem{
				{
					Name:     customDevFactString("Cloud", domain.SourceMeeting),
					UnitRate: domain.MoneyInput{Amount: ptrInt64(500_000), Source: domain.SourceMeeting},
					Quantity: customDevFactInt(1, domain.SourceMeeting),
				},
			}
		},
		"untrusted payment nominal from meeting": func(input *PromptCustomDevelopmentServiceInput) {
			input.PaymentSchedule[0].Nominal.Source = domain.SourceMeeting
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptCustomDevelopmentEcoflowState()
			serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
			mutate(&serviceInput)
			state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

			_, calc, _, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
			require.NoError(t, err)
			assert.Zero(t, calc.TotalInvestment)
			assert.NotEmpty(t, blockingCustomDevIssues(issues))
		})
	}
}

func TestCalculatePromptCustomDevelopment_Modes(t *testing.T) {
	modes := []struct {
		mode               domain.CustomDevelopmentTableMode
		expectedRateHeader string
		expectedPeriodHead string
		showRateCol        bool
	}{
		{domain.CustomDevelopmentMonthlyPeriod, "Rate (Monthly)", "Period (Month)", true},
		{domain.CustomDevelopmentDailyPeriod, "Rate (Monthly)", "Period (Day)", true},
		{domain.CustomDevelopmentFixedDays, "", "Period (Days)", false},
	}

	for _, tc := range modes {
		t.Run(string(tc.mode), func(t *testing.T) {
			state := validPromptCustomDevelopmentEcoflowState()
			serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
			serviceInput.DevelopmentTableMode = customDevFactString(string(tc.mode), domain.SourceMeeting)
			state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

			_, _, profile, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
			require.NoError(t, err)
			assert.Empty(t, blockingCustomDevIssues(issues), "%+v", issues)
			assert.Equal(t, tc.mode, profile.TableMode)
			assert.Equal(t, tc.expectedRateHeader, profile.RateHeader)
			assert.Equal(t, tc.expectedPeriodHead, profile.PeriodHeader)
			assert.Equal(t, tc.showRateCol, profile.ShowRateColumn)
		})
	}
}

func TestCalculatePromptCustomDevelopment_MonthlyItemsAndPaymentMismatch(t *testing.T) {
	t.Run("missing monthly payment row for monthly total", func(t *testing.T) {
		state := validPromptCustomDevelopmentEcoflowState()
		serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
		serviceInput.MonthlyItems = []PromptCustomDevelopmentMonthlyItem{
			{
				Name:     customDevFactString("Maintenance", domain.SourceMeeting),
				UnitRate: customDevMoneyInput(1_000_000, domain.SourceCatalog, ""),
				Quantity: customDevFactInt(1, domain.SourceMeeting),
			},
		}
		// Payment schedule does not have monthly payment row
		state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalInvestment)
		assert.True(t, hasCustomDevIssue(issues, "custom_development.payment.missing_monthly"), "%+v", issues)
	})

	t.Run("monthly payment nominal mismatch", func(t *testing.T) {
		state := validPromptCustomDevelopmentEcoflowState()
		serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
		serviceInput.MonthlyItems = []PromptCustomDevelopmentMonthlyItem{
			{
				Name:     customDevFactString("Maintenance", domain.SourceMeeting),
				UnitRate: customDevMoneyInput(1_000_000, domain.SourceCatalog, ""),
				Quantity: customDevFactInt(1, domain.SourceMeeting),
			},
		}
		serviceInput.PaymentSchedule = append(serviceInput.PaymentSchedule, PromptCustomDevelopmentPaymentRow{
			WorkPeriod:       customDevFactString("Bulan 1", domain.SourceMeeting),
			Terms:            customDevFactString("Maintenance", domain.SourceMeeting),
			InvoiceDateLabel: customDevFactString("Awal bulan", domain.SourceMeeting),
			DueDateLabel:     customDevFactString("H+7", domain.SourceMeeting),
			Component:        domain.CustomDevelopmentPaymentMonthly,
			Nominal:          customDevMoneyInput(800_000, domain.SourceCatalog, ""), // 800k != 1M
		})
		state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalInvestment)
		assert.True(t, hasCustomDevIssue(issues, "custom_development.payment.monthly_nominal_mismatch"), "%+v", issues)
	})

	t.Run("project payment nominal mismatch", func(t *testing.T) {
		state := validPromptCustomDevelopmentEcoflowState()
		serviceInput := mustPromptCustomDevelopmentServiceInput(t, state.ServiceInput)
		// Total investment is 15M, make payment total 14M
		serviceInput.PaymentSchedule[0].Nominal = customDevMoneyInput(6_500_000, domain.SourceCatalog, "")
		state.ServiceInput = mustPromptCustomDevelopmentJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptCustomDevelopment(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.TotalInvestment)
		assert.True(t, hasCustomDevIssue(issues, "custom_development.payment.development_mismatch"), "%+v", issues)
	})
}

func TestPromptServiceAdapterRegistry_CustomDevelopment(t *testing.T) {
	registry := NewPromptServiceAdapterRegistry(nil, nil, nil, nil, nil)
	require.NotNil(t, registry)
	adapter := registry.CustomDevelopment()
	require.NotNil(t, adapter)
	assert.Equal(t, domain.CommercialSchemeCustomDevelopment, adapter.Scheme())
}
