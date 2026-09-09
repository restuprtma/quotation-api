package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func ptrInt64Test(v int64) *int64 {
	return &v
}

func sampleCustomDevelopmentInput() domain.CustomDevelopmentInput {
	return domain.CustomDevelopmentInput{
		SystemName:           "Sistem Kopi Studio24",
		QuotationDate:        time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		DevelopmentTableMode: domain.CustomDevelopmentMonthlyPeriod,
		TaxTreatment:         domain.CustomDevelopmentTaxExclusive,
		DevelopmentGroups: []domain.CustomDevelopmentCostGroupInput{
			{
				Title: "Fase 1: Backend & Core Engine",
				Items: []domain.CustomDevelopmentCostItemInput{
					{
						Name:            "Senior Backend Developer",
						DisplayRate:     ptrInt64Test(15_000_000),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64Test(15_000_000),
					},
					{
						Name:            "Frontend Developer",
						DisplayRate:     ptrInt64Test(5_062_500),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64Test(5_062_500),
					},
				},
			},
		},
		DiscountLabel:   "DISCOUNT",
		DiscountAmount:  2_062_500,
		ShowDiscountRow: true,
		MonthlyItems: []domain.CustomDevelopmentMonthlyItemInput{
			{
				Name:          "Venturo Cloud Mini",
				Specification: "Cloud Server Standard",
				UnitRate:      800_000,
				Quantity:      1,
			},
			{
				Name:          "Support",
				Specification: "4 Jam Maintenance Support",
				UnitRate:      93_000,
				Quantity:      4,
			},
			{
				Name:          "WhatsApp Connect",
				Specification: "Integrasi Notifikasi WA",
				UnitRate:      100_000,
				Quantity:      1,
			},
		},
		MonthlyNotes: []string{
			"Biaya API Gemini ditandai out-of-pocket.",
		},
		ModuleGroups: []domain.CustomDevelopmentModuleGroupInput{
			{
				Name: "Modul Transaksi & Kasir",
				Features: []string{
					"Pencatatan order dan invoice",
					"Integrasi payment gateway",
				},
			},
		},
		TimelineRows: []domain.CustomDevelopmentTimelineRowInput{
			{
				Description: "Fase 1: Pengembangan dan Testing",
				Timeline:    "4 Minggu",
			},
		},
		PaymentSchedule: []domain.CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Down Payment 50%",
				Terms:            "Termin 1",
				InvoiceDateLabel: "Saat tanda tangan kontrak",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          9_000_000,
			},
			{
				WorkPeriod:       "Pelunasan 50%",
				Terms:            "Termin 2",
				InvoiceDateLabel: "Setelah serah terima",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          9_000_000,
			},
			{
				WorkPeriod:       "Bulan 1",
				Terms:            "Layanan Bulanan Cloud & Support",
				InvoiceDateLabel: "Awal bulan berjalan",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentMonthly,
				Nominal:          1_272_000,
			},
		},
		RightsStatements: []string{
			"Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan.",
		},
		LimitationCodes: []string{
			"out_of_pocket",
			"overtime",
			"third_party",
			"scope_change",
		},
		AdditionalLimitations: []string{
			"Koneksi internet dan perangkat kasir disediakan oleh klien.",
		},
	}
}

func sampleDocumentInput() CustomDevelopmentDocumentInput {
	input := sampleCustomDevelopmentInput()
	calc, err := CalculateCustomDevelopment(input)
	if err != nil {
		panic(err)
	}
	profile, err := ResolveCustomDevelopmentProfile(input)
	if err != nil {
		panic(err)
	}
	return CustomDevelopmentDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber:   "QTN/2026/07/0001",
			Subject:           "Penawaran Sistem Kopi Studio24",
			ClientName:        "Bpk. Hendra",
			ClientCompanyName: "PT Kopi Studio Nusantara",
			IssueDate:         "15 Juli 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   `<ol><li>Ketentuan umum dan kerahasiaan data.</li></ol>`,
	}
}

func TestCustomDevelopmentHTMLRenderer_ExactSectionMarkers(t *testing.T) {
	docIn := sampleDocumentInput()
	renderer := NewCustomDevelopmentRenderer()

	htmlOut, err := renderer.Render(docIn)
	require.NoError(t, err)

	// Assert the 9 required section markers
	assert.Contains(t, htmlOut, `<section data-quotation-section="commercial-development"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="commercial-monthly"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="scope"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="timeline"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="payment"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="rights"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="limitations"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="legal"`)
	assert.Contains(t, htmlOut, `<section data-quotation-section="signature"`)

	// Assert root element
	assert.True(t, strings.HasPrefix(htmlOut, `<article class="quotation-document">`))
	assert.True(t, strings.HasSuffix(htmlOut, `</article>`))

	// Parse DOM and verify structure
	root, err := html.Parse(strings.NewReader(htmlOut))
	require.NoError(t, err)
	nodes := collectNodes(root)

	requiredSections := []string{
		"commercial-development",
		"commercial-monthly",
		"scope",
		"timeline",
		"payment",
		"rights",
		"limitations",
		"legal",
		"signature",
	}
	for _, sec := range requiredSections {
		node, err := findExactlyOne(nodes, "data-quotation-section", sec)
		require.NoError(t, err, "failed finding section %q", sec)
		require.NotNil(t, node, "section %q must exist", sec)
	}

	// Verify opening paragraph
	assert.Contains(t, htmlOut, CustomDevelopmentOpeningParagraph)
	assert.Contains(t, htmlOut, "lebih dari 100 orang")
}

