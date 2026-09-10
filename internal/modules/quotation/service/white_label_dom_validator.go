package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type WhiteLabelIntegrityExpectations struct {
	Calculation domain.WhiteLabelCalculation
	Profile     domain.ResolvedWhiteLabelProfile
	LegalHTML   string
	Rights      []string
}

// ValidateWhiteLabelIntegrity validates the full DOM structure and semantic
// data integrity of a generated/sanitized White Label document.
func ValidateWhiteLabelIntegrity(document string, expected WhiteLabelIntegrityExpectations) error {
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

	if expected.Profile.ShowMaintenanceHosting {
		block, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance-hosting")
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: commercial-maintenance-hosting block is missing", ErrUnsafeHTML)
		}
	} else {
		block, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance-hosting")
		if err != nil {
			return err
		}
		if block != nil {
			return fmt.Errorf("%w: commercial-maintenance-hosting block is unexpected", ErrUnsafeHTML)
		}
	}

	if expected.Profile.ShowScopeTimeline {
		block, err := findExactlyOne(nodes, "data-block-type", "scope")
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: scope block is missing", ErrUnsafeHTML)
		}
	} else {
		block, err := findExactlyOne(nodes, "data-block-type", "scope")
		if err != nil {
			return err
		}
		if block != nil {
			return fmt.Errorf("%w: scope block is unexpected", ErrUnsafeHTML)
		}
	}

	if expected.Profile.ShowRights && len(expected.Rights) > 0 {
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

	commMod, err := CheckWhiteLabelCommercialModified(document, expected.Calculation)
	if err != nil {
		return err
	}
	if commMod {
		return fmt.Errorf("%w: commercial content was modified", ErrUnsafeHTML)
	}

	rightsMod, err := CheckWhiteLabelRightsModified(document, expected.Rights)
	if err != nil {
		return err
	}
	if rightsMod {
		return fmt.Errorf("%w: rights content was modified", ErrUnsafeHTML)
	}

	if strings.TrimSpace(expected.LegalHTML) != "" {
		legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
		if err != nil {
			return err
		}
		if legalBlock == nil {
			return fmt.Errorf("%w: legal block is missing", ErrUnsafeHTML)
		}
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
				return err
			}
		}
		renderedLegal, err := canonicalizeFragmentHTML(buf.String())
		if err != nil {
			return err
		}
		resolvedExpectedLegal, err := ResolveWhiteLabelLegalHTML(expected.LegalHTML, expected.Profile.TaxTreatment)
		if err != nil {
			return fmt.Errorf("%w: failed to resolve expected legal html: %v", ErrUnsafeHTML, err)
		}
		expectedLegal, err := canonicalizeFragmentHTML(resolvedExpectedLegal)
		if err != nil {
			return err
		}
		if renderedLegal != expectedLegal {
			return fmt.Errorf("%w: legal content was modified", ErrUnsafeHTML)
		}
	}

	// Tax clause validation
	taxFields := findAllOrdered(nodes, "data-field", "tax_clause")
	switch expected.Profile.TaxTreatment {
	case domain.WhiteLabelTaxExclusive:
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
	case domain.WhiteLabelTaxInclusive:
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
	case domain.WhiteLabelTaxNotStated:
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

// CheckWhiteLabelCommercialModified checks whether commercial, maintenance/hosting,
// or payment structures in document differ from the stored snapshot expectations.
// Used in Save Draft audits and generation integrity validation.
func CheckWhiteLabelCommercialModified(document string, calc domain.WhiteLabelCalculation) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// 1. Commercial (License) Block
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil {
		return true, err
	}
	if commBlock == nil {
		return true, nil
	}
	commNodes := collectNodes(commBlock)

	totalLicenseCell, err := findExactlyOne(commNodes, "data-field", "total_license")
	if err != nil {
		return true, err
	}
	if totalLicenseCell == nil {
		return true, nil
	}
	wantTotalLicense := "Rp " + formatNumber(calc.LicenseSubtotal)
	if nodeText(totalLicenseCell) != wantTotalLicense {
		return true, nil
	}

	for i, item := range calc.License {
		roleKey := fmt.Sprintf("license_%d", i)
		row, err := findExactlyOne(commNodes, "data-role", roleKey)
		if err != nil {
			return true, err
		}
		if row == nil {
			return true, nil
		}
		cells := directChildElements(row, "td")
		if len(cells) != 4 {
			return true, nil
		}
		if nodeText(cells[0]) != item.Name {
			return true, nil
		}
		if nodeText(cells[1]) != "Rp "+formatNumber(item.UnitRate) {
			return true, nil
		}
		if nodeText(cells[2]) != strconv.Itoa(int(item.Quantity)) {
			return true, nil
		}
		lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "license_line_total")
		if err != nil {
			return true, err
		}
		if lineTotalCell == nil {
			return true, nil
		}
		if nodeText(lineTotalCell) != "Rp "+formatNumber(item.LineTotal) {
			return true, nil
		}
	}

	licenseRowCount := 0
	for _, n := range commNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "license_") {
			licenseRowCount++
		}
	}
	if licenseRowCount != len(calc.License) {
		return true, nil
	}

	// 2. Commercial Maintenance & Hosting Block
	if len(calc.Maintenance) > 0 || len(calc.Hosting) > 0 {
		mhBlock, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance-hosting")
		if err != nil {
			return true, err
		}
		if mhBlock == nil {
			return true, nil
		}
		mhNodes := collectNodes(mhBlock)

		totalMHCell, err := findExactlyOne(mhNodes, "data-field", "maintenance_hosting_total")
		if err != nil {
			return true, err
		}
		if totalMHCell == nil {
			return true, nil
		}
		wantTotalMH := "Rp " + formatNumber(calc.MaintenanceHostingSubtotal)
		if nodeText(totalMHCell) != wantTotalMH {
			return true, nil
		}

		for i, item := range calc.Maintenance {
			roleKey := fmt.Sprintf("maintenance_%d", i)
			row, err := findExactlyOne(mhNodes, "data-role", roleKey)
			if err != nil {
				return true, err
			}
			if row == nil {
				return true, nil
			}
			cells := directChildElements(row, "td")
			if len(cells) != 5 {
				return true, nil
			}
			if nodeText(cells[0]) != item.Name {
				return true, nil
			}
			if nodeText(cells[1]) != "Rp "+formatNumber(item.MonthlyRate) {
				return true, nil
			}
			if nodeText(cells[2]) != formatFTE(item.FTEQuarters) {
				return true, nil
			}
			if !isMatchingMonthDuration(nodeText(cells[3]), item.PeriodMonths) {
				return true, nil
			}
			lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "maintenance_line_total")
			if err != nil {
				return true, err
			}
			if lineTotalCell == nil {
				return true, nil
			}
			if nodeText(lineTotalCell) != "Rp "+formatNumber(item.LineTotal) {
				return true, nil
			}
		}

		maintRowCount := 0
		for _, n := range mhNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "maintenance_") {
				maintRowCount++
			}
		}
		if maintRowCount != len(calc.Maintenance) {
			return true, nil
		}

		for i, item := range calc.Hosting {
			roleKey := fmt.Sprintf("hosting_%d", i)
			row, err := findExactlyOne(mhNodes, "data-role", roleKey)
			if err != nil {
				return true, err
			}
			if row == nil {
				return true, nil
			}
			cells := directChildElements(row, "td")
			if len(cells) != 5 {
				return true, nil
			}
			if nodeText(cells[0]) != item.Name {
				return true, nil
			}
			if nodeText(cells[1]) != "Rp "+formatNumber(item.MonthlyRate) {
				return true, nil
			}
			wantQuantity := "-"
			if item.Quantity > 1 {
				wantQuantity = fmt.Sprintf("%d Unit", item.Quantity)
			}
			if nodeText(cells[2]) != wantQuantity {
				return true, nil
			}
			if !isMatchingMonthDuration(nodeText(cells[3]), item.PeriodMonths) {
				return true, nil
			}
			lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "hosting_line_total")
			if err != nil {
				return true, err
			}
			if lineTotalCell == nil {
				return true, nil
			}
			if nodeText(lineTotalCell) != "Rp "+formatNumber(item.LineTotal) {
				return true, nil
			}
		}

		hostingRowCount := 0
		for _, n := range mhNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "hosting_") {
				hostingRowCount++
			}
		}
		if hostingRowCount != len(calc.Hosting) {
			return true, nil
		}
	} else {
		mhBlock, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance-hosting")
		if err != nil {
			return true, err
		}
		if mhBlock != nil {
			return true, nil
		}
	}

	// 3. Payment Block
	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil {
		return true, err
	}
	if payBlock == nil {
		return true, nil
	}
	payNodes := collectNodes(payBlock)

	payTotalCell, err := findExactlyOne(payNodes, "data-field", "payment_total")
	if err != nil {
		return true, err
	}
	if payTotalCell == nil {
		return true, nil
	}
	wantTotalPay := "Rp " + formatNumber(calc.PaymentTotal)
	if nodeText(payTotalCell) != wantTotalPay {
		return true, nil
	}

	var payTbodyRows []*html.Node
	for _, n := range payNodes {
		if n.Type == html.ElementNode && n.Data == "tbody" {
			payTbodyRows = directChildElements(n, "tr")
			break
		}
	}
	if len(payTbodyRows) < 2 {
		return true, nil
	}
	paymentTrs := payTbodyRows[:len(payTbodyRows)-1]

	if len(calc.PaymentSchedule) > 0 && len(paymentTrs) != len(calc.PaymentSchedule) {
		return true, nil
	}

	payRowCount := 0
	for _, n := range payNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "payment_") {
			payRowCount++
		}
	}
	if len(calc.PaymentSchedule) > 0 && payRowCount != len(calc.PaymentSchedule) {
		return true, nil
	}
	if payRowCount != len(paymentTrs) {
		return true, nil
	}

	var licenseSum int64
	var mhSum int64

	for i, tr := range paymentTrs {
		roleAttr := attributeValue(tr, "data-role")
		expectedRole := fmt.Sprintf("payment_%d", i)
		if roleAttr != expectedRole {
			return true, nil
		}

		indexAttr := attributeValue(tr, "data-payment-index")
		expectedIndex := strconv.Itoa(i)
		if indexAttr != expectedIndex {
			return true, nil
		}

		compAttr := attributeValue(tr, "data-component")
		if compAttr != string(domain.WhiteLabelPaymentLicense) && compAttr != string(domain.WhiteLabelPaymentMaintenanceHosting) {
			return true, nil
		}

		cells := directChildElements(tr, "td")
		if len(cells) != 5 {
			return true, nil
		}

		workPeriod := nodeText(cells[0])
		terms := nodeText(cells[1])
		invoiceDate := nodeText(cells[2])
		dueDate := nodeText(cells[3])
		nominalCell := cells[4]

		if attributeValue(nominalCell, "data-field") != "payment_nominal" {
			return true, nil
		}

		txt := nodeText(nominalCell)
		if !strings.HasPrefix(txt, "Rp ") {
			return true, nil
		}
		numStr := strings.ReplaceAll(strings.TrimPrefix(txt, "Rp "), ".", "")
		numStr = strings.TrimSpace(numStr)
		val, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil || val <= 0 {
			return true, nil
		}

		if compAttr == string(domain.WhiteLabelPaymentLicense) {
			licenseSum += val
		} else if compAttr == string(domain.WhiteLabelPaymentMaintenanceHosting) {
			mhSum += val
		}

		if len(calc.PaymentSchedule) > i {
			expectedRow := calc.PaymentSchedule[i]
			if string(expectedRow.Component) != compAttr {
				return true, nil
			}
			if workPeriod != expectedRow.WorkPeriod {
				return true, nil
			}
			if terms != expectedRow.Terms {
				return true, nil
			}
			if invoiceDate != expectedRow.InvoiceDateLabel {
				return true, nil
			}
			if dueDate != expectedRow.DueDateLabel {
				return true, nil
			}
			if val != expectedRow.Nominal {
				return true, nil
			}
		}
	}

	if licenseSum != calc.LicenseSubtotal || licenseSum != calc.LicensePaymentTotal {
		return true, nil
	}
	if mhSum != calc.MaintenanceHostingSubtotal || mhSum != calc.MaintenanceHostingPaymentTotal {
		return true, nil
	}
	if (licenseSum + mhSum) != calc.PaymentTotal {
		return true, nil
	}

	return false, nil
}

// CheckWhiteLabelRightsModified checks whether rights statements in document
// differ from expected statements. Used in Save Draft audits and generation integrity validation.
func CheckWhiteLabelRightsModified(document string, expectedRights []string) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	rightsBlock, err := findExactlyOne(nodes, "data-block-type", "rights")
	if err != nil {
		return true, err
	}

	if len(expectedRights) == 0 {
		if rightsBlock != nil {
			return true, nil
		}
		return false, nil
	}

	if rightsBlock == nil {
		return true, nil
	}

	pElements := directChildElements(rightsBlock, "p")
	if len(pElements) != len(expectedRights) {
		return true, nil
	}
	for i, stmt := range expectedRights {
		if nodeText(pElements[i]) != stmt {
			return true, nil
		}
	}

	return false, nil
}
