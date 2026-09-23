package service

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type CustomDevelopmentIntegrityExpectations struct {
	Input       domain.CustomDevelopmentInput
	Calculation domain.CustomDevelopmentCalculation
	Profile     domain.ResolvedCustomDevelopmentProfile
	LegalHTML   string
}

func ValidateCustomDevelopmentIntegrity(document string, expected CustomDevelopmentIntegrityExpectations) error {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	if err := validateOfficialHeaderFields(root); err != nil {
		return err
	}
	nodes := collectNodes(root)

	requiredSections := []string{
		"commercial-development",
		"scope",
		"timeline",
		"payment",
		"limitations",
		"legal",
		"signature",
	}
	for _, sec := range requiredSections {
		node, err := findExactlyOne(nodes, "data-quotation-section", sec)
		if err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("%w: %s section is missing", ErrUnsafeHTML, sec)
		}
	}

	if expected.Profile.ShowMonthly {
		node, err := findExactlyOne(nodes, "data-quotation-section", "commercial-monthly")
		if err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("%w: commercial-monthly section is missing", ErrUnsafeHTML)
		}
	} else {
		node, err := findExactlyOne(nodes, "data-quotation-section", "commercial-monthly")
		if err != nil {
			return err
		}
		if node != nil {
			return fmt.Errorf("%w: commercial-monthly section is unexpected", ErrUnsafeHTML)
		}
	}

	if expected.Profile.ShowRights && len(expected.Input.RightsStatements) > 0 {
		node, err := findExactlyOne(nodes, "data-quotation-section", "rights")
		if err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("%w: rights section is missing", ErrUnsafeHTML)
		}
	} else {
		node, err := findExactlyOne(nodes, "data-quotation-section", "rights")
		if err != nil {
			return err
		}
		if node != nil {
			return fmt.Errorf("%w: rights section is unexpected", ErrUnsafeHTML)
		}
	}

	// Verify no duplicate section numbers
	seenNumbers := make(map[string]string)
	for _, n := range nodes {
		if num := attributeValue(n, "data-quotation-section-number"); num != "" {
			secName := attributeValue(n, "data-quotation-section")
			if prevSec, exists := seenNumbers[num]; exists {
				return fmt.Errorf("%w: duplicate section number %s on %q and %q", ErrUnsafeHTML, num, prevSec, secName)
			}
			seenNumbers[num] = secName
		}
	}

	// Verify sequential section numbers matching profile
	for _, sec := range expected.Profile.Sections {
		var secMarker string
		switch sec.Code {
		case "development":
			secMarker = "commercial-development"
		case "monthly":
			secMarker = "commercial-monthly"
		case "scope":
			secMarker = "scope"
		case "payment":
			secMarker = "payment"
		case "rights":
			secMarker = "rights"
		case "limitations":
			secMarker = "limitations"
		}
		node, err := findExactlyOne(nodes, "data-quotation-section", secMarker)
		if err != nil {
			return err
		}
		if node != nil {
			expectedNum := strconv.Itoa(sec.Number)
			if gotNum := attributeValue(node, "data-quotation-section-number"); gotNum != expectedNum {
				return fmt.Errorf("%w: section %s number mismatch: got %s, want %s", ErrUnsafeHTML, secMarker, gotNum, expectedNum)
			}
		}
	}

	// Commercial check
	commMod, err := CheckCustomDevelopmentCommercialModified(document, expected.Input, expected.Calculation)
	if err != nil {
		return err
	}
	if commMod {
		return fmt.Errorf("%w: commercial content was modified", ErrUnsafeHTML)
	}

	// Module features check
	scopeNode, err := findExactlyOne(nodes, "data-quotation-section", "scope")
	if err != nil {
		return err
	}
	if scopeNode == nil {
		return fmt.Errorf("%w: scope section is missing", ErrUnsafeHTML)
	}
	scopeText := nodeText(scopeNode)
	for _, group := range expected.Input.ModuleGroups {
		if !strings.Contains(scopeText, strings.TrimSpace(group.Name)) {
			return fmt.Errorf("%w: module group %q is missing", ErrUnsafeHTML, group.Name)
		}
		for _, feat := range group.Features {
			if !strings.Contains(scopeText, strings.TrimSpace(feat)) {
				return fmt.Errorf("%w: module feature %q is missing", ErrUnsafeHTML, feat)
			}
		}
	}

	// Timeline check
	timelineNode, err := findExactlyOne(nodes, "data-quotation-section", "timeline")
	if err != nil {
		return err
	}
	if timelineNode == nil {
		return fmt.Errorf("%w: timeline section is missing", ErrUnsafeHTML)
	}
	timelineText := nodeText(timelineNode)
	for _, row := range expected.Input.TimelineRows {
		if !strings.Contains(timelineText, strings.TrimSpace(row.Description)) {
			return fmt.Errorf("%w: timeline description %q is missing", ErrUnsafeHTML, row.Description)
		}
		if !strings.Contains(timelineText, strings.TrimSpace(row.Timeline)) {
			return fmt.Errorf("%w: timeline %q is missing", ErrUnsafeHTML, row.Timeline)
		}
	}

	// Rights check
	rightsMod, err := CheckCustomDevelopmentRightsModified(document, expected.Input.RightsStatements)
	if err != nil {
		return err
	}
	if rightsMod {
		return fmt.Errorf("%w: rights content was modified", ErrUnsafeHTML)
	}

	// Limitations check
	limMod, err := CheckCustomDevelopmentLimitationsModified(document, expected.Input.LimitationCodes, expected.Input.AdditionalLimitations)
	if err != nil {
		return err
	}
	if limMod {
		return fmt.Errorf("%w: limitations content was modified", ErrUnsafeHTML)
	}

	// Legal check
	if strings.TrimSpace(expected.LegalHTML) != "" {
		legalNode, err := findExactlyOne(nodes, "data-quotation-section", "legal")
		if err != nil {
			return err
		}
		if legalNode == nil {
			return fmt.Errorf("%w: legal section is missing", ErrUnsafeHTML)
		}
		renderedLegal, err := innerHTML(legalNode)
		if err != nil {
			return err
		}
		renderedCanon, err := canonicalizeFragmentHTML(renderedLegal)
		if err != nil {
			return err
		}
		resolvedExpectedLegal, err := ResolveCustomDevelopmentLegalHTML(expected.LegalHTML, expected.Input.TaxTreatment)
		if err != nil {
			return fmt.Errorf("%w: failed to resolve expected legal html: %v", ErrUnsafeHTML, err)
		}
		expectedCanon, err := canonicalizeFragmentHTML(resolvedExpectedLegal)
		if err != nil {
			return err
		}
		if renderedCanon != expectedCanon {
			return fmt.Errorf("%w: legal content was modified", ErrUnsafeHTML)
		}
	}

	// Tax clause validation
	taxFields := findAllOrdered(nodes, "data-quotation-field", "tax_clause")
	switch expected.Input.TaxTreatment {
	case domain.CustomDevelopmentTaxExclusive:
		if len(taxFields) != 1 {
			return fmt.Errorf("%w: exclusive_ppn must have exactly one tax clause, found %d", ErrUnsafeHTML, len(taxFields))
		}
		taxText := nodeText(taxFields[0])
		if !strings.Contains(taxText, "belum termasuk PPN") {
			return fmt.Errorf("%w: exclusive_ppn tax clause must state belum termasuk PPN", ErrUnsafeHTML)
		}
		fullDocText := nodeText(root)
		if strings.Contains(fullDocText, "telah termasuk PPN") || strings.Contains(fullDocText, "sudah termasuk PPN") {
			return fmt.Errorf("%w: exclusive_ppn document contains contradictory inclusive tax statements", ErrUnsafeHTML)
		}
	case domain.CustomDevelopmentTaxInclusive:
		if len(taxFields) != 1 {
			return fmt.Errorf("%w: inclusive_ppn must have exactly one tax clause, found %d", ErrUnsafeHTML, len(taxFields))
		}
		taxText := nodeText(taxFields[0])
		if !strings.Contains(taxText, "termasuk PPN") {
			return fmt.Errorf("%w: inclusive_ppn tax clause must state termasuk PPN", ErrUnsafeHTML)
		}
		fullDocText := nodeText(root)
		if strings.Contains(fullDocText, "belum termasuk PPN") || strings.Contains(fullDocText, "belum termasuk Pajak Pertambahan Nilai") {
			return fmt.Errorf("%w: inclusive_ppn document contains contradictory exclusive tax statements", ErrUnsafeHTML)
		}
	case domain.CustomDevelopmentTaxNotStated:
		if len(taxFields) != 0 {
			return fmt.Errorf("%w: not_stated must have zero tax clauses, found %d", ErrUnsafeHTML, len(taxFields))
		}
		fullDocText := nodeText(root)
		if strings.Contains(fullDocText, "belum termasuk PPN") || strings.Contains(fullDocText, "belum termasuk Pajak Pertambahan Nilai") || strings.Contains(fullDocText, "telah termasuk PPN") {
			return fmt.Errorf("%w: not_stated document contains unexpected tax statements", ErrUnsafeHTML)
		}
	}

	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}

	return nil
}

