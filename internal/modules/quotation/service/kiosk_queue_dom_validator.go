package service

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ValidateKioskQueueIntegrity validates the full DOM structure and semantic
// data integrity of a generated/sanitized Kiosk Queue document.
func ValidateKioskQueueIntegrity(
	document string,
	input domain.KioskQueueSystemInput,
	calc domain.KioskQueueCalculation,
	profile domain.ResolvedKioskQueueProfile,
	legalHTML string,
) error {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// 1. Root article validation
	var articleNodes []*html.Node
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "article" {
			articleNodes = append(articleNodes, n)
		}
	}
	if len(articleNodes) != 1 {
		return fmt.Errorf("%w: document must have exactly one root article, found %d", ErrUnsafeHTML, len(articleNodes))
	}
	cls := attributeValue(articleNodes[0], "class")
	if !strings.Contains(cls, "quotation-document") || !strings.Contains(cls, "kiosk-queue-document") {
		return fmt.Errorf("%w: root article must have quotation-document and kiosk-queue-document classes", ErrUnsafeHTML)
	}

	// 2. Metadata block
	metaBlock, err := findExactlyOne(nodes, "data-block-type", "metadata")
	if err != nil || metaBlock == nil {
		return fmt.Errorf("%w: metadata block is missing", ErrUnsafeHTML)
	}
	numNode, err := findExactlyOne(collectNodes(metaBlock), "data-field", "quotation_number")
	if err != nil || numNode == nil {
		return fmt.Errorf("%w: quotation_number is missing in metadata", ErrUnsafeHTML)
	}
	if nodeText(numNode) != input.DocumentNumber {
		return fmt.Errorf("%w: document number mismatch: got %q, want %q", ErrUnsafeHTML, nodeText(numNode), input.DocumentNumber)
	}

	// 3. Opening block
	openingBlock, err := findExactlyOne(nodes, "data-block-type", "opening")
	if err != nil || openingBlock == nil {
		return fmt.Errorf("%w: opening block is missing", ErrUnsafeHTML)
	}

	// 4. Ordered sections & contiguous numbering
	var sectionNodes []*html.Node
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "section" && attributeValue(n, "data-section-number") != "" {
			sectionNodes = append(sectionNodes, n)
		}
	}
	if len(sectionNodes) != len(profile.Sections) {
		return fmt.Errorf("%w: section count mismatch: got %d, want %d", ErrUnsafeHTML, len(sectionNodes), len(profile.Sections))
	}
	for i, sec := range profile.Sections {
		secNode := sectionNodes[i]
		expectedNum := strconv.Itoa(sec.Number)
		gotNum := attributeValue(secNode, "data-section-number")
		if gotNum != expectedNum {
			return fmt.Errorf("%w: section number gap or mismatch: got %s, want %s", ErrUnsafeHTML, gotNum, expectedNum)
		}
		// Heading check
		h2List := directChildElements(secNode, "h2")
		if len(h2List) != 1 {
			return fmt.Errorf("%w: section %s missing h2 heading", ErrUnsafeHTML, expectedNum)
		}
		expectedHeadingPrefix := fmt.Sprintf("%d. %s", sec.Number, sec.Title)
		if !strings.HasPrefix(nodeText(h2List[0]), expectedHeadingPrefix) {
			return fmt.Errorf("%w: section heading mismatch: got %q, want prefix %q", ErrUnsafeHTML, nodeText(h2List[0]), expectedHeadingPrefix)
		}
	}

	// 5. Cross-variant protection
	switch input.Variant {
	case domain.KioskQueueGovernmentMPP:
		for _, n := range nodes {
			bType := attributeValue(n, "data-block-type")
			if bType == "commercial-maintenance" {
				return fmt.Errorf("%w: commercial-maintenance block is unexpected in government document", ErrUnsafeHTML)
			}
			role := attributeValue(n, "data-role")
			if strings.HasPrefix(role, "maintenance_") || strings.HasPrefix(role, "private_payment_") || strings.HasPrefix(role, "timeline_") {
				return fmt.Errorf("%w: unexpected private role %q in government document", ErrUnsafeHTML, role)
			}
		}
	case domain.KioskQueuePrivateFoodCourt:
		for _, n := range nodes {
			bType := attributeValue(n, "data-block-type")
			if bType == "experts" {
				return fmt.Errorf("%w: experts block is unexpected in private document", ErrUnsafeHTML)
			}
			role := attributeValue(n, "data-role")
			if role == "service_cost_table" || role == "hardware_cost_table" ||
				strings.HasPrefix(role, "hardware_") || strings.HasPrefix(role, "government_payment_") || strings.HasPrefix(role, "expert_") {
				return fmt.Errorf("%w: unexpected government role %q in private document", ErrUnsafeHTML, role)
			}
		}
	}

	// 6. Explicit component and row checks
	if input.Variant == domain.KioskQueueGovernmentMPP {
		if profile.ShowHardware {
			hwTable, err := findExactlyOne(nodes, "data-role", "hardware_cost_table")
			if err != nil || hwTable == nil {
				return fmt.Errorf("%w: hardware_cost_table is missing", ErrUnsafeHTML)
			}
			hwNodes := collectNodes(hwTable)
			for hIdx := range calc.Hardware {
				row, err := findExactlyOne(hwNodes, "data-role", fmt.Sprintf("hardware_%d", hIdx))
				if err != nil || row == nil {
					return fmt.Errorf("%w: hardware row hardware_%d is missing", ErrUnsafeHTML, hIdx)
				}
			}
		}
	}

	// 7. Limitations clauses check
	limitationsBlock, err := findExactlyOne(nodes, "data-block-type", "limitations")
	if err != nil || limitationsBlock == nil {
		return fmt.Errorf("%w: limitations block is missing", ErrUnsafeHTML)
	}
	limitationsText := nodeText(limitationsBlock)

	if input.Variant == domain.KioskQueueGovernmentMPP {
		civilExcl := KioskQueueLimitationCatalog[domain.KioskLimitCivilElectrical]
		if !strings.Contains(limitationsText, civilExcl) {
			return fmt.Errorf("%w: civil_electrical exclusion is missing", ErrUnsafeHTML)
		}
		if input.HardwareProvision == domain.HardwareClientSupplied {
			clientHwExcl := KioskQueueLimitationCatalog[domain.KioskLimitClientHardware]
			if !strings.Contains(limitationsText, clientHwExcl) {
				return fmt.Errorf("%w: client_hardware exclusion is missing", ErrUnsafeHTML)
			}
		}
	} else {
		clientHwExcl := KioskQueueLimitationCatalog[domain.KioskLimitClientHardware]
		if !strings.Contains(limitationsText, clientHwExcl) {
			return fmt.Errorf("%w: client_hardware exclusion is missing", ErrUnsafeHTML)
		}
	}

	// 8. Commercial modification check
	commMod, err := CheckKioskQueueCommercialModified(document, input, calc, profile)
	if err != nil {
		return err
	}
	if commMod {
		return fmt.Errorf("%w: commercial content was modified", ErrUnsafeHTML)
	}

	// 9. Rights modification check
	rightsBlock, err := findExactlyOne(nodes, "data-block-type", "rights")
	if err != nil {
		return err
	}
	if len(input.RightsStatements) == 0 {
		if rightsBlock != nil {
			return fmt.Errorf("%w: unexpected rights block in document", ErrUnsafeHTML)
		}
		docText := nodeText(root)
		if strings.Contains(strings.ToLower(docText), "hak kekayaan intelektual") || strings.Contains(strings.ToLower(docText), "hak cipta milik") {
			return fmt.Errorf("%w: unexpected ownership or rights claim in document", ErrUnsafeHTML)
		}
	} else {
		if rightsBlock == nil {
			return fmt.Errorf("%w: expected rights block is missing", ErrUnsafeHTML)
		}
		rightsMod, err := CheckKioskQueueRightsModified(document, input.RightsStatements)
		if err != nil {
			return err
		}
		if rightsMod {
			return fmt.Errorf("%w: rights content was modified", ErrUnsafeHTML)
		}
	}

	// 10. Legal block content check
	if strings.TrimSpace(legalHTML) != "" {
		legalBlock, err := findExactlyOne(nodes, "data-block-type", "legal")
		if err != nil || legalBlock == nil {
			return fmt.Errorf("%w: legal block is missing", ErrUnsafeHTML)
		}
		renderedLegal, err := innerHTML(legalBlock)
		if err != nil {
			return err
		}
		gotCanonical, err := canonicalizeFragmentHTML(renderedLegal)
		if err != nil {
			return err
		}
		expCanonical, err := canonicalizeFragmentHTML(legalHTML)
		if err != nil {
			return err
		}
		if gotCanonical != expCanonical {
			return fmt.Errorf("%w: legal content was modified", ErrUnsafeHTML)
		}
	}

	// 11. Signature block check
	sigBlock, err := findExactlyOne(nodes, "data-block-type", "signature")
	if err != nil || sigBlock == nil {
		return fmt.Errorf("%w: signature block is missing", ErrUnsafeHTML)
	}
	sigTable, err := findExactlyOne(collectNodes(sigBlock), "class", "signature-table")
	if err != nil || sigTable == nil {
		return fmt.Errorf("%w: signature table is missing", ErrUnsafeHTML)
	}
	sigText := nodeText(sigTable)
	if !strings.Contains(sigText, "Hormat Kami") || !strings.Contains(sigText, "Disetujui Oleh") {
		return fmt.Errorf("%w: signature columns are invalid", ErrUnsafeHTML)
	}

	// 12. Forbidden terms check
	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}

	return nil
}

