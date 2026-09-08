package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type whiteLabelBenchmarkFixture struct {
	SchemaVersion string                      `json:"schema_version"`
	Metadata      benchmarkMetadata           `json:"metadata"`
	LegalHTML     string                      `json:"legal_html"`
	Input         domain.WhiteLabelInput      `json:"input"`
	Expected      whiteLabelBenchmarkExpected `json:"expected"`
}

type whiteLabelBenchmarkExpected struct {
	Calculation   domain.WhiteLabelCalculation     `json:"calculation"`
	Profile       domain.ResolvedWhiteLabelProfile `json:"profile"`
	SectionTitles []string                         `json:"section_titles"`
	DOM           whiteLabelBenchmarkDOM           `json:"dom"`
}

type whiteLabelBenchmarkDOM struct {
	RootClass                 string   `json:"root_class"`
	Heading                   string   `json:"heading"`
	BlockTypes                []string `json:"block_types"`
	SectionNumbers            []int    `json:"section_numbers"`
	SectionTitles             []string `json:"section_titles"`
	CommercialHeaders         []string `json:"commercial_headers"`
	MaintenanceHostingHeaders []string `json:"maintenance_hosting_headers,omitempty"`
	PaymentHeaders            []string `json:"payment_headers"`
	LicenseSubtotal           string   `json:"license_subtotal"`
	MaintenanceHostingTotal   string   `json:"maintenance_hosting_total,omitempty"`
	PaymentTotal              string   `json:"payment_total"`
	PaymentNominals           []string `json:"payment_nominals"`
	ModuleCount               int      `json:"module_count"`
	HasRights                 bool     `json:"has_rights"`
	SignatureColumnCount      int      `json:"signature_column_count"`
}

func TestWhiteLabelBenchmark(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "white_label")
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), func(t *testing.T) {
			fixture := loadWhiteLabelBenchmarkFixture(t, filepath.Join(fixtureDir, entry.Name()))
			require.Equal(t, "white-label-benchmark-v1", fixture.SchemaVersion)
			require.NotEmpty(t, fixture.Metadata.SourceSheet)
			require.NotEmpty(t, fixture.Metadata.SourceGID)
			require.NotEmpty(t, fixture.Metadata.VerifiedOn)
			require.NotEmpty(t, fixture.Metadata.SourceStatus)
			require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
			require.NotEmpty(t, fixture.Metadata.SourceVariance)

			if strings.Contains(entry.Name(), "ponpes") {
				require.Equal(t, "historical_sheet_reference_current_engine_expected", fixture.Metadata.SourceStatus)
			} else {
				require.Equal(t, "structure_only_synthetic_amounts", fixture.Metadata.SourceStatus)
			}

			// 1. Validation
			err := fixture.Input.Validate()
			require.NoError(t, err, "validation error")

			// 2. Calculator
			calc, err := CalculateWhiteLabel(fixture.Input)
			require.NoError(t, err, "calculation error")
			assert.Equal(t, fixture.Expected.Calculation, calc, "calculation output")

			// 3. Profile Resolver
			profile, err := ResolveWhiteLabelProfile(fixture.Input)
			require.NoError(t, err, "profile error")
			assert.Equal(t, fixture.Expected.Profile, profile, "profile output")

			// 4. Section Plan
			sectionTitles := make([]string, len(profile.Sections))
			for i, s := range profile.Sections {
				sectionTitles[i] = s.Title
			}
			assert.Equal(t, fixture.Expected.SectionTitles, sectionTitles, "section plan titles")

			// 5. HTML Renderer
			renderer := NewWhiteLabelHTMLRenderer()
			docHTML, err := renderer.Render(WhiteLabelDocumentInput{
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
			require.NotEmpty(t, sanitizedHTML, "sanitized html should not be empty")

			// 7. DOM Integrity Validator
			err = ValidateWhiteLabelIntegrity(sanitizedHTML, WhiteLabelIntegrityExpectations{
				Calculation: calc,
				Profile:     profile,
				LegalHTML:   fixture.LegalHTML,
				Rights:      fixture.Input.RightsStatements,
			})
			require.NoError(t, err, "dom integrity validation error")

			// 8. Assert DOM structure and content
			assertWhiteLabelDOM(t, sanitizedHTML, fixture.Expected.DOM)
		})
	}
}

