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
	QuotationNumber   string
	Subject           string
	ClientName        string
	ClientCompanyName string
	IssueDate         string
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
	b.WriteString(r.renderSignatureBlock())
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
	b.WriteString(`<section data-block-type="metadata">`)
	b.WriteString("<h1>PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD</h1>")
	b.WriteString("<p><strong>Nomor Quotation:</strong> " + htmlEscape(m.QuotationNumber) + "</p>")
	b.WriteString("<p><strong>Subjek:</strong> " + htmlEscape(m.Subject) + "</p>")
	b.WriteString("<p><strong>Klien:</strong> " + htmlEscape(m.ClientName) + "</p>")
	if m.ClientCompanyName != "" {
		b.WriteString("<p><strong>Perusahaan:</strong> " + htmlEscape(m.ClientCompanyName) + "</p>")
	}
	if m.IssueDate != "" {
		b.WriteString("<p><strong>Tanggal:</strong> " + htmlEscape(m.IssueDate) + "</p>")
	}
	b.WriteString("</section>")
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
	b.WriteString(`<section data-block-type="commercial">`)
	b.WriteString(`<h2>Development Investment</h2>`)
	b.WriteString(`<table>`)
	b.WriteString(`<thead><tr><th>Peran</th><th>Senioritas</th><th>Jumlah SDM</th><th>Durasi (bulan)</th><th>Tarif/Bulan</th><th>Total</th></tr></thead>`)
	b.WriteString(`<tbody>`)
	for _, res := range a.Resources {
		rowKey := strings.ToLower(res.Role) + "_" + strings.ToLower(res.Seniority)
		b.WriteString(`<tr data-role="` + htmlEscape(rowKey) + `">`)
		b.WriteString("<td>" + htmlEscape(humanizeLabel(res.Role)) + "</td>")
		b.WriteString("<td>" + htmlEscape(humanizeLabel(res.Seniority)) + "</td>")
		b.WriteString(fmt.Sprintf("<td>%d</td>", res.EmployeeCount))
		b.WriteString("<td>" + formatQuarterDuration(res.PeriodQuarters, res.PeriodMonths) + "</td>")
		b.WriteString("<td>" + formatNumber(res.AmountPerMonth) + "</td>")
		b.WriteString(`<td data-field="line_total">` + formatNumber(res.LineTotal) + "</td>")
		b.WriteString("</tr>")
	}
	b.WriteString(`<tr><td colspan="5"><strong>TOTAL INVESTMENT</strong></td><td data-field="total_investment"><strong>` + formatCurrency(a.TotalInvestment, a.Currency) + `</strong></td></tr>`)
	b.WriteString(`</tbody></table>`)
	b.WriteString(`</section>`)
	return b.String()
}

func (r *defaultHTMLRenderer) renderPaymentSchedule(p dto.PaymentTermsInput) string {
	var b strings.Builder
	b.WriteString(`<section data-block-type="payment">`)
	b.WriteString(`<h2>Ketentuan Pembayaran</h2>`)
	b.WriteString(`<table><thead><tr><th>Terms</th><th>Persentase (%)</th><th>Keterangan</th></tr></thead><tbody>`)
	for _, item := range p.Items {
		b.WriteString("<tr>")
		b.WriteString("<td>" + htmlEscape(item.MilestoneName) + "</td>")
		fmt.Fprintf(&b, `<td data-field="payment_percentage">%d%%</td>`, item.Percentage)
		b.WriteString("<td>" + htmlEscape(item.Description) + "</td>")
		b.WriteString("</tr>")
	}
	b.WriteString(`</tbody></table>`)
	b.WriteString(`</section>`)
	return b.String()
}

func (r *defaultHTMLRenderer) renderLegalBlock(legalHTML string) string {
	return `<section data-block-type="legal">` + legalHTML + `</section>`
}

func (r *defaultHTMLRenderer) renderSignatureBlock() string {
	return `<section data-block-type="signature">
  <h2>Persetujuan</h2>
  <table class="signature-table">
    <tbody>
      <tr>
        <td class="signature-cell">
          <p>Hormat Kami,</p>
          <br><br><br><br>
          <p><strong>PT Venturo Pro Solusindo</strong></p>
        </td>
        <td class="signature-cell">
          <p>Disetujui Oleh,</p>
          <br><br><br><br>
          <p><strong>(Nama Klien)</strong></p>
        </td>
      </tr>
    </tbody>
  </table>
</section>`
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
	for _, marker := range []string{"data-block-type=\"commercial\"", "data-block-type=\"payment\"", "data-block-type=\"legal\"", "data-block-type=\"signature\"", "data-field=", "data-role=", "data-section-number=", "data-column-width="} {
		if strings.Contains(compacted, marker) {
			return "", fmt.Errorf("%w: narrative contains protected block marker %q", ErrUnsafeHTML, marker)
		}
	}

	criticalNode, err := html.Parse(strings.NewReader(criticalHTML))
	if err != nil {
		return "", err
	}
	narrativeNode, err := html.Parse(strings.NewReader(narrativeHTML))
	if err != nil {
		return "", err
	}

	criticalNodes := collectNodes(criticalNode)
	narrativeNodes := collectNodes(narrativeNode)

	sections := map[string]*html.Node{}
	for _, node := range criticalNodes {
		if node.Type == html.ElementNode && node.Data == "section" {
			for _, attr := range node.Attr {
				if attr.Key == "data-block-type" {
					sections[attr.Val] = node
				}
			}
		}
	}
	for _, node := range narrativeNodes {
		if node.Type == html.ElementNode && node.Data == "section" {
			for _, attr := range node.Attr {
				if attr.Key == "data-block-type" {
					sections[attr.Val] = node
				}
			}
		}
	}

	order := []string{"metadata", "opening", "commercial", "scope", "payment", "legal", "closing", "signature"}
	var b strings.Builder
	for _, blockType := range order {
		if node, ok := sections[blockType]; ok {
			html.Render(&b, node)
		}
	}

	return b.String(), nil
}

