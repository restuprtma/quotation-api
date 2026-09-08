package service_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func renderValidRetainerDoc(t *testing.T) (string, domain.StandaloneMaintenanceInput, domain.StandaloneMaintenanceCalculation, domain.ResolvedStandaloneMaintenanceProfile, string) {
	t.Helper()
	input := domain.StandaloneMaintenanceInput{
		SystemName:    "Aplikasi Nex Health",
		QuotationDate: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		Mode:          domain.StandaloneMaintenanceRetainer,
		TaxTreatment:  domain.StandaloneTaxExclusive,
		RetainerItems: []domain.MaintenanceRetainerItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 16_050_000, FTEQuarters: 4, PeriodMonths: 1},
			{Name: "Senior Quality Assurance", MonthlyRate: 14_070_000, FTEQuarters: 2, PeriodMonths: 1},
		},
		DefectGroups: []domain.MaintenanceDefectGroupInput{
			{
				Title: "1. Detail Pasien",
				Items: []domain.MaintenanceDefectItemInput{
					{
						ItemNumber:  "1.1",
						Priority:    domain.StandalonePriorityP0,
						Function:    "Melihat riwayat medis",
						ScreenFlow:  "Pasien > History",
						Existing:    "Registrasi awal saja",
						Improvement: "Semua riwayat SOAP",
						Reference:   "Ref 1.01",
					},
				},
			},
		},
		ShowDiscountRow:  true,
		DiscountAmount:   0,
		RightsStatements: []string{"Hak cipta bersama.", "Laporan bulanan wajib disampaikan."},
		PaymentSchedule: []domain.MaintenancePaymentRowInput{
			{
				WorkPeriod:       "13 Juli - 13 Agustus 2026",
				Terms:            "Maintenance",
				InvoiceDateLabel: "13 Agustus 2026",
				DueDateLabel:     "27 Agustus 2026",
				Nominal:          23_085_000,
			},
		},
	}

	calc, err := service.CalculateStandaloneMaintenance(input)
	require.NoError(t, err)
	profile := service.ResolveStandaloneMaintenanceProfile(input)
	legalHTML := "<p>Klausul batasan dan pengecualian legal.</p>"

	renderer := service.NewStandaloneMaintenanceHTMLRenderer()
	html, err := renderer.Render(service.StandaloneMaintenanceDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber:   "Q-001",
			Subject:           "Penawaran Maintenance Nex",
			ClientName:        "dr. Budi Santoso",
			ClientCompanyName: "PT Nex Digital Health",
			IssueDate:         "7 Juli 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   legalHTML,
	})
	require.NoError(t, err)
	return html, input, calc, profile, legalHTML
}

func renderValidHourlyDoc(t *testing.T) (string, domain.StandaloneMaintenanceInput, domain.StandaloneMaintenanceCalculation, domain.ResolvedStandaloneMaintenanceProfile, string) {
	t.Helper()
	input := domain.StandaloneMaintenanceInput{
		SystemName:    "Portal Berita Liputan6",
		QuotationDate: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		Mode:          domain.StandaloneMaintenanceHourlyTask,
		TaxTreatment:  domain.StandaloneTaxNotStated,
		HourlyCostItems: []domain.MaintenanceHourlyCostItemInput{
			{Name: "Senior Mobile Programmer", DisplayMonthlyRate: 16_050_000, Headcount: 1, HoursHalfUnits: 16, QuotedLineTotal: 730_000},
			{Name: "Senior QA", DisplayMonthlyRate: 12_500_000, Headcount: 1, HoursHalfUnits: 4, QuotedLineTotal: 140_000},
		},
		HourlyTasks: []domain.MaintenanceHourlyTaskInput{
			{Description: "Fix login crash", Platform: "Android", HoursHalfUnits: 16},
			{Description: "Regression test", Platform: "QA Test", HoursHalfUnits: 4},
		},
		ShowDiscountRow: false,
		DiscountAmount:  0,
		PaymentSchedule: []domain.MaintenancePaymentRowInput{
			{
				WorkPeriod:       "Support Juli 2026",
				Terms:            "Pelunasan",
				InvoiceDateLabel: "31 Juli 2026",
				DueDateLabel:     "14 Agustus 2026",
				Nominal:          870_000,
			},
		},
	}

	calc, err := service.CalculateStandaloneMaintenance(input)
	require.NoError(t, err)
	profile := service.ResolveStandaloneMaintenanceProfile(input)
	legalHTML := "<p>Batasan legal liputan6.</p>"

	renderer := service.NewStandaloneMaintenanceHTMLRenderer()
	html, err := renderer.Render(service.StandaloneMaintenanceDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber: "Q-002",
			Subject:         "Support Liputan6",
			ClientName:      "Ahmad Fauzi",
			IssueDate:       "10 Juli 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   legalHTML,
	})
	require.NoError(t, err)
	return html, input, calc, profile, legalHTML
}

