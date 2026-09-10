package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type kioskQueueBenchmarkFixture struct {
	SchemaVersion string                       `json:"schema_version"`
	Metadata      benchmarkMetadata            `json:"metadata"`
	LegalHTML     string                       `json:"legal_html"`
	Input         domain.KioskQueueSystemInput `json:"input"`
	Expected      kioskQueueBenchmarkExpected  `json:"expected"`
}

type kioskQueueBenchmarkExpected struct {
	Calculation   domain.KioskQueueCalculation     `json:"calculation"`
	Profile       domain.ResolvedKioskQueueProfile `json:"profile"`
	SectionTitles []string                         `json:"section_titles"`
	DOM           kioskQueueBenchmarkDOM           `json:"dom"`
}

type kioskQueueBenchmarkDOM struct {
	RootClass            string   `json:"root_class"`
	Heading              string   `json:"heading"`
	SectionNumbers       []int    `json:"section_numbers"`
	SectionTitles        []string `json:"section_titles"`
	ServiceSubtotal      string   `json:"service_subtotal"`
	ServiceDiscount      string   `json:"service_discount,omitempty"`
	ServiceTax           string   `json:"service_tax,omitempty"`
	ServiceTotal         string   `json:"service_total"`
	HardwareSubtotal     string   `json:"hardware_subtotal,omitempty"`
	HardwareTax          string   `json:"hardware_tax,omitempty"`
	HardwareTotal        string   `json:"hardware_total,omitempty"`
	MaintenanceTotal     string   `json:"maintenance_total,omitempty"`
	PaymentTotal         string   `json:"payment_total"`
	PaymentNominals      []string `json:"payment_nominals"`
	HasHardware          bool     `json:"has_hardware"`
	HasMaintenance       bool     `json:"has_maintenance"`
	HasRights            bool     `json:"has_rights"`
	SignatureColumnCount int      `json:"signature_column_count"`
}

func loadKioskQueueFixture(t *testing.T, relPath string) kioskQueueBenchmarkFixture {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fullPath := filepath.Join(filepath.Dir(sourceFile), relPath)
	data, err := os.ReadFile(fullPath)
	require.NoError(t, err)
	var fixture kioskQueueBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func TestKioskQueueBenchmark(t *testing.T) {
	tests := []struct {
		name             string
		file             string
		serviceTotal     int64
		hardwareTotal    int64
		maintenanceTotal int64
		paymentTarget    int64
	}{
		{"mpp government hardware", "testdata/kiosk_queue_system/mpp-government-hardware.json", 48_385_000, 51_615_000, 0, 100_000_000},
		{"mpp government service only", "testdata/kiosk_queue_system/mpp-government-service-only.json", 62_000_000, 0, 0, 62_000_000},
		{"self ordering food court", "testdata/kiosk_queue_system/self-ordering-food-court.json", 96_305_000, 0, 83_625_000, 96_305_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := loadKioskQueueFixture(t, tt.file)
			require.Equal(t, "kiosk-queue-benchmark-v1", fixture.SchemaVersion)
			require.NotEmpty(t, fixture.Metadata.SourceSheet)
			require.NotEmpty(t, fixture.Metadata.SourceGID)
			require.NotEmpty(t, fixture.Metadata.VerifiedOn)
			require.Equal(t, "historical_sheet_reference_current_engine_expected", fixture.Metadata.SourceStatus)
			require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
			require.NotEmpty(t, fixture.Metadata.SourceVariance)

			// 1. Validation
			err := fixture.Input.Validate()
			require.NoError(t, err, "validation error")

			// 2. Calculator
			calc, err := CalculateKioskQueueSystem(fixture.Input)
			require.NoError(t, err, "calculation error")
			assert.Equal(t, tt.serviceTotal, calc.ServiceTotal)
			assert.Equal(t, tt.hardwareTotal, calc.HardwareTotal)
			assert.Equal(t, tt.maintenanceTotal, calc.MaintenanceTotal)
			assert.Equal(t, tt.paymentTarget, calc.PaymentTargetTotal)
			assert.Equal(t, fixture.Expected.Calculation, calc, "calculation exact match")

			// 3. Profile Resolver
			profile, err := ResolveKioskQueueProfile(fixture.Input)
			require.NoError(t, err, "profile error")
			assert.Equal(t, fixture.Expected.Profile, profile, "profile exact match")

			// 4. Section Plan
			sectionTitles := make([]string, len(profile.Sections))
			for i, s := range profile.Sections {
				sectionTitles[i] = s.Title
			}
			assert.Equal(t, fixture.Expected.SectionTitles, sectionTitles, "section titles match")

			// 5. HTML Renderer
			renderer := NewKioskQueueHTMLRenderer()
			docHTML, err := renderer.Render(KioskQueueDocumentInput{
				Metadata: QuotationMetadata{
					QuotationNumber:   fixture.Metadata.QuotationNumber,
					Subject:           fixture.Metadata.Subject,
					ClientName:        fixture.Metadata.ClientName,
					ClientCompanyName: fixture.Metadata.ClientCompany,
					IssueDate:         fixture.Metadata.IssueDate,
				},
				Input:       fixture.Input,
				Calculation: calc,
				Profile:     profile,
				LegalHTML:   fixture.LegalHTML,
			})
			require.NoError(t, err, "render error")

			// 6. Sanitizer
			sanitizer := NewHTMLSanitizer()
			sanitizedHTML, err := sanitizer.Sanitize(docHTML)
			require.NoError(t, err, "sanitization error")
			require.NotEmpty(t, sanitizedHTML, "sanitized html not empty")

			// 7. DOM Integrity Validator
			err = ValidateKioskQueueIntegrity(sanitizedHTML, fixture.Input, calc, profile, fixture.LegalHTML)
			require.NoError(t, err, "dom integrity validation error")

			// 8. Assert DOM structure, tax copy, limitation copy, payment values/order, legal marker, and signature columns
			assertKioskQueueDOM(t, sanitizedHTML, fixture.Input, calc, profile, fixture.Expected.DOM)
		})
	}
}