func TestCustomDevelopmentHTMLRenderer_NumericFields(t *testing.T) {
	docIn := sampleDocumentInput()
	renderer := NewCustomDevelopmentRenderer()

	htmlOut, err := renderer.Render(docIn)
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(htmlOut))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Check required numeric fields
	expectedFields := []struct {
		field string
		want  string
	}{
		{"development_line_0_0", "Rp 15.000.000"},
		{"development_line_0_1", "Rp 5.062.500"},
		{"development_subtotal", "Rp 20.062.500"},
		{"discount_amount", "Rp 2.062.500"},
		{"total_investment", "Rp 18.000.000"},
		{"monthly_line_0", "Rp 800.000"},
		{"monthly_line_1", "Rp 372.000"},
		{"monthly_line_2", "Rp 100.000"},
		{"monthly_total", "Rp 1.272.000"},
		{"payment_nominal_0", "Rp 9.000.000"},
		{"payment_nominal_1", "Rp 9.000.000"},
		{"payment_nominal_2", "Rp 1.272.000"},
	}

	for _, ef := range expectedFields {
		cell, err := findExactlyOne(nodes, "data-quotation-field", ef.field)
		require.NoError(t, err, "field %q lookup error", ef.field)
		require.NotNil(t, cell, "field %q must exist", ef.field)
		assert.Equal(t, ef.want, nodeText(cell), "field %q value mismatch", ef.field)
	}

	// Verify monthly notes marker
	notesList, err := findExactlyOne(nodes, "data-quotation-component", "monthly-notes")
	require.NoError(t, err)
	require.NotNil(t, notesList)
	assert.Contains(t, nodeText(notesList), "Biaya API Gemini ditandai out-of-pocket.")
}

func TestCustomDevelopmentHTMLRenderer_Escaping(t *testing.T) {
	docIn := sampleDocumentInput()
	xssPayload := `<script>alert('xss')</script>`

	// Inject script tag into plain-text fields
	docIn.Metadata.QuotationNumber = xssPayload
	docIn.Metadata.Subject = xssPayload
	docIn.Metadata.ClientName = xssPayload
	docIn.Metadata.ClientCompanyName = xssPayload
	docIn.Input.SystemName = xssPayload
	docIn.Input.DiscountLabel = xssPayload
	docIn.Input.DevelopmentGroups[0].Title = xssPayload
	docIn.Input.DevelopmentGroups[0].Items[0].Name = xssPayload
	docIn.Input.DevelopmentGroups[0].Items[0].PeriodLabel = xssPayload
	docIn.Input.MonthlyItems[0].Name = xssPayload
	docIn.Input.MonthlyItems[0].Specification = xssPayload
	docIn.Input.MonthlyNotes[0] = xssPayload
	docIn.Input.ModuleGroups[0].Name = xssPayload
	docIn.Input.ModuleGroups[0].Features[0] = xssPayload
	docIn.Input.TimelineRows[0].Description = xssPayload
	docIn.Input.TimelineRows[0].Timeline = xssPayload
	docIn.Input.PaymentSchedule[0].WorkPeriod = xssPayload
	docIn.Input.PaymentSchedule[0].Terms = xssPayload
	docIn.Input.PaymentSchedule[0].InvoiceDateLabel = xssPayload
	docIn.Input.PaymentSchedule[0].DueDateLabel = xssPayload
	docIn.Input.RightsStatements[0] = xssPayload
	docIn.Input.AdditionalLimitations[0] = xssPayload

	renderer := NewCustomDevelopmentRenderer()
	htmlOut, err := renderer.Render(docIn)
	require.NoError(t, err)

	// The raw tag <script> must NEVER appear in the HTML
	assert.NotContains(t, htmlOut, "<script>")
	assert.NotContains(t, htmlOut, "</script>")
	assert.Contains(t, htmlOut, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;")

	// Parse DOM and verify no <script> element exists
	root, err := html.Parse(strings.NewReader(htmlOut))
	require.NoError(t, err)
	nodes := collectNodes(root)
	for _, n := range nodes {
		if n.Type == html.ElementNode {
			assert.NotEqual(t, "script", n.Data, "script element node must not exist in DOM")
		}
	}
}

func TestCustomDevelopmentHTMLRenderer_IncludedLineRendersEmptyTotal(t *testing.T) {
	// Ecoflow case: QA Tester has quoted_line_total == nil
	input := sampleCustomDevelopmentInput()
	input.DevelopmentTableMode = domain.CustomDevelopmentFixedDays
	input.DevelopmentGroups = []domain.CustomDevelopmentCostGroupInput{
		{
			Title: "Website 1: Landing Page",
			Items: []domain.CustomDevelopmentCostItemInput{
				{
					Name:            "Developer",
					DisplayRate:     nil,
					Headcount:       1,
					PeriodLabel:     "15 Hari",
					QuotedLineTotal: ptrInt64Test(15_000_000),
				},
				{
					Name:            "QA Tester (Included)",
					DisplayRate:     nil,
					Headcount:       1,
					PeriodLabel:     "15 Hari",
					QuotedLineTotal: nil, // Included line
				},
			},
		},
	}
	input.MonthlyItems = nil
	input.MonthlyNotes = nil
	input.RightsStatements = nil
	input.PaymentSchedule = []domain.CustomDevelopmentPaymentRowInput{
		{
			WorkPeriod:       "Termin 1",
			Terms:            "DP 100%",
			InvoiceDateLabel: "Kontrak",
			DueDateLabel:     "H+7",
			Component:        domain.CustomDevelopmentPaymentProject,
			Nominal:          15_000_000,
		},
	}
	input.DiscountAmount = 0
	input.ShowDiscountRow = false

	calc, err := CalculateCustomDevelopment(input)
	require.NoError(t, err)
	profile, err := ResolveCustomDevelopmentProfile(input)
	require.NoError(t, err)

	docIn := CustomDevelopmentDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber: "QTN/2026/07/0003",
			Subject:         "Company Profile Ecoflow",
			ClientName:      "Bpk. Eko",
			IssueDate:       "25 Juli 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   `<p>Legal clauses.</p>`,
	}

	renderer := NewCustomDevelopmentRenderer()
	htmlOut, err := renderer.Render(docIn)
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(htmlOut))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Row 0 item 0 has line total Rp 15.000.000
	cell0, err := findExactlyOne(nodes, "data-quotation-field", "development_line_0_0")
	require.NoError(t, err)
	assert.Equal(t, "Rp 15.000.000", nodeText(cell0))

	// Row 0 item 1 (QA Tester Included) has empty line total
	cell1, err := findExactlyOne(nodes, "data-quotation-field", "development_line_0_1")
	require.NoError(t, err)
	assert.Empty(t, nodeText(cell1))

	// Table headers in fixed_days mode must NOT have Rate column
	assert.NotContains(t, htmlOut, "Rate (Monthly)")
	assert.Contains(t, htmlOut, "Period (Days)")
}

