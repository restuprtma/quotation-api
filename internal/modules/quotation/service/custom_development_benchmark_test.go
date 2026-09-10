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

type customDevelopmentBenchmarkFixture struct {
	SchemaVersion           string                             `json:"schema_version"`
	Metadata                benchmarkMetadata                  `json:"metadata"`
	LegalHTML               string                             `json:"legal_html,omitempty"`
	ExpectedValidationError string                             `json:"expected_validation_error,omitempty"`
	Input                   domain.CustomDevelopmentInput      `json:"input"`
	Expected                customDevelopmentBenchmarkExpected `json:"expected,omitempty"`
}

type customDevelopmentBenchmarkExpected struct {
	Calculation   domain.CustomDevelopmentCalculation     `json:"calculation"`
	Profile       domain.ResolvedCustomDevelopmentProfile `json:"profile"`
	SectionTitles []string                                `json:"section_titles"`
	DOM           customDevelopmentBenchmarkDOM           `json:"dom"`
}

type customDevelopmentBenchmarkDOM struct {
	RootClass            string   `json:"root_class"`
	Heading              string   `json:"heading"`
	SectionNumbers       []int    `json:"section_numbers"`
	SectionTitles        []string `json:"section_titles"`
	CommercialHeaders    []string `json:"commercial_headers"`
	MonthlyHeaders       []string `json:"monthly_headers,omitempty"`
	PaymentHeaders       []string `json:"payment_headers"`
	DevelopmentSubtotal  string   `json:"development_subtotal"`
	DiscountAmount       string   `json:"discount_amount,omitempty"`
	TotalInvestment      string   `json:"total_investment"`
	MonthlyTotal         string   `json:"monthly_total,omitempty"`
	PaymentNominals      []string `json:"payment_nominals"`
	HasMonthly           bool     `json:"has_monthly"`
	HasRights            bool     `json:"has_rights"`
	SignatureColumnCount int      `json:"signature_column_count"`
}

func TestCustomDevelopmentBenchmark(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "custom_development")
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), func(t *testing.T) {
			fixturePath := filepath.Join(fixtureDir, entry.Name())
			fixture := loadCustomDevelopmentBenchmarkFixture(t, fixturePath)

			require.Equal(t, "custom-development-benchmark-v1", fixture.SchemaVersion)
			require.NotEmpty(t, fixture.Metadata.SourceSheet)
			require.NotEmpty(t, fixture.Metadata.SourceGID)
			require.NotEmpty(t, fixture.Metadata.VerifiedOn)
			require.NotEmpty(t, fixture.Metadata.SourceStatus)
			require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
			require.NotEmpty(t, fixture.Metadata.SourceVariance)

			// Handle anomaly fixtures that MUST be rejected during validation and calculation
			if fixture.ExpectedValidationError != "" || strings.Contains(entry.Name(), "anomaly") {
				require.Equal(t, "historical_source_anomaly_must_reject", fixture.Metadata.SourceStatus)

				t.Run("NegativeDiscount", func(t *testing.T) {
					err := fixture.Input.Validate()
					require.Error(t, err, "anomaly fixture must fail validation due to negative discount")
					assert.Contains(t, err.Error(), "discount_amount", "validation error must target discount_amount")

					_, calcErr := CalculateCustomDevelopment(fixture.Input)
					require.Error(t, calcErr, "calculator must reject invalid input with negative discount")
					assert.Contains(t, calcErr.Error(), "discount_amount")
				})

				t.Run("PaymentScheduleMismatch", func(t *testing.T) {
					// When negative discount is repaired to a valid non-negative amount (0),
					// CalculateCustomDevelopment must still deterministically reject because
					// payment schedule total (Rp 303.864 x 2 = Rp 607.728) does not equal total investment (Rp 607.727).
					repairedInput := fixture.Input
					repairedInput.DiscountAmount = 0
					repairedInput.ShowDiscountRow = false

					valErr := repairedInput.Validate()
					require.NoError(t, valErr, "repaired input must pass structural validation")

					_, calcErr := CalculateCustomDevelopment(repairedInput)
					require.Error(t, calcErr, "calculation must fail due to payment schedule mismatch")
					assert.Contains(t, calcErr.Error(), "payment_schedule: development payment total 607728 must equal total investment 607727")
				})
				return
			}

			require.Equal(t, "historical_sheet_reference_current_engine_expected", fixture.Metadata.SourceStatus)

			// 1. Strict Validation
			err := fixture.Input.Validate()
			require.NoError(t, err, "validation error")

			// 2. Calculator
			calc, err := CalculateCustomDevelopment(fixture.Input)
			require.NoError(t, err, "calculation error")
			assert.Equal(t, fixture.Expected.Calculation, calc, "calculation output")
			assert.Equal(t, calc.TotalInvestment, calc.DevelopmentPaymentTotal, "development payment total must equal total investment")

			// 3. Profile Resolver
			profile, err := ResolveCustomDevelopmentProfile(fixture.Input)
			require.NoError(t, err, "profile error")
			assert.Equal(t, fixture.Expected.Profile, profile, "profile output")

			// 4. Section Plan
			sectionTitles := make([]string, len(profile.Sections))
			for i, s := range profile.Sections {
				sectionTitles[i] = s.Title
			}
			assert.Equal(t, fixture.Expected.SectionTitles, sectionTitles, "section plan titles")

			// 5. HTML Renderer
			renderer := NewCustomDevelopmentHTMLRenderer()
			docHTML, err := renderer.Render(CustomDevelopmentDocumentInput{
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
			err = ValidateCustomDevelopmentIntegrity(sanitizedHTML, CustomDevelopmentIntegrityExpectations{
				Input:       fixture.Input,
				Calculation: calc,
				Profile:     profile,
				LegalHTML:   fixture.LegalHTML,
			})
			require.NoError(t, err, "dom integrity validation error")

			// 8. Assert DOM structure, tables, cells, signatures
			assertCustomDevelopmentDOM(t, sanitizedHTML, fixture.Input, calc, fixture.Expected.DOM)
		})
	}
}