func assertKioskQueueDOM(
	t *testing.T,
	document string,
	input domain.KioskQueueSystemInput,
	calc domain.KioskQueueCalculation,
	profile domain.ResolvedKioskQueueProfile,
	expected kioskQueueBenchmarkDOM,
) {
	t.Helper()
	root, err := html.Parse(strings.NewReader(document))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Article
	var articles []*html.Node
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "article" {
			articles = append(articles, n)
		}
	}
	require.Len(t, articles, 1)
	assert.Contains(t, attributeValue(articles[0], "class"), "quotation-document")
	assert.Contains(t, attributeValue(articles[0], "class"), "kiosk-queue-document")

	// Heading
	headings := findElementsByTag(nodes, "h1")
	require.Len(t, headings, 1)
	assert.Equal(t, expected.Heading, nodeText(headings[0]))

	// Ordered Sections
	var sectionNumbers []int
	var sectionTitles []string
	for _, node := range nodes {
		if node.Type != html.ElementNode || node.Data != "section" {
			continue
		}
		if value := attributeValue(node, "data-section-number"); value != "" {
			num, err := strconv.Atoi(value)
			require.NoError(t, err)
			sectionNumbers = append(sectionNumbers, num)
			h2List := directChildElements(node, "h2")
			require.Len(t, h2List, 1)
			sectionTitles = append(sectionTitles, nodeText(h2List[0]))
		}
	}
	assert.Equal(t, expected.SectionNumbers, sectionNumbers, "section numbers in DOM")
	assert.Equal(t, expected.SectionTitles, sectionTitles, "section titles in DOM")

	// Service Subtotal
	subCell, err := findExactlyOne(nodes, "data-field", "service_subtotal")
	require.NoError(t, err)
	require.NotNil(t, subCell)
	assert.Equal(t, expected.ServiceSubtotal, nodeText(subCell), "service subtotal in DOM")

	// Service Discount
	if expected.ServiceDiscount != "" {
		discCell, err := findExactlyOne(nodes, "data-field", "service_discount_amount")
		require.NoError(t, err)
		require.NotNil(t, discCell)
		assert.Equal(t, expected.ServiceDiscount, nodeText(discCell), "service discount in DOM")
	}

	// Service Tax
	if expected.ServiceTax != "" {
		taxCell, err := findExactlyOne(nodes, "data-field", "service_tax_amount")
		require.NoError(t, err)
		require.NotNil(t, taxCell)
		assert.Equal(t, expected.ServiceTax, nodeText(taxCell), "service tax in DOM")
	}

	// Service Total
	svcTotalCell, err := findExactlyOne(nodes, "data-field", "service_total")
	require.NoError(t, err)
	require.NotNil(t, svcTotalCell)
	assert.Equal(t, expected.ServiceTotal, nodeText(svcTotalCell), "service total in DOM")

	// Hardware Cost Table & Totals
	hwTable, _ := findExactlyOne(nodes, "data-role", "hardware_cost_table")
	if expected.HasHardware {
		require.NotNil(t, hwTable, "hardware cost table expected")
		hwSubCell, err := findExactlyOne(nodes, "data-field", "hardware_subtotal")
		require.NoError(t, err)
		require.NotNil(t, hwSubCell)
		assert.Equal(t, expected.HardwareSubtotal, nodeText(hwSubCell), "hardware subtotal in DOM")

		hwTaxCell, err := findExactlyOne(nodes, "data-field", "hardware_tax_amount")
		require.NoError(t, err)
		require.NotNil(t, hwTaxCell)
		assert.Equal(t, expected.HardwareTax, nodeText(hwTaxCell), "hardware tax in DOM")

		hwTotCell, err := findExactlyOne(nodes, "data-field", "hardware_total")
		require.NoError(t, err)
		require.NotNil(t, hwTotCell)
		assert.Equal(t, expected.HardwareTotal, nodeText(hwTotCell), "hardware total in DOM")
	} else {
		assert.Nil(t, hwTable, "no hardware table expected")
	}

	// Maintenance Total
	if expected.HasMaintenance {
		maintCell, err := findExactlyOne(nodes, "data-field", "maintenance_total")
		require.NoError(t, err)
		require.NotNil(t, maintCell)
		assert.Equal(t, expected.MaintenanceTotal, nodeText(maintCell), "maintenance total in DOM")
	}

	// Payment Total
	payTotalCell, err := findExactlyOne(nodes, "data-field", "payment_total")
	require.NoError(t, err)
	require.NotNil(t, payTotalCell)
	assert.Equal(t, expected.PaymentTotal, nodeText(payTotalCell), "payment total in DOM")

	// Payment Nominals in order
	payCells := findAllOrdered(nodes, "data-field", "payment_nominal")
	var nominals []string
	for _, cell := range payCells {
		nominals = append(nominals, nodeText(cell))
	}
	assert.Equal(t, expected.PaymentNominals, nominals, "payment nominals order and values in DOM")

	// Tax clause copy
	taxClauseNode, err := findExactlyOne(nodes, "data-field", "tax_clause")
	require.NoError(t, err)
	require.NotNil(t, taxClauseNode)
	assert.Equal(t, profile.TaxClauseText, nodeText(taxClauseNode), "tax clause copy in DOM")

	// Payment due clause
	if profile.PaymentDueDays > 0 {
		dueNode, err := findExactlyOne(nodes, "data-field", "payment_due_clause")
		require.NoError(t, err)
		require.NotNil(t, dueNode)
		expectedDueText := fmt.Sprintf("Pembayaran dilakukan maksimal H+%d hari kalender setelah invoice diterbitkan.", profile.PaymentDueDays)
		assert.Equal(t, expectedDueText, nodeText(dueNode), "payment due clause in DOM")
	}

	// Limitations copy
	limSection, err := findExactlyOne(nodes, "data-block-type", "limitations")
	require.NoError(t, err)
	require.NotNil(t, limSection)
	olNodes := findElementsByTag(collectNodes(limSection), "ol")
	require.Len(t, olNodes, 1)
	liNodes := findElementsByTag(collectNodes(olNodes[0]), "li")
	assert.Equal(t, len(profile.LimitationTexts), len(liNodes), "limitation items count")
	for i, text := range profile.LimitationTexts {
		assert.Equal(t, text, nodeText(liNodes[i]), "limitation item %d text", i)
	}

	// Legal marker
	legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
	require.NoError(t, err)
	require.NotNil(t, legalBlock, "legal block must be present")

	// Signature columns
	sigBlock, err := findExactlyOne(nodes, "data-block-type", "signature")
	require.NoError(t, err)
	require.NotNil(t, sigBlock)
	sigTables := findElementsByTag(collectNodes(sigBlock), "table")
	require.NotEmpty(t, sigTables)
	sigTds := findElementsByTag(collectNodes(sigTables[0]), "td")
	assert.Equal(t, expected.SignatureColumnCount, len(sigTds), "signature column count")
}
