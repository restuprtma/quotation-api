package service

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ValidateStandaloneMaintenanceIntegrity validates the full DOM structure and semantic
// data integrity of a generated/sanitized Standalone Maintenance document.
func ValidateStandaloneMaintenanceIntegrity(
	document string,
	input domain.StandaloneMaintenanceInput,
	calc domain.StandaloneMaintenanceCalculation,
	profile domain.ResolvedStandaloneMaintenanceProfile,
	legalHTML string,
) error {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	if err := validateOfficialHeaderFields(root); err != nil {
		return err
	}
	nodes := collectNodes(root)

	// 1. Required blocks
	requiredBlocks := []string{"metadata", "opening", "commercial", "scope", "payment", "legal", "signature"}
	for _, bType := range requiredBlocks {
		block, err := findExactlyOne(nodes, "data-block-type", bType)
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: %s block is missing", ErrUnsafeHTML, bType)
		}
	}

	// 2. Rights block presence
	if profile.ShowRights && len(input.RightsStatements) > 0 {
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

	// 3. Inactive mode rows injection and scope table validation
	scopeBlock, _ := findExactlyOne(nodes, "data-block-type", "scope")
	scopeNodes := collectNodes(scopeBlock)
	commBlock, _ := findExactlyOne(nodes, "data-block-type", "commercial")
	commNodes := collectNodes(commBlock)
	allCommScopeNodes := append(append([]*html.Node(nil), commNodes...), scopeNodes...)

	switch input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		for _, n := range allCommScopeNodes {
			role := attributeValue(n, "data-role")
			if strings.HasPrefix(role, "hourly_cost_") || strings.HasPrefix(role, "hourly_task_") {
				return fmt.Errorf("%w: unexpected hourly element in retainer mode: %s", ErrUnsafeHTML, role)
			}
		}
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
		if groupCount != len(input.DefectGroups) {
			return fmt.Errorf("%w: defect group count %d does not match expected %d", ErrUnsafeHTML, groupCount, len(input.DefectGroups))
		}
		expectedItemCount := 0
		for _, g := range input.DefectGroups {
			expectedItemCount += len(g.Items)
		}
		if itemCount != expectedItemCount {
			return fmt.Errorf("%w: defect item count %d does not match expected %d", ErrUnsafeHTML, itemCount, expectedItemCount)
		}

	case domain.StandaloneMaintenanceHourlyTask:
		for _, n := range allCommScopeNodes {
			role := attributeValue(n, "data-role")
			if strings.HasPrefix(role, "retainer_cost_") || strings.HasPrefix(role, "defect_group_") || strings.HasPrefix(role, "defect_") {
				return fmt.Errorf("%w: unexpected retainer element in hourly mode: %s", ErrUnsafeHTML, role)
			}
		}
		taskCount := 0
		for _, n := range scopeNodes {
			role := attributeValue(n, "data-role")
			if strings.HasPrefix(role, "hourly_task_") {
				taskCount++
			}
		}
		if taskCount != len(input.HourlyTasks) {
			return fmt.Errorf("%w: hourly task count %d does not match expected %d", ErrUnsafeHTML, taskCount, len(input.HourlyTasks))
		}
	}

	// 4. Commercial modified check
	commMod, err := CheckStandaloneMaintenanceCommercialModified(document, input, calc)
	if err != nil {
		return err
	}
	if commMod {
		return fmt.Errorf("%w: commercial content was modified", ErrUnsafeHTML)
	}

	// 5. Rights modified check
	rightsMod, err := CheckStandaloneMaintenanceRightsModified(document, input)
	if err != nil {
		return err
	}
	if rightsMod {
		return fmt.Errorf("%w: rights content was modified", ErrUnsafeHTML)
	}

	// 6. Legal block content check
	if strings.TrimSpace(legalHTML) != "" {
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
		expectedLegal, err := canonicalizeFragmentHTML(legalHTML)
		if err != nil {
			return err
		}
		if renderedLegal != expectedLegal {
			return fmt.Errorf("%w: legal content was modified", ErrUnsafeHTML)
		}
	}

	// 7. Tax clause validation
	taxFields := findAllOrdered(nodes, "data-field", "tax_clause")
	switch input.TaxTreatment {
	case domain.StandaloneTaxExclusive:
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
	case domain.StandaloneTaxInclusive:
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
	case domain.StandaloneTaxNotStated:
		if len(taxFields) != 0 {
			return fmt.Errorf("%w: not_stated must have zero tax clauses, found %d", ErrUnsafeHTML, len(taxFields))
		}
		fullDocText := nodeText(root)
		if strings.Contains(fullDocText, "belum termasuk PPN") || strings.Contains(fullDocText, "belum termasuk Pajak Pertambahan Nilai") || strings.Contains(fullDocText, "telah termasuk PPN") {
			return fmt.Errorf("%w: not_stated document contains unexpected tax statements", ErrUnsafeHTML)
		}
	}

	// 8. Forbidden terms check
	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}

	return nil
}

