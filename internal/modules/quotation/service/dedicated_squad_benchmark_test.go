package service

import (
	"context"
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

// Dedicated squad benchmark fixtures intentionally use the current seeded
// master card through an in-memory lookup.  This keeps the golden test
// deterministic and makes it independent of PostgreSQL state.
type dedicatedSquadBenchmarkFixture struct {
	SchemaVersion string                     `json:"schema_version"`
	Metadata      benchmarkMetadata          `json:"metadata"`
	RateCard      benchmarkRateCard          `json:"rate_card"`
	Input         domain.DedicatedSquadInput `json:"input"`
	Modules       []domain.Module            `json:"modules,omitempty"`
	Expected      benchmarkExpected          `json:"expected"`
}

type benchmarkMetadata struct {
	SourceSheet     string   `json:"source_sheet"`
	SourceGID       string   `json:"source_gid"`
	VerifiedOn      string   `json:"verified_on"`
	SourceStatus    string   `json:"source_status"`
	SourceVariance  []string `json:"source_variance"`
	VisualParity    string   `json:"visual_parity"`
	QuotationNumber string   `json:"quotation_number"`
	Subject         string   `json:"subject"`
	ClientName      string   `json:"client_name"`
	ClientCompany   string   `json:"client_company"`
	IssueDate       string   `json:"issue_date"`
}

type benchmarkRateCard struct {
	Code   string `json:"code"`
	Market string `json:"market"`
	Source string `json:"source"`
}

type benchmarkExpected struct {
	Calculation   domain.DedicatedSquadCalculation `json:"calculation"`
	Rules         DedicatedSquadRules              `json:"rules"`
	Timeline      []domain.TimelineRow             `json:"timeline"`
	SectionTitles []string                         `json:"section_titles"`
	DOM           benchmarkDOM                     `json:"dom"`
}

type benchmarkDOM struct {
	RootClass            string   `json:"root_class"`
	Heading              string   `json:"heading"`
	BlockTypes           []string `json:"block_types"`
	SectionNumbers       []int    `json:"section_numbers"`
	SectionTitles        []string `json:"section_titles"`
	DevelopmentLineKeys  []string `json:"development_line_keys"`
	MaintenanceLineKeys  []string `json:"maintenance_line_keys"`
	PaymentNominals      []string `json:"payment_nominals"`
	CostHeaders          []string `json:"cost_headers"`
	MaintenanceHeaders   []string `json:"maintenance_headers,omitempty"`
	TimelineHeaders      []string `json:"timeline_headers,omitempty"`
	TimelineColumnWidths []string `json:"timeline_column_widths,omitempty"`
	PaymentHeaders       []string `json:"payment_headers"`
	LegalExclusionCount  int      `json:"legal_exclusion_count"`
	SignatureColumnCount int      `json:"signature_column_count"`
}

type benchmarkRateLookup struct {
	cardCode string
	items    map[string]domain.RateCardItem
}

func (f benchmarkRateLookup) ResolveItem(_ context.Context, _, rateCardID, role, seniority, term string) (*domain.RateCardItem, error) {
	if rateCardID != f.cardCode {
		return nil, fmt.Errorf("rate card %q is not the approved benchmark card %q", rateCardID, f.cardCode)
	}
	item, ok := f.items[role+"|"+seniority+"|"+term]
	if !ok {
		return nil, fmt.Errorf("missing current master rate for %s/%s/%s", role, seniority, term)
	}
	return &item, nil
}

func currentMasterItem(role, seniority, term string, rate, kickoff, recruitment, onboarding int64) domain.RateCardItem {
	return domain.RateCardItem{
		Role: role, Seniority: seniority, TermType: term,
		AmountPerMonth: rate, KickoffFee: kickoff,
		RecruitmentFee: recruitment, OnboardingFee: onboarding,
	}
}

func currentMasterBenchmarkLookup(cardCode string) benchmarkRateLookup {
	items := make(map[string]domain.RateCardItem)
	add := func(role, seniority string, longRate, shortRate, kickoff, recruitment, onboarding int64) {
		items[role+"|"+seniority+"|long_term"] = currentMasterItem(role, seniority, "long_term", longRate, kickoff, recruitment, onboarding)
		items[role+"|"+seniority+"|short_term"] = currentMasterItem(role, seniority, "short_term", shortRate, kickoff, recruitment, onboarding)
	}
	// Values mirror the engineering, analysis, and support formulas in
	// 001_venturo_master_rate_cards.sql for the national standard card.
	add("ui_ux_designer", "senior", 11_550_000, 16_170_000, 11_550_000, 3_000_000, 13_600_000)
	add("ui_ux_designer", "intermediate", 9_550_000, 13_370_000, 9_550_000, 1_500_000, 11_000_000)
	add("business_analyst", "senior", 16_700_000, 23_380_000, 16_700_000, 3_000_000, 13_600_000)
	add("backend_programmer", "senior", 11_550_000, 16_170_000, 11_550_000, 3_000_000, 13_600_000)
	add("backend_programmer", "intermediate", 9_550_000, 13_370_000, 9_550_000, 1_500_000, 11_000_000)
	add("mobile_developer", "senior", 11_550_000, 16_170_000, 11_550_000, 3_000_000, 13_600_000)
	add("functional_tester", "intermediate", 8_000_000, 11_200_000, 8_000_000, 1_500_000, 10_000_000)
	add("devops", "senior", 11_550_000, 16_170_000, 11_550_000, 3_000_000, 13_600_000)
	add("data_analyst", "intermediate", 13_500_000, 18_900_000, 13_500_000, 1_500_000, 11_000_000)
	return benchmarkRateLookup{cardCode: cardCode, items: items}
}

func TestDedicatedSquadBenchmark(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "testdata", "dedicated_squad")
	entries, err := os.ReadDir(fixtureDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), func(t *testing.T) {
			fixture := loadDedicatedSquadBenchmarkFixture(t, filepath.Join(fixtureDir, entry.Name()))
			require.Equal(t, "dedicated-squad-benchmark-v1", fixture.SchemaVersion)
			require.NotEmpty(t, fixture.Metadata.SourceSheet)
			require.NotEmpty(t, fixture.Metadata.SourceGID)
			require.NotEmpty(t, fixture.Metadata.VerifiedOn)
			require.Equal(t, "historical_sheet_reference_current_card_expected", fixture.Metadata.SourceStatus)
			require.Equal(t, "not_claimed", fixture.Metadata.VisualParity)
			require.NotEmpty(t, fixture.Metadata.SourceVariance)
			require.Equal(t, "VENTURO-STD-2026", fixture.RateCard.Code)
			require.Equal(t, "national", fixture.RateCard.Market)
			require.Contains(t, fixture.RateCard.Source, "001_venturo_master_rate_cards.sql")

			calc, err := NewDedicatedSquadCalculator(currentMasterBenchmarkLookup(fixture.RateCard.Code)).Calculate(
				context.Background(), "benchmark-company", fixture.RateCard.Code, fixture.Input,
			)
			require.NoError(t, err)
			assert.Equal(t, fixture.Expected.Calculation, calc, "financial output")

			rules, err := EvaluateDedicatedSquadRules(fixture.Input, calc.DevelopmentSubtotal)
			require.NoError(t, err)
			assert.Equal(t, fixture.Expected.Rules, rules, "rule output")

			timeline, err := GenerateTimeline(fixture.Input, rules, fixture.Modules)
			require.NoError(t, err)
			assert.Equal(t, fixture.Expected.Timeline, timeline, "timeline output")

			sections := BuildSectionPlan(rules)
			sectionTitles := make([]string, len(sections))
			for i, section := range sections {
				sectionTitles[i] = section.Title
			}
			assert.Equal(t, fixture.Expected.SectionTitles, sectionTitles, "section visibility and order")

			document, err := NewDedicatedSquadHTMLRenderer().Render(DedicatedSquadDocumentInput{
				Metadata: QuotationMetadata{
					QuotationNumber:   fixture.Metadata.QuotationNumber,
					Subject:           fixture.Metadata.Subject,
					ClientName:        fixture.Metadata.ClientName,
					ClientCompanyName: fixture.Metadata.ClientCompany,
					IssueDate:         fixture.Metadata.IssueDate,
				},
				Input: fixture.Input, Calculation: calc, Rules: rules, Timeline: timeline,
			})
			require.NoError(t, err)
			assertDedicatedSquadDOM(t, document, fixture.Expected.DOM, calc, timeline)
		})
	}
}

