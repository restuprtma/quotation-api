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

func ponpesBenchmarkWhiteLabelInput() domain.WhiteLabelInput {
	return domain.WhiteLabelInput{
		ProductName:   "POS Mini Market",
		QuotationDate: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		TaxTreatment:  domain.WhiteLabelTaxExclusive,
		LicenseItems: []domain.WhiteLabelLicenseItemInput{
			{Name: "POS Mini Market", UnitRate: 35_000_000, Quantity: 1},
		},
		MaintenanceItems: []domain.WhiteLabelMaintenanceItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 16_170_000, FTEQuarters: 2, PeriodMonths: 3},
		},
		HostingItems: []domain.WhiteLabelHostingItemInput{
			{Name: "Shared Hosting Small", MonthlyRate: 350_000, Quantity: 1, PeriodMonths: 3},
		},
		ModuleGroups: []domain.WhiteLabelModuleGroupInput{
			{Name: "Master Data", Features: []string{"Data Produk", "Data Pelanggan", "Data Supplier"}},
			{Name: "Inventory", Features: []string{"Stok Masuk", "Stok Keluar", "Opname Stok"}},
			{Name: "Pembelian", Features: []string{"PO Supplier", "Penerimaan Barang"}},
			{Name: "Kasir POS", Features: []string{"Transaksi Kasir", "Cetak Struk"}},
			{Name: "Kas & Bank", Features: []string{"Arus Kas", "Rekonsiliasi Bank"}},
			{Name: "Laporan", Features: []string{"Laporan Penjualan", "Laporan Laba Rugi"}},
		},
		TimelineRows: []domain.WhiteLabelTimelineRowInput{
			{Description: "Instalasi dan Konfigurasi Sistem", Timeline: "Bulan 1"},
			{Description: "Pelatihan Pengguna dan Go-Live", Timeline: "Bulan 2"},
		},
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 2", Terms: "Bulanan", InvoiceDateLabel: "Bulan 2", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 3", Terms: "Bulanan", InvoiceDateLabel: "Bulan 3", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
		},
		RightsStatements: nil,
		LimitationCodes:  []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func extractH2Headings(nodes []*html.Node) []string {
	var headings []string
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "h2" {
			headings = append(headings, strings.TrimSpace(nodeText(n)))
		}
	}
	return headings
}

func extractH3Headings(nodes []*html.Node) []string {
	var headings []string
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "h3" {
			headings = append(headings, strings.TrimSpace(nodeText(n)))
		}
	}
	return headings
}