// CheckKioskQueueCommercialModified checks whether cost, discount, tax,
// hardware, maintenance, payment structures, or document number in document
// differ from the stored calculation/input expectations.
// Scope and timeline edits do not trigger commercial modification.
func CheckKioskQueueCommercialModified(
	document string,
	input domain.KioskQueueSystemInput,
	calc domain.KioskQueueCalculation,
	profile domain.ResolvedKioskQueueProfile,
) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// 1. Document number in header
	numNode, err := findExactlyOne(nodes, "data-field", "quotation_number")
	if err != nil || numNode == nil || nodeText(numNode) != input.DocumentNumber {
		return true, nil
	}

	// 2. Commercial block
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || commBlock == nil {
		return true, nil
	}
	commNodes := collectNodes(commBlock)

	// Check service line totals
	for gIdx, group := range calc.ServiceGroups {
		for iIdx, item := range group.Items {
			row, err := findExactlyOne(commNodes, "data-role", fmt.Sprintf("service_%d_%d", gIdx, iIdx))
			if err != nil || row == nil {
				return true, nil
			}
			cell, err := findExactlyOne(collectNodes(row), "data-field", "service_line_total")
			if err != nil || cell == nil || nodeText(cell) != "Rp "+formatNumber(item.LineTotal) {
				return true, nil
			}
		}
	}

	// Check service subtotal
	subtotalCell, err := findExactlyOne(commNodes, "data-field", "service_subtotal")
	if err != nil || subtotalCell == nil || nodeText(subtotalCell) != "Rp "+formatNumber(calc.ServiceSubtotal) {
		return true, nil
	}

	// Check service discount if applicable
	discountCell, err := findExactlyOne(commNodes, "data-field", "service_discount_amount")
	if calc.ServiceDiscountAmount > 0 {
		if err != nil || discountCell == nil || nodeText(discountCell) != "Rp "+formatNumber(calc.ServiceDiscountAmount) {
			return true, nil
		}
	} else if discountCell != nil {
		return true, nil
	}

	// Check service total
	totalCell, err := findExactlyOne(commNodes, "data-field", "service_total")
	if err != nil || totalCell == nil || nodeText(totalCell) != "Rp "+formatNumber(calc.ServiceTotal) {
		return true, nil
	}

	// 3. Variant-specific commercial checks
	switch input.Variant {
	case domain.KioskQueueGovernmentMPP:
		// Check service tax amount
		taxCell, err := findExactlyOne(commNodes, "data-field", "service_tax_amount")
		if err != nil || taxCell == nil || nodeText(taxCell) != "Rp "+formatNumber(calc.ServiceTaxAmount) {
			return true, nil
		}

		// Check hardware table if profile.ShowHardware
		hwTable, _ := findExactlyOne(commNodes, "data-role", "hardware_cost_table")
		if profile.ShowHardware {
			if hwTable == nil {
				return true, nil
			}
			hwNodes := collectNodes(hwTable)
			for hIdx, item := range calc.Hardware {
				row, err := findExactlyOne(hwNodes, "data-role", fmt.Sprintf("hardware_%d", hIdx))
				if err != nil || row == nil {
					return true, nil
				}
				cell, err := findExactlyOne(collectNodes(row), "data-field", "hardware_line_total")
				if err != nil || cell == nil || nodeText(cell) != "Rp "+formatNumber(item.LineTotal) {
					return true, nil
				}
			}
			hwSub, err := findExactlyOne(hwNodes, "data-field", "hardware_subtotal")
			if err != nil || hwSub == nil || nodeText(hwSub) != "Rp "+formatNumber(calc.HardwareSubtotal) {
				return true, nil
			}
			hwTax, err := findExactlyOne(hwNodes, "data-field", "hardware_tax_amount")
			if err != nil || hwTax == nil || nodeText(hwTax) != "Rp "+formatNumber(calc.HardwareTaxAmount) {
				return true, nil
			}
			hwTot, err := findExactlyOne(hwNodes, "data-field", "hardware_total")
			if err != nil || hwTot == nil || nodeText(hwTot) != "Rp "+formatNumber(calc.HardwareTotal) {
				return true, nil
			}
		} else if hwTable != nil {
			return true, nil
		}

	case domain.KioskQueuePrivateFoodCourt:
		// Check maintenance block if profile.ShowMaintenance
		mBlock, _ := findExactlyOne(nodes, "data-block-type", "commercial-maintenance")
		if profile.ShowMaintenance {
			if mBlock == nil {
				return true, nil
			}
			mNodes := collectNodes(mBlock)
			for mIdx, item := range calc.Maintenance {
				row, err := findExactlyOne(mNodes, "data-role", fmt.Sprintf("maintenance_%d", mIdx))
				if err != nil || row == nil {
					return true, nil
				}
				cell, err := findExactlyOne(collectNodes(row), "data-field", "maintenance_line_total")
				if err != nil || cell == nil || nodeText(cell) != "Rp "+formatNumber(item.LineTotal) {
					return true, nil
				}
			}
			mTot, err := findExactlyOne(mNodes, "data-field", "maintenance_total")
			if err != nil || mTot == nil || nodeText(mTot) != "Rp "+formatNumber(calc.MaintenanceTotal) {
				return true, nil
			}
		} else if mBlock != nil {
			return true, nil
		}
	}

	// 4. Payment block
	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil || payBlock == nil {
		return true, nil
	}
	payNodes := collectNodes(payBlock)

	payTotalCell, err := findExactlyOne(payNodes, "data-field", "payment_total")
	if err != nil || payTotalCell == nil || nodeText(payTotalCell) != "Rp "+formatNumber(calc.PaymentTotal) {
		return true, nil
	}

	switch input.Variant {
	case domain.KioskQueueGovernmentMPP:
		var paySum int64
		for pIdx, p := range input.GovernmentPaymentSchedule {
			row, err := findExactlyOne(payNodes, "data-role", fmt.Sprintf("government_payment_%d", pIdx))
			if err != nil || row == nil {
				return true, nil
			}
			nomCell, err := findExactlyOne(collectNodes(row), "data-field", "payment_nominal")
			if err != nil || nomCell == nil || nodeText(nomCell) != "Rp "+formatNumber(p.Nominal) {
				return true, nil
			}
			paySum += p.Nominal
		}
		if paySum != calc.PaymentTotal {
			return true, nil
		}

	case domain.KioskQueuePrivateFoodCourt:
		var paySum int64
		for pIdx, p := range input.PrivatePaymentSchedule {
			row, err := findExactlyOne(payNodes, "data-role", fmt.Sprintf("private_payment_%d", pIdx))
			if err != nil || row == nil {
				return true, nil
			}
			nomCell, err := findExactlyOne(collectNodes(row), "data-field", "payment_nominal")
			if err != nil || nomCell == nil || nodeText(nomCell) != "Rp "+formatNumber(p.Nominal) {
				return true, nil
			}
			paySum += p.Nominal
		}
		if paySum != calc.PaymentTotal {
			return true, nil
		}
	}

	// Tax clause in payment
	if profile.TaxClauseText != "" {
		taxClauseNode, err := findExactlyOne(payNodes, "data-field", "tax_clause")
		if err != nil || taxClauseNode == nil || nodeText(taxClauseNode) != profile.TaxClauseText {
			return true, nil
		}
	}

	return false, nil
}

// CheckKioskQueueRightsModified checks whether rights statements in document
// differ from expected statements. Treated as a legal modification in Save Draft.
func CheckKioskQueueRightsModified(document string, rights []string) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	rightsBlock, err := findExactlyOne(nodes, "data-block-type", "rights")
	if err != nil {
		return true, err
	}

	if len(rights) == 0 {
		if rightsBlock != nil {
			return true, nil
		}
		return false, nil
	}

	if rightsBlock == nil {
		return true, nil
	}

	pElements := directChildElements(rightsBlock, "p")
	if len(pElements) != len(rights) {
		return true, nil
	}
	for i, stmt := range rights {
		if nodeText(pElements[i]) != stmt {
			return true, nil
		}
	}

	return false, nil
}
