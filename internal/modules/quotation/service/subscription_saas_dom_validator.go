package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type SubscriptionSaaSIntegrityExpectations struct {
	Calculation domain.SubscriptionSaaSCalculation
	Profile     domain.ResolvedSubscriptionSaaSProfile
	LegalHTML   string
}

func ValidateSubscriptionSaaSIntegrity(document string, expected SubscriptionSaaSIntegrityExpectations) error {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	requiredBlocks := []string{"metadata", "opening", "commercial", "payment", "legal", "signature"}
	for _, bType := range requiredBlocks {
		block, err := findExactlyOne(nodes, "data-block-type", bType)
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: %s block is missing", ErrUnsafeHTML, bType)
		}
	}

	if expected.Profile.HasRights() {
		block, err := findExactlyOne(nodes, "data-block-type", "rights")
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: rights block is missing", ErrUnsafeHTML)
		}
	} else {
		block, err := findExactlyOne(nodes, "data-block-type", "rights")
		if err != nil {
			return err
		}
		if block != nil {
			return fmt.Errorf("%w: rights block is unexpected", ErrUnsafeHTML)
		}
	}

	if expected.Profile.HasFeatures {
		block, err := findExactlyOne(nodes, "data-block-type", "features")
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: features block is missing", ErrUnsafeHTML)
		}
	}

	commMod, err := CheckSubscriptionSaaSCommercialModified(document, expected)
	if err != nil {
		return err
	}
	if commMod {
		return fmt.Errorf("%w: commercial content was modified", ErrUnsafeHTML)
	}

	legalMod, err := CheckSubscriptionSaaSLegalModified(document, expected)
	if err != nil {
		return err
	}
	if legalMod {
		return fmt.Errorf("%w: legal content was modified", ErrUnsafeHTML)
	}

	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}

	return nil
}

// CheckSubscriptionSaaSCommercialModified checks whether commercial or payment
// structures in document differ from the stored snapshot expectations. Used in
// generation integrity validation and Save Draft audits.
func CheckSubscriptionSaaSCommercialModified(document string, expected SubscriptionSaaSIntegrityExpectations) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || commBlock == nil {
		return true, nil
	}
	commNodes := collectNodes(commBlock)

	totalCell, err := findExactlyOne(commNodes, "data-field", "total_contract")
	if err != nil || totalCell == nil {
		return true, nil
	}
	wantTotal := "Rp " + formatNumber(expected.Calculation.TotalContract)
	if nodeText(totalCell) != wantTotal {
		return true, nil
	}

	for i, period := range expected.Calculation.PricePeriods {
		roleKey := fmt.Sprintf("subscription_period_%d", i)
		row, err := findExactlyOne(commNodes, "data-role", roleKey)
		if err != nil || row == nil {
			return true, nil
		}
		cells := directChildElements(row, "td")
		if len(cells) != 4 {
			return true, nil
		}
		if nodeText(cells[1]) != "Rp "+formatNumber(period.MonthlyRate) {
			return true, nil
		}
		if !isMatchingMonthDuration(nodeText(cells[2]), period.DurationMonths) {
			return true, nil
		}
		lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "subscription_line_total")
		if err != nil || lineTotalCell == nil {
			return true, nil
		}
		if nodeText(lineTotalCell) != "Rp "+formatNumber(period.Subtotal) {
			return true, nil
		}
	}

	periodRowCount := 0
	for _, n := range commNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "subscription_period_") {
			periodRowCount++
		}
	}
	if periodRowCount != len(expected.Calculation.PricePeriods) {
		return true, nil
	}

	for i, item := range expected.Calculation.OneTimeItems {
		roleKey := fmt.Sprintf("subscription_one_time_%d", i)
		row, err := findExactlyOne(commNodes, "data-role", roleKey)
		if err != nil || row == nil {
			return true, nil
		}
		lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "subscription_line_total")
		if err != nil || lineTotalCell == nil {
			return true, nil
		}
		if nodeText(lineTotalCell) != "Rp "+formatNumber(item.Amount) {
			return true, nil
		}
	}

	oneTimeRowCount := 0
	for _, n := range commNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "subscription_one_time_") {
			oneTimeRowCount++
		}
	}
	if oneTimeRowCount != len(expected.Calculation.OneTimeItems) {
		return true, nil
	}

	// Count tbody rows in commercial block
	var commTbodyRows []*html.Node
	for _, n := range commNodes {
		if n.Type == html.ElementNode && n.Data == "tbody" {
			commTbodyRows = directChildElements(n, "tr")
			break
		}
	}
	expectedCommRows := len(expected.Calculation.PricePeriods) + len(expected.Calculation.OneTimeItems) + 1 // +1 for total row
	if len(commTbodyRows) != expectedCommRows {
		return true, nil
	}

	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil || payBlock == nil {
		return true, nil
	}

	var payTbodyRows []*html.Node
	for _, n := range collectNodes(payBlock) {
		if n.Type == html.ElementNode && n.Data == "tbody" {
			payTbodyRows = directChildElements(n, "tr")
			break
		}
	}
	if len(payTbodyRows) != len(expected.Calculation.Payments) {
		return true, nil
	}

	payNominalCells := findAllOrdered(collectNodes(payBlock), "data-field", "payment_nominal")
	if len(payNominalCells) != len(expected.Calculation.Payments) {
		return true, nil
	}
	for i, pay := range expected.Calculation.Payments {
		want := "Rp " + formatNumber(pay.Nominal)
		if nodeText(payNominalCells[i]) != want {
			return true, nil
		}
	}

	return false, nil
}

