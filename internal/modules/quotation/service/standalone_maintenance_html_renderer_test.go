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

func nexRetainerInputForRenderer() domain.StandaloneMaintenanceInput {
	return domain.StandaloneMaintenanceInput{
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
						Function:    "Melihat riwayat medis pasien",
						ScreenFlow:  "Pasien > Data Pasien > Medical History",
						Existing:    "Hanya mencantumkan catatan registrasi awal",
						Improvement: "Mencantumkan seluruh riwayat SOAP (Dokter, Terapis, Perawat)",
						Reference:   "Ref 1.01",
					},
					{
						ItemNumber:  "1.2",
						Priority:    domain.StandalonePriorityP1,
						Function:    "Riwayat foto pasien",
						ScreenFlow:  "Pasien > Foto Pasien",
						Existing:    "Format foto hanya JPG dan PNG",
						Improvement: "Mendukung format JPG, PNG, dan PDF",
						Reference:   "Ref 1.02",
					},
				},
			},
		},
		ShowDiscountRow: true,
		DiscountAmount:  0,
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
}

func liputan6HourlyInputForRenderer() domain.StandaloneMaintenanceInput {
	return domain.StandaloneMaintenanceInput{
		SystemName:    "Portal Berita Liputan6",
		QuotationDate: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		Mode:          domain.StandaloneMaintenanceHourlyTask,
		TaxTreatment:  domain.StandaloneTaxNotStated,
		HourlyCostItems: []domain.MaintenanceHourlyCostItemInput{
			{Name: "Senior Mobile Programmer", DisplayMonthlyRate: 16_050_000, Headcount: 1, HoursHalfUnits: 16, QuotedLineTotal: 730_000},
			{Name: "QA Specialist", DisplayMonthlyRate: 12_500_000, Headcount: 1, HoursHalfUnits: 4, QuotedLineTotal: 140_000},
			{Name: "Support Tech", DisplayMonthlyRate: 8_000_000, Headcount: 1, HoursHalfUnits: 3, QuotedLineTotal: 100_000},
			{Name: "Junior Engineer", DisplayMonthlyRate: 6_000_000, Headcount: 1, HoursHalfUnits: 2, QuotedLineTotal: 50_000},
		},
		HourlyTasks: []domain.MaintenanceHourlyTaskInput{
			{Description: "Perbaikan crash login", Platform: "Android", HoursHalfUnits: 16},
			{Description: "Regression testing", Platform: "QA Test", HoursHalfUnits: 4},
			{Description: "API bug fix", Platform: "Backend", HoursHalfUnits: 3},
			{Description: "Minor UI update", Platform: "iOS", HoursHalfUnits: 2},
		},
		ShowDiscountRow: false,
		DiscountAmount:  0,
		PaymentSchedule: []domain.MaintenancePaymentRowInput{
			{
				WorkPeriod:       "Support Juli 2026",
				Terms:            "Pelunasan",
				InvoiceDateLabel: "31 Juli 2026",
				DueDateLabel:     "14 Agustus 2026",
				Nominal:          1_020_000,
			},
		},
	}
}