func TestWhiteLabelHTMLRenderer_PonpesBenchmark(t *testing.T) {
	input := ponpesBenchmarkWhiteLabelInput()
	calc, err := CalculateWhiteLabel(input)
	require.NoError(t, err)

	profile, err := ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/09/0001",
		Subject:           "Penawaran White Label POS Mini Market",
		ClientName:        "Gus H. Ahmad",
		ClientCompanyName: "Ponpes Al Hidayat 5",
		IssueDate:         "7 September 2026",
	}

	renderer := NewWhiteLabelHTMLRenderer()
	docHTML, err := renderer.Render(WhiteLabelDocumentInput{
		Metadata:    metadata,
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<ol><li>Ketentuan operasional dan batasan layanan.</li></ol>",
	})
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// 1. Judul PENAWARAN LAYANAN WHITE LABEL PRODUCT
	headerH1, err := findExactlyOne(nodes, "data-block-type", "metadata")
	require.NoError(t, err)
	require.NotNil(t, headerH1)
	assert.Contains(t, nodeText(headerH1), "PENAWARAN LAYANAN WHITE LABEL PRODUCT")

	// 2. Lima heading Ponpes dengan nomor berurutan
	h2Headings := extractH2Headings(nodes)
	expectedH2 := []string{
		"1. Biaya White Label",
		"2. Biaya Maintenance & Hosting",
		"3. Lingkup Pekerjaan & Timeline",
		"4. Pembayaran",
		"5. Batasan dan Pengecualian",
	}
	assert.Equal(t, expectedH2, h2Headings)

	// Section numbers on sections
	sectionNumbers := extractSectionNumbers(nodes)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, sectionNumbers)

	// 3. License total Rp35.000.000 in Section 1
	commSection, err := findExactlyOne(nodes, "data-block-type", "commercial")
	require.NoError(t, err)
	require.NotNil(t, commSection)
	commNodes := collectNodes(commSection)
	licenseLineCell, err := findExactlyOne(commNodes, "data-field", "license_line_total")
	require.NoError(t, err)
	assert.Equal(t, "Rp 35.000.000", nodeText(licenseLineCell))

	licenseSubtotalCell, err := findExactlyOne(commNodes, "data-field", "total_license")
	require.NoError(t, err)
	assert.Equal(t, "Rp 35.000.000", nodeText(licenseSubtotalCell))

	// Section 1 total is Rp 35.000.000, NOT grand total Rp 60.305.000
	assert.NotContains(t, nodeText(commSection), "60.305.000")

	// 4. Maintenance total Rp24.255.000, hosting total Rp1.050.000, total MH Rp25.305.000
	mhSection, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance-hosting")
	require.NoError(t, err)
	require.NotNil(t, mhSection)
	mhNodes := collectNodes(mhSection)

	maintLineCell, err := findExactlyOne(mhNodes, "data-field", "maintenance_line_total")
	require.NoError(t, err)
	assert.Equal(t, "Rp 24.255.000", nodeText(maintLineCell))

	hostLineCell, err := findExactlyOne(mhNodes, "data-field", "hosting_line_total")
	require.NoError(t, err)
	assert.Equal(t, "Rp 1.050.000", nodeText(hostLineCell))

	mhTotalCell, err := findExactlyOne(mhNodes, "data-field", "maintenance_hosting_total")
	require.NoError(t, err)
	assert.Equal(t, "Rp 25.305.000", nodeText(mhTotalCell))

	// Check row roles in maintenance & hosting
	_, err = findExactlyOne(mhNodes, "data-role", "maintenance_0")
	require.NoError(t, err)
	_, err = findExactlyOne(mhNodes, "data-role", "hosting_0")
	require.NoError(t, err)

	// 5. Total investasi keseluruhan Rp60.305.000 hanya pada preview summary / payment
	// Empat payment rows dan nominal total Rp60.305.000
	paymentSection, err := findExactlyOne(nodes, "data-block-type", "payment")
	require.NoError(t, err)
	require.NotNil(t, paymentSection)
	paymentNodes := collectNodes(paymentSection)

	paymentNominals := findAllOrdered(paymentNodes, "data-field", "payment_nominal")
	require.Len(t, paymentNominals, 4)
	assert.Equal(t, "Rp 35.000.000", nodeText(paymentNominals[0]))
	assert.Equal(t, "Rp 8.435.000", nodeText(paymentNominals[1]))
	assert.Equal(t, "Rp 8.435.000", nodeText(paymentNominals[2]))
	assert.Equal(t, "Rp 8.435.000", nodeText(paymentNominals[3]))

	paymentTotalCell, err := findExactlyOne(paymentNodes, "data-field", "payment_total")
	require.NoError(t, err)
	assert.Equal(t, "Rp 60.305.000", nodeText(paymentTotalCell))

	// Tax clause in payment
	taxCell, err := findExactlyOne(paymentNodes, "data-field", "tax_clause")
	require.NoError(t, err)
	assert.Equal(t, "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.", nodeText(taxCell))

	// 6. Enam module names sumber: Master Data, Inventory, Pembelian, Kasir POS, Kas & Bank, Laporan
	scopeSection, err := findExactlyOne(nodes, "data-block-type", "scope")
	require.NoError(t, err)
	require.NotNil(t, scopeSection)
	scopeNodes := collectNodes(scopeSection)
	h3Modules := extractH3Headings(scopeNodes)
	expectedModules := []string{"Master Data", "Inventory", "Pembelian", "Kasir POS", "Kas & Bank", "Laporan"}
	assert.Equal(t, expectedModules, h3Modules)

	// Timeline block within scope/timeline
	timelineSection, err := findExactlyOne(scopeNodes, "data-block-type", "timeline")
	require.NoError(t, err)
	require.NotNil(t, timelineSection)
	assert.Contains(t, nodeText(timelineSection), "Instalasi dan Konfigurasi Sistem")
	assert.Contains(t, nodeText(timelineSection), "Pelatihan Pengguna dan Go-Live")

	// 7. Tidak ada teks source-code/rebranding ketika rights kosong
	assert.NotContains(t, docHTML, `data-block-type="rights"`)
	assert.NotContains(t, docHTML, "source-code")
	assert.NotContains(t, docHTML, "source code")
	assert.NotContains(t, docHTML, "rebranding")
	assert.NotContains(t, docHTML, "re-branding")

	// 8. Legal section
	legalSection, err := findExactlyOne(nodes, "data-block-type", "legal")
	require.NoError(t, err)
	require.NotNil(t, legalSection)
	assert.Contains(t, nodeText(legalSection), "Ketentuan operasional dan batasan layanan.")

	// 9. Signature section
	sigSection, err := findExactlyOne(nodes, "data-block-type", "signature")
	require.NoError(t, err)
	require.NotNil(t, sigSection)
	assert.Contains(t, nodeText(sigSection), "Yulianto Frandi")
	assert.Contains(t, nodeText(sigSection), "Gus H. Ahmad")
	assert.Contains(t, nodeText(sigSection), "Ponpes Al Hidayat 5")
	assert.Contains(t, nodeText(sigSection), "7 September 2026")
}