func CheckCustomDevelopmentCommercialModified(document string, in domain.CustomDevelopmentInput, calc domain.CustomDevelopmentCalculation) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// 1. Commercial Development
	commSec, err := findExactlyOne(nodes, "data-quotation-section", "commercial-development")
	if err != nil {
		return true, err
	}
	if commSec == nil {
		return true, nil
	}
	commNodes := collectNodes(commSec)

	subtotalCell, err := findExactlyOne(commNodes, "data-quotation-field", "development_subtotal")
	if err != nil {
		return true, err
	}
	if subtotalCell == nil {
		return true, nil
	}
	if nodeText(subtotalCell) != "Rp "+formatNumber(calc.DevelopmentSubtotal) {
		return true, nil
	}

	totalCell, err := findExactlyOne(commNodes, "data-quotation-field", "total_investment")
	if err != nil {
		return true, err
	}
	if totalCell == nil {
		return true, nil
	}
	if nodeText(totalCell) != "Rp "+formatNumber(calc.TotalInvestment) {
		return true, nil
	}

	discCell, err := findExactlyOne(commNodes, "data-quotation-field", "discount_amount")
	if err != nil {
		return true, err
	}
	if in.ShowDiscountRow {
		if discCell == nil {
			return true, nil
		}
		if nodeText(discCell) != "Rp "+formatNumber(calc.DiscountAmount) {
			return true, nil
		}
	} else if discCell != nil {
		return true, nil
	}

	totalItemsCount := 0
	for gIdx, group := range in.DevelopmentGroups {
		for iIdx, item := range group.Items {
			totalItemsCount++
			fieldKey := fmt.Sprintf("development_line_%d_%d", gIdx, iIdx)
			lineCell, err := findExactlyOne(commNodes, "data-quotation-field", fieldKey)
			if err != nil {
				return true, err
			}
			if lineCell == nil {
				return true, nil
			}
			if item.QuotedLineTotal != nil {
				want := "Rp " + formatNumber(*item.QuotedLineTotal)
				if nodeText(lineCell) != want {
					return true, nil
				}
			} else {
				if nodeText(lineCell) != "" {
					return true, nil
				}
			}

			// Verify row metadata
			roleKey := fmt.Sprintf("development_line_%d_%d", gIdx, iIdx)
			row, err := findExactlyOne(commNodes, "data-role", roleKey)
			if err != nil {
				return true, err
			}
			if row == nil {
				return true, nil
			}
			tds := directChildElements(row, "td")
			if len(tds) < 4 {
				return true, nil
			}
			if strings.TrimSpace(nodeText(tds[0])) != strings.TrimSpace(item.Name) {
				return true, nil
			}
			var hcCell, periodCell *html.Node
			if len(tds) == 5 {
				if item.DisplayRate != nil {
					if !strings.Contains(nodeText(tds[1]), formatNumber(*item.DisplayRate)) {
						return true, nil
					}
				}
				hcCell = tds[2]
				periodCell = tds[3]
			} else {
				hcCell = tds[1]
				periodCell = tds[2]
			}
			if strings.TrimSpace(nodeText(hcCell)) != strconv.Itoa(int(item.Headcount)) {
				return true, nil
			}
			if strings.TrimSpace(nodeText(periodCell)) != strings.TrimSpace(item.PeriodLabel) {
				return true, nil
			}
		}
	}

	devRowCount := 0
	for _, n := range commNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "development_line_") {
			devRowCount++
		}
	}
	if devRowCount != totalItemsCount {
		return true, nil
	}

	// 2. Commercial Monthly
	if len(in.MonthlyItems) > 0 {
		monthSec, err := findExactlyOne(nodes, "data-quotation-section", "commercial-monthly")
		if err != nil {
			return true, err
		}
		if monthSec == nil {
			return true, nil
		}
		monthNodes := collectNodes(monthSec)

		totalMonthCell, err := findExactlyOne(monthNodes, "data-quotation-field", "monthly_total")
		if err != nil {
			return true, err
		}
		if totalMonthCell == nil {
			return true, nil
		}
		if nodeText(totalMonthCell) != "Rp "+formatNumber(calc.MonthlyTotal) {
			return true, nil
		}

		for i, item := range calc.MonthlyLines {
			fieldKey := fmt.Sprintf("monthly_line_%d", i)
			lineCell, err := findExactlyOne(monthNodes, "data-quotation-field", fieldKey)
			if err != nil {
				return true, err
			}
			if lineCell == nil {
				return true, nil
			}
			if nodeText(lineCell) != "Rp "+formatNumber(item.LineTotal) {
				return true, nil
			}

			roleKey := fmt.Sprintf("monthly_%d", i)
			row, err := findExactlyOne(monthNodes, "data-role", roleKey)
			if err != nil {
				return true, err
			}
			if row == nil {
				return true, nil
			}
			tds := directChildElements(row, "td")
			if len(tds) != 5 {
				return true, nil
			}
			if strings.TrimSpace(nodeText(tds[0])) != strings.TrimSpace(item.Name) {
				return true, nil
			}
			if item.Specification != "" && strings.TrimSpace(nodeText(tds[1])) != strings.TrimSpace(item.Specification) {
				return true, nil
			}
			if !strings.Contains(nodeText(tds[2]), formatNumber(item.UnitRate)) {
				return true, nil
			}
			if strings.TrimSpace(nodeText(tds[3])) != strconv.Itoa(int(item.Quantity)) {
				return true, nil
			}
		}

		monthRowCount := 0
		for _, n := range monthNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "monthly_") {
				monthRowCount++
			}
		}
		if monthRowCount != len(calc.MonthlyLines) {
			return true, nil
		}

		// Monthly notes
		if len(in.MonthlyNotes) > 0 {
			notesList, err := findExactlyOne(monthNodes, "data-quotation-component", "monthly-notes")
			if err != nil {
				return true, err
			}
			if notesList == nil {
				return true, nil
			}
			liElements := directChildElements(notesList, "li")
			if len(liElements) != len(in.MonthlyNotes) {
				return true, nil
			}
			for i, note := range in.MonthlyNotes {
				if strings.TrimSpace(nodeText(liElements[i])) != strings.TrimSpace(note) {
					return true, nil
				}
			}
		}
	} else {
		monthSec, err := findExactlyOne(nodes, "data-quotation-section", "commercial-monthly")
		if err != nil {
			return true, err
		}
		if monthSec != nil {
			return true, nil
		}
	}

	// 3. Payment Schedule
	paySec, err := findExactlyOne(nodes, "data-quotation-section", "payment")
	if err != nil {
		return true, err
	}
	if paySec == nil {
		return true, nil
	}
	payNodes := collectNodes(paySec)

	for i, row := range in.PaymentSchedule {
		fieldKey := fmt.Sprintf("payment_nominal_%d", i)
		nominalCell, err := findExactlyOne(payNodes, "data-quotation-field", fieldKey)
		if err != nil {
			return true, err
		}
		if nominalCell == nil {
			return true, nil
		}
		if nodeText(nominalCell) != "Rp "+formatNumber(row.Nominal) {
			return true, nil
		}

		roleKey := fmt.Sprintf("payment_%d", i)
		tr, err := findExactlyOne(payNodes, "data-role", roleKey)
		if err != nil {
			return true, err
		}
		if tr == nil {
			return true, nil
		}
		if attributeValue(tr, "data-quotation-component") != string(row.Component) {
			return true, nil
		}
		tds := directChildElements(tr, "td")
		if len(tds) != 5 {
			return true, nil
		}
		if strings.TrimSpace(nodeText(tds[0])) != strings.TrimSpace(row.WorkPeriod) {
			return true, nil
		}
		if strings.TrimSpace(nodeText(tds[1])) != strings.TrimSpace(row.Terms) {
			return true, nil
		}
		if strings.TrimSpace(nodeText(tds[2])) != strings.TrimSpace(row.InvoiceDateLabel) {
			return true, nil
		}
		if strings.TrimSpace(nodeText(tds[3])) != strings.TrimSpace(row.DueDateLabel) {
			return true, nil
		}
	}

	payRowCount := 0
	for _, n := range payNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "payment_") {
			payRowCount++
		}
	}
	if payRowCount != len(in.PaymentSchedule) {
		return true, nil
	}

	return false, nil
}