// Step 1: Retainer renderer tests
func TestStandaloneMaintenanceHTMLRenderer_Retainer(t *testing.T) {
	input := nexRetainerInputForRenderer()
	calc, err := service.CalculateStandaloneMaintenance(input)
	require.NoError(t, err)

	profile := service.ResolveStandaloneMaintenanceProfile(input)
	renderer := service.NewStandaloneMaintenanceHTMLRenderer()

	metadata := service.QuotationMetadata{
		QuotationNumber:   "Q/2026/07/001",
		Subject:           "Penawaran Maintenance Aplikasi Nex Health",
		ClientName:        "dr. Budi Santoso",
		ClientCompanyName: "PT Nex Digital Health",
		IssueDate:         "7 Juli 2026",
	}

	docInput := service.StandaloneMaintenanceDocumentInput{
		Metadata:    metadata,
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Klausul batasan dan pengecualian standar venturo.</p>",
	}

	html, err := renderer.Render(docInput)
	require.NoError(t, err)

	// Outer document
	assert.Contains(t, html, `<article class="quotation-document">`)
	assert.True(t, strings.HasPrefix(html, `<article class="quotation-document">`))
	assert.True(t, strings.HasSuffix(html, `</article>`))

	// Title
	assert.Contains(t, html, `<h1>PENAWARAN LAYANAN STANDALONE MAINTENANCE & SUPPORT</h1>`)

	// Opening paragraph exact literal
	assert.Contains(t, html, `<section data-block-type="opening"><p>`+service.StandaloneMaintenanceOpeningParagraph+`</p></section>`)

	// Metadata
	assert.Contains(t, html, `data-block-type="metadata"`)
	assert.Contains(t, html, `<strong>Nomor:</strong> <span data-field="quotation_number">Q/2026/07/001</span>`)
	assert.Contains(t, html, `<strong>Perihal:</strong> <span data-field="subject">Penawaran Maintenance Aplikasi Nex Health</span>`)
	assert.Contains(t, html, `<strong>Tanggal:</strong> <span data-field="issue_date_header">7 Juli 2026</span>`)
	assert.Contains(t, html, `<strong>Ditujukan kepada:</strong> <span data-field="client_pic">dr. Budi Santoso</span>`)
	assert.Contains(t, html, `<p data-field="client_company">PT Nex Digital Health</p>`)

	// Section 1: Biaya Maintenance
	assert.Contains(t, html, `<section data-block-type="commercial" data-section-number="1"><h2>1. Biaya Maintenance</h2>`)
	assert.Contains(t, html, `<th>Maintenance Squad</th><th>t(Monthly)</th><th>Employee</th><th>t(Month)</th><th>Total</th>`)

	// Retainer rows
	assert.Contains(t, html, `data-role="retainer_cost_0"`)
	assert.Contains(t, html, `<td>Senior Web Programmer</td><td>Rp 16.050.000</td><td>1</td><td>1 Bulan</td><td data-field="cost_line_total">Rp 16.050.000</td>`)
	assert.Contains(t, html, `data-role="retainer_cost_1"`)
	assert.Contains(t, html, `<td>Senior Quality Assurance</td><td>Rp 14.070.000</td><td>0.5</td><td>1 Bulan</td><td data-field="cost_line_total">Rp 7.035.000</td>`)

	// Commercial totals
	assert.Contains(t, html, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="cost_subtotal"><strong>Rp 23.085.000</strong></td></tr>`)
	assert.Contains(t, html, `<tr><td colspan="4"><strong>DISCOUNT</strong></td><td data-field="discount_amount"><strong>Rp 0</strong></td></tr>`)
	assert.Contains(t, html, `<tr><td colspan="4"><strong>TOTAL INVESTMENT</strong></td><td data-field="total_investment"><strong>Rp 23.085.000</strong></td></tr>`)

	// Section 2: Lingkup Pekerjaan & Timeline (Defect Matrix)
	assert.Contains(t, html, `<section data-block-type="scope" data-section-number="2"><h2>2. Lingkup Pekerjaan &amp; Timeline</h2>`)
	assert.Contains(t, html, `<th># / Priority</th><th>Function</th><th>Screen / Flow</th><th>Existing</th><th>Improvement</th><th>Reference</th>`)

	// Defect group and items
	assert.Contains(t, html, `data-role="defect_group_0"`)
	assert.Contains(t, html, `<td colspan="6"><strong>1. Detail Pasien</strong></td>`)

	assert.Contains(t, html, `data-role="defect_0_0"`)
	assert.Contains(t, html, `<td>1.1<br/>P0</td>`)
	assert.Contains(t, html, `<td>Melihat riwayat medis pasien</td>`)
	assert.Contains(t, html, `<td>Pasien &gt; Data Pasien &gt; Medical History</td>`)
	assert.Contains(t, html, `<td>Hanya mencantumkan catatan registrasi awal</td>`)
	assert.Contains(t, html, `<td>Mencantumkan seluruh riwayat SOAP (Dokter, Terapis, Perawat)</td>`)
	assert.Contains(t, html, `<td>Ref 1.01</td>`)

	assert.Contains(t, html, `data-role="defect_0_1"`)
	assert.Contains(t, html, `<td>1.2<br/>P1</td>`)
	assert.Contains(t, html, `<td>Riwayat foto pasien</td>`)
	assert.Contains(t, html, `<td>Ref 1.02</td>`)

	// Payment block
	assert.Contains(t, html, `<section data-block-type="payment" data-section-number="3"><h2>3. Pembayaran</h2>`)
	assert.Contains(t, html, `<th>Work Period</th><th>Terms</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th>`)
	assert.Contains(t, html, `data-role="payment_0"`)
	assert.Contains(t, html, `<td>13 Juli - 13 Agustus 2026</td><td>Maintenance</td><td>13 Agustus 2026</td><td>27 Agustus 2026</td><td data-field="payment_nominal">Rp 23.085.000</td>`)
	assert.Contains(t, html, `<td data-field="payment_total"><strong>Rp 23.085.000</strong></td>`)

	// Tax text (exclusive PPN)
	assert.Contains(t, html, `<p data-field="tax_clause">Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.</p>`)

	// Legal block
	assert.Contains(t, html, `<section data-block-type="legal" data-section-number="4"><h2>4. Batasan dan Pengecualian</h2>`)
	assert.Contains(t, html, `<p>Klausul batasan dan pengecualian standar venturo.</p>`)

	// Signature block
	assert.Contains(t, html, `<section data-block-type="signature">`)
	assert.Contains(t, html, `<p>Malang, <span data-field="signature_date">7 Juli 2026</span></p>`)
	assert.Contains(t, html, `<table class="signature-table"><tbody><tr>`)
	assert.Contains(t, html, `<strong>Yulianto Frandi</strong>`)
	assert.Contains(t, html, `<strong>dr. Budi Santoso</strong>`)
	assert.Contains(t, html, `PT Nex Digital Health`)
}

// Step 2: Hourly renderer tests
func TestStandaloneMaintenanceHTMLRenderer_Hourly(t *testing.T) {
	input := liputan6HourlyInputForRenderer()
	calc, err := service.CalculateStandaloneMaintenance(input)
	require.NoError(t, err)

	profile := service.ResolveStandaloneMaintenanceProfile(input)
	renderer := service.NewStandaloneMaintenanceHTMLRenderer()

	metadata := service.QuotationMetadata{
		QuotationNumber: "Q/2026/07/002",
		Subject:         "Penawaran Support Liputan6",
		ClientName:      "Ahmad Fauzi",
		IssueDate:       "10 Juli 2026",
	}

	docInput := service.StandaloneMaintenanceDocumentInput{
		Metadata:    metadata,
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal limitations here</p>",
	}

	html, err := renderer.Render(docInput)
	require.NoError(t, err)

	// Section 1: Biaya Development
	assert.Contains(t, html, `<section data-block-type="commercial" data-section-number="1"><h2>1. Biaya Development</h2>`)
	assert.Contains(t, html, `<th>Development</th><th>t(Monthly)</th><th>Employee</th><th>t(Hours)</th><th>Total</th>`)

	// Hourly cost rows
	assert.Contains(t, html, `data-role="hourly_cost_0"`)
	assert.Contains(t, html, `<td>Senior Mobile Programmer</td><td>Rp 16.050.000</td><td>1</td><td>8</td><td data-field="cost_line_total">Rp 730.000</td>`)
	assert.Contains(t, html, `data-role="hourly_cost_1"`)
	assert.Contains(t, html, `<td>QA Specialist</td><td>Rp 12.500.000</td><td>1</td><td>2</td><td data-field="cost_line_total">Rp 140.000</td>`)
	assert.Contains(t, html, `data-role="hourly_cost_2"`)
	assert.Contains(t, html, `<td>Support Tech</td><td>Rp 8.000.000</td><td>1</td><td>1,5</td><td data-field="cost_line_total">Rp 100.000</td>`)
	assert.Contains(t, html, `data-role="hourly_cost_3"`)
	assert.Contains(t, html, `<td>Junior Engineer</td><td>Rp 6.000.000</td><td>1</td><td>1</td><td data-field="cost_line_total">Rp 50.000</td>`)

	// Total investment row exists, discount row MUST NOT exist when show_discount_row=false
	assert.Contains(t, html, `<td data-field="total_investment"><strong>Rp 1.020.000</strong></td>`)
	assert.NotContains(t, html, `data-field="discount_amount"`)
	assert.NotContains(t, html, `DISCOUNT`)

	// Scope section: Description / Platform / Hours
	assert.Contains(t, html, `<section data-block-type="scope" data-section-number="2"><h2>2. Lingkup Pekerjaan &amp; Timeline</h2>`)
	assert.Contains(t, html, `<th>Description</th><th>Platform</th><th>Hours</th>`)
	assert.Contains(t, html, `data-role="hourly_task_0"`)
	assert.Contains(t, html, `<td>Perbaikan crash login</td><td>Android</td><td>8</td>`)
	assert.Contains(t, html, `data-role="hourly_task_1"`)
	assert.Contains(t, html, `<td>Regression testing</td><td>QA Test</td><td>2</td>`)
	assert.Contains(t, html, `data-role="hourly_task_2"`)
	assert.Contains(t, html, `<td>API bug fix</td><td>Backend</td><td>1,5</td>`)
	assert.Contains(t, html, `data-role="hourly_task_3"`)
	assert.Contains(t, html, `<td>Minor UI update</td><td>iOS</td><td>1</td>`)

	// Hours displays: 1, 1,5, 2
	assert.Contains(t, html, `<td>1</td>`)
	assert.Contains(t, html, `<td>1,5</td>`)
	assert.Contains(t, html, `<td>2</td>`)

	// No defect matrix or Gantt markup
	assert.NotContains(t, html, `defect_group_`)
	assert.NotContains(t, html, `defect_0_`)
	assert.NotContains(t, html, `Screen / Flow`)
	assert.NotContains(t, html, `data-block-type="timeline"`)
	assert.NotContains(t, html, `gantt`)
}

// Step 3: Escape and optional-content tests
func TestStandaloneMaintenanceHTMLRenderer_EscapeAndOptionalContent(t *testing.T) {
	t.Run("escapes special characters safely", func(t *testing.T) {
		input := nexRetainerInputForRenderer()
		input.SystemName = "System <script>alert(1)</script>"
		input.RetainerItems[0].Name = "Frontend & Backend Dev <script>alert(2)</script>"
		input.DefectGroups[0].Title = "Security & Pen-test <tag>"
		input.DefectGroups[0].Items[0].Function = "Test <script>alert('XSS')</script> & More"
		input.DefectGroups[0].Items[0].ScreenFlow = "Flow A -> B & C <tag>"
		input.DefectGroups[0].Items[0].Existing = "Bug & Issue <script>"
		input.DefectGroups[0].Items[0].Improvement = "Fixed & Tested <script>"
		input.DefectGroups[0].Items[0].Reference = "Ref & <tag>"

		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)

		profile := service.ResolveStandaloneMaintenanceProfile(input)
		renderer := service.NewStandaloneMaintenanceHTMLRenderer()

		metadata := service.QuotationMetadata{
			QuotationNumber:   "Q-001 <bad>",
			Subject:           "Subject <script>alert(1)</script> & Details",
			ClientName:        "Client & Partner <script>",
			ClientCompanyName: "Company & Co <script>",
			IssueDate:         "7 Juli 2026",
		}

		docInput := service.StandaloneMaintenanceDocumentInput{
			Metadata:    metadata,
			Input:       input,
			Calculation: calc,
			Profile:     profile,
			LegalHTML:   "<p>Trusted legal snapshot</p>",
		}

		html, err := renderer.Render(docInput)
		require.NoError(t, err)

		// No raw scripts
		assert.NotContains(t, html, "<script>")
		assert.NotContains(t, html, "</script>")

		// Escaped text
		assert.Contains(t, html, "&lt;script&gt;alert(1)&lt;/script&gt;")
		assert.Contains(t, html, "Frontend &amp; Backend Dev &lt;script&gt;alert(2)&lt;/script&gt;")
		assert.Contains(t, html, "Security &amp; Pen-test &lt;tag&gt;")
		assert.Contains(t, html, "Test &lt;script&gt;alert(&#39;XSS&#39;)&lt;/script&gt; &amp; More")
		assert.Contains(t, html, "Flow A -&gt; B &amp; C &lt;tag&gt;")
		assert.Contains(t, html, "Bug &amp; Issue &lt;script&gt;")
		assert.Contains(t, html, "Fixed &amp; Tested &lt;script&gt;")
		assert.Contains(t, html, "Ref &amp; &lt;tag&gt;")
		assert.Contains(t, html, "Company &amp; Co &lt;script&gt;")
	})

	t.Run("blank reference renders empty cell", func(t *testing.T) {
		input := nexRetainerInputForRenderer()
		input.DefectGroups[0].Items[0].Reference = ""
		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)

		profile := service.ResolveStandaloneMaintenanceProfile(input)
		renderer := service.NewStandaloneMaintenanceHTMLRenderer()

		docInput := service.StandaloneMaintenanceDocumentInput{
			Metadata:    service.QuotationMetadata{QuotationNumber: "Q-001", Subject: "S", ClientName: "C", IssueDate: "7 Juli 2026"},
			Input:       input,
			Calculation: calc,
			Profile:     profile,
			LegalHTML:   "<p>Legal</p>",
		}

		html, err := renderer.Render(docInput)
		require.NoError(t, err)

		assert.Contains(t, html, `<td></td>`)
	})

	t.Run("not_stated tax treatment omits tax paragraph", func(t *testing.T) {
		input := nexRetainerInputForRenderer()
		input.TaxTreatment = domain.StandaloneTaxNotStated
		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)

		profile := service.ResolveStandaloneMaintenanceProfile(input)
		renderer := service.NewStandaloneMaintenanceHTMLRenderer()

		docInput := service.StandaloneMaintenanceDocumentInput{
			Metadata:    service.QuotationMetadata{QuotationNumber: "Q-001", Subject: "S", ClientName: "C", IssueDate: "7 Juli 2026"},
			Input:       input,
			Calculation: calc,
			Profile:     profile,
			LegalHTML:   "<p>Legal</p>",
		}

		html, err := renderer.Render(docInput)
		require.NoError(t, err)

		assert.NotContains(t, html, `data-field="tax_clause"`)
		assert.NotContains(t, html, `PPN`)
	})

	t.Run("empty rights statements omits rights section", func(t *testing.T) {
		input := nexRetainerInputForRenderer()
		input.RightsStatements = nil
		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)

		profile := service.ResolveStandaloneMaintenanceProfile(input)
		renderer := service.NewStandaloneMaintenanceHTMLRenderer()

		docInput := service.StandaloneMaintenanceDocumentInput{
			Metadata:    service.QuotationMetadata{QuotationNumber: "Q-001", Subject: "S", ClientName: "C", IssueDate: "7 Juli 2026"},
			Input:       input,
			Calculation: calc,
			Profile:     profile,
			LegalHTML:   "<p>Legal</p>",
		}

		html, err := renderer.Render(docInput)
		require.NoError(t, err)

		assert.NotContains(t, html, `data-block-type="rights"`)
	})

	t.Run("non-empty rights statements renders rights section", func(t *testing.T) {
		input := nexRetainerInputForRenderer()
		input.RightsStatements = []string{
			"Venturo berhak melakukan audit teknis berkala.",
			"Klien berhak menerima laporan bulanan.",
		}
		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)

		profile := service.ResolveStandaloneMaintenanceProfile(input)
		renderer := service.NewStandaloneMaintenanceHTMLRenderer()

		docInput := service.StandaloneMaintenanceDocumentInput{
			Metadata:    service.QuotationMetadata{QuotationNumber: "Q-001", Subject: "S", ClientName: "C", IssueDate: "7 Juli 2026"},
			Input:       input,
			Calculation: calc,
			Profile:     profile,
			LegalHTML:   "<p>Legal</p>",
		}

		html, err := renderer.Render(docInput)
		require.NoError(t, err)

		assert.Contains(t, html, `<section data-block-type="rights" data-section-number="4"><h2>4. Hak Para Pihak</h2>`)
		assert.Contains(t, html, `<p>Venturo berhak melakukan audit teknis berkala.</p>`)
		assert.Contains(t, html, `<p>Klien berhak menerima laporan bulanan.</p>`)
		assert.Contains(t, html, `data-section-number="5"><h2>5. Batasan dan Pengecualian</h2>`)
	})
}