func TestWhiteLabelHTMLRenderer_LicenseOnlyLayout(t *testing.T) {
	input := ponpesBenchmarkWhiteLabelInput()
	input.MaintenanceItems = nil
	input.HostingItems = nil
	input.ModuleGroups = nil
	input.TimelineRows = nil
	input.RightsStatements = nil
	input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
		{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
	}

	calc, err := CalculateWhiteLabel(input)
	require.NoError(t, err)

	profile, err := ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	renderer := NewWhiteLabelHTMLRenderer()
	docHTML, err := renderer.Render(WhiteLabelDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber: "QTN/2026/09/0002",
			Subject:         "Lisensi POS Mini Market",
			ClientName:      "Bpk. Budi",
			IssueDate:       "8 September 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<ol><li>Syarat lisensi.</li></ol>",
	})
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Exactly 3 headings: 1. Biaya White Label, 2. Pembayaran, 3. Batasan dan Pengecualian
	h2Headings := extractH2Headings(nodes)
	expectedH2 := []string{
		"1. Biaya White Label",
		"2. Pembayaran",
		"3. Batasan dan Pengecualian",
	}
	assert.Equal(t, expectedH2, h2Headings)
	assert.Equal(t, []int{1, 2, 3}, extractSectionNumbers(nodes))

	// No maintenance_hosting, no scope, no timeline, no rights
	assert.NotContains(t, docHTML, `data-block-type="commercial-maintenance-hosting"`)
	assert.NotContains(t, docHTML, `data-block-type="scope"`)
	assert.NotContains(t, docHTML, `data-block-type="timeline"`)
	assert.NotContains(t, docHTML, `data-block-type="rights"`)
}

func TestWhiteLabelHTMLRenderer_RightsPopulated(t *testing.T) {
	input := ponpesBenchmarkWhiteLabelInput()
	input.RightsStatements = []string{
		"Kepemilikan source code dialihkan kepada klien setelah pelunasan.",
		"Klien berhak melakukan re-branding sistem.",
	}

	calc, err := CalculateWhiteLabel(input)
	require.NoError(t, err)

	profile, err := ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	renderer := NewWhiteLabelHTMLRenderer()
	docHTML, err := renderer.Render(WhiteLabelDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber: "QTN/2026/09/0003",
			Subject:         "White Label dengan Hak Cipta",
			ClientName:      "Ibu Siti",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal terms</p>",
	})
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	rightsDiv, err := findExactlyOne(nodes, "data-block-type", "rights")
	require.NoError(t, err)
	require.NotNil(t, rightsDiv)
	assert.Contains(t, nodeText(rightsDiv), "Kepemilikan source code dialihkan kepada klien setelah pelunasan.")
	assert.Contains(t, nodeText(rightsDiv), "Klien berhak melakukan re-branding sistem.")

	// Heading count remains 5 (rights does not introduce a numbered heading)
	h2Headings := extractH2Headings(nodes)
	assert.Len(t, h2Headings, 5)
}

