package rnd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type DeterministicBlocks struct {
	CommercialHTML string
	PaymentHTML    string
	LegalHTML      string
	CommercialHash string
	PaymentHash    string
	LegalHash      string
}

type HTMLExpectation struct {
	Input      GenerationInput
	Allocation EnrichedAllocation
	Blocks     DeterministicBlocks
	LegalHTML  string
}

type ValidationCheck struct {
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

type HTMLValidation struct {
	Security   map[string]ValidationCheck `json:"security"`
	Structural map[string]ValidationCheck `json:"structural"`
}

type SecurityValidationError struct {
	Checks []string
}

func (e *SecurityValidationError) Error() string {
	return "HTML security validation failed: " + strings.Join(e.Checks, ", ")
}

type StructuralValidationError struct {
	Checks []string
}

func (e *StructuralValidationError) Error() string {
	return "HTML structural validation failed: " + strings.Join(e.Checks, ", ")
}

var decimalSpanPattern = regexp.MustCompile(`^(?:[1-9]|1[0-9]|20)$`)

var allowedElements = map[string]bool{
	"section": true, "p": true, "br": true, "strong": true, "em": true, "u": true, "s": true,
	"h1": true, "h2": true, "h3": true, "ul": true, "ol": true, "li": true, "blockquote": true,
	"hr": true, "a": true, "table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
}

var activeElements = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true, "svg": true, "math": true,
	"form": true, "input": true, "button": true, "textarea": true, "select": true, "option": true, "video": true,
	"audio": true, "source": true, "canvas": true, "template": true, "noscript": true,
}

var sectionOrder = []string{"metadata", "opening", "commercial", "scope", "payment", "legal", "closing", "signature"}

func RenderDeterministicBlocks(input GenerationInput, allocation EnrichedAllocation, legalHTML string) DeterministicBlocks {
	commercial := element("section", attr("data-block-type", "commercial"),
		element("h2", text("Development Investment")),
		element("table",
			element("thead", element("tr", element("th", text("Role")), element("th", text("Rate Monthly")), element("th", text("Employee")), element("th", text("Period Month")), element("th", text("Total")))),
			element("tbody"),
		),
	)
	tbody := findFirstElement(commercial, "tbody")
	for _, resource := range allocation.Resources {
		tbody.AppendChild(element("tr",
			element("td", text(displayResource(resource))),
			element("td", text(formatIDR(resource.AmountPerMonth))),
			element("td", text(strconv.FormatInt(resource.EmployeeCount, 10))),
			element("td", text(strconv.FormatInt(resource.PeriodMonths, 10))),
			element("td", text(formatIDR(resource.LineTotal))),
		))
	}
	tbody.AppendChild(element("tr",
		element("td", attr("colspan", "4"), element("strong", text("Total Investment"))),
		element("td", element("strong", text(formatIDR(allocation.TotalInvestment)))),
	))

	paymentBody := element("tbody")
	for _, item := range input.PaymentTerms.Items {
		paymentBody.AppendChild(element("tr", element("td", text(item.Label)), element("td", text(strconv.FormatInt(item.Percentage, 10)+"%"))))
	}
	payment := element("section", attr("data-block-type", "payment"),
		element("h2", text("Jadwal Pembayaran")),
		element("table",
			element("thead", element("tr", element("th", text("Termin")), element("th", text("Persentase")))),
			paymentBody,
		),
	)

	commercialHTML := renderNode(commercial)
	paymentHTML := renderNode(payment)
	commercialHash, _ := hashHTMLFragment(commercialHTML)
	paymentHash, _ := hashHTMLFragment(paymentHTML)
	legalHash, _ := hashHTMLFragment(legalHTML)
	return DeterministicBlocks{
		CommercialHTML: commercialHTML, PaymentHTML: paymentHTML, LegalHTML: legalHTML,
		CommercialHash: commercialHash, PaymentHash: paymentHash, LegalHash: legalHash,
	}
}

