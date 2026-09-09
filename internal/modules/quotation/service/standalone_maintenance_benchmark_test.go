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

type standaloneMaintenanceBenchmarkFixture struct {
	SchemaVersion string                                 `json:"schema_version"`
	Metadata      benchmarkMetadata                      `json:"metadata"`
	LegalHTML     string                                 `json:"legal_html"`
	Input         domain.StandaloneMaintenanceInput      `json:"input"`
	Expected      standaloneMaintenanceBenchmarkExpected `json:"expected"`
}

type standaloneMaintenanceBenchmarkExpected struct {
	Calculation   domain.StandaloneMaintenanceCalculation     `json:"calculation"`
	Profile       domain.ResolvedStandaloneMaintenanceProfile `json:"profile"`
	SectionTitles []string                                    `json:"section_titles"`
	DOM           standaloneMaintenanceBenchmarkDOM           `json:"dom"`
}

type standaloneMaintenanceBenchmarkDOM struct {
	RootClass            string   `json:"root_class"`
	Heading              string   `json:"heading"`
	BlockTypes           []string `json:"block_types"`
	SectionNumbers       []int    `json:"section_numbers"`
	SectionTitles        []string `json:"section_titles"`
	CommercialHeaders    []string `json:"commercial_headers"`
	ScopeHeaders         []string `json:"scope_headers"`
	PaymentHeaders       []string `json:"payment_headers"`
	CostSubtotal         string   `json:"cost_subtotal,omitempty"`
	DiscountAmount       string   `json:"discount_amount,omitempty"`
	TotalInvestment      string   `json:"total_investment"`
	PaymentTotal         string   `json:"payment_total"`
	PaymentNominals      []string `json:"payment_nominals"`
	DefectGroupCount     int      `json:"defect_group_count,omitempty"`
	DefectItemCount      int      `json:"defect_item_count,omitempty"`
	TaskCount            int      `json:"task_count,omitempty"`
	HasRights            bool     `json:"has_rights"`
	SignatureColumnCount int      `json:"signature_column_count"`
}

func TestStandaloneMaintenanceBenchmark(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "standalone_maintenance")
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), func(t *testing.T) {
			fixture := loadStandaloneMaintenanceBenchmarkFixture(t, filepath.Join(fixtureDir, entry.Name()))
			require.Equal(t, "standalone-maintenance-benchmark-v1", fixture.SchemaVersion)
			require.NotEmpty(t, fixture.Metadata.SourceSheet)
			require.NotEmpty(t, fixture.Metadata.SourceGID)
			require.NotEmpty(t, fixture.Metadata.VerifiedOn)
			require.NotEmpty(t, fixture.Metadata.SourceStatus)
			require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
			require.NotEmpty(t, fixture.Metadata.SourceVariance)

			if strings.Contains(entry.Name(), "nex") {
				require.Equal(t, "blueprint_excerpt_with_workbook_costs", fixture.Metadata.SourceStatus)
			} else {
				require.Equal(t, "historical_sheet_reference_current_engine_expected", fixture.Metadata.SourceStatus)
			}

			// 1. Strict Validation
			err := fixture.Input.Validate()
			require.NoError(t, err, "validation error")

			// 2. Calculator
			calc, err := CalculateStandaloneMaintenance(fixture.Input)
			require.NoError(t, err, "calculation error")
			assert.Equal(t, fixture.Expected.Calculation, calc, "calculation output")
			assert.Equal(t, calc.TotalInvestment, calc.PaymentTotal, "payment total must equal total investment")

			// 3. Profile Resolver
			profile := ResolveStandaloneMaintenanceProfile(fixture.Input)
			assert.Equal(t, fixture.Expected.Profile, profile, "profile output")

			// 4. Section Plan
			sectionTitles := make([]string, len(profile.Sections))
			for i, s := range profile.Sections {
				sectionTitles[i] = s.Title
			}
			assert.Equal(t, fixture.Expected.SectionTitles, sectionTitles, "section plan titles")

			// 5. HTML Renderer
			renderer := NewStandaloneMaintenanceHTMLRenderer()
			docHTML, err := renderer.Render(StandaloneMaintenanceDocumentInput{
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
			err = ValidateStandaloneMaintenanceIntegrity(sanitizedHTML, fixture.Input, calc, profile, fixture.LegalHTML)
			require.NoError(t, err, "dom integrity validation error")

			// 8. Assert DOM structure and content
			assertStandaloneMaintenanceDOM(t, sanitizedHTML, fixture.Input, calc, fixture.Expected.DOM)
		})
	}
}