func TestWhiteLabelHTMLRenderer_EscapesUnsafeHTML(t *testing.T) {
	input := ponpesBenchmarkWhiteLabelInput()
	input.ProductName = `<script>alert('product')</script>POS`
	input.LicenseItems[0].Name = `<img src=x onerror=alert('license')>Item`
	input.MaintenanceItems[0].Name = `<b onclick="steal()">Maint</b>`
	input.HostingItems[0].Name = `Hosting <script>`
	input.ModuleGroups[0].Name = `<iframe src="evil.com">Module</iframe>`
	input.ModuleGroups[0].Features[0] = `Feature <script>alert(1)</script>`
	input.TimelineRows[0].Description = `Timeline <style>body{}</style>`
	input.TimelineRows[0].Timeline = `Waktu <script>`
	input.PaymentSchedule[0].WorkPeriod = `<script>alert('pay')</script>`
	input.PaymentSchedule[0].Terms = `<span onmouseover="x()">Term</span>`
	input.RightsStatements = []string{`<script>alert('rights')</script>Rights text`}

	calc, err := CalculateWhiteLabel(input)
	require.NoError(t, err)

	profile, err := ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	renderer := NewWhiteLabelHTMLRenderer()
	docHTML, err := renderer.Render(WhiteLabelDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber:   "Q-<script>",
			Subject:           "<b>Subject</b>",
			ClientName:        "<script>alert('client')</script>",
			ClientCompanyName: "PT <Evil>",
			IssueDate:         "<script>date</script>",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal terms</p>",
	})
	require.NoError(t, err)

	assert.NotContains(t, docHTML, "<script>")
	assert.NotContains(t, docHTML, "<script")
	assert.NotContains(t, docHTML, "<img")
	assert.NotContains(t, docHTML, "<iframe")
	assert.NotContains(t, docHTML, "<style>")
	assert.NotContains(t, docHTML, "<span onmouseover")
	assert.NotContains(t, docHTML, "<b ")
	assert.NotContains(t, docHTML, "<b>")

	assert.Contains(t, docHTML, "&lt;script&gt;")
	assert.Contains(t, docHTML, "&lt;iframe")
	assert.Contains(t, docHTML, "&lt;img")
	assert.Contains(t, docHTML, "&lt;b")
	assert.Contains(t, docHTML, "&lt;span")
}

func TestWhiteLabelHTMLRenderer_ValidationErrors(t *testing.T) {
	renderer := NewWhiteLabelHTMLRenderer()

	t.Run("empty license calculation", func(t *testing.T) {
		input := ponpesBenchmarkWhiteLabelInput()
		calc, err := CalculateWhiteLabel(input)
		require.NoError(t, err)
		calc.License = nil

		_, err = renderer.Render(WhiteLabelDocumentInput{
			Input:       input,
			Calculation: calc,
			LegalHTML:   "<p>Legal</p>",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "license")
	})

	t.Run("empty payment schedule", func(t *testing.T) {
		input := ponpesBenchmarkWhiteLabelInput()
		calc, err := CalculateWhiteLabel(input)
		require.NoError(t, err)
		input.PaymentSchedule = nil

		_, err = renderer.Render(WhiteLabelDocumentInput{
			Input:       input,
			Calculation: calc,
			LegalHTML:   "<p>Legal</p>",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "payment_schedule")
	})

	t.Run("empty legal html", func(t *testing.T) {
		input := ponpesBenchmarkWhiteLabelInput()
		calc, err := CalculateWhiteLabel(input)
		require.NoError(t, err)

		_, err = renderer.Render(WhiteLabelDocumentInput{
			Input:       input,
			Calculation: calc,
			LegalHTML:   "   ",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "legal_html")
	})
}
