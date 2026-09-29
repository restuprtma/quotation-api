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

func TestDedicatedSquadHTMLRendererModels(t *testing.T) {
	models := []struct {
		name, model                  string
		rules                        DedicatedSquadRules
		daily, timeline, maintenance bool
		sections                     int
	}{
		{"1A", "1A", DedicatedSquadRules{IncludeTimeline: true, IncludeMaintenance: true}, false, true, true, 5},
		{"1B", "1B", DedicatedSquadRules{IncludeTimeline: true}, false, true, false, 4},
		{"1C", "1C", DedicatedSquadRules{}, false, false, false, 3},
		{"1D", "1D", DedicatedSquadRules{IncludeTimeline: true}, true, true, false, 4},
	}
	for _, tt := range models {
		t.Run(tt.name, func(t *testing.T) {
			in := dedicatedRenderFixture(domain.SquadModel(tt.model), tt.rules)
			got, err := NewDedicatedSquadHTMLRenderer().Render(in)
			require.NoError(t, err)
			assertCanonicalOfficialHeader(t, got, "PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD")
			root, err := html.Parse(strings.NewReader(got))
			require.NoError(t, err)
			nodes := collectNodes(root)
			assert.Contains(t, nodeText(root), "PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD")
			assert.Contains(t, nodeText(root), DedicatedSquadOpeningParagraph)
			assert.Equal(t, tt.sections, countAttr(nodes, "data-section-number"))
			for i := 1; i <= tt.sections; i++ {
				assert.Equal(t, 1, countAttrValue(nodes, "data-section-number", string(rune('0'+i))))
			}
			assert.Equal(t, tt.timeline, countAttrValue(nodes, "data-block-type", "timeline") == 1)
			assert.Equal(t, tt.maintenance, countAttrValue(nodes, "data-block-type", "commercial-maintenance") == 1)
			assert.Equal(t, tt.daily, strings.Contains(got, "Rate (Hari)"))
			if !tt.daily {
				assert.Contains(t, got, "Rate (Monthly)")
			}
			assert.Contains(t, got, `data-field="total"`)
			assert.Contains(t, got, `data-field="discount"`)
			assert.Contains(t, got, `data-field="total_investment"`)
			assert.Contains(t, got, "H+14 (empat belas) hari kalender")
			assert.Contains(t, got, "belum termasuk PPN")
			assert.Contains(t, got, "penghentian sementara pekerjaan")
			assert.Equal(t, 4, exclusionCount(nodes))
			assert.Contains(t, got, "Yulianto Frandi")
			assert.Contains(t, got, "Budi &lt;script&gt;")
			assert.NotContains(t, got, "<script>")
			assert.Equal(t, 5, paymentHeaderCount(nodes))
			if tt.timeline {
				assert.Equal(t, 1, countAttrValue(nodes, "data-column-width", "75"))
				assert.Equal(t, 1, countAttrValue(nodes, "data-column-width", "25"))
			}
		})
	}
}

func TestDedicatedSquadHTMLRenderer_PreservesHistoricalRoleSnapshot(t *testing.T) {
	in := dedicatedRenderFixture(domain.ModelSOWSmall, DedicatedSquadRules{IncludeTimeline: true})
	in.Calculation.Development[0].Role = "frontend_programmer"

	got, err := NewDedicatedSquadHTMLRenderer().Render(in)

	require.NoError(t, err)
	assert.Contains(t, got, "Frontend Programmer")
	assert.NotContains(t, got, "Backend Programmer")
}

func dedicatedRenderFixture(model domain.SquadModel, rules DedicatedSquadRules) DedicatedSquadDocumentInput {
	date := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	line := domain.SquadLineCalculation{SquadLineInput: domain.SquadLineInput{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}, AmountPerUnit: 10_000_000, LineTotal: 10_000_000}
	if model == domain.ModelSprint {
		line.PeriodQuarters, line.WorkdayCount, line.AmountPerUnit, line.LineTotal = 0, 4, 500_000, 2_000_000
	}
	calc := domain.DedicatedSquadCalculation{Model: model, Development: []domain.SquadLineCalculation{line}, DevelopmentSubtotal: line.LineTotal, Total: line.LineTotal, Discount: 0, TotalInvestment: line.LineTotal, Payments: []domain.PaymentRow{{WorkPeriod: "Penyelesaian", Terms: "Pelunasan", InvoiceDate: date, DueDate: date.AddDate(0, 0, 14), Nominal: line.LineTotal}}}
	if rules.IncludeMaintenance {
		calc.Maintenance = []domain.SquadLineCalculation{line}
		calc.MaintenanceSubtotal = line.LineTotal
	}
	return DedicatedSquadDocumentInput{Metadata: QuotationMetadata{QuotationNumber: "Q-001", Subject: "Portal <b>", ClientName: "Budi <script>", ClientCompanyName: "PT Aman & Makmur", IssueDate: "4 September 2026"}, Calculation: calc, Rules: rules, Timeline: []domain.TimelineRow{{Description: "Development <unsafe>", Timeline: "Bulan 1", Deliverables: []string{"Modul <img>"}}}}
}

func countAttr(nodes []*html.Node, key string) int {
	n := 0
	for _, node := range nodes {
		if attributeValue(node, key) != "" {
			n++
		}
	}
	return n
}
func countAttrValue(nodes []*html.Node, key, value string) int {
	n := 0
	for _, node := range nodes {
		if attributeValue(node, key) == value {
			n++
		}
	}
	return n
}
func exclusionCount(nodes []*html.Node) int {
	for _, node := range nodes {
		if attributeValue(node, "data-field") == "exclusions" {
			return len(directChildElements(node, "li"))
		}
	}
	return 0
}
func paymentHeaderCount(nodes []*html.Node) int {
	for _, node := range nodes {
		if attributeValue(node, "data-block-type") == "payment" {
			for _, child := range collectNodes(node) {
				if child.Type == html.ElementNode && child.Data == "tr" && child.Parent != nil && child.Parent.Data == "thead" {
					return len(directChildElements(child, "th"))
				}
			}
		}
	}
	return 0
}