func loadCustomDevelopmentBenchmarkFixture(t *testing.T, path string) customDevelopmentBenchmarkFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture customDevelopmentBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func assertCustomDevelopmentDOM(
	t *testing.T,
	document string,
	input domain.CustomDevelopmentInput,
	calc domain.CustomDevelopmentCalculation,
	expected customDevelopmentBenchmarkDOM,
) {
	t.Helper()
	root, err := html.Parse(strings.NewReader(document))
	require.NoError(t, err)
	nodes := collectNodes(root)

	// Article check
	articles := make([]*html.Node, 0, 1)
	for _, node := range nodes {
		if node.Type == html.ElementNode && node.Data == "article" {
			articles = append(articles, node)
		}
	}
	require.Len(t, articles, 1)
	assert.Equal(t, expected.RootClass, attributeValue(articles[0], "class"))

	// H1 heading
	headings := findElementsByTag(nodes, "h1")
	require.Len(t, headings, 1)
	assert.Equal(t, expected.Heading, nodeText(headings[0]))

	// Sections, numbers, and titles
	var sectionNumbers []int
	var sectionTitles []string
	for _, node := range nodes {
		if node.Type != html.ElementNode || node.Data != "section" {
			continue
		}
		if value := attributeValue(node, "data-quotation-section-number"); value != "" {
			num, err := strconv.Atoi(value)
			require.NoError(t, err)
			sectionNumbers = append(sectionNumbers, num)
			h2 := directChildElements(node, "h2")
			require.Len(t, h2, 1)
			sectionTitles = append(sectionTitles, nodeText(h2[0]))
		}
	}
	assert.Equal(t, expected.SectionNumbers, sectionNumbers, "section numbers in DOM")
	assert.Equal(t, expected.SectionTitles, sectionTitles, "section titles in DOM")

	// Table headers
	assert.Equal(t, expected.CommercialHeaders, tableHeadersForQuotationSection(nodes, "commercial-development"), "commercial headers")
	if expected.HasMonthly {
		assert.Equal(t, expected.MonthlyHeaders, tableHeadersForQuotationSection(nodes, "commercial-monthly"), "monthly headers")
	}
	assert.Equal(t, expected.PaymentHeaders, tableHeadersForQuotationSection(nodes, "payment"), "payment headers")

	// Development figures
	subtotalCell, err := findExactlyOne(nodes, "data-quotation-field", "development_subtotal")
	require.NoError(t, err)
	require.NotNil(t, subtotalCell)
	assert.Equal(t, expected.DevelopmentSubtotal, nodeText(subtotalCell), "development subtotal in DOM")

	if expected.DiscountAmount != "" {
		discountCell, err := findExactlyOne(nodes, "data-quotation-field", "discount_amount")
		require.NoError(t, err)
		require.NotNil(t, discountCell)
		assert.Equal(t, expected.DiscountAmount, nodeText(discountCell), "discount amount in DOM")
	}

	totalInvestmentCell, err := findExactlyOne(nodes, "data-quotation-field", "total_investment")
	require.NoError(t, err)
	require.NotNil(t, totalInvestmentCell)
	assert.Equal(t, expected.TotalInvestment, nodeText(totalInvestmentCell), "total investment in DOM")

	// Monthly figures
	if expected.HasMonthly {
		monthlyTotalCell, err := findExactlyOne(nodes, "data-quotation-field", "monthly_total")
		require.NoError(t, err)
		require.NotNil(t, monthlyTotalCell)
		assert.Equal(t, expected.MonthlyTotal, nodeText(monthlyTotalCell), "monthly total in DOM")
	}

	// Payment nominals
	var nominals []string
	for i := range input.PaymentSchedule {
		cell, err := findExactlyOne(nodes, "data-quotation-field", fmt.Sprintf("payment_nominal_%d", i))
		require.NoError(t, err)
		require.NotNil(t, cell)
		nominals = append(nominals, nodeText(cell))
	}
	assert.Equal(t, expected.PaymentNominals, nominals, "payment nominals in DOM")

	// Check included QA row (nil quoted line total -> empty cell)
	for gIdx, group := range input.DevelopmentGroups {
		for iIdx, item := range group.Items {
			cell, err := findExactlyOne(nodes, "data-quotation-field", fmt.Sprintf("development_line_%d_%d", gIdx, iIdx))
			require.NoError(t, err)
			require.NotNil(t, cell)
			if item.QuotedLineTotal == nil {
				assert.Empty(t, nodeText(cell), "included line without quoted total must render empty cell")
			} else {
				assert.NotEmpty(t, nodeText(cell), "priced line must render formatted amount")
			}
		}
	}

	// Payment components check
	for i, row := range input.PaymentSchedule {
		tr, err := findExactlyOne(nodes, "data-role", fmt.Sprintf("payment_%d", i))
		require.NoError(t, err)
		require.NotNil(t, tr)
		assert.Equal(t, string(row.Component), attributeValue(tr, "data-quotation-component"), "payment row component attribute")
	}

	// Rights section presence / absence
	rightsSec, err := findExactlyOne(nodes, "data-quotation-section", "rights")
	require.NoError(t, err)
	if expected.HasRights {
		assert.NotNil(t, rightsSec, "expected rights section to exist")
	} else {
		assert.Nil(t, rightsSec, "expected no rights section")
	}

	// Legal section presence
	legalSec, err := findExactlyOne(nodes, "data-quotation-section", "legal")
	require.NoError(t, err)
	require.NotNil(t, legalSec, "legal section must exist")

	// Signature two-column layout
	sigBlock, err := findExactlyOne(nodes, "data-quotation-section", "signature")
	require.NoError(t, err)
	require.NotNil(t, sigBlock)
	sigTables := findElementsByTag(collectNodes(sigBlock), "table")
	require.NotEmpty(t, sigTables)
	sigTds := findElementsByTag(collectNodes(sigTables[0]), "td")
	assert.Equal(t, expected.SignatureColumnCount, len(sigTds), "signature column count")
}

func tableHeadersForQuotationSection(nodes []*html.Node, sectionMarker string) []string {
	for _, node := range nodes {
		if attributeValue(node, "data-quotation-section") != sectionMarker && attributeValue(node, "data-block-type") != sectionMarker {
			continue
		}
		for _, child := range collectNodes(node) {
			if child.Type != html.ElementNode || child.Data != "tr" || child.Parent == nil || child.Parent.Data != "thead" {
				continue
			}
			cells := directChildElements(child, "th")
			result := make([]string, len(cells))
			for i, cell := range cells {
				result[i] = nodeText(cell)
			}
			return result
		}
	}
	return nil
}