func SanitizeAndValidate(raw string, expectation HTMLExpectation) (string, HTMLValidation, error) {
	result := newHTMLValidation()
	lowerRaw := strings.ToLower(raw)
	if strings.Contains(raw, "```") || strings.Contains(lowerRaw, "<html") || strings.Contains(lowerRaw, "<head") || strings.Contains(lowerRaw, "<body") || strings.Contains(lowerRaw, "<!doctype") || strings.Contains(lowerRaw, "<?xml") {
		failCheck(result.Security, "S1", "document wrapper or markdown fence")
		failCheck(result.Structural, "T8", "document wrapper or markdown fence")
		return "", result, validationError(result)
	}

	root, err := parseFragment(raw)
	if err != nil {
		failCheck(result.Structural, "T6", "HTML parse failure")
		return "", result, &StructuralValidationError{Checks: []string{"T6"}}
	}
	state := sanitizerState{}
	sanitizeChildren(root, &state)
	sanitized, err := renderChildren(root)
	if err != nil {
		failCheck(result.Structural, "T6", "HTML serialization failure")
		return "", result, &StructuralValidationError{Checks: []string{"T6"}}
	}
	cleanRoot, err := parseFragment(sanitized)
	if err != nil {
		failCheck(result.Structural, "T6", "sanitized HTML reparse failure")
		return "", result, &StructuralValidationError{Checks: []string{"T6"}}
	}

	if state.invalidURL {
		failCheck(result.Security, "S3", "forbidden URL")
	}
	if state.invalidAttribute {
		failCheck(result.Security, "S2", "invalid allowed attribute value")
	}
	if containsForbiddenElement(cleanRoot) {
		failCheck(result.Security, "S1", "forbidden element remained after sanitization")
	}
	if invalidRemainingAttributes(cleanRoot) {
		failCheck(result.Security, "S2", "forbidden attribute remained after sanitization")
	}

	visible := nodeVisibleText(cleanRoot)
	visibleLower := strings.ToLower(visible)
	if leaksBudget(sanitized, visibleLower, expectation.Input.BudgetMaximum, expectation.Allocation.TotalInvestment) {
		failCheck(result.Security, "S4", "internal budget leakage")
	}
	if containsInjectionCompliance(visibleLower) {
		failCheck(result.Security, "S7", "prompt-injection instruction appeared in output")
	}
	if containsInternalLeakage(sanitized) {
		failCheck(result.Security, "S8", "internal prompt, retry, or placeholder leakage")
		failCheck(result.Structural, "T8", "unresolved internal content")
	}
	validateIntegritySubtrees(cleanRoot, expectation, result)

	sections, structureOK := collectRequiredSections(cleanRoot)
	if !structureOK {
		failCheck(result.Structural, "T6", "required sections must appear exactly once in contract order without nesting")
	} else {
		validateSectionContent(sections, expectation, result)
	}

	return sanitized, result, validationError(result)
}

func validateSectionContent(sections map[string]*html.Node, expectation HTMLExpectation, result HTMLValidation) {
	metadataText := nodeVisibleText(sections["metadata"])
	for _, required := range []string{expectation.Input.Quotation.Number, expectation.Input.Quotation.Date, expectation.Input.Quotation.Subject, expectation.Input.Client.CompanyName, expectation.Input.Client.PICName} {
		if !strings.Contains(metadataText, required) {
			failCheck(result.Structural, "T1", "metadata does not match generation input")
			break
		}
	}

	scopeItems := listItemTexts(sections["scope"])
	for _, mandatory := range expectation.Input.MandatoryScope {
		if !containsString(scopeItems, mandatory) {
			failCheck(result.Structural, "T2", "mandatory scope is missing")
			break
		}
	}
	allowedScope := append([]string(nil), expectation.Input.MandatoryScope...)
	allowedScope = append(allowedScope, expectation.Allocation.SelectedOptionalScope...)
	allowedScope = append(allowedScope, expectation.Allocation.StandardActivities...)
	for _, scope := range scopeItems {
		if !containsString(allowedScope, scope) {
			failCheck(result.Structural, "T3", "scope contains an unapproved item")
			break
		}
	}
	for _, selected := range append(append([]string(nil), expectation.Allocation.SelectedOptionalScope...), expectation.Allocation.StandardActivities...) {
		if !containsString(scopeItems, selected) {
			failCheck(result.Structural, "T3", "selected scope or activity is missing")
			break
		}
	}

	if !result.Security["S6"].Pass {
		failCheck(result.Structural, "T4", "resource identifiers differ from enriched allocation")
		failCheck(result.Structural, "T5", "investment values differ from deterministic snapshot")
	}
	if !validCommercialTable(sections["commercial"]) {
		failCheck(result.Structural, "T6", "commercial table structure is invalid")
	}

	paymentHTML := renderNode(sections["payment"])
	paymentHash, _ := hashHTMLFragment(paymentHTML)
	expectedPaymentHash := expectation.Blocks.PaymentHash
	if expectedPaymentHash == "" {
		expectedPaymentHash, _ = hashHTMLFragment(expectation.Blocks.PaymentHTML)
	}
	if paymentHash != expectedPaymentHash || !validPaymentTable(sections["payment"]) {
		failCheck(result.Structural, "T6", "payment table differs from deterministic input")
	}

	prose := strings.ToLower(nodeVisibleText(sections["opening"]) + " " + nodeVisibleText(sections["closing"]))
	words := 0
	for _, word := range []string{"dan", "yang", "untuk", "dengan", "sesuai"} {
		if containsWord(prose, word) {
			words++
		}
	}
	if strings.TrimSpace(prose) == "" || words < 2 {
		failCheck(result.Structural, "T7", "automated_proxy")
	} else {
		result.Structural["T7"] = ValidationCheck{Pass: true, Detail: "automated_proxy"}
	}
}