func TestStandaloneMaintenanceBenchmark_BlankReferenceBehavior(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	nexPath := filepath.Join(filepath.Dir(sourceFile), "testdata", "standalone_maintenance", "nex-digital-health.json")
	fixture := loadStandaloneMaintenanceBenchmarkFixture(t, nexPath)

	// Set one reference to blank
	fixture.Input.DefectGroups[0].Items[0].Reference = ""

	calc, err := CalculateStandaloneMaintenance(fixture.Input)
	require.NoError(t, err)

	profile := ResolveStandaloneMaintenanceProfile(fixture.Input)
	renderer := NewStandaloneMaintenanceHTMLRenderer()
	docHTML, err := renderer.Render(StandaloneMaintenanceDocumentInput{
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
	require.NoError(t, err)

	sanitizer := NewHTMLSanitizer()
	sanitizedHTML, err := sanitizer.Sanitize(docHTML)
	require.NoError(t, err)

	// Integrity validation passes with blank reference
	err = ValidateStandaloneMaintenanceIntegrity(sanitizedHTML, fixture.Input, calc, profile, fixture.LegalHTML)
	require.NoError(t, err)

	// Verify blank reference produces an empty cell, not missing cell or "undefined"
	root, err := html.Parse(strings.NewReader(sanitizedHTML))
	require.NoError(t, err)
	nodes := collectNodes(root)
	row, err := findExactlyOne(nodes, "data-role", "defect_0_0")
	require.NoError(t, err)
	require.NotNil(t, row)
	cells := directChildElements(row, "td")
	require.Len(t, cells, 6)
	assert.Empty(t, strings.TrimSpace(nodeText(cells[5])), "reference cell should be blank")
}

func loadStandaloneMaintenanceBenchmarkFixture(t *testing.T, path string) standaloneMaintenanceBenchmarkFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture standaloneMaintenanceBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func assertStandaloneMaintenanceDOM(
	t *testing.T,
	document string,
	input domain.StandaloneMaintenanceInput,
	calc domain.StandaloneMaintenanceCalculation,
	expected standaloneMaintenanceBenchmarkDOM,
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

	// Sections and block types
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

	// Table headers
	assert.Equal(t, expected.CommercialHeaders, tableHeadersForBlock(nodes, "commercial"), "commercial headers")
	assert.Equal(t, expected.ScopeHeaders, tableHeadersForBlock(nodes, "scope"), "scope headers")
	assert.Equal(t, expected.PaymentHeaders, tableHeadersForBlock(nodes, "payment"), "payment headers")

	// Commercial figures
	if expected.CostSubtotal != "" {
		subtotalCell, err := findExactlyOne(nodes, "data-field", "cost_subtotal")
		require.NoError(t, err)
		require.NotNil(t, subtotalCell)
		assert.Equal(t, expected.CostSubtotal, nodeText(subtotalCell), "cost subtotal")
	}
	if expected.DiscountAmount != "" {
		discountCell, err := findExactlyOne(nodes, "data-field", "discount_amount")
		require.NoError(t, err)
		require.NotNil(t, discountCell)
		assert.Equal(t, expected.DiscountAmount, nodeText(discountCell), "discount amount")
	}

	totalInvestmentCell, err := findExactlyOne(nodes, "data-field", "total_investment")
	require.NoError(t, err)
	require.NotNil(t, totalInvestmentCell)
	assert.Equal(t, expected.TotalInvestment, nodeText(totalInvestmentCell), "total investment")

	paymentTotalCell, err := findExactlyOne(nodes, "data-field", "payment_total")
	require.NoError(t, err)
	require.NotNil(t, paymentTotalCell)
	assert.Equal(t, expected.PaymentTotal, nodeText(paymentTotalCell), "payment total")

	// Payment nominals
	paymentCells := findAllOrdered(nodes, "data-field", "payment_nominal")
	var nominals []string
	for _, cell := range paymentCells {
		nominals = append(nominals, nodeText(cell))
	}
	assert.Equal(t, expected.PaymentNominals, nominals, "payment nominals")

	// Mode-specific scope assertions
	scopeBlock, err := findExactlyOne(nodes, "data-block-type", "scope")
	require.NoError(t, err)
	require.NotNil(t, scopeBlock)
	scopeNodes := collectNodes(scopeBlock)

	switch input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		groupCount := 0
		itemCount := 0
		for _, n := range scopeNodes {
			role := attributeValue(n, "data-role")
			if strings.HasPrefix(role, "defect_group_") {
				groupCount++
			} else if strings.HasPrefix(role, "defect_") {
				itemCount++
			}
		}
		assert.Equal(t, expected.DefectGroupCount, groupCount, "defect group count")
		assert.Equal(t, expected.DefectItemCount, itemCount, "defect item count")

		// Verify individual defect rows in order
		for gIdx, group := range input.DefectGroups {
			for iIdx, item := range group.Items {
				row, err := findExactlyOne(scopeNodes, "data-role", fmt.Sprintf("defect_%d_%d", gIdx, iIdx))
				require.NoError(t, err)
				require.NotNil(t, row)
				cells := directChildElements(row, "td")
				require.Len(t, cells, 6)
				assert.Contains(t, nodeText(cells[0]), item.ItemNumber)
				assert.Contains(t, nodeText(cells[0]), string(item.Priority))
				assert.Equal(t, item.Function, nodeText(cells[1]))
				assert.Equal(t, item.ScreenFlow, nodeText(cells[2]))
				assert.Equal(t, item.Existing, nodeText(cells[3]))
				assert.Equal(t, item.Improvement, nodeText(cells[4]))
				assert.Equal(t, item.Reference, strings.TrimSpace(nodeText(cells[5])))
			}
		}

	case domain.StandaloneMaintenanceHourlyTask:
		taskCount := 0
		for _, n := range scopeNodes {
			role := attributeValue(n, "data-role")
			if strings.HasPrefix(role, "hourly_task_") {
				taskCount++
			}
		}
		assert.Equal(t, expected.TaskCount, taskCount, "hourly task count")

		// Verify individual task rows in order
		for i, task := range input.HourlyTasks {
			row, err := findExactlyOne(scopeNodes, "data-role", fmt.Sprintf("hourly_task_%d", i))
			require.NoError(t, err)
			require.NotNil(t, row)
			cells := directChildElements(row, "td")
			require.Len(t, cells, 3)
			assert.Equal(t, task.Description, nodeText(cells[0]))
			assert.Equal(t, task.Platform, nodeText(cells[1]))
			assert.Equal(t, formatHoursHalfUnits(task.HoursHalfUnits), nodeText(cells[2]))
		}
	}

	// Rights block presence / absence
	rightsBlock, err := findExactlyOne(nodes, "data-block-type", "rights")
	require.NoError(t, err)
	if expected.HasRights {
		assert.NotNil(t, rightsBlock, "expected rights block")
	} else {
		assert.Nil(t, rightsBlock, "expected no rights block")
	}

	// Legal marker
	legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
	require.NoError(t, err)
	require.NotNil(t, legalBlock, "legal block must be present")

	// Signature two-column layout
	sigBlock, err := findExactlyOne(nodes, "data-block-type", "signature")
	require.NoError(t, err)
	require.NotNil(t, sigBlock)
	sigTables := findElementsByTag(collectNodes(sigBlock), "table")
	require.NotEmpty(t, sigTables)
	sigTds := findElementsByTag(collectNodes(sigTables[0]), "td")
	assert.Equal(t, expected.SignatureColumnCount, len(sigTds), "signature column count")

	// Absence of forbidden terms in document
	fullText := nodeText(root)
	assert.NotContains(t, fullText, "Hiring Fee")
	assert.NotContains(t, fullText, "Engineering Kick-off")
	assert.NotContains(t, fullText, "Gantt")
	assert.NotContains(t, fullText, "Biaya Persiapan")
}
