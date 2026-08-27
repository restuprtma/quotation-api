package service

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var whitespaceRemoved = strings.NewReplacer(
	" ", "",
	"\t", "",
	"\n", "",
	"\r", "",
)

var (
	ErrUnsafeHTML             = errors.New("unsafe_html")
	ErrInvalidPaymentTerms    = errors.New("invalid_payment_terms")
	ErrInvalidAllocationInput = errors.New("invalid_allocation_input")
)

type QuotationMetadata struct {
	QuotationNumber string
	Subject         string
	ClientName      string
	IssueDate       string
}

type CriticalBlocksInput struct {
	Metadata   QuotationMetadata
	Allocation dto.EnrichedAllocation
	Payment    dto.PaymentTermsInput
	LegalHTML  string
}

type HTMLRenderer interface {
	RenderCriticalBlocks(in CriticalBlocksInput) (string, error)
	Assemble(criticalHTML, narrativeHTML string) (string, error)
}

type defaultHTMLRenderer struct{}

func NewHTMLRenderer() HTMLRenderer {
	return &defaultHTMLRenderer{}
}

func (r *defaultHTMLRenderer) RenderCriticalBlocks(in CriticalBlocksInput) (string, error) {
	if len(in.Allocation.Resources) == 0 {
		return "", fmt.Errorf("%w: allocation has no resources", ErrInvalidPaymentTerms)
	}
	if err := validateResourceLabels(in.Allocation.Resources); err != nil {
		return "", err
	}
	if err := validatePaymentTerms(in.Payment); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.LegalHTML) == "" {
		return "", fmt.Errorf("%w: legal template snapshot is empty", ErrInvalidPaymentTerms)
	}

	var b strings.Builder
	b.WriteString(r.renderMetadata(in.Metadata))
	b.WriteString(r.renderInvestmentTable(in.Allocation))
	b.WriteString(r.renderPaymentSchedule(in.Payment))
	b.WriteString(r.renderLegalBlock(in.LegalHTML))
	return b.String(), nil
}

func validatePaymentTerms(p dto.PaymentTermsInput) error {
	if p.Mode != "milestone" {
		return fmt.Errorf("%w: unsupported payment mode %q", ErrInvalidPaymentTerms, p.Mode)
	}
	if len(p.Items) == 0 {
		return fmt.Errorf("%w: payment items are empty", ErrInvalidPaymentTerms)
	}
	sum := int64(0)
	for _, item := range p.Items {
		if item.Percentage <= 0 {
			return fmt.Errorf("%w: percentage must be positive", ErrInvalidPaymentTerms)
		}
		sum += item.Percentage
	}
	if sum != 100 {
		return fmt.Errorf("%w: percentages sum to %d, want 100", ErrInvalidPaymentTerms, sum)
	}
	return nil
}

func (r *defaultHTMLRenderer) renderMetadata(m QuotationMetadata) string {
	var b strings.Builder
	b.WriteString(`<div class="qtr-block qtr-metadata" data-block="metadata">`)
	b.WriteString("<p><strong>Nomor Quotation:</strong> " + htmlEscape(m.QuotationNumber) + "</p>")
	b.WriteString("<p><strong>Subjek:</strong> " + htmlEscape(m.Subject) + "</p>")
	b.WriteString("<p><strong>Klien:</strong> " + htmlEscape(m.ClientName) + "</p>")
	if m.IssueDate != "" {
		b.WriteString("<p><strong>Tanggal:</strong> " + htmlEscape(m.IssueDate) + "</p>")
	}
	b.WriteString("</div>")
	return b.String()
}

func validateResourceLabels(resources []dto.EnrichedResource) error {
	for i, res := range resources {
		if !slugPattern.MatchString(strings.ToLower(res.Role)) {
			return fmt.Errorf("%w: resources[%d].role %q is not a lowercase slug", ErrInvalidAllocationInput, i, res.Role)
		}
		if !slugPattern.MatchString(strings.ToLower(res.Seniority)) {
			return fmt.Errorf("%w: resources[%d].seniority %q is not a lowercase slug", ErrInvalidAllocationInput, i, res.Seniority)
		}
	}
	return nil
}