// CheckSubscriptionSaaSLegalModified checks whether legal terms, rights, income sharing,
// auto-renewal, or tax clauses differ from stored snapshot expectations. Used in
// generation integrity validation and Save Draft audits.
func CheckSubscriptionSaaSLegalModified(document string, expected SubscriptionSaaSIntegrityExpectations) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
	if err != nil || legalBlock == nil {
		return true, nil
	}
	if strings.TrimSpace(expected.LegalHTML) != "" {
		var buf bytes.Buffer
		isFirstH2 := true
		for child := legalBlock.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.CommentNode {
				continue
			}
			if child.Type == html.ElementNode && child.Data == "h2" && isFirstH2 {
				isFirstH2 = false
				continue
			}
			if err := html.Render(&buf, child); err != nil {
				return true, err
			}
		}
		renderedLegal, err := canonicalizeFragmentHTML(buf.String())
		if err != nil {
			return true, err
		}
		expectedLegal, err := canonicalizeFragmentHTML(expected.LegalHTML)
		if err != nil {
			return true, err
		}
		if renderedLegal != expectedLegal {
			return true, nil
		}
	}

	if expected.Profile.HasRights() {
		block, err := findExactlyOne(nodes, "data-block-type", "rights")
		if err != nil || block == nil {
			return true, nil
		}
	} else {
		block, err := findExactlyOne(nodes, "data-block-type", "rights")
		if err != nil || block != nil {
			return true, nil
		}
	}

	if expected.Profile.RightsClauseText != "" {
		cell, err := findExactlyOne(nodes, "data-field", "rights_clause")
		if err != nil || cell == nil {
			return true, nil
		}
		if nodeText(cell) != expected.Profile.RightsClauseText {
			return true, nil
		}
	} else {
		cell, err := findExactlyOne(nodes, "data-field", "rights_clause")
		if err != nil || cell != nil {
			return true, nil
		}
	}

	if expected.Profile.IncomeSharingText != "" {
		cell, err := findExactlyOne(nodes, "data-field", "income_sharing")
		if err != nil || cell == nil {
			return true, nil
		}
		if nodeText(cell) != expected.Profile.IncomeSharingText {
			return true, nil
		}
	} else {
		cell, err := findExactlyOne(nodes, "data-field", "income_sharing")
		if err != nil || cell != nil {
			return true, nil
		}
	}

	if expected.Profile.AutoRenewalText != "" {
		cell, err := findExactlyOne(nodes, "data-field", "auto_renewal")
		if err != nil || cell == nil {
			return true, nil
		}
		if nodeText(cell) != expected.Profile.AutoRenewalText {
			return true, nil
		}
	} else {
		cell, err := findExactlyOne(nodes, "data-field", "auto_renewal")
		if err != nil || cell != nil {
			return true, nil
		}
	}

	if expected.Profile.TaxClauseText != "" {
		cell, err := findExactlyOne(nodes, "data-field", "tax_clause")
		if err != nil || cell == nil {
			return true, nil
		}
		if nodeText(cell) != expected.Profile.TaxClauseText {
			return true, nil
		}
	} else {
		cell, err := findExactlyOne(nodes, "data-field", "tax_clause")
		if err != nil || cell != nil {
			return true, nil
		}
	}

	return false, nil
}

func isMatchingMonthDuration(got string, wantMonths int32) bool {
	trimmed := strings.TrimSpace(got)
	bare := strings.TrimSpace(strings.TrimSuffix(trimmed, "Bulan"))
	bare = strings.TrimSpace(strings.TrimSuffix(bare, "bulan"))
	bare = strings.TrimSpace(strings.TrimSuffix(bare, "bln"))
	wantStr := strconv.FormatInt(int64(wantMonths), 10)
	return bare == wantStr
}
