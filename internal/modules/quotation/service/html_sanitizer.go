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
		"p", "strong", "em",
		"ul", "ol", "li",
		"h1", "h2", "h3",
		"br",
		"table", "thead", "tbody", "tr", "th", "td",
		"span", "div",
	)

	p.SkipElementsContent("script", "style", "iframe", "object", "embed", "noscript")

	markerPattern := regexp.MustCompile(`^[a-z_]{1,64}$`)
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).Globally()
	p.AllowAttrs("data-block", "data-field", "data-role").Matching(markerPattern).Globally()

	return p
}

func (s *defaultHTMLSanitizer) Sanitize(rawHTML string) (string, error) {
	return s.policy.Sanitize(rawHTML), nil
}
