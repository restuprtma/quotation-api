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
	numVal := nodeText(numNode)
	if valNode, valErr := findExactlyOne(collectNodes(numNode), "data-role", "metadata-value"); valErr == nil && valNode != nil {
		numVal = nodeText(valNode)
	}
	if numVal != input.DocumentNumber {
		return fmt.Errorf("%w: document number mismatch: got %q, want %q", ErrUnsafeHTML, numVal, input.DocumentNumber)
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
		// Finding 3: Validate presence and row counts for experts block
		expertBlock, err := findExactlyOne(nodes, "data-block-type", "experts")
		if err != nil || expertBlock == nil {
			return fmt.Errorf("%w: experts block is missing", ErrUnsafeHTML)
		}
		expertCount := countNodesWithRolePrefix(nodes, "expert_", "")
		if expertCount != len(input.ExpertRequirements) {
			return fmt.Errorf("%w: expert row count mismatch: got %d, want %d", ErrUnsafeHTML, expertCount, len(input.ExpertRequirements))
		}
		expertNodes := collectNodes(expertBlock)
		for i := range input.ExpertRequirements {
			row, err := findExactlyOne(expertNodes, "data-role", fmt.Sprintf("expert_%d", i))
			if err != nil || row == nil {
				return fmt.Errorf("%w: expert row expert_%d is missing", ErrUnsafeHTML, i)
			}
		}
		if tbodyTrCount(findFirstTable(expertBlock)) != len(input.ExpertRequirements) {
			return fmt.Errorf("%w: expert table tbody row count mismatch", ErrUnsafeHTML)
		}

		// Hardware checks
		if profile.ShowHardware {
			hwTable, err := findExactlyOne(nodes, "data-role", "hardware_cost_table")
			if err != nil || hwTable == nil {
				return fmt.Errorf("%w: hardware_cost_table is missing", ErrUnsafeHTML)
			}
			hwCount := countNodesWithRolePrefix(nodes, "hardware_", "hardware_cost_table")
			if hwCount != len(calc.Hardware) {
				return fmt.Errorf("%w: hardware row count mismatch: got %d, want %d", ErrUnsafeHTML, hwCount, len(calc.Hardware))
			}
			hwNodes := collectNodes(hwTable)
			for hIdx := range calc.Hardware {
				row, err := findExactlyOne(hwNodes, "data-role", fmt.Sprintf("hardware_%d", hIdx))
				if err != nil || row == nil {
					return fmt.Errorf("%w: hardware row hardware_%d is missing", ErrUnsafeHTML, hIdx)
				}
			}
			if tbodyTrCount(hwTable) != len(calc.Hardware)+3 {
				return fmt.Errorf("%w: hardware table tbody row count mismatch", ErrUnsafeHTML)
			}
		} else {
			if hwCount := countNodesWithRolePrefix(nodes, "hardware_", "hardware_cost_table"); hwCount > 0 {
				return fmt.Errorf("%w: unexpected hardware rows in service-only document: found %d", ErrUnsafeHTML, hwCount)
			}
			if hwTable, _ := findExactlyOne(nodes, "data-role", "hardware_cost_table"); hwTable != nil {
				return fmt.Errorf("%w: unexpected hardware_cost_table in service-only document", ErrUnsafeHTML)
			}
		}
	} else if input.Variant == domain.KioskQueuePrivateFoodCourt {
		// Finding 3: Validate presence and row counts for timeline block
		scopeBlock, err := findExactlyOne(nodes, "data-block-type", "scope")
		if err != nil || scopeBlock == nil {
			return fmt.Errorf("%w: scope/timeline block is missing", ErrUnsafeHTML)
		}
		timelineCount := countNodesWithRolePrefix(nodes, "timeline_", "")
		if timelineCount != len(input.PrivateTimelineRows) {
			return fmt.Errorf("%w: timeline row count mismatch: got %d, want %d", ErrUnsafeHTML, timelineCount, len(input.PrivateTimelineRows))
		}
		timelineNodes := collectNodes(scopeBlock)
		for i := range input.PrivateTimelineRows {
			row, err := findExactlyOne(timelineNodes, "data-role", fmt.Sprintf("timeline_%d", i))
			if err != nil || row == nil {
				return fmt.Errorf("%w: timeline row timeline_%d is missing", ErrUnsafeHTML, i)
			}
		}
		if tbodyTrCount(findFirstTable(scopeBlock)) != len(input.PrivateTimelineRows) {
			return fmt.Errorf("%w: timeline table tbody row count mismatch", ErrUnsafeHTML)
		}

		// Maintenance checks
		if profile.ShowMaintenance {
			mBlock, err := findExactlyOne(nodes, "data-block-type", "commercial-maintenance")
			if err != nil || mBlock == nil {
				return fmt.Errorf("%w: commercial-maintenance block is missing", ErrUnsafeHTML)
			}
			mCount := countNodesWithRolePrefix(nodes, "maintenance_", "")
			if mCount != len(calc.Maintenance) {
				return fmt.Errorf("%w: maintenance row count mismatch: got %d, want %d", ErrUnsafeHTML, mCount, len(calc.Maintenance))
			}
			mNodes := collectNodes(mBlock)
			for mIdx := range calc.Maintenance {
				row, err := findExactlyOne(mNodes, "data-role", fmt.Sprintf("maintenance_%d", mIdx))
				if err != nil || row == nil {
					return fmt.Errorf("%w: maintenance row maintenance_%d is missing", ErrUnsafeHTML, mIdx)
				}
			}
			if tbodyTrCount(findFirstTable(mBlock)) != len(calc.Maintenance)+1 {
				return fmt.Errorf("%w: maintenance table tbody row count mismatch", ErrUnsafeHTML)
			}
		} else {
			if mCount := countNodesWithRolePrefix(nodes, "maintenance_", ""); mCount > 0 {
				return fmt.Errorf("%w: unexpected maintenance rows: found %d", ErrUnsafeHTML, mCount)
			}
			if mBlock, _ := findExactlyOne(nodes, "data-block-type", "commercial-maintenance"); mBlock != nil {
				return fmt.Errorf("%w: unexpected commercial-maintenance block", ErrUnsafeHTML)
			}
		}
	}

	// Service row count and table structure validation (Finding 2)
	expectedServiceRows := 0
	for _, g := range calc.ServiceGroups {
		expectedServiceRows += len(g.Items)
	}
	if sCount := countNodesWithRolePrefix(nodes, "service_", "service_cost_table"); sCount != expectedServiceRows {
		return fmt.Errorf("%w: service row count mismatch: got %d, want %d", ErrUnsafeHTML, sCount, expectedServiceRows)
	}
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || commBlock == nil {
		return fmt.Errorf("%w: commercial block is missing", ErrUnsafeHTML)
	}
	if input.Variant == domain.KioskQueueGovernmentMPP {
		sTable, err := findExactlyOne(collectNodes(commBlock), "data-role", "service_cost_table")
		if err != nil || sTable == nil {
			return fmt.Errorf("%w: service_cost_table is missing", ErrUnsafeHTML)
		}
		expectedTrs := len(calc.ServiceGroups) + expectedServiceRows + 3
		if calc.ServiceDiscountAmount > 0 {
			expectedTrs++
		}
		if tbodyTrCount(sTable) != expectedTrs {
			return fmt.Errorf("%w: service table tbody row count mismatch", ErrUnsafeHTML)
		}
	} else {
		sTable := findFirstTable(commBlock)
		groupHeaders := 0
		for _, g := range calc.ServiceGroups {
			if len(calc.ServiceGroups) > 1 || g.Title != "" {
				groupHeaders++
			}
		}
		expectedTrs := groupHeaders + expectedServiceRows + 2
		if calc.ServiceDiscountAmount > 0 {
			expectedTrs++
		}
		if tbodyTrCount(sTable) != expectedTrs {
			return fmt.Errorf("%w: private service table tbody row count mismatch", ErrUnsafeHTML)
		}
	}

	// Payment row count and table structure validation (Finding 2)
	payBlock, err := findExactlyOne(nodes, "data-block-type", "payment")
	if err != nil || payBlock == nil {
		return fmt.Errorf("%w: payment block is missing", ErrUnsafeHTML)
	}
	payTable := findFirstTable(payBlock)
	if input.Variant == domain.KioskQueueGovernmentMPP {
		if pCount := countNodesWithRolePrefix(nodes, "government_payment_", ""); pCount != len(input.GovernmentPaymentSchedule) {
			return fmt.Errorf("%w: government payment row count mismatch: got %d, want %d", ErrUnsafeHTML, pCount, len(input.GovernmentPaymentSchedule))
		}
		if pPrivCount := countNodesWithRolePrefix(nodes, "private_payment_", ""); pPrivCount > 0 {
			return fmt.Errorf("%w: unexpected private payment rows in government document", ErrUnsafeHTML)
		}
		if tbodyTrCount(payTable) != len(input.GovernmentPaymentSchedule)+1 {
			return fmt.Errorf("%w: government payment table tbody row count mismatch", ErrUnsafeHTML)
		}
	} else {
		if pCount := countNodesWithRolePrefix(nodes, "private_payment_", ""); pCount != len(input.PrivatePaymentSchedule) {
			return fmt.Errorf("%w: private payment row count mismatch: got %d, want %d", ErrUnsafeHTML, pCount, len(input.PrivatePaymentSchedule))
		}
		if pGovCount := countNodesWithRolePrefix(nodes, "government_payment_", ""); pGovCount > 0 {
			return fmt.Errorf("%w: unexpected government payment rows in private document", ErrUnsafeHTML)
		}
		if tbodyTrCount(payTable) != len(input.PrivatePaymentSchedule)+1 {
			return fmt.Errorf("%w: private payment table tbody row count mismatch", ErrUnsafeHTML)
		}
	}

	// Finding 1: Payment due clause validation
	if profile.PaymentDueDays > 0 {
		expectedDueText := fmt.Sprintf("Pembayaran dilakukan maksimal H+%d hari kalender setelah invoice diterbitkan.", profile.PaymentDueDays)
		dueNode, err := findExactlyOne(nodes, "data-field", "payment_due_clause")
		if err != nil || dueNode == nil {
			return fmt.Errorf("%w: payment_due_clause is missing", ErrUnsafeHTML)
		}
		if nodeText(dueNode) != expectedDueText {
			return fmt.Errorf("%w: payment_due_clause text mismatch: got %q, want %q", ErrUnsafeHTML, nodeText(dueNode), expectedDueText)
		}
	} else {
		dueNode, _ := findExactlyOne(nodes, "data-field", "payment_due_clause")
		if dueNode != nil {
			return fmt.Errorf("%w: unexpected payment_due_clause present", ErrUnsafeHTML)
		}
	}

	// Tax clause validation
	if profile.TaxClauseText != "" {
		taxNode, err := findExactlyOne(nodes, "data-field", "tax_clause")
		if err != nil || taxNode == nil {
			return fmt.Errorf("%w: tax_clause is missing", ErrUnsafeHTML)
		}
		if nodeText(taxNode) != profile.TaxClauseText {
			return fmt.Errorf("%w: tax_clause text mismatch: got %q, want %q", ErrUnsafeHTML, nodeText(taxNode), profile.TaxClauseText)
		}
	} else {
		taxNode, _ := findExactlyOne(nodes, "data-field", "tax_clause")
		if taxNode != nil {
			return fmt.Errorf("%w: unexpected tax_clause present", ErrUnsafeHTML)
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
	if err != nil || numNode == nil {
		return true, nil
	}
	numVal := nodeText(numNode)
	if valNode, valErr := findExactlyOne(collectNodes(numNode), "data-role", "metadata-value"); valErr == nil && valNode != nil {
		numVal = nodeText(valNode)
	}
	if numVal != input.DocumentNumber {
		return true, nil
	}

	// 2. Commercial block
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || commBlock == nil {
		return true, nil
	}
	commNodes := collectNodes(commBlock)

	expectedServiceRows := 0
	for _, group := range calc.ServiceGroups {
		expectedServiceRows += len(group.Items)
	}
	if countNodesWithRolePrefix(nodes, "service_", "service_cost_table") != expectedServiceRows {
		return true, nil
	}

	if input.Variant == domain.KioskQueueGovernmentMPP {
		sTable, _ := findExactlyOne(commNodes, "data-role", "service_cost_table")
		expectedTrs := len(calc.ServiceGroups) + expectedServiceRows + 3
		if calc.ServiceDiscountAmount > 0 {
			expectedTrs++
		}
		if sTable == nil || tbodyTrCount(sTable) != expectedTrs {
			return true, nil
		}
	} else {
		sTable := findFirstTable(commBlock)
		groupHeaders := 0
		for _, g := range calc.ServiceGroups {
			if len(calc.ServiceGroups) > 1 || g.Title != "" {
				groupHeaders++
			}
		}
		expectedTrs := groupHeaders + expectedServiceRows + 2
		if calc.ServiceDiscountAmount > 0 {
			expectedTrs++
		}
		if sTable == nil || tbodyTrCount(sTable) != expectedTrs {
			return true, nil
		}
	}

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
			if countNodesWithRolePrefix(nodes, "hardware_", "hardware_cost_table") != len(calc.Hardware) {
				return true, nil
			}
			if tbodyTrCount(hwTable) != len(calc.Hardware)+3 {
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
		} else {
			if hwTable != nil || countNodesWithRolePrefix(nodes, "hardware_", "hardware_cost_table") > 0 {
				return true, nil
			}
		}
		if countNodesWithRolePrefix(nodes, "maintenance_", "") > 0 {
			return true, nil
		}
		if countNodesWithRolePrefix(nodes, "private_payment_", "") > 0 {
			return true, nil
		}

	case domain.KioskQueuePrivateFoodCourt:
		// Check maintenance block if profile.ShowMaintenance
		mBlock, _ := findExactlyOne(nodes, "data-block-type", "commercial-maintenance")
		if profile.ShowMaintenance {
			if mBlock == nil {
				return true, nil
			}
			if countNodesWithRolePrefix(nodes, "maintenance_", "") != len(calc.Maintenance) {
				return true, nil
			}
			mTable := findFirstTable(mBlock)
			if mTable == nil || tbodyTrCount(mTable) != len(calc.Maintenance)+1 {
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
		} else {
			if mBlock != nil || countNodesWithRolePrefix(nodes, "maintenance_", "") > 0 {
				return true, nil
			}
		}
		if countNodesWithRolePrefix(nodes, "hardware_", "") > 0 {
			return true, nil
		}
		if countNodesWithRolePrefix(nodes, "government_payment_", "") > 0 {
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

	payTable := findFirstTable(payBlock)
	if payTable == nil {
		return true, nil
	}

	switch input.Variant {
	case domain.KioskQueueGovernmentMPP:
		if countNodesWithRolePrefix(nodes, "government_payment_", "") != len(input.GovernmentPaymentSchedule) {
			return true, nil
		}
		if countNodesWithRolePrefix(nodes, "private_payment_", "") > 0 {
			return true, nil
		}
		if tbodyTrCount(payTable) != len(input.GovernmentPaymentSchedule)+1 {
			return true, nil
		}
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
		if countNodesWithRolePrefix(nodes, "private_payment_", "") != len(input.PrivatePaymentSchedule) {
			return true, nil
		}
		if countNodesWithRolePrefix(nodes, "government_payment_", "") > 0 {
			return true, nil
		}
		if tbodyTrCount(payTable) != len(input.PrivatePaymentSchedule)+1 {
			return true, nil
		}
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
	} else {
		taxClauseNode, _ := findExactlyOne(payNodes, "data-field", "tax_clause")
		if taxClauseNode != nil {
			return true, nil
		}
	}

	// Finding 1: Payment due clause
	if profile.PaymentDueDays > 0 {
		expectedDueText := fmt.Sprintf("Pembayaran dilakukan maksimal H+%d hari kalender setelah invoice diterbitkan.", profile.PaymentDueDays)
		dueNode, err := findExactlyOne(nodes, "data-field", "payment_due_clause")
		if err != nil || dueNode == nil || nodeText(dueNode) != expectedDueText {
			return true, nil
		}
	} else {
		dueNode, _ := findExactlyOne(nodes, "data-field", "payment_due_clause")
		if dueNode != nil {
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

func countNodesWithRolePrefix(nodes []*html.Node, prefix string, exactExclude string) int {
	count := 0
	for _, n := range nodes {
		role := attributeValue(n, "data-role")
		if role != "" && strings.HasPrefix(role, prefix) {
			if exactExclude != "" && role == exactExclude {
				continue
			}
			count++
		}
	}
	return count
}

func findTbody(tableNode *html.Node) *html.Node {
	if tableNode == nil {
		return nil
	}
	for child := tableNode.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "tbody" {
			return child
		}
	}
	return nil
}

func tbodyTrCount(tableNode *html.Node) int {
	tb := findTbody(tableNode)
	if tb == nil {
		return 0
	}
	return len(directChildElements(tb, "tr"))
}

func findFirstTable(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}
	for _, n := range collectNodes(node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			return n
		}
	}
	return nil
}