func TestStandaloneMaintenanceDOMValidator_CanonicalClean(t *testing.T) {
	t.Run("retainer canonical passes integrity and modified checks", func(t *testing.T) {
		doc, input, calc, profile, legal := renderValidRetainerDoc(t)

		err := service.ValidateStandaloneMaintenanceIntegrity(doc, input, calc, profile, legal)
		require.NoError(t, err)

		commMod, err := service.CheckStandaloneMaintenanceCommercialModified(doc, input, calc)
		require.NoError(t, err)
		assert.False(t, commMod)

		rightsMod, err := service.CheckStandaloneMaintenanceRightsModified(doc, input)
		require.NoError(t, err)
		assert.False(t, rightsMod)
	})

	t.Run("hourly canonical passes integrity and modified checks", func(t *testing.T) {
		doc, input, calc, profile, legal := renderValidHourlyDoc(t)

		err := service.ValidateStandaloneMaintenanceIntegrity(doc, input, calc, profile, legal)
		require.NoError(t, err)

		commMod, err := service.CheckStandaloneMaintenanceCommercialModified(doc, input, calc)
		require.NoError(t, err)
		assert.False(t, commMod)

		rightsMod, err := service.CheckStandaloneMaintenanceRightsModified(doc, input)
		require.NoError(t, err)
		assert.False(t, rightsMod)
	})

	t.Run("scope editing in TipTap does not trigger commercial modification", func(t *testing.T) {
		doc, input, calc, _, _ := renderValidRetainerDoc(t)

		// User edits defect function and improvement text in editor
		edited := strings.Replace(doc, "Melihat riwayat medis", "Melihat seluruh riwayat medis terenkripsi", 1)
		edited = strings.Replace(edited, "Semua riwayat SOAP", "Semua riwayat SOAP terverifikasi dokter spesialis", 1)

		commMod, err := service.CheckStandaloneMaintenanceCommercialModified(edited, input, calc)
		require.NoError(t, err)
		assert.False(t, commMod, "editing scope in TipTap must NOT trigger commercial modification")
	})
}

