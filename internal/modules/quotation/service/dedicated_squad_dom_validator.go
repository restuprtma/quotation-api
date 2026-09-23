package service

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type DedicatedSquadIntegrityExpectations struct {
	Input       domain.DedicatedSquadInput
	Calculation domain.DedicatedSquadCalculation
	Profile     ResolvedQuotationProfile
	Rules       DedicatedSquadRules
}

// ValidateDedicatedSquadIntegrity performs end-to-end integrity verification
// of modern dedicated squad rendered HTML for both generation and unmodified checks.
func ValidateDedicatedSquadIntegrity(documentHTML string, expected DedicatedSquadIntegrityExpectations) error {
	if err := ValidateDedicatedSquadStructureForDraft(documentHTML, expected); err != nil {
		return err
	}
	commMod, err := CheckDedicatedSquadCommercialModified(documentHTML, expected)
	if err != nil {
		return err
	}
	if commMod {
		return fmt.Errorf("%w: commercial content was modified", ErrUnsafeHTML)
	}
	legalMod, err := CheckDedicatedSquadLegalModified(documentHTML, expected)
	if err != nil {
		return err
	}
	if legalMod {
		return fmt.Errorf("%w: legal content was modified", ErrUnsafeHTML)
	}
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	if err := scanForbiddenTerms(collectNodes(root)); err != nil {
		return err
	}
	return nil
}