func (r *defaultHTMLRenderer) renderInvestmentTable(a dto.EnrichedAllocation) string {
	var b strings.Builder
	b.WriteString(`<div class="qtr-block qtr-investment" data-block="investment">`)
	b.WriteString(`<table>`)
	b.WriteString(`<thead><tr><th>Peran</th><th>Senioritas</th><th>Jumlah SDM</th><th>Durasi (bulan)</th><th>Tarif/Bulan</th><th>Total</th></tr></thead>`)
	b.WriteString(`<tbody>`)
	for _, res := range a.Resources {
		rowKey := strings.ToLower(res.Role) + "_" + strings.ToLower(res.Seniority)
		fmt.Fprintf(&b, `<tr data-role="%s">`, htmlEscape(rowKey))
		b.WriteString("<td>" + htmlEscape(humanizeLabel(res.Role)) + "</td>")
		b.WriteString("<td>" + htmlEscape(humanizeLabel(res.Seniority)) + "</td>")
		fmt.Fprintf(&b, "<td>%d</td>", res.EmployeeCount)
		fmt.Fprintf(&b, "<td>%d</td>", res.PeriodMonths)
		b.WriteString("<td>" + formatNumber(res.AmountPerMonth) + "</td>")
		fmt.Fprintf(&b, `<td data-field="line_total">%s</td>`, formatNumber(res.LineTotal))
		b.WriteString("</tr>")
	}
	totalRow := fmt.Sprintf(
		`<tr class="qtr-total-row"><td colspan="5"><strong>Total Investasi</strong></td>`+
			`<td data-field="total_investment"><strong>%s</strong></td></tr>`,
		formatCurrency(a.TotalInvestment, a.Currency),
	)
	b.WriteString(totalRow)
	b.WriteString(`</tbody></table>`)
	b.WriteString("</div>")
	return b.String()
}

func (r *defaultHTMLRenderer) renderPaymentSchedule(p dto.PaymentTermsInput) string {
	var b strings.Builder
	b.WriteString(`<div class="qtr-block qtr-payment" data-block="payment_schedule">`)
	b.WriteString(`<table><thead><tr><th>Milestone</th><th>Persentase</th><th>Keterangan</th></tr></thead><tbody>`)
	for _, item := range p.Items {
		b.WriteString("<tr>")
		b.WriteString("<td>" + htmlEscape(item.MilestoneName) + "</td>")
		fmt.Fprintf(&b, `<td data-field="payment_percentage">%d%%</td>`, item.Percentage)
		b.WriteString("<td>" + htmlEscape(item.Description) + "</td>")
		b.WriteString("</tr>")
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

func (r *defaultHTMLRenderer) renderLegalBlock(legalHTML string) string {
	return `<div class="qtr-block qtr-legal" data-block="legal">` + legalHTML + `</div><!-- /qtr-legal -->`
}

func htmlEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	).Replace(s)
}

