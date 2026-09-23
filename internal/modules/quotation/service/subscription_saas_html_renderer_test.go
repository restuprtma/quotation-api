package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validPercetakanTestInput() domain.SubscriptionSaaSInput {
	return domain.SubscriptionSaaSInput{
		ProductName:       "Subscription AI Percetakan",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      domain.SubscriptionBillingQuarterly,
		TaxTreatment:      domain.SubscriptionTaxNotStated,
		InitialTermMonths: 12,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Subscription 12 bulan", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_250_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Agustus-Oktober 2026", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Nominal: 3_750_000},
			{WorkPeriod: "November 2026-Januari 2027", StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: "1 Oktober 2026", DueDateLabel: "15 Oktober 2026", Nominal: 3_750_000},
			{WorkPeriod: "Februari-April 2027", StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: "1 Januari 2027", DueDateLabel: "15 Januari 2027", Nominal: 3_750_000},
			{WorkPeriod: "Mei-Juli 2027", StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: "1 April 2027", DueDateLabel: "15 April 2027", Nominal: 3_750_000},
		},
		IsPartnership: true,
		IncomeSharing: &domain.IncomeSharingInput{
			VenturoShareBPS:            5_000,
			PartnerShareBPS:            5_000,
			EligibleRevenueDescription: "Penjualan dari referensi mitra",
			SettlementBusinessDays:     30,
			ProhibitUnilateralDiscount: true,
		},
	}
}

func extractBlockTypes(nodes []*html.Node) []string {
	var blockTypes []string
	for _, node := range nodes {
		if node.Type != html.ElementNode {
			continue
		}
		if blockType := attributeValue(node, "data-block-type"); blockType != "" {
			blockTypes = append(blockTypes, blockType)
		}
	}
	return blockTypes
}

func extractSectionNumbers(nodes []*html.Node) []int {
	var numbers []int
	for _, node := range nodes {
		if node.Type == html.ElementNode && node.Data == "section" {
			if val := attributeValue(node, "data-section-number"); val != "" {
				var n int
				if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
					numbers = append(numbers, n)
				}
			}
		}
	}
	return numbers
}

func TestSubscriptionSaaSHTMLRenderer_PercetakanFixture(t *testing.T) {
	input := validPercetakanTestInput()
	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/08/0002",
		Subject:           "Pengembangan Subscription AI Percetakan",
		ClientName:        "Bpk. Hendra",
		ClientCompanyName: "PT Percetakan Jaya",
		IssueDate:         "1 Agustus 2026",
	}

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         input,
		Calculation:   calc,
		Profile:       profile,
		FeatureGroups: input.FeatureGroups,
		LegalHTML:     "<ol><li>Batasan resmi penawaran.</li></ol>",
	})
	require.NoError(t, err)
	assertCanonicalOfficialHeader(t, docHTML, "PENAWARAN LAYANAN SOFTWARE SUBSCRIPTION / SAAS")

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Block types order
	expectedBlockTypes := []string{"metadata", "opening", "commercial", "payment", "rights", "legal", "signature"}
	actualBlockTypes := extractBlockTypes(nodes)
	assert.Equal(t, expectedBlockTypes, actualBlockTypes)

	// Total contract
	totalCell, err := findExactlyOne(nodes, "data-field", "total_contract")
	require.NoError(t, err)
	require.NotNil(t, totalCell)
	assert.Equal(t, "Rp 15.000.000", nodeText(totalCell))

	// 4 payment nominals Rp 3.750.000
	paymentCells := findAllOrdered(nodes, "data-field", "payment_nominal")
	require.Len(t, paymentCells, 4)
	for _, pc := range paymentCells {
		assert.Equal(t, "Rp 3.750.000", nodeText(pc))
	}

	// Two share 50% & copy 30 hari kerja
	incomeSharingCell, err := findExactlyOne(nodes, "data-field", "income_sharing")
	require.NoError(t, err)
	require.NotNil(t, incomeSharingCell)
	sharingText := nodeText(incomeSharingCell)
	assert.Equal(t, 2, strings.Count(sharingText, "50%"))
	assert.Contains(t, sharingText, "30 hari kerja")
	assert.Contains(t, sharingText, "Diskon tidak dapat diberikan secara sepihak.")

	// No tax clause when not_stated
	taxCell, _ := findExactlyOne(nodes, "data-field", "tax_clause")
	assert.Nil(t, taxCell)

	// No timeline and no >Server<
	assert.NotContains(t, docHTML, `data-block-type="timeline"`)
	assert.NotContains(t, docHTML, ">Server<")

	// Opening paragraph exact
	assert.Contains(t, nodeText(root), "Melalui penawaran ini, Venturo menyampaikan skema penggunaan layanan software berbasis subscription beserta biaya, periode layanan, ketentuan pembayaran, dan hak para pihak yang disepakati.")

	// Layout A table header
	commercialSection, err := findExactlyOne(nodes, "data-block-type", "commercial")
	require.NoError(t, err)
	require.NotNil(t, commercialSection)
	commText := nodeText(commercialSection)
	assert.Contains(t, commText, "Nama Paket / Komponen")
	assert.Contains(t, commText, "Rate (Monthly)")
	assert.Contains(t, commText, "Period (Month)")
	assert.Contains(t, commText, "Total")

	// Commercial row marker
	periodRow, err := findExactlyOne(nodes, "data-role", "subscription_period_0")
	require.NoError(t, err)
	require.NotNil(t, periodRow)
	lineTotalCell, err := findExactlyOne(collectNodes(periodRow), "data-field", "subscription_line_total")
	require.NoError(t, err)
	require.NotNil(t, lineTotalCell)
	assert.Equal(t, "Rp 15.000.000", nodeText(lineTotalCell))

	// Payment table header
	paymentSection, err := findExactlyOne(nodes, "data-block-type", "payment")
	require.NoError(t, err)
	require.NotNil(t, paymentSection)
	payText := nodeText(paymentSection)
	assert.Contains(t, payText, "Work Period")
	assert.Contains(t, payText, "Invoice Date")
	assert.Contains(t, payText, "Due Date")
	assert.Contains(t, payText, "Nominal")

	// Legal HTML verbatim
	assert.Contains(t, docHTML, "<ol><li>Batasan resmi penawaran.</li></ol>")

	// Signature block contains client name and company
	sigSection, err := findExactlyOne(nodes, "data-block-type", "signature")
	require.NoError(t, err)
	require.NotNil(t, sigSection)
	assert.Contains(t, nodeText(sigSection), "Bpk. Hendra")
	assert.Contains(t, nodeText(sigSection), "PT Percetakan Jaya")
	assert.Contains(t, nodeText(sigSection), "1 Agustus 2026")
}