// ValidateDedicatedSquadStructureForDraft validates the immutable structural skeleton
// of a dedicated squad document, ensuring all required sections, 5-column tables,
// rows, and markers exist and are unique according to rules and calculation.
func ValidateDedicatedSquadStructureForDraft(documentHTML string, expected DedicatedSquadIntegrityExpectations) error {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	if err := validateOfficialHeaderFields(root); err != nil {
		return err
	}
	nodes := collectNodes(root)

	// Verify root article element
	var article *html.Node
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "article" && strings.Contains(attributeValue(n, "class"), "quotation-document") {
			article = n
			break
		}
	}
	if article == nil {
		return fmt.Errorf("%w: quotation document root article is missing", ErrUnsafeHTML)
	}

	// Verify metadata, opening, and signature blocks
	for _, bType := range []string{"metadata", "opening", "signature"} {
		block, err := findExactlyOne(nodes, "data-block-type", bType)
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: %s block is missing", ErrUnsafeHTML, bType)
		}
	}

	// Check section numbers for duplicates
	seenSectionNumbers := make(map[string]string)
	for _, n := range nodes {
		if num := attributeValue(n, "data-section-number"); num != "" {
			bType := attributeValue(n, "data-block-type")
			if prev, exists := seenSectionNumbers[num]; exists {
				return fmt.Errorf("%w: duplicate section number %s on %q and %q", ErrUnsafeHTML, num, prev, bType)
			}
			seenSectionNumbers[num] = bType
		}
	}

	// Verify plan sections
	sections := BuildSectionPlan(expected.Rules)
	hasMaintenanceSection := false
	hasTimelineSection := false
	for _, sec := range sections {
		var bType string
		switch sec.Code {
		case "development":
			bType = "commercial"
		case "maintenance":
			bType = "commercial-maintenance"
			hasMaintenanceSection = true
		case "timeline":
			bType = "timeline"
			hasTimelineSection = true
		case "payment":
			bType = "payment"
		case "limitations":
			bType = "legal"
		default:
			return fmt.Errorf("%w: unknown section code in plan: %s", ErrUnsafeHTML, sec.Code)
		}

		block, err := findExactlyOne(nodes, "data-block-type", bType)
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: %s section (%s) is missing", ErrUnsafeHTML, bType, sec.Code)
		}
		expectedNum := strconv.Itoa(sec.Number)
		if gotNum := attributeValue(block, "data-section-number"); gotNum != expectedNum {
			return fmt.Errorf("%w: section %s number mismatch: got %q, want %q", ErrUnsafeHTML, bType, gotNum, expectedNum)
		}
	}

	// Disallow unexpected sections
	if !hasMaintenanceSection {
		block, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance")
		if err != nil {
			return err
		}
		if block != nil {
			return fmt.Errorf("%w: unexpected commercial-maintenance section", ErrUnsafeHTML)
		}
	}
	if !hasTimelineSection {
		block, err := findExactlyOne(nodes, "data-block-type", "timeline")
		if err != nil {
			return err
		}
		if block != nil {
			return fmt.Errorf("%w: unexpected timeline section", ErrUnsafeHTML)
		}
	}

	// 1. Commercial section validation
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || commBlock == nil {
		return fmt.Errorf("%w: commercial section is missing or duplicate", ErrUnsafeHTML)
	}
	commNodes := collectNodes(commBlock)

	// 5-column table headers
	thCount := 0
	for _, n := range commNodes {
		if n.Type == html.ElementNode && n.Data == "th" {
			thCount++
		}
	}
	if thCount != 5 {
		return fmt.Errorf("%w: commercial table header must have exactly 5 columns, got %d", ErrUnsafeHTML, thCount)
	}

	// Development lines
	expectedDevLines := len(expected.Calculation.Development)
	if expected.Rules.MaintenanceEmbedded {
		expectedDevLines += len(expected.Calculation.Maintenance)
	}
	devLineTotals := findAllOrdered(commNodes, "data-field", "line_total")
	if len(devLineTotals) != expectedDevLines {
		return fmt.Errorf("%w: commercial table line total count mismatch: got %d, want %d", ErrUnsafeHTML, len(devLineTotals), expectedDevLines)
	}
	for i, lt := range devLineTotals {
		row := lt.Parent
		if row == nil || row.Data != "tr" {
			return fmt.Errorf("%w: line_total[%d] must be inside a tr", ErrUnsafeHTML, i)
		}
		cells := directChildElements(row, "td")
		if len(cells) != 5 {
			return fmt.Errorf("%w: commercial row %d must have exactly 5 td cells, got %d", ErrUnsafeHTML, i, len(cells))
		}
	}

	// Preparation row
	prepRow, err := findExactlyOne(commNodes, "data-field", "preparation")
	if err != nil {
		return err
	}
	if expected.Calculation.Preparation.Gross > 0 {
		if prepRow == nil {
			return fmt.Errorf("%w: preparation row is missing", ErrUnsafeHTML)
		}
		cells := directChildElements(prepRow, "td")
		if len(cells) != 5 {
			return fmt.Errorf("%w: preparation row must have 5 td cells, got %d", ErrUnsafeHTML, len(cells))
		}
	} else if prepRow != nil {
		return fmt.Errorf("%w: unexpected preparation row", ErrUnsafeHTML)
	}

	// Total, Discount, TotalInvestment fields
	for _, field := range []string{"total", "discount", "total_investment"} {
		cell, err := findExactlyOne(commNodes, "data-field", field)
		if err != nil {
			return err
		}
		if cell == nil {
			return fmt.Errorf("%w: %s field is missing in commercial section", ErrUnsafeHTML, field)
		}
	}

	// 2. Separate maintenance section validation
	if hasMaintenanceSection {
		maintBlock, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance")
		if err != nil || maintBlock == nil {
			return fmt.Errorf("%w: commercial-maintenance section is missing or duplicate", ErrUnsafeHTML)
		}
		maintNodes := collectNodes(maintBlock)
		maintTHs := 0
		for _, n := range maintNodes {
			if n.Type == html.ElementNode && n.Data == "th" {
				maintTHs++
			}
		}
		if maintTHs != 5 {
			return fmt.Errorf("%w: maintenance table header must have exactly 5 columns, got %d", ErrUnsafeHTML, maintTHs)
		}
		maintLineTotals := findAllOrdered(maintNodes, "data-field", "line_total")
		if len(maintLineTotals) != len(expected.Calculation.Maintenance) {
			return fmt.Errorf("%w: maintenance line total count mismatch: got %d, want %d", ErrUnsafeHTML, len(maintLineTotals), len(expected.Calculation.Maintenance))
		}
		for i, lt := range maintLineTotals {
			row := lt.Parent
			if row == nil || row.Data != "tr" {
				return fmt.Errorf("%w: maintenance line_total[%d] must be inside a tr", ErrUnsafeHTML, i)
			}
			cells := directChildElements(row, "td")
			if len(cells) != 5 {
				return fmt.Errorf("%w: maintenance row %d must have exactly 5 td cells, got %d", ErrUnsafeHTML, i, len(cells))
			}
		}
		subtotalCell, err := findExactlyOne(maintNodes, "data-field", "maintenance_subtotal")
		if err != nil || subtotalCell == nil {
			return fmt.Errorf("%w: maintenance_subtotal field is missing in maintenance section", ErrUnsafeHTML)
		}
	}

	// 3. Payment section validation
	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil || payBlock == nil {
		return fmt.Errorf("%w: payment section is missing or duplicate", ErrUnsafeHTML)
	}
	payNodes := collectNodes(payBlock)
	payTHs := 0
	for _, n := range payNodes {
		if n.Type == html.ElementNode && n.Data == "th" {
			payTHs++
		}
	}
	if payTHs != 5 {
		return fmt.Errorf("%w: payment table header must have exactly 5 columns, got %d", ErrUnsafeHTML, payTHs)
	}
	nominalCells := findAllOrdered(payNodes, "data-field", "payment_nominal")
	if len(nominalCells) != len(expected.Calculation.Payments) {
		return fmt.Errorf("%w: payment nominal count mismatch: got %d, want %d", ErrUnsafeHTML, len(nominalCells), len(expected.Calculation.Payments))
	}
	for i, nc := range nominalCells {
		row := nc.Parent
		if row == nil || row.Data != "tr" {
			return fmt.Errorf("%w: payment_nominal[%d] must be inside a tr", ErrUnsafeHTML, i)
		}
		cells := directChildElements(row, "td")
		if len(cells) != 5 {
			return fmt.Errorf("%w: payment row %d must have exactly 5 td cells, got %d", ErrUnsafeHTML, i, len(cells))
		}
	}

	// 4. Legal section validation
	legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
	if err != nil || legalBlock == nil {
		return fmt.Errorf("%w: legal section is missing or duplicate", ErrUnsafeHTML)
	}
	legalNodes := collectNodes(legalBlock)
	exclOl, err := findExactlyOne(legalNodes, "data-field", "exclusions")
	if err != nil || exclOl == nil {
		return fmt.Errorf("%w: exclusions list is missing or duplicate", ErrUnsafeHTML)
	}
	if exclOl.Data != "ol" {
		return fmt.Errorf("%w: exclusions marker must be an ol", ErrUnsafeHTML)
	}

	// Global uniqueness check on key data markers
	for _, field := range []string{"total", "discount", "total_investment", "exclusions"} {
		matches := findAllOrdered(nodes, "data-field", field)
		if len(matches) != 1 {
			return fmt.Errorf("%w: marker data-field=%q must appear exactly once, got %d", ErrUnsafeHTML, field, len(matches))
		}
	}
	if expected.Calculation.Preparation.Gross > 0 {
		matches := findAllOrdered(nodes, "data-field", "preparation")
		if len(matches) != 1 {
			return fmt.Errorf("%w: marker data-field=\"preparation\" must appear exactly once, got %d", ErrUnsafeHTML, len(matches))
		}
	}
	if hasMaintenanceSection {
		matches := findAllOrdered(nodes, "data-field", "maintenance_subtotal")
		if len(matches) != 1 {
			return fmt.Errorf("%w: marker data-field=\"maintenance_subtotal\" must appear exactly once, got %d", ErrUnsafeHTML, len(matches))
		}
	}

	return nil
}