func validateIntegritySubtrees(root *html.Node, expectation HTMLExpectation, result HTMLValidation) {
	commercial := elementsWithBlockType(root, "commercial")
	expectedCommercialHash := expectation.Blocks.CommercialHash
	if expectedCommercialHash == "" {
		expectedCommercialHash, _ = hashHTMLFragment(expectation.Blocks.CommercialHTML)
	}
	if len(commercial) != 1 {
		failCheck(result.Security, "S6", "commercial section is missing or duplicated")
	} else {
		actualHash, _ := hashHTMLFragment(renderNode(commercial[0]))
		if actualHash != expectedCommercialHash {
			failCheck(result.Security, "S6", "commercial subtree differs from deterministic snapshot")
		}
	}

	legal := elementsWithBlockType(root, "legal")
	expectedLegalHash := expectation.Blocks.LegalHash
	if expectedLegalHash == "" {
		expectedLegalHash, _ = hashHTMLFragment(expectation.LegalHTML)
	}
	if len(legal) != 1 {
		failCheck(result.Security, "S5", "legal section is missing or duplicated")
	} else {
		actualHash, _ := hashHTMLFragment(renderNode(legal[0]))
		if actualHash != expectedLegalHash {
			failCheck(result.Security, "S5", "legal subtree differs from approved snapshot")
		}
	}
}

func elementsWithBlockType(root *html.Node, blockType string) []*html.Node {
	var matches []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "section" && attributeValue(node, "data-block-type") == blockType {
			matches = append(matches, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return matches
}

type sanitizerState struct {
	invalidURL       bool
	invalidAttribute bool
}

func sanitizeChildren(parent *html.Node, state *sanitizerState) {
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		switch child.Type {
		case html.CommentNode, html.DoctypeNode:
			parent.RemoveChild(child)
		case html.ElementNode:
			name := strings.ToLower(child.Data)
			if activeElements[name] {
				parent.RemoveChild(child)
			} else if !allowedElements[name] {
				sanitizeChildren(child, state)
				for grandchild := child.FirstChild; grandchild != nil; {
					grandchildNext := grandchild.NextSibling
					child.RemoveChild(grandchild)
					parent.InsertBefore(grandchild, child)
					grandchild = grandchildNext
				}
				parent.RemoveChild(child)
			} else {
				sanitizeAttributes(child, state)
				sanitizeChildren(child, state)
			}
		}
		child = next
	}
}

func sanitizeAttributes(node *html.Node, state *sanitizerState) {
	attributes := make([]html.Attribute, 0, len(node.Attr))
	for _, attribute := range node.Attr {
		name := strings.ToLower(attribute.Key)
		switch node.Data {
		case "section":
			if name == "data-block-type" || name == "data-template-code" || name == "data-modified" {
				attributes = append(attributes, html.Attribute{Key: name, Val: attribute.Val})
			}
		case "a":
			if name == "href" {
				if normalized, valid := normalizeURL(attribute.Val); valid {
					attributes = append(attributes, html.Attribute{Key: "href", Val: normalized})
				} else {
					state.invalidURL = true
				}
			}
		case "th", "td":
			if name == "colspan" || name == "rowspan" {
				if decimalSpanPattern.MatchString(attribute.Val) {
					attributes = append(attributes, html.Attribute{Key: name, Val: attribute.Val})
				} else {
					state.invalidAttribute = true
				}
			}
		}
	}
	node.Attr = attributes
}

func normalizeURL(raw string) (string, bool) {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return normalized, parsed.Host != ""
	case "mailto":
		return normalized, parsed.Opaque != "" || parsed.Path != ""
	default:
		return "", false
	}
}