type ExpectedLine struct {
	Role           string
	Seniority      string
	EmployeeCount  int64
	PeriodQuarters int64
	PeriodMonths   int64
	AmountPerMonth int64
	LineTotal      int64
}

// formatQuarterDuration converts integer quarter-months to human-friendly Indonesian label
// e.g. 1 quarter = 0,25 bln, 2 = 0,5 bln, 4 = 1 bln, 6 = 1,5 bln, 8 = 2 bln, 12 = 3 bln
func formatQuarterDuration(quarters, months int64) string {
	if quarters <= 0 && months > 0 {
		quarters = months * 4
	}
	if quarters <= 0 {
		return "0 bln"
	}
	whole := quarters / 4
	rem := quarters % 4
	fractional := map[int64]string{1: "25", 2: "5", 3: "75"}
	if rem == 0 {
		return fmt.Sprintf("%d bln", whole)
	}
	return fmt.Sprintf("%d,%s bln", whole, fractional[rem])
}

func isMatchingDuration(got string, quarters, months int64) bool {
	if quarters <= 0 && months > 0 {
		quarters = months * 4
	}
	if months <= 0 && quarters > 0 {
		months = (quarters + 3) / 4
	}

	formatted := formatQuarterDuration(quarters, months)
	gotTrimmed := strings.TrimSpace(got)
	if gotTrimmed == formatted {
		return true
	}

	// Also accept without " bln" suffix, e.g. "0,25", "0,5", "1", "1,5", "2"
	bare := strings.TrimSuffix(gotTrimmed, " bln")
	bare = strings.TrimSuffix(bare, " bl")
	bare = strings.TrimSuffix(bare, " bulan")
	bare = strings.TrimSpace(bare)

	formattedBare := strings.TrimSuffix(formatted, " bln")
	if bare == formattedBare {
		return true
	}

	// Also accept dot instead of comma: "0.25", "0.5", "1.5"
	if strings.ReplaceAll(bare, ".", ",") == formattedBare {
		return true
	}

	// Also accept integer months or quarters representation
	if bare == strconv.FormatInt(months, 10) || bare == strconv.FormatInt(quarters, 10) {
		return true
	}

	return false
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

	for _, requiredBlock := range []string{"metadata", "commercial", "payment"} {
		block, err := findExactlyOne(nodes, "data-block-type", requiredBlock)
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

	legal, err := findExactlyOne(nodes, "data-block-type", "legal")
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
	legal, err := findExactlyOne(nodes, "data-block-type", "legal")
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
	investment, err := findExactlyOne(nodes, "data-block-type", "commercial")
	if err != nil || investment == nil {
		return true, nil
	}
	investmentNodes := collectNodes(investment)

	// Check total investment
	totalCell, err := findExactlyOne(investmentNodes, "data-field", "total_investment")
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
		row, err := findExactlyOne(investmentNodes, "data-role", rowKey)
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
		if line.EmployeeCount > 0 || line.PeriodQuarters > 0 || line.PeriodMonths > 0 || line.AmountPerMonth > 0 {
			cells := directChildElements(row, "td")
			if len(cells) != 6 ||
				nodeText(cells[0]) != humanizeLabel(line.Role) ||
				nodeText(cells[1]) != humanizeLabel(line.Seniority) ||
				nodeText(cells[2]) != strconv.FormatInt(line.EmployeeCount, 10) ||
				!isMatchingDuration(nodeText(cells[3]), line.PeriodQuarters, line.PeriodMonths) ||
				nodeText(cells[4]) != formatNumber(line.AmountPerMonth) {
				return true, nil
			}
		}
	}
	resourceRows := make(map[string]struct{}, len(expected.Lines))
	resourceRowCount := 0
	validateRowShape := false
	for _, line := range expected.Lines {
		if line.EmployeeCount > 0 || line.PeriodQuarters > 0 || line.PeriodMonths > 0 || line.AmountPerMonth > 0 {
			validateRowShape = true
			break
		}
	}
	for _, n := range investmentNodes {
		if key := attributeValue(n, "data-role"); key != "" {
			resourceRows[key] = struct{}{}
		}
		if n.Type == html.ElementNode && n.Data == "tr" && len(directChildElements(n, "td")) == 6 {
			resourceRowCount++
		}
	}
	if len(resourceRows) != len(expected.Lines) || (validateRowShape && resourceRowCount != len(expected.Lines)) {
		return true, nil
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
		digits := strconv.FormatInt(*budgetMaximum, 10)
		if strings.Contains(decimalDigits(nodeText(root)), digits) {
			return fmt.Errorf("%w: budget maximum %s leaked into document", ErrUnsafeHTML, digits)
		}
	}
	return nil
}

func directChildElements(parent *html.Node, tag string) []*html.Node {
	var children []*html.Node
	for child := parent.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == tag {
			children = append(children, child)
		}
	}
	return children
}

func attributeValue(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func decimalDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
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
	block, err := findExactlyOne(nodes, "data-block-type", "payment")
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