func loadDedicatedSquadBenchmarkFixture(t *testing.T, path string) dedicatedSquadBenchmarkFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var fixture dedicatedSquadBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func assertDedicatedSquadDOM(t *testing.T, document string, expected benchmarkDOM, calc domain.DedicatedSquadCalculation, timeline []domain.TimelineRow) {
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
	assert.Equal(t, expected.BlockTypes, blockTypes)
	assert.Equal(t, expected.SectionNumbers, sectionNumbers)
	assert.Equal(t, expected.SectionTitles, sectionTitles)

	developmentKeys, maintenanceKeys := lineKeysByBlock(nodes)
	if len(expected.DevelopmentLineKeys) == 0 {
		expected.DevelopmentLineKeys = nil
	}
	if len(expected.MaintenanceLineKeys) == 0 {
		expected.MaintenanceLineKeys = nil
	}
	assert.Equal(t, expected.DevelopmentLineKeys, developmentKeys)
	assert.Equal(t, expected.MaintenanceLineKeys, maintenanceKeys)

	var paymentNominals []string
	for _, node := range nodes {
		if attributeValue(node, "data-field") == "payment_nominal" {
			paymentNominals = append(paymentNominals, nodeText(node))
		}
	}
	assert.Equal(t, expected.PaymentNominals, paymentNominals)
	assert.Equal(t, expected.CostHeaders, tableHeadersForBlock(nodes, "commercial"))
	if len(expected.MaintenanceHeaders) > 0 {
		assert.Equal(t, expected.MaintenanceHeaders, tableHeadersForBlock(nodes, "commercial-maintenance"))
	}
	if len(expected.TimelineHeaders) > 0 {
		assert.Equal(t, expected.TimelineHeaders, tableHeadersForBlock(nodes, "timeline"))
		var widths []string
		for _, node := range nodes {
			if value := attributeValue(node, "data-column-width"); value != "" {
				widths = append(widths, value)
			}
		}
		assert.Equal(t, expected.TimelineColumnWidths, widths)
	}
	assert.Equal(t, expected.PaymentHeaders, tableHeadersForBlock(nodes, "payment"))

	for _, line := range append(append([]domain.SquadLineCalculation(nil), calc.Development...), calc.Maintenance...) {
		key := strings.ToLower(line.Role) + "_" + strings.ToLower(line.Seniority)
		rows := findAllOrdered(nodes, "data-role", key)
		require.Len(t, rows, 1, "DOM line %s", key)
		cells := directChildElements(rows[0], "td")
		require.Len(t, cells, 5, "DOM line %s shape", key)
		assert.Equal(t, humanizeLabel(line.Seniority)+" "+humanizeLabel(line.Role), nodeText(cells[0]), "DOM line %s label", key)
		assert.Equal(t, "Rp "+formatNumber(line.AmountPerUnit), nodeText(cells[1]), "DOM line %s rate", key)
		assert.Equal(t, formatQuarters32(line.HeadcountQuarters), nodeText(cells[2]), "DOM line %s employee", key)
		period := formatQuarters32(line.PeriodQuarters)
		if calc.Model == domain.ModelSprint {
			period = fmt.Sprintf("%d", line.WorkdayCount)
		}
		assert.Equal(t, period, nodeText(cells[3]), "DOM line %s period", key)
		assert.Equal(t, "Rp "+formatNumber(line.LineTotal), nodeText(cells[4]), "DOM line %s total", key)
	}

	timelineBlock := findAllOrdered(nodes, "data-block-type", "timeline")
	if len(timeline) == 0 {
		require.Empty(t, timelineBlock)
	} else {
		require.Len(t, timelineBlock, 1)
		rows := bodyRows(timelineBlock[0])
		require.Len(t, rows, len(timeline))
		for i, want := range timeline {
			cells := directChildElements(rows[i], "td")
			require.Len(t, cells, 2, "DOM timeline %d shape", i)
			strong := findElementsByTag(collectNodes(cells[0]), "strong")
			require.Len(t, strong, 1, "DOM timeline %d description", i)
			assert.Equal(t, want.Description, nodeText(strong[0]))
			assert.Equal(t, want.Timeline, nodeText(cells[1]))
			lists := findElementsByTag(collectNodes(cells[0]), "ul")
			require.Len(t, lists, 1, "DOM timeline %d deliverables list", i)
			items := directChildElements(lists[0], "li")
			require.Len(t, items, len(want.Deliverables), "DOM timeline %d deliverable count", i)
			for j, deliverable := range want.Deliverables {
				assert.Equal(t, deliverable, nodeText(items[j]))
			}
		}
	}

	paymentBlock := findAllOrdered(nodes, "data-block-type", "payment")
	require.Len(t, paymentBlock, 1)
	paymentRows := bodyRows(paymentBlock[0])
	require.Len(t, paymentRows, len(calc.Payments))
	for i, row := range calc.Payments {
		cells := directChildElements(paymentRows[i], "td")
		require.Len(t, cells, 5, "DOM payment %d shape", i)
		assert.Equal(t, row.WorkPeriod, nodeText(cells[0]))
		assert.Equal(t, row.Terms, nodeText(cells[1]))
		assert.Equal(t, formatIndonesianDate(row.InvoiceDate), nodeText(cells[2]))
		assert.Equal(t, formatIndonesianDate(row.DueDate), nodeText(cells[3]))
		assert.Equal(t, "Rp "+formatNumber(row.Nominal), nodeText(cells[4]))
	}

	for field, amount := range map[string]int64{
		"total":            calc.Total,
		"discount":         calc.Discount,
		"total_investment": calc.TotalInvestment,
	} {
		matches := findAllOrdered(nodes, "data-field", field)
		require.Len(t, matches, 1, "DOM field %s", field)
		assert.Equal(t, "Rp "+formatNumber(amount), nodeText(matches[0]), "DOM field %s", field)
	}
	if calc.Preparation.Gross > 0 {
		matches := findAllOrdered(nodes, "data-field", "preparation")
		require.Len(t, matches, 1)
		assert.Contains(t, nodeText(matches[0]), formatNumber(calc.Preparation.Gross))
	}

	legal := findAllOrdered(nodes, "data-field", "exclusions")
	require.Len(t, legal, 1)
	assert.Equal(t, expected.LegalExclusionCount, len(directChildElements(legal[0], "li")))
	for _, node := range nodes {
		if attributeValue(node, "data-block-type") != "signature" {
			continue
		}
		rows := collectNodes(node)
		for _, row := range rows {
			if row.Type == html.ElementNode && row.Data == "tr" {
				assert.Equal(t, expected.SignatureColumnCount, len(directChildElements(row, "td")))
				return
			}
		}
	}
	t.Errorf("signature row not found")
}

func findElementsByTag(nodes []*html.Node, tag string) []*html.Node {
	result := make([]*html.Node, 0)
	for _, node := range nodes {
		if node.Type == html.ElementNode && node.Data == tag {
			result = append(result, node)
		}
	}
	return result
}

func bodyRows(block *html.Node) []*html.Node {
	for _, node := range collectNodes(block) {
		if node.Type == html.ElementNode && node.Data == "tbody" {
			return directChildElements(node, "tr")
		}
	}
	return nil
}

func lineKeysByBlock(nodes []*html.Node) (development, maintenance []string) {
	for _, node := range nodes {
		key := attributeValue(node, "data-role")
		if key == "" {
			continue
		}
		block := node
		for block != nil && !(block.Type == html.ElementNode && block.Data == "section") {
			block = block.Parent
		}
		switch attributeValue(block, "data-block-type") {
		case "commercial":
			development = append(development, key)
		case "commercial-maintenance":
			maintenance = append(maintenance, key)
		}
	}
	return development, maintenance
}

func tableHeadersForBlock(nodes []*html.Node, blockType string) []string {
	for _, node := range nodes {
		if attributeValue(node, "data-block-type") != blockType {
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