func humanizeLabel(raw string) string {
	parts := strings.Split(raw, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func formatNumber(amount int64) string {
	digits := strconv.FormatInt(amount, 10)
	neg := strings.HasPrefix(digits, "-")
	if neg {
		digits = digits[1:]
	}
	var groups []string
	for len(digits) > 3 {
		groups = append([]string{digits[len(digits)-3:]}, groups...)
		digits = digits[:len(digits)-3]
	}
	groups = append([]string{digits}, groups...)
	out := strings.Join(groups, ".")
	if neg {
		return "-" + out
	}
	return out
}

func formatCurrency(amount int64, currency string) string {
	if currency == "IDR" {
		return "Rp " + formatNumber(amount)
	}
	return currency + " " + formatNumber(amount)
}

func (r *defaultHTMLRenderer) Assemble(criticalHTML, narrativeHTML string) (string, error) {
	if strings.TrimSpace(criticalHTML) == "" {
		return "", fmt.Errorf("%w: critical blocks are empty", ErrUnsafeHTML)
	}
	if strings.TrimSpace(narrativeHTML) == "" {
		return "", fmt.Errorf("%w: narrative is empty", ErrUnsafeHTML)
	}
	lower := strings.ToLower(narrativeHTML)
	compacted := whitespaceRemoved.Replace(lower)
	for _, marker := range []string{"data-block=", "data-field=", "data-role="} {
		if strings.Contains(compacted, marker) {
			return "", fmt.Errorf("%w: narrative contains protected block marker %q", ErrUnsafeHTML, marker)
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="quotation-document">`)
	b.WriteString(`<div class="qtr-narrative">` + narrativeHTML + `</div>`)
	b.WriteString(criticalHTML)
	b.WriteString(`</div>`)
	return b.String(), nil
}

type ExpectedLine struct {
	Role      string
	Seniority string
	LineTotal int64
}

type ExpectedPayment struct {
	Percentage int64
}

type IntegrityExpectations struct {
	Currency        string
	TotalInvestment int64
	Lines           []ExpectedLine
	PaymentItems    []ExpectedPayment
	LegalHTML       string
	BudgetMaximum   *int64
}

type DOMValidator interface {
	ValidateIntegrity(documentHTML string, expected IntegrityExpectations) error
	CheckLegalModified(documentHTML string, expectedLegalHTML string) (bool, error)
	CheckCommercialModified(documentHTML string, expected IntegrityExpectations) (bool, error)
	ValidateDocumentSafety(documentHTML string, budgetMaximum *int64, totalInvestment int64) error
}

type defaultDOMValidator struct{}

func NewDOMValidator() DOMValidator {
	return &defaultDOMValidator{}
}

func (v *defaultDOMValidator) ValidateIntegrity(documentHTML string, expected IntegrityExpectations) error {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	for _, requiredBlock := range []string{"metadata", "investment", "payment_schedule"} {
		block, err := findExactlyOne(nodes, "data-block", requiredBlock)
		if err != nil {
			return err
		}
		if block == nil {
			return fmt.Errorf("%w: %s block is missing", ErrUnsafeHTML, requiredBlock)
		}
	}

	totalCell, err := findExactlyOne(nodes, "data-field", "total_investment")
	if err != nil {
		return err
	}
	if totalCell == nil {
		return fmt.Errorf("%w: total investment cell is missing", ErrUnsafeHTML)
	}
	wantTotal := formatCurrency(expected.TotalInvestment, expected.Currency)
	if got := nodeText(totalCell); got != wantTotal {
		return fmt.Errorf("%w: total investment mismatch: got %q, want %q", ErrUnsafeHTML, got, wantTotal)
	}

	for _, line := range expected.Lines {
		rowKey := strings.ToLower(line.Role) + "_" + strings.ToLower(line.Seniority)
		row, err := findExactlyOne(nodes, "data-role", rowKey)
		if err != nil {
			return err
		}
		if row == nil {
			return fmt.Errorf("%w: allocation row %q is missing", ErrUnsafeHTML, rowKey)
		}
		lineCell, err := findExactlyOne(collectNodes(row), "data-field", "line_total")
		if err != nil {
			return err
		}
		if lineCell == nil {
			return fmt.Errorf("%w: line total cell for %q is missing", ErrUnsafeHTML, rowKey)
		}
		if got := nodeText(lineCell); got != formatNumber(line.LineTotal) {
			return fmt.Errorf("%w: line total mismatch for %q: got %q, want %q", ErrUnsafeHTML, rowKey, got, formatNumber(line.LineTotal))
		}
	}

	legal, err := findExactlyOne(nodes, "data-block", "legal")
	if err != nil {
		return err
	}
	if legal == nil {
		return fmt.Errorf("%w: legal block is missing", ErrUnsafeHTML)
	}
	renderedLegal, err := innerHTML(legal)
	if err != nil {
		return err
	}
	expectedLegal, err := canonicalizeFragmentHTML(expected.LegalHTML)
	if err != nil {
		return err
	}
	if renderedLegal != expectedLegal {
		return fmt.Errorf("%w: legal block content was modified", ErrUnsafeHTML)
	}

	if len(expected.PaymentItems) > 0 {
		if err := validatePaymentSchedule(nodes, expected.PaymentItems); err != nil {
			return err
		}
	}

	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}

	if expected.BudgetMaximum != nil && *expected.BudgetMaximum != expected.TotalInvestment {
		digits := formatNumber(*expected.BudgetMaximum)
		for _, n := range nodes {
			if n.Type == html.TextNode && strings.Contains(n.Data, digits) {
				return fmt.Errorf("%w: budget maximum %s leaked into document", ErrUnsafeHTML, digits)
			}
		}
	}
	return nil
}

func (v *defaultDOMValidator) CheckLegalModified(documentHTML string, expectedLegalHTML string) (bool, error) {
	if strings.TrimSpace(expectedLegalHTML) == "" {
		return false, nil
	}
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)
	legal, err := findExactlyOne(nodes, "data-block", "legal")
	if err != nil || legal == nil {
		return true, nil
	}
	renderedLegal, err := innerHTML(legal)
	if err != nil {
		return true, err
	}
	expectedLegal, err := canonicalizeFragmentHTML(expectedLegalHTML)
	if err != nil {
		return true, err
	}
	return renderedLegal != expectedLegal, nil
}

func (v *defaultDOMValidator) CheckCommercialModified(documentHTML string, expected IntegrityExpectations) (bool, error) {
	if expected.TotalInvestment == 0 && len(expected.Lines) == 0 {
		return false, nil
	}

	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return true, fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)

	// Check total investment
	totalCell, err := findExactlyOne(nodes, "data-field", "total_investment")
	if err != nil || totalCell == nil {
		return true, nil
	}
	wantTotal := formatCurrency(expected.TotalInvestment, expected.Currency)
	if got := nodeText(totalCell); got != wantTotal {
		return true, nil
	}

	// Check lines
	for _, line := range expected.Lines {
		rowKey := strings.ToLower(line.Role) + "_" + strings.ToLower(line.Seniority)
		row, err := findExactlyOne(nodes, "data-role", rowKey)
		if err != nil || row == nil {
			return true, nil
		}
		lineCell, err := findExactlyOne(collectNodes(row), "data-field", "line_total")
		if err != nil || lineCell == nil {
			return true, nil
		}
		if got := nodeText(lineCell); got != formatNumber(line.LineTotal) {
			return true, nil
		}
	}

	// Check payment schedule if applicable
	if len(expected.PaymentItems) > 0 {
		if err := validatePaymentSchedule(nodes, expected.PaymentItems); err != nil {
			return true, nil
		}
	}

	return false, nil
}

func (v *defaultDOMValidator) ValidateDocumentSafety(documentHTML string, budgetMaximum *int64, totalInvestment int64) error {
	root, err := html.Parse(strings.NewReader(documentHTML))
	if err != nil {
		return fmt.Errorf("%w: failed to parse document: %v", ErrUnsafeHTML, err)
	}
	nodes := collectNodes(root)
	if err := scanForbiddenTerms(nodes); err != nil {
		return err
	}
	if budgetMaximum != nil && *budgetMaximum != totalInvestment {
		digits := formatNumber(*budgetMaximum)
		for _, n := range nodes {
			if n.Type == html.TextNode && strings.Contains(n.Data, digits) {
				return fmt.Errorf("%w: budget maximum %s leaked into document", ErrUnsafeHTML, digits)
			}
		}
	}
	return nil
}

func collectNodes(root *html.Node) []*html.Node {
	var nodes []*html.Node
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		nodes = append(nodes, n)
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return nodes
}

func findExactlyOne(nodes []*html.Node, key, val string) (*html.Node, error) {
	var found *html.Node
	count := 0
	for _, n := range nodes {
		if !hasAttribute(n, key, val) {
			continue
		}
		count++
		found = n
	}
	if count > 1 {
		return nil, fmt.Errorf("%w: duplicate marker %s=%q", ErrUnsafeHTML, key, val)
	}
	return found, nil
}

func findAllOrdered(nodes []*html.Node, key, val string) []*html.Node {
	var found []*html.Node
	for _, n := range nodes {
		if hasAttribute(n, key, val) {
			found = append(found, n)
		}
	}
	return found
}

func validatePaymentSchedule(nodes []*html.Node, items []ExpectedPayment) error {
	block, err := findExactlyOne(nodes, "data-block", "payment_schedule")
	if err != nil {
		return err
	}
	if block == nil {
		return fmt.Errorf("%w: payment schedule block is missing", ErrUnsafeHTML)
	}

	cells := findAllOrdered(collectNodes(block), "data-field", "payment_percentage")
	if len(cells) != len(items) {
		return fmt.Errorf("%w: payment schedule has %d rows, want %d", ErrUnsafeHTML, len(cells), len(items))
	}
	for i, item := range items {
		want := fmt.Sprintf("%d%%", item.Percentage)
		if got := nodeText(cells[i]); got != want {
			return fmt.Errorf("%w: payment row %d percentage mismatch: got %q, want %q", ErrUnsafeHTML, i, got, want)
		}
	}
	return nil
}

func hasAttribute(n *html.Node, key, val string) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == key && attr.Val == val {
			return true
		}
	}
	return false
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

func innerHTML(n *html.Node) (string, error) {
	var buf bytes.Buffer
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.CommentNode {
			continue
		}
		if err := html.Render(&buf, child); err != nil {
			return "", fmt.Errorf("%w: failed to render node: %v", ErrUnsafeHTML, err)
		}
	}
	return buf.String(), nil
}

func canonicalizeFragmentHTML(fragment string) (string, error) {
	parsed, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return "", fmt.Errorf("%w: failed to parse expected legal html: %w", ErrUnsafeHTML, err)
	}
	var body *html.Node
	for c := parsed.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "html" {
			continue
		}
		for b := c.FirstChild; b != nil; b = b.NextSibling {
			if b.Type == html.ElementNode && b.Data == "body" {
				body = b
			}
		}
	}
	if body == nil {
		return "", fmt.Errorf("%w: expected legal html has no parseable body", ErrUnsafeHTML)
	}
	return innerHTML(body)
}

func scanForbiddenTerms(nodes []*html.Node) error {
	terms := []string{"budget", "unused", "utilization"}
	for _, n := range nodes {
		switch n.Type {
		case html.TextNode:
			if term := matchForbiddenTerm(n.Data, terms); term != "" {
				return fmt.Errorf("%w: forbidden term %q found in document text", ErrUnsafeHTML, term)
			}
		case html.ElementNode:
			for _, attr := range n.Attr {
				if term := matchForbiddenTerm(attr.Val, terms); term != "" {
					return fmt.Errorf("%w: forbidden term %q found in attribute %q", ErrUnsafeHTML, term, attr.Key)
				}
			}
		}
	}
	return nil
}

func matchForbiddenTerm(value string, terms []string) string {
	lower := strings.ToLower(value)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return term
		}
	}
	return ""
}