// Step 6: DOM tamper tests
func TestStandaloneMaintenanceDOMValidator_TamperDetection(t *testing.T) {
	doc, input, calc, profile, legal := renderValidRetainerDoc(t)
	hourlyDoc, hourlyInput, hourlyCalc, hourlyProfile, hourlyLegal := renderValidHourlyDoc(t)

	t.Run("1. tampered cost rate", func(t *testing.T) {
		tampered := strings.Replace(doc, "Rp 16.050.000", "Rp 17.000.000", 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("2. tampered headcount/FTE", func(t *testing.T) {
		// Retainer FTE tamper
		tampered := strings.Replace(doc, "<td>1</td>", "<td>0.5</td>", 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		// Hourly headcount tamper
		tamperedHourly := strings.Replace(hourlyDoc, "<td>1</td>", "<td>2</td>", 1)
		modHourly, err := service.CheckStandaloneMaintenanceCommercialModified(tamperedHourly, hourlyInput, hourlyCalc)
		require.NoError(t, err)
		assert.True(t, modHourly)
	})

	t.Run("3. tampered hours", func(t *testing.T) {
		tampered := strings.Replace(hourlyDoc, "<td>8</td>", "<td>10</td>", 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, hourlyInput, hourlyCalc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, hourlyInput, hourlyCalc, hourlyProfile, hourlyLegal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("4. tampered line total", func(t *testing.T) {
		tampered := strings.Replace(doc, `<td data-field="cost_line_total">Rp 16.050.000</td>`, `<td data-field="cost_line_total">Rp 18.000.000</td>`, 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("5. tampered subtotal", func(t *testing.T) {
		tampered := strings.Replace(doc, `<td data-field="cost_subtotal"><strong>Rp 23.085.000</strong></td>`, `<td data-field="cost_subtotal"><strong>Rp 25.000.000</strong></td>`, 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("6. tampered discount", func(t *testing.T) {
		tampered := strings.Replace(doc, `<td data-field="discount_amount"><strong>Rp 0</strong></td>`, `<td data-field="discount_amount"><strong>Rp 1.000.000</strong></td>`, 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("7. tampered grand total", func(t *testing.T) {
		tampered := strings.Replace(doc, `<td data-field="total_investment"><strong>Rp 23.085.000</strong></td>`, `<td data-field="total_investment"><strong>Rp 20.000.000</strong></td>`, 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("8. tampered payment nominal", func(t *testing.T) {
		tampered := strings.Replace(doc, `<td data-field="payment_nominal">Rp 23.085.000</td>`, `<td data-field="payment_nominal">Rp 20.000.000</td>`, 1)
		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("9. tampered payment row count", func(t *testing.T) {
		extraRow := `<tr data-role="payment_1"><td>Extra</td><td>Extra</td><td>14 Ags</td><td>28 Ags</td><td data-field="payment_nominal">Rp 1.000.000</td></tr>`
		tampered := strings.Replace(doc, `<tr data-role="payment_0">`, extraRow+`<tr data-role="payment_0">`, 1)

		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("10. duplicate commercial block", func(t *testing.T) {
		commStart := strings.Index(doc, `<section data-block-type="commercial"`)
		commEnd := strings.Index(doc[commStart:], `</section>`) + commStart + len("</section>")
		commBlock := doc[commStart:commEnd]
		tampered := doc[:commEnd] + commBlock + doc[commEnd:]

		_, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("11. delete payment block", func(t *testing.T) {
		payStart := strings.Index(doc, `<section data-block-type="payment"`)
		payEnd := strings.Index(doc[payStart:], `</section>`) + payStart + len("</section>")
		tampered := doc[:payStart] + doc[payEnd:]

		mod, err := service.CheckStandaloneMaintenanceCommercialModified(tampered, input, calc)
		require.NoError(t, err)
		assert.True(t, mod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
		assert.Contains(t, err.Error(), "payment block is missing")
	})

	t.Run("12. inject inactive mode row", func(t *testing.T) {
		// In retainer doc, inject hourly row
		injectedHourly := strings.Replace(doc, `</tbody></table>`, `<tr data-role="hourly_cost_0"><td>Injected</td><td>Rp 1.000.000</td><td>1</td><td>1</td><td data-field="cost_line_total">Rp 1.000.000</td></tr></tbody></table>`, 1)
		err := service.ValidateStandaloneMaintenanceIntegrity(injectedHourly, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)

		// In hourly doc, inject retainer row
		injectedRetainer := strings.Replace(hourlyDoc, `</tbody></table>`, `<tr data-role="retainer_cost_0"><td>Injected</td><td>Rp 1.000.000</td><td>1</td><td>1 Bulan</td><td data-field="cost_line_total">Rp 1.000.000</td></tr></tbody></table>`, 1)
		err = service.ValidateStandaloneMaintenanceIntegrity(injectedRetainer, hourlyInput, hourlyCalc, hourlyProfile, hourlyLegal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	})

	t.Run("13. modify rights", func(t *testing.T) {
		tampered := strings.Replace(doc, "Hak cipta bersama.", "Hak cipta eksklusif milik Venturo sepenuhnya.", 1)

		rightsMod, err := service.CheckStandaloneMaintenanceRightsModified(tampered, input)
		require.NoError(t, err)
		assert.True(t, rightsMod)

		err = service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
		assert.Contains(t, err.Error(), "rights content was modified")
	})

	t.Run("14. delete legal block", func(t *testing.T) {
		legalStart := strings.Index(doc, `<section data-block-type="legal"`)
		legalEnd := strings.Index(doc[legalStart:], `</section>`) + legalStart + len("</section>")
		tampered := doc[:legalStart] + doc[legalEnd:]

		err := service.ValidateStandaloneMaintenanceIntegrity(tampered, input, calc, profile, legal)
		require.Error(t, err)
		assert.ErrorIs(t, err, service.ErrUnsafeHTML)
		assert.Contains(t, err.Error(), "legal block is missing")
	})
}