func CheckCustomDevelopmentRightsModified(document string, expectedRights []string) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	rightsSec, err := findExactlyOne(nodes, "data-quotation-section", "rights")
	if err != nil {
		return true, err
	}

	if len(expectedRights) == 0 {
		if rightsSec != nil {
			return true, nil
		}
		return false, nil
	}

	if rightsSec == nil {
		return true, nil
	}

	pElements := directChildElements(rightsSec, "p")
	if len(pElements) != len(expectedRights) {
		return true, nil
	}
	for i, stmt := range expectedRights {
		if strings.TrimSpace(nodeText(pElements[i])) != strings.TrimSpace(stmt) {
			return true, nil
		}
	}

	return false, nil
}

func CheckCustomDevelopmentLimitationsModified(document string, limitationCodes []string, additionalLimitations []string) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	limSec, err := findExactlyOne(nodes, "data-quotation-section", "limitations")
	if err != nil {
		return true, err
	}
	if limSec == nil {
		return true, nil
	}

	var expected []string
	for _, code := range limitationCodes {
		if text, ok := CustomDevelopmentLimitationCatalog[code]; ok {
			expected = append(expected, strings.TrimSpace(text))
		}
	}
	for _, add := range additionalLimitations {
		trimmed := strings.TrimSpace(add)
		if trimmed != "" {
			expected = append(expected, trimmed)
		}
	}

	olNodes := directChildElements(limSec, "ol")
	if len(olNodes) != 1 {
		return true, nil
	}
	liElements := directChildElements(olNodes[0], "li")
	if len(liElements) != len(expected) {
		return true, nil
	}
	for i, item := range expected {
		if strings.TrimSpace(nodeText(liElements[i])) != item {
			return true, nil
		}
	}

	return false, nil
}