// CheckDedicatedSquadCommercialModified checks whether commercial amounts or line data
// differ from expected snapshot calculation.
func CheckDedicatedSquadCommercialModified(documentHTML string, expected DedicatedSquadIntegrityExpectations) (bool, error) {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// Total Investment
	totInv, err := findExactlyOne(nodes, "data-field", "total_investment")
	if err != nil || totInv == nil {
		return true, nil
	}
	wantTotalInvestment := "Rp " + formatNumber(expected.Calculation.TotalInvestment)
	if nodeText(totInv) != wantTotalInvestment {
		return true, nil
	}

	// Total
	tot, err := findExactlyOne(nodes, "data-field", "total")
	if err != nil || tot == nil {
		return true, nil
	}
	wantTotal := "Rp " + formatNumber(expected.Calculation.Total)
	if nodeText(tot) != wantTotal {
		return true, nil
	}

	// Discount
	disc, err := findExactlyOne(nodes, "data-field", "discount")
	if err != nil || disc == nil {
		return true, nil
	}
	wantDiscount := "Rp " + formatNumber(expected.Calculation.Discount)
	if nodeText(disc) != wantDiscount {
		return true, nil
	}

	// Development lines
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || commBlock == nil {
		return true, nil
	}
	commNodes := collectNodes(commBlock)
	lineTotals := findAllOrdered(commNodes, "data-field", "line_total")

	daily := expected.Calculation.Model == domain.ModelSprint
	idx := 0
	for _, line := range expected.Calculation.Development {
		if idx >= len(lineTotals) {
			return true, nil
		}
		if nodeText(lineTotals[idx]) != "Rp "+formatNumber(line.LineTotal) {
			return true, nil
		}
		row := lineTotals[idx].Parent
		if row != nil {
			tds := directChildElements(row, "td")
			if len(tds) == 5 {
				wantRate := "Rp " + formatNumber(line.AmountPerUnit)
				wantHeadcount := formatQuarters32(line.HeadcountQuarters)
				wantPeriod := formatQuarters32(line.PeriodQuarters)
				if daily {
					wantPeriod = fmt.Sprintf("%d", line.WorkdayCount)
				}
				if nodeText(tds[1]) != wantRate || nodeText(tds[2]) != wantHeadcount || nodeText(tds[3]) != wantPeriod {
					return true, nil
				}
			}
		}
		idx++
	}

	// Embedded maintenance lines
	if expected.Rules.MaintenanceEmbedded {
		for _, line := range expected.Calculation.Maintenance {
			if idx >= len(lineTotals) {
				return true, nil
			}
			if nodeText(lineTotals[idx]) != "Rp "+formatNumber(line.LineTotal) {
				return true, nil
			}
			row := lineTotals[idx].Parent
			if row != nil {
				tds := directChildElements(row, "td")
				if len(tds) == 5 {
					wantRate := "Rp " + formatNumber(line.AmountPerUnit)
					wantHeadcount := formatQuarters32(line.HeadcountQuarters)
					wantPeriod := formatQuarters32(line.PeriodQuarters)
					if nodeText(tds[1]) != wantRate || nodeText(tds[2]) != wantHeadcount || nodeText(tds[3]) != wantPeriod {
						return true, nil
					}
				}
			}
			idx++
		}
	}

	// Separate maintenance section
	if expected.Rules.IncludeMaintenanceSection || (expected.Rules.Profile.PreparationKind == "" && expected.Rules.IncludeMaintenance) {
		maintBlock, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance")
		if err != nil || maintBlock == nil {
			return true, nil
		}
		maintNodes := collectNodes(maintBlock)
		maintSubtotalNode, err := findExactlyOne(maintNodes, "data-field", "maintenance_subtotal")
		if err != nil || maintSubtotalNode == nil {
			return true, nil
		}
		if nodeText(maintSubtotalNode) != "Rp "+formatNumber(expected.Calculation.MaintenanceSubtotal) {
			return true, nil
		}
		maintLineTotals := findAllOrdered(maintNodes, "data-field", "line_total")
		for mIdx, line := range expected.Calculation.Maintenance {
			if mIdx >= len(maintLineTotals) {
				return true, nil
			}
			if nodeText(maintLineTotals[mIdx]) != "Rp "+formatNumber(line.LineTotal) {
				return true, nil
			}
			row := maintLineTotals[mIdx].Parent
			if row != nil {
				tds := directChildElements(row, "td")
				if len(tds) == 5 {
					wantRate := "Rp " + formatNumber(line.AmountPerUnit)
					wantHeadcount := formatQuarters32(line.HeadcountQuarters)
					wantPeriod := formatQuarters32(line.PeriodQuarters)
					if nodeText(tds[1]) != wantRate || nodeText(tds[2]) != wantHeadcount || nodeText(tds[3]) != wantPeriod {
						return true, nil
					}
				}
			}
		}
	}

	// Preparation row
	if expected.Calculation.Preparation.Gross > 0 {
		prepRow, err := findExactlyOne(commNodes, "data-field", "preparation")
		if err != nil || prepRow == nil {
			return true, nil
		}
		tds := directChildElements(prepRow, "td")
		if len(tds) == 5 {
			wantGross := "Rp " + formatNumber(expected.Calculation.Preparation.Gross)
			if nodeText(tds[4]) != wantGross {
				return true, nil
			}
		}
	}

	// Payment nominals
	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil || payBlock == nil {
		return true, nil
	}
	payNodes := collectNodes(payBlock)
	nominals := findAllOrdered(payNodes, "data-field", "payment_nominal")
	if len(nominals) != len(expected.Calculation.Payments) {
		return true, nil
	}
	for pIdx, p := range expected.Calculation.Payments {
		if nodeText(nominals[pIdx]) != "Rp "+formatNumber(p.Nominal) {
			return true, nil
		}
	}

	return false, nil
}