// CheckStandaloneMaintenanceCommercialModified checks whether cost, discount,
// or payment structures in document differ from the stored calculation/input expectations.
// Scope remains editable normally in TipTap without triggering commercial modification.
func CheckStandaloneMaintenanceCommercialModified(
	document string,
	input domain.StandaloneMaintenanceInput,
	calc domain.StandaloneMaintenanceCalculation,
) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// 1. Commercial block
	commBlock, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil {
		return true, err
	}
	if commBlock == nil {
		return true, nil
	}
	commNodes := collectNodes(commBlock)

	// Total Investment
	totalCell, err := findExactlyOne(commNodes, "data-field", "total_investment")
	if err != nil {
		return true, err
	}
	if totalCell == nil {
		return true, nil
	}
	if nodeText(totalCell) != "Rp "+formatNumber(calc.TotalInvestment) {
		return true, nil
	}

	// Subtotal & Discount
	subtotalCell, err := findExactlyOne(commNodes, "data-field", "cost_subtotal")
	if err != nil {
		return true, err
	}
	discountCell, err := findExactlyOne(commNodes, "data-field", "discount_amount")
	if err != nil {
		return true, err
	}

	if input.ShowDiscountRow {
		if subtotalCell == nil || nodeText(subtotalCell) != "Rp "+formatNumber(calc.Subtotal) {
			return true, nil
		}
		if discountCell == nil || nodeText(discountCell) != "Rp "+formatNumber(calc.DiscountAmount) {
			return true, nil
		}
	} else {
		if discountCell != nil {
			return true, nil
		}
		if subtotalCell != nil && nodeText(subtotalCell) != "Rp "+formatNumber(calc.Subtotal) {
			return true, nil
		}
	}

	// Mode-specific cost items
	switch input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		for _, n := range commNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "hourly_cost_") {
				return true, nil
			}
		}
		retainerRowCount := 0
		for _, n := range commNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "retainer_cost_") {
				retainerRowCount++
			}
		}
		if retainerRowCount != len(calc.Retainer) {
			return true, nil
		}

		for i, item := range calc.Retainer {
			row, err := findExactlyOne(commNodes, "data-role", fmt.Sprintf("retainer_cost_%d", i))
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
			lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "cost_line_total")
			if err != nil {
				return true, err
			}
			if lineTotalCell == nil || nodeText(lineTotalCell) != "Rp "+formatNumber(item.LineTotal) {
				return true, nil
			}
		}

	case domain.StandaloneMaintenanceHourlyTask:
		for _, n := range commNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "retainer_cost_") {
				return true, nil
			}
		}
		hourlyRowCount := 0
		for _, n := range commNodes {
			if strings.HasPrefix(attributeValue(n, "data-role"), "hourly_cost_") {
				hourlyRowCount++
			}
		}
		if hourlyRowCount != len(calc.Hourly) {
			return true, nil
		}

		for i, item := range calc.Hourly {
			row, err := findExactlyOne(commNodes, "data-role", fmt.Sprintf("hourly_cost_%d", i))
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
			if nodeText(cells[1]) != "Rp "+formatNumber(item.DisplayMonthlyRate) {
				return true, nil
			}
			hcStr := nodeText(cells[2])
			if hcStr != strconv.Itoa(int(item.Headcount)) && strings.TrimSuffix(hcStr, " Orang") != strconv.Itoa(int(item.Headcount)) {
				return true, nil
			}
			hoursStr := nodeText(cells[3])
			wantHours := formatHoursHalfUnits(item.HoursHalfUnits)
			if hoursStr != wantHours && strings.TrimSuffix(hoursStr, " Jam") != wantHours {
				return true, nil
			}
			lineTotalCell, err := findExactlyOne(collectNodes(row), "data-field", "cost_line_total")
			if err != nil {
				return true, err
			}
			if lineTotalCell == nil || nodeText(lineTotalCell) != "Rp "+formatNumber(item.LineTotal) {
				return true, nil
			}
		}
	}

	// 2. Payment block
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
	if payTotalCell == nil || nodeText(payTotalCell) != "Rp "+formatNumber(calc.PaymentTotal) {
		return true, nil
	}

	paymentRowCount := 0
	for _, n := range payNodes {
		if strings.HasPrefix(attributeValue(n, "data-role"), "payment_") {
			paymentRowCount++
		}
	}
	if paymentRowCount != len(input.PaymentSchedule) {
		return true, nil
	}

	var paymentSum int64
	for i, row := range input.PaymentSchedule {
		tr, err := findExactlyOne(payNodes, "data-role", fmt.Sprintf("payment_%d", i))
		if err != nil {
			return true, err
		}
		if tr == nil {
			return true, nil
		}
		cells := directChildElements(tr, "td")
		if len(cells) != 5 {
			return true, nil
		}
		if nodeText(cells[0]) != row.WorkPeriod {
			return true, nil
		}
		if nodeText(cells[1]) != row.Terms {
			return true, nil
		}
		if nodeText(cells[2]) != row.InvoiceDateLabel {
			return true, nil
		}
		if nodeText(cells[3]) != row.DueDateLabel {
			return true, nil
		}
		nominalCell, err := findExactlyOne(collectNodes(cells[4]), "data-field", "payment_nominal")
		if err != nil {
			return true, err
		}
		if nominalCell == nil {
			return true, nil
		}
		txt := nodeText(nominalCell)
		if txt != "Rp "+formatNumber(row.Nominal) {
			return true, nil
		}
		numStr := strings.ReplaceAll(strings.TrimPrefix(txt, "Rp "), ".", "")
		numStr = strings.TrimSpace(numStr)
		val, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil || val <= 0 {
			return true, nil
		}
		paymentSum += val
	}

	if paymentSum != calc.PaymentTotal {
		return true, nil
	}

	return false, nil
}

// CheckStandaloneMaintenanceRightsModified checks whether rights statements in document
// differ from expected statements. Treated as a legal modification in Save Draft.
func CheckStandaloneMaintenanceRightsModified(
	document string,
	input domain.StandaloneMaintenanceInput,
) (bool, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	rightsBlock, err := findExactlyOne(nodes, "data-block-type", "rights")
	if err != nil {
		return true, err
	}

	if len(input.RightsStatements) == 0 {
		if rightsBlock != nil {
			return true, nil
		}
		return false, nil
	}

	if rightsBlock == nil {
		return true, nil
	}

	pElements := directChildElements(rightsBlock, "p")
	if len(pElements) != len(input.RightsStatements) {
		return true, nil
	}
	for i, stmt := range input.RightsStatements {
		if nodeText(pElements[i]) != stmt {
			return true, nil
		}
	}

	return false, nil
}