func TestCustomDevelopmentHTMLRenderer_ConditionalSections(t *testing.T) {
	t.Run("without monthly and without rights", func(t *testing.T) {
		input := sampleCustomDevelopmentInput()
		input.MonthlyItems = nil
		input.MonthlyNotes = nil
		input.RightsStatements = nil
		input.PaymentSchedule = []domain.CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Termin 1",
				Terms:            "Pelunasan",
				InvoiceDateLabel: "Kontrak",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          18_000_000,
			},
		}

		calc, err := CalculateCustomDevelopment(input)
		require.NoError(t, err)
		profile, err := ResolveCustomDevelopmentProfile(input)
		require.NoError(t, err)

		docIn := CustomDevelopmentDocumentInput{
			Metadata: QuotationMetadata{
				QuotationNumber: "QTN/001",
				Subject:         "Test No Monthly No Rights",
				ClientName:      "Klien",
			},
			Input:       input,
			Calculation: calc,
			Profile:     profile,
			LegalHTML:   `<p>Syarat.</p>`,
		}

		renderer := NewCustomDevelopmentRenderer()
		htmlOut, err := renderer.Render(docIn)
		require.NoError(t, err)

		assert.NotContains(t, htmlOut, `data-quotation-section="commercial-monthly"`)
		assert.NotContains(t, htmlOut, `data-quotation-section="rights"`)

		// Verify sequential section numbering
		root, err := html.Parse(strings.NewReader(htmlOut))
		require.NoError(t, err)
		nodes := collectNodes(root)

		devSec, err := findExactlyOne(nodes, "data-quotation-section", "commercial-development")
		require.NoError(t, err)
		assert.Equal(t, "1", attributeValue(devSec, "data-quotation-section-number"))

		scopeSec, err := findExactlyOne(nodes, "data-quotation-section", "scope")
		require.NoError(t, err)
		assert.Equal(t, "2", attributeValue(scopeSec, "data-quotation-section-number"))

		paySec, err := findExactlyOne(nodes, "data-quotation-section", "payment")
		require.NoError(t, err)
		assert.Equal(t, "3", attributeValue(paySec, "data-quotation-section-number"))

		limSec, err := findExactlyOne(nodes, "data-quotation-section", "limitations")
		require.NoError(t, err)
		assert.Equal(t, "4", attributeValue(limSec, "data-quotation-section-number"))
	})
}
