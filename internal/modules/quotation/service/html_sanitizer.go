package service

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

type HTMLSanitizer interface {
	Sanitize(rawHTML string) (string, error)
}

type defaultHTMLSanitizer struct {
	policy *bluemonday.Policy
}

var bareAnchorPattern = regexp.MustCompile(`(?i)<a\s*>`)

func NewHTMLSanitizer() HTMLSanitizer {
	return &defaultHTMLSanitizer{policy: buildStrictQuotationPolicy()}
}

func buildStrictQuotationPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements(
		"p", "strong", "em", "u", "s", "strike", "del",
		"ul", "ol", "li",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"br", "hr", "blockquote",
		"table", "thead", "tbody", "tr", "th", "td",
		"span", "div", "section", "article", "header",
	)

	p.AllowElements("a")
	// Keep a bare narrative anchor as a semantic tag, but reject every URL
	// attribute in generated content. Dedicated documents never need links;
	// this also prevents external navigation from user/AI supplied HTML.
	p.AllowAttrs("href").Matching(regexp.MustCompile(`^$`)).OnElements("a")

	p.AllowAttrs("colspan", "rowspan").Matching(regexp.MustCompile(`^[0-9]{1,3}$`)).OnElements("td", "th")

	p.SkipElementsContent("script", "style", "iframe", "object", "embed", "noscript")

	markerPattern := regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^(quotation-document|signature-table|qtr-block|qtr-legal)( (quotation-document|signature-table|qtr-block|qtr-legal))*$`)).Globally()
	p.AllowAttrs("data-block", "data-field", "data-role", "data-block-type", "data-template-code", "data-modified").Matching(markerPattern).Globally()
	p.AllowAttrs("data-section-number").Matching(regexp.MustCompile(`^[1-9][0-9]{0,2}$`)).Globally()
	p.AllowAttrs("data-column-width").Matching(regexp.MustCompile(`^(25|75)$`)).OnElements("th", "td")

	return p
}

func (s *defaultHTMLSanitizer) Sanitize(rawHTML string) (string, error) {
	// bluemonday drops an otherwise harmless bare <a> element because anchors
	// normally require a URL. Preserve the semantic tag while still stripping
	// every href/URL attribute by using an internal marker during sanitization.
	marked := bareAnchorPattern.ReplaceAllString(rawHTML, `<a data-bare-anchor="1">`)
	markedPolicy := s.policy
	// The marker is allowed only for this internal round-trip and is removed
	// immediately after policy processing.
	markedPolicy.AllowAttrs("data-bare-anchor").Matching(regexp.MustCompile(`^1$`)).OnElements("a")
	out := markedPolicy.Sanitize(marked)
	return strings.ReplaceAll(out, ` data-bare-anchor="1"`, ""), nil
}