func TestSubscriptionSaaSHTMLRenderer_AnnualLayoutWithoutRights(t *testing.T) {
	input := domain.SubscriptionSaaSInput{
		ProductName:       "Annual SaaS",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutDevelopmentMaintenance,
		BillingCycle:      domain.SubscriptionBillingYearly,
		TaxTreatment:      domain.SubscriptionTaxExclusive,
		InitialTermMonths: 24,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Tahun 1 - Development", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_000_000},
			{Label: "Tahun 2 - Maintenance", StartMonthOffset: 12, DurationMonths: 12, MonthlyRate: 1_500_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Tahun 1", StartMonthOffset: 0, CoveredMonths: 12, InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Nominal: 12_000_000},
			{WorkPeriod: "Tahun 2", StartMonthOffset: 12, CoveredMonths: 12, InvoiceDateLabel: "Awal tahun kedua", DueDateLabel: "H+14", Nominal: 18_000_000},
		},
		FeatureGroups: []domain.SubscriptionFeatureGroupInput{
			{Name: "Core Modules", Features: []string{"User Authentication", "Role-based Access Control"}},
			{Name: "Reporting", Features: []string{"Export PDF & Excel", "Realtime Dashboard"}},
		},
		IsPartnership: false,
	}

	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/08/0003",
		Subject:           "Annual Enterprise SaaS Solution",
		ClientName:        "Ibu Ratna",
		ClientCompanyName: "PT Sukses Bersama",
		IssueDate:         "1 Agustus 2026",
	}

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         input,
		Calculation:   calc,
		Profile:       profile,
		FeatureGroups: input.FeatureGroups,
		LegalHTML:     "<ol><li>Ketentuan hukum resmi.</li></ol>",
	})
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Block types order
	expectedBlockTypes := []string{"metadata", "opening", "commercial", "features", "payment", "legal", "signature"}
	actualBlockTypes := extractBlockTypes(nodes)
	assert.Equal(t, expectedBlockTypes, actualBlockTypes)

	// Section numbers 1..4
	sectionNumbers := extractSectionNumbers(nodes)
	assert.Equal(t, []int{1, 2, 3, 4}, sectionNumbers)

	// Layout B table header
	commercialSection, err := findExactlyOne(nodes, "data-block-type", "commercial")
	require.NoError(t, err)
	require.NotNil(t, commercialSection)
	commText := nodeText(commercialSection)
	assert.Contains(t, commText, "Fase / Tahun")
	assert.Contains(t, commText, "Rate (Monthly)")
	assert.Contains(t, commText, "Period (Month)")
	assert.Contains(t, commText, "Total")

	// Tax clause exact for exclusive
	taxCell, err := findExactlyOne(nodes, "data-field", "tax_clause")
	require.NoError(t, err)
	require.NotNil(t, taxCell)
	assert.Equal(t, "Harga belum termasuk PPN.", nodeText(taxCell))

	// Features section content
	featuresSection, err := findExactlyOne(nodes, "data-block-type", "features")
	require.NoError(t, err)
	require.NotNil(t, featuresSection)
	fText := nodeText(featuresSection)
	assert.Contains(t, fText, "Core Modules")
	assert.Contains(t, fText, "User Authentication")
	assert.Contains(t, fText, "Role-based Access Control")
	assert.Contains(t, fText, "Reporting")
	assert.Contains(t, fText, "Export PDF & Excel")
	assert.Contains(t, fText, "Realtime Dashboard")

	// No rights section
	rightsSection, _ := findExactlyOne(nodes, "data-block-type", "rights")
	assert.Nil(t, rightsSection)

	// Inclusive PPN test
	inputInclusive := input
	inputInclusive.TaxTreatment = domain.SubscriptionTaxInclusive
	profileInclusive, err := ResolveSubscriptionSaaSProfile(inputInclusive)
	require.NoError(t, err)
	docInclusiveHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         inputInclusive,
		Calculation:   calc,
		Profile:       profileInclusive,
		FeatureGroups: inputInclusive.FeatureGroups,
		LegalHTML:     "<ol><li>Ketentuan hukum resmi.</li></ol>",
	})
	require.NoError(t, err)
	assert.Contains(t, docInclusiveHTML, "Harga sudah termasuk PPN.")
}

