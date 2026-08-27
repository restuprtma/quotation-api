package service

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

type HTMLSanitizer interface {
	Sanitize(rawHTML string) (string, error)
}

type defaultHTMLSanitizer struct {
	policy *bluemonday.Policy
}

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
		"span", "div", "section",
	)

	p.AllowElements("a")
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowAttrs("href").OnElements("a")

	p.AllowAttrs("colspan", "rowspan").Matching(regexp.MustCompile(`^[0-9]{1,3}$`)).OnElements("td", "th")

	p.SkipElementsContent("script", "style", "iframe", "object", "embed", "noscript")

	markerPattern := regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).Globally()
	p.AllowAttrs("data-block", "data-field", "data-role", "data-block-type", "data-template-code", "data-modified").Matching(markerPattern).Globally()

	return p
}

func (s *defaultHTMLSanitizer) Sanitize(rawHTML string) (string, error) {
	return s.policy.Sanitize(rawHTML), nil
}