func collectRequiredSections(root *html.Node) (map[string]*html.Node, bool) {
	sections := make([]*html.Node, 0, 8)
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode && strings.TrimSpace(child.Data) == "" {
			continue
		}
		if child.Type != html.ElementNode || child.Data != "section" {
			return nil, false
		}
		sections = append(sections, child)
	}
	if len(sections) != len(sectionOrder) {
		return nil, false
	}
	result := make(map[string]*html.Node, len(sections))
	for i, section := range sections {
		if countDescendantElements(section, "section") != 0 {
			return nil, false
		}
		blockType := attributeValue(section, "data-block-type")
		if blockType != sectionOrder[i] {
			return nil, false
		}
		if _, exists := result[blockType]; exists {
			return nil, false
		}
		if blockType == "legal" {
			if attributeValue(section, "data-template-code") != "quotation-legal-v1" || attributeValue(section, "data-modified") != "false" || len(section.Attr) != 3 {
				return nil, false
			}
		} else if len(section.Attr) != 1 {
			return nil, false
		}
		result[blockType] = section
	}
	return result, true
}

func newHTMLValidation() HTMLValidation {
	result := HTMLValidation{Security: make(map[string]ValidationCheck, 8), Structural: make(map[string]ValidationCheck, 8)}
	for i := 1; i <= 8; i++ {
		result.Security[fmt.Sprintf("S%d", i)] = ValidationCheck{Pass: true}
		result.Structural[fmt.Sprintf("T%d", i)] = ValidationCheck{Pass: true}
	}
	return result
}

func failCheck(checks map[string]ValidationCheck, id, detail string) {
	checks[id] = ValidationCheck{Pass: false, Detail: detail}
}

func validationError(result HTMLValidation) error {
	securityFailures := failedChecks(result.Security)
	if len(securityFailures) > 0 {
		return &SecurityValidationError{Checks: securityFailures}
	}
	structuralFailures := failedChecks(result.Structural)
	if len(structuralFailures) > 0 {
		return &StructuralValidationError{Checks: structuralFailures}
	}
	return nil
}

func failedChecks(checks map[string]ValidationCheck) []string {
	failed := make([]string, 0)
	for id, check := range checks {
		if !check.Pass {
			failed = append(failed, id)
		}
	}
	sort.Strings(failed)
	return failed
}

func parseFragment(raw string) (*html.Node, error) {
	context := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(raw), context)
	if err != nil {
		return nil, err
	}
	root := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	return root, nil
}

func renderChildren(root *html.Node) (string, error) {
	var buffer bytes.Buffer
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&buffer, child); err != nil {
			return "", err
		}
	}
	return buffer.String(), nil
}

func hashHTMLFragment(raw string) (string, error) {
	root, err := parseFragment(raw)
	if err != nil {
		return "", err
	}
	var normalized strings.Builder
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		normalizeNode(&normalized, child)
	}
	sum := sha256.Sum256([]byte(normalized.String()))
	return hex.EncodeToString(sum[:]), nil
}

func normalizeNode(buffer *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		if strings.TrimSpace(node.Data) != "" {
			buffer.WriteString("T[")
			buffer.WriteString(node.Data)
			buffer.WriteByte(']')
		}
	case html.ElementNode:
		buffer.WriteString("E[")
		buffer.WriteString(node.Data)
		attributes := append([]html.Attribute(nil), node.Attr...)
		sort.Slice(attributes, func(i, j int) bool {
			if attributes[i].Key == attributes[j].Key {
				return attributes[i].Val < attributes[j].Val
			}
			return attributes[i].Key < attributes[j].Key
		})
		for _, attribute := range attributes {
			buffer.WriteByte('|')
			buffer.WriteString(attribute.Key)
			buffer.WriteByte('=')
			buffer.WriteString(attribute.Val)
		}
		buffer.WriteByte(']')
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			normalizeNode(buffer, child)
		}
		buffer.WriteString("/E[")
		buffer.WriteString(node.Data)
		buffer.WriteByte(']')
	}
}

func element(name string, values ...any) *html.Node {
	node := &html.Node{Type: html.ElementNode, DataAtom: atom.Lookup([]byte(name)), Data: name}
	for _, value := range values {
		switch typed := value.(type) {
		case html.Attribute:
			node.Attr = append(node.Attr, typed)
		case *html.Node:
			node.AppendChild(typed)
		}
	}
	return node
}