func TestSubscriptionSaaSHTMLRenderer_OneTimeItems(t *testing.T) {
	input := validPercetakanTestInput()
	input.OneTimeItems = []domain.SubscriptionOneTimeItemInput{
		{Name: "Biaya Setup & Onboarding", Amount: 2_500_000},
	}
	input.PaymentSchedule = []domain.SubscriptionPaymentRowInput{
		{WorkPeriod: "Setup & Q1", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Kontrak", DueDateLabel: "H+14", Nominal: 6_250_000},
		{WorkPeriod: "Q2", StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: "1 Okt", DueDateLabel: "15 Okt", Nominal: 3_750_000},
		{WorkPeriod: "Q3", StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: "1 Jan", DueDateLabel: "15 Jan", Nominal: 3_750_000},
		{WorkPeriod: "Q4", StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: "1 Apr", DueDateLabel: "15 Apr", Nominal: 3_750_000},
	}

	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	assert.Equal(t, int64(17_500_000), calc.TotalContract)

	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:    QuotationMetadata{QuotationNumber: "Q-004", Subject: "Test", ClientName: "Client"},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal</p>",
	})
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	oneTimeRow, err := findExactlyOne(nodes, "data-role", "subscription_one_time_0")
	require.NoError(t, err)
	require.NotNil(t, oneTimeRow)
	assert.Contains(t, nodeText(oneTimeRow), "Biaya Setup & Onboarding")
	assert.Contains(t, nodeText(oneTimeRow), "Rp 2.500.000")

	totalCell, err := findExactlyOne(nodes, "data-field", "total_contract")
	require.NoError(t, err)
	require.NotNil(t, totalCell)
	assert.Equal(t, "Rp 17.500.000", nodeText(totalCell))
}

func TestSubscriptionSaaSHTMLRenderer_AutoRenewalAndRights(t *testing.T) {
	input := validPercetakanTestInput()
	input.AutoRenewal = true
	input.RightsClauseCode = domain.RightsClauseVenturoSoftwareOwnershipV1

	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:    QuotationMetadata{QuotationNumber: "Q-005", Subject: "Test", ClientName: "Client"},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal</p>",
	})
	require.NoError(t, err)

	root, err := html.Parse(strings.NewReader(docHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Auto renewal text
	autoRenewalCell, err := findExactlyOne(nodes, "data-field", "auto_renewal")
	require.NoError(t, err)
	require.NotNil(t, autoRenewalCell)
	assert.Contains(t, nodeText(autoRenewalCell), "Masa berlangganan awal adalah 12 bulan dan diperpanjang otomatis")

	// Rights clause text
	rightsCell, err := findExactlyOne(nodes, "data-field", "rights_clause")
	require.NoError(t, err)
	require.NotNil(t, rightsCell)
	assert.Equal(t, "Aplikasi dan source code merupakan milik Venturo.", nodeText(rightsCell))
}

func TestSubscriptionSaaSHTMLRenderer_EscapesUnsafeHTML(t *testing.T) {
	input := validPercetakanTestInput()
	input.ProductName = `<script>alert('xss')</script>SaaS`

	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber:   "Q-006",
			Subject:           "<b>Subject</b>",
			ClientName:        "<script>alert('client')</script>",
			ClientCompanyName: "PT <Evil>",
			IssueDate:         "1 Agustus 2026",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal</p>",
	})
	require.NoError(t, err)

	assert.NotContains(t, docHTML, "<script>")
	assert.Contains(t, docHTML, "&lt;script&gt;")
}
