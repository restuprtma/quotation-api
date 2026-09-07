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

type subscriptionSaaSBenchmarkFixture struct {
	SchemaVersion string                            `json:"schema_version"`
	Metadata      benchmarkMetadata                 `json:"metadata"`
	LegalHTML     string                            `json:"legal_html"`
	Input         domain.SubscriptionSaaSInput      `json:"input"`
	Expected      subscriptionSaaSBenchmarkExpected `json:"expected"`
}

type subscriptionSaaSBenchmarkExpected struct {
	Calculation   domain.SubscriptionSaaSCalculation     `json:"calculation"`
	Profile       domain.ResolvedSubscriptionSaaSProfile `json:"profile"`
	SectionTitles []string                               `json:"section_titles"`
	DOM           subscriptionSaaSBenchmarkDOM           `json:"dom"`
}

type subscriptionSaaSBenchmarkDOM struct {
	RootClass            string   `json:"root_class"`
	Heading              string   `json:"heading"`
	BlockTypes           []string `json:"block_types"`
	SectionNumbers       []int    `json:"section_numbers"`
	SectionTitles        []string `json:"section_titles"`
	CommercialHeaders    []string `json:"commercial_headers"`
	PaymentHeaders       []string `json:"payment_headers"`
	TotalContract        string   `json:"total_contract"`
	RecurringTotal       string   `json:"recurring_total"`
	PaymentNominals      []string `json:"payment_nominals"`
	FeatureCount         int      `json:"feature_count"`
	HasRights            bool     `json:"has_rights"`
	SignatureColumnCount int      `json:"signature_column_count"`
}

func TestSubscriptionSaaSBenchmark(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "subscription_saas")
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), func(t *testing.T) {
			fixture := loadSubscriptionSaaSBenchmarkFixture(t, filepath.Join(fixtureDir, entry.Name()))
			require.Equal(t, "subscription-saas-benchmark-v1", fixture.SchemaVersion)
			require.NotEmpty(t, fixture.Metadata.SourceSheet)
			require.NotEmpty(t, fixture.Metadata.SourceGID)
			require.NotEmpty(t, fixture.Metadata.VerifiedOn)
			require.NotEmpty(t, fixture.Metadata.SourceStatus)
			require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
			require.NotEmpty(t, fixture.Metadata.SourceVariance)

			if strings.Contains(entry.Name(), "annual") {
				require.Equal(t, "structure_only_synthetic_amounts", fixture.Metadata.SourceStatus)
			} else {
				require.Equal(t, "historical_sheet_reference_current_engine_expected", fixture.Metadata.SourceStatus)
			}

			// 1. Validation
			err := fixture.Input.Validate()
			require.NoError(t, err, "validation error")

			// 2. Calculator
			calc, err := CalculateSubscriptionSaaS(fixture.Input)
			require.NoError(t, err, "calculation error")
			assert.Equal(t, fixture.Expected.Calculation, calc, "calculation output")

			// 3. Profile Resolver
			profile, err := ResolveSubscriptionSaaSProfile(fixture.Input)
			require.NoError(t, err, "profile error")
			assert.Equal(t, fixture.Expected.Profile, profile, "profile output")

			// 4. Section Plan
			sections := BuildSubscriptionSectionPlan(profile)
			sectionTitles := make([]string, len(sections))
			for i, s := range sections {
				sectionTitles[i] = s.Title
			}
			assert.Equal(t, fixture.Expected.SectionTitles, sectionTitles, "section plan titles")

			// 5. HTML Renderer
			renderer := NewSubscriptionSaaSHTMLRenderer()
			docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
				Metadata: QuotationMetadata{
					QuotationNumber:   fixture.Metadata.QuotationNumber,
					Subject:           fixture.Metadata.Subject,
					ClientName:        fixture.Metadata.ClientName,
					ClientCompanyName: fixture.Metadata.ClientCompany,
					IssueDate:         fixture.Metadata.IssueDate,
				},
				Input:         fixture.Input,
				Calculation:   calc,
				Profile:       profile,
				FeatureGroups: fixture.Input.FeatureGroups,
				LegalHTML:     fixture.LegalHTML,
			})
			require.NoError(t, err, "render error")

			// 6. Sanitizer
			sanitizer := NewHTMLSanitizer()
			sanitizedHTML, err := sanitizer.Sanitize(docHTML)
			require.NoError(t, err, "sanitization error")
			require.NotEmpty(t, sanitizedHTML, "sanitized html should not be empty")

			// 7. DOM Integrity Validator
			err = ValidateSubscriptionSaaSIntegrity(sanitizedHTML, SubscriptionSaaSIntegrityExpectations{
				Calculation: calc,
				Profile:     profile,
				LegalHTML:   fixture.LegalHTML,
			})
			require.NoError(t, err, "dom integrity validation error")

			// 8. Assert DOM structure and content
			assertSubscriptionSaaSDOM(t, sanitizedHTML, fixture.Expected.DOM)
		})
	}
}

func loadSubscriptionSaaSBenchmarkFixture(t *testing.T, path string) subscriptionSaaSBenchmarkFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture subscriptionSaaSBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func assertSubscriptionSaaSDOM(t *testing.T, document string, expected subscriptionSaaSBenchmarkDOM) {
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
	assert.Equal(t, expected.PaymentHeaders, tableHeadersForBlock(nodes, "payment"), "payment headers")

	// Total contract
	totalCell, err := findExactlyOne(nodes, "data-field", "total_contract")
	require.NoError(t, err)
	require.NotNil(t, totalCell)
	assert.Equal(t, expected.TotalContract, nodeText(totalCell), "total contract in DOM")

	// Payment nominals
	paymentCells := findAllOrdered(nodes, "data-field", "payment_nominal")
	var nominals []string
	for _, cell := range paymentCells {
		nominals = append(nominals, nodeText(cell))
	}
	assert.Equal(t, expected.PaymentNominals, nominals, "payment nominals")

	// Features count
	featuresBlock, err := findExactlyOne(nodes, "data-block-type", "features")
	require.NoError(t, err)
	if expected.FeatureCount > 0 {
		require.NotNil(t, featuresBlock)
		featureLis := findElementsByTag(collectNodes(featuresBlock), "li")
		assert.Equal(t, expected.FeatureCount, len(featureLis), "feature count in DOM")
	} else {
		assert.Nil(t, featuresBlock)
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