func loadWhiteLabelBenchmarkFixture(t *testing.T, path string) whiteLabelBenchmarkFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture whiteLabelBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func assertWhiteLabelDOM(t *testing.T, document string, expected whiteLabelBenchmarkDOM) {
	t.Helper()
	root, err := html.Parse(strings.NewReader(document))
	require.NoError(t, err)
	nodes := collectNodes(root)

	articles := make([]*html.Node, 0, 1)
	for _, node := range nodes {
		if node.Type == html.ElementNode && node.Data == "article" {
			articles = append(articles, node)
		}
	}
	require.Len(t, articles, 1)
	assert.Equal(t, expected.RootClass, attributeValue(articles[0], "class"))

	headings := findElementsByTag(nodes, "h1")
	require.Len(t, headings, 1)
	assert.Equal(t, expected.Heading, nodeText(headings[0]))

	var blockTypes []string
	var sectionNumbers []int
	var sectionTitles []string
	for _, node := range nodes {
		if node.Type != html.ElementNode {
			continue
		}
		if blockType := attributeValue(node, "data-block-type"); blockType != "" {
			blockTypes = append(blockTypes, blockType)
		}
		if node.Data != "section" {
			continue
		}
		if value := attributeValue(node, "data-section-number"); value != "" {
			var number int
			_, err := fmt.Sscanf(value, "%d", &number)
			require.NoError(t, err)
			sectionNumbers = append(sectionNumbers, number)
			h2 := directChildElements(node, "h2")
			require.Len(t, h2, 1)
			sectionTitles = append(sectionTitles, nodeText(h2[0]))
		}
	}
	assert.Equal(t, expected.BlockTypes, blockTypes, "block types")
	assert.Equal(t, expected.SectionNumbers, sectionNumbers, "section numbers")
	assert.Equal(t, expected.SectionTitles, sectionTitles, "section titles")

	// Header tables
	assert.Equal(t, expected.CommercialHeaders, tableHeadersForBlock(nodes, "commercial"), "commercial headers")
	if len(expected.MaintenanceHostingHeaders) > 0 {
		assert.Equal(t, expected.MaintenanceHostingHeaders, tableHeadersForBlock(nodes, "commercial-maintenance-hosting"), "maintenance hosting headers")
	}
	assert.Equal(t, expected.PaymentHeaders, tableHeadersForBlock(nodes, "payment"), "payment headers")

	// License Subtotal
	licenseCell, err := findExactlyOne(nodes, "data-field", "total_license")
	require.NoError(t, err)
	require.NotNil(t, licenseCell)
	assert.Equal(t, expected.LicenseSubtotal, nodeText(licenseCell), "license subtotal in DOM")

	// Maintenance Hosting Total
	if expected.MaintenanceHostingTotal != "" {
		mhCell, err := findExactlyOne(nodes, "data-field", "maintenance_hosting_total")
		require.NoError(t, err)
		require.NotNil(t, mhCell)
		assert.Equal(t, expected.MaintenanceHostingTotal, nodeText(mhCell), "maintenance hosting total in DOM")
	}

	// Payment Total
	paymentTotalCell, err := findExactlyOne(nodes, "data-field", "payment_total")
	require.NoError(t, err)
	require.NotNil(t, paymentTotalCell)
	assert.Equal(t, expected.PaymentTotal, nodeText(paymentTotalCell), "payment total in DOM")

	// Payment Nominals
	paymentCells := findAllOrdered(nodes, "data-field", "payment_nominal")
	var nominals []string
	for _, cell := range paymentCells {
		nominals = append(nominals, nodeText(cell))
	}
	assert.Equal(t, expected.PaymentNominals, nominals, "payment nominals")

	// Module Count (h3 elements in scope block)
	scopeBlock, err := findExactlyOne(nodes, "data-block-type", "scope")
	require.NoError(t, err)
	if expected.ModuleCount > 0 {
		require.NotNil(t, scopeBlock)
		h3s := findElementsByTag(collectNodes(scopeBlock), "h3")
		assert.Equal(t, expected.ModuleCount, len(h3s), "module count in DOM")
	} else {
		assert.Nil(t, scopeBlock)
	}

	// Rights block presence / absence
	rightsBlock, err := findExactlyOne(nodes, "data-block-type", "rights")
	require.NoError(t, err)
	if expected.HasRights {
		assert.NotNil(t, rightsBlock, "expected rights block")
	} else {
		assert.Nil(t, rightsBlock, "expected no rights block")
	}

	// Signature two-column layout
	sigBlock, err := findExactlyOne(nodes, "data-block-type", "signature")
	require.NoError(t, err)
	require.NotNil(t, sigBlock)
	sigTables := findElementsByTag(collectNodes(sigBlock), "table")
	require.NotEmpty(t, sigTables)
	sigTds := findElementsByTag(collectNodes(sigTables[0]), "td")
	assert.Equal(t, expected.SignatureColumnCount, len(sigTds), "signature column count")
}