// CheckDedicatedSquadLegalModified checks whether legal exclusions, binding clauses,
// or tax/payment terms clauses differ from expected snapshot profile options.
func CheckDedicatedSquadLegalModified(documentHTML string, expected DedicatedSquadIntegrityExpectations) (bool, error) {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// Legal section
	legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
	if err != nil || legalBlock == nil {
		return true, nil
	}
	legalNodes := collectNodes(legalBlock)
	exclNode, err := findExactlyOne(legalNodes, "data-field", "exclusions")
	if err != nil || exclNode == nil {
		return true, nil
	}
	lis := directChildElements(exclNode, "li")
	expectedClauses := dedicatedClauses(expected.Profile.DocumentOptions)
	if len(lis) != len(expectedClauses) {
		return true, nil
	}
	for i, li := range lis {
		if normalizeWhitespace(nodeText(li)) != normalizeWhitespace(expectedClauses[i]) {
			return true, nil
		}
	}
	if !strings.Contains(normalizeWhitespace(nodeText(legalBlock)), normalizeWhitespace(DedicatedBindingClause)) {
		return true, nil
	}

	// Payment section clauses
	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil || payBlock == nil {
		return true, nil
	}
	payText := normalizeWhitespace(nodeText(payBlock))
	wantTax := normalizeWhitespace(dedicatedTaxClause(expected.Profile.DocumentOptions))
	if !strings.Contains(payText, wantTax) {
		return true, nil
	}
	if !strings.Contains(payText, normalizeWhitespace(DedicatedInvoiceClause)) {
		return true, nil
	}
	if !strings.Contains(payText, normalizeWhitespace(DedicatedSuspensionClause)) {
		return true, nil
	}

	return false, nil
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