func attr(name, value string) html.Attribute { return html.Attribute{Key: name, Val: value} }
func text(value string) *html.Node           { return &html.Node{Type: html.TextNode, Data: value} }

func renderNode(node *html.Node) string {
	var buffer bytes.Buffer
	if err := html.Render(&buffer, node); err != nil {
		return ""
	}
	return buffer.String()
}

func formatIDR(value int64) string {
	digits := strconv.FormatInt(value, 10)
	start := 0
	if strings.HasPrefix(digits, "-") {
		start = 1
	}
	for i := len(digits) - 3; i > start; i -= 3 {
		digits = digits[:i] + "." + digits[i:]
	}
	return "Rp" + digits
}

func displayResource(resource EnrichedResource) string {
	return titleIdentifier(resource.Seniority) + " " + titleIdentifier(resource.Role)
}

func titleIdentifier(value string) string {
	parts := strings.Split(value, "_")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func findFirstElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func attributeValue(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func nodeVisibleText(node *html.Node) string {
	var values []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode && strings.TrimSpace(current.Data) != "" {
			values = append(values, strings.TrimSpace(current.Data))
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(values, " ")
}

func listItemTexts(node *html.Node) []string {
	var values []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && current.Data == "li" {
			values = append(values, nodeVisibleText(current))
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return values
}

func countDescendantElements(node *html.Node, name string) int {
	count := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			count++
		}
		count += countDescendantElements(child, name)
	}
	return count
}

func containsForbiddenElement(node *html.Node) bool {
	if node.Type == html.ElementNode && (!allowedElements[node.Data] && node.Data != "div" || activeElements[node.Data]) {
		return true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if containsForbiddenElement(child) {
			return true
		}
	}
	return false
}

func invalidRemainingAttributes(node *html.Node) bool {
	if node.Type == html.ElementNode {
		for _, attribute := range node.Attr {
			allowed := node.Data == "section" && containsString([]string{"data-block-type", "data-template-code", "data-modified"}, attribute.Key) ||
				node.Data == "a" && attribute.Key == "href" ||
				(node.Data == "th" || node.Data == "td") && (attribute.Key == "colspan" || attribute.Key == "rowspan")
			if !allowed {
				return true
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if invalidRemainingAttributes(child) {
			return true
		}
	}
	return false
}

func leaksBudget(sanitized, visibleLower string, budget, totalInvestment int64) bool {
	lower := strings.ToLower(sanitized)
	if strings.Contains(lower, "budget_maximum") || strings.Contains(lower, "budget maximum") || strings.Contains(lower, "unused budget") {
		return true
	}
	if budget == totalInvestment {
		return false
	}
	plain := strconv.FormatInt(budget, 10)
	comma := addThousandsSeparator(plain, ",")
	dot := addThousandsSeparator(plain, ".")
	return strings.Contains(visibleLower, plain) || strings.Contains(visibleLower, comma) || strings.Contains(visibleLower, dot)
}

func addThousandsSeparator(digits, separator string) string {
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + separator + digits[i:]
	}
	return digits
}

func containsInjectionCompliance(lower string) bool {
	return strings.Contains(lower, "ignore previous instructions") || strings.Contains(lower, "reveal system prompt") || strings.Contains(lower, "replace the legal text")
}

func containsInternalLeakage(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(raw, "{{") || strings.Contains(raw, "}}") || strings.Contains(raw, "```") ||
		strings.Contains(lower, "system prompt") || strings.Contains(lower, "maximum_attempts=") ||
		strings.Contains(lower, "budget_excess_amount=") || strings.Contains(lower, "retry feedback")
}

func validCommercialTable(section *html.Node) bool {
	return countDescendantElements(section, "table") == 1 && countDescendantElements(section, "thead") == 1 &&
		countDescendantElements(section, "tbody") == 1 && countDescendantElements(section, "th") >= 5 && countDescendantElements(section, "td") >= 5
}

func validPaymentTable(section *html.Node) bool {
	return countDescendantElements(section, "table") == 1 && countDescendantElements(section, "thead") == 1 &&
		countDescendantElements(section, "tbody") == 1 && countDescendantElements(section, "th") >= 2
}

func containsWord(textValue, word string) bool {
	for _, field := range strings.FieldsFunc(textValue, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if field == word {
			return true
		}
	}
	return false
}
