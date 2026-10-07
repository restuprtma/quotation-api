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

	p.AllowElements("img")
	p.AllowAttrs("src").Matching(regexp.MustCompile(`^/logo/venturo-logo\.png$`)).OnElements("img")
	p.AllowAttrs("alt").Matching(regexp.MustCompile(`^Venturo — Expert Programmers$`)).OnElements("img")

	p.AllowElements("a")
	// Keep a bare narrative anchor as a semantic tag, but reject every URL
	// attribute in generated content. Dedicated documents never need links;
	// this also prevents external navigation from user/AI supplied HTML.
	p.AllowAttrs("href").Matching(regexp.MustCompile(`^$`)).OnElements("a")

	p.AllowAttrs("colspan", "rowspan").Matching(regexp.MustCompile(`^[0-9]{1,3}$`)).OnElements("td", "th")

	p.SkipElementsContent("script", "style", "iframe", "object", "embed", "noscript")

	markerPattern := regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
	sectionMarkerPattern := regexp.MustCompile(`^(scope|commercial|payment|timeline|narrative|rights|limitations|legal|signature|commercial-development|commercial-monthly|information-missing|deleted-(rights|limitations|payment|timeline|signature)|removed-(rights|limitations|payment|timeline|signature))$`)
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^(quotation-document|quotation-document--cloud-infrastructure|kiosk-queue-document|signature-table|qtr-block|qtr-legal)( (quotation-document|quotation-document--cloud-infrastructure|kiosk-queue-document|signature-table|qtr-block|qtr-legal))*$`)).Globally()
	p.AllowAttrs("data-section", "data-block", "data-role", "data-block-type", "data-template-code", "data-modified", "data-component", "data-payment-index", "data-quotation-field", "data-quotation-component").Matching(markerPattern).Globally()
	p.AllowAttrs("data-quotation-section").Matching(sectionMarkerPattern).OnElements("section", "div", "header")
	p.AllowAttrs("data-section-version").Matching(regexp.MustCompile(`^[1-9][0-9]{0,2}$`)).OnElements("section")
	p.AllowAttrs("data-managed").Matching(regexp.MustCompile(`^(true|false)$`)).OnElements("section")
	p.AllowAttrs("data-field").Matching(regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,96}$`)).Globally()
	p.AllowAttrs("data-section-number", "data-quotation-section-number").Matching(regexp.MustCompile(`^[1-9][0-9]{0,2}$`)).Globally()
	p.AllowAttrs("data-column-width").Matching(regexp.MustCompile(`^(25|75)$`)).OnElements("th", "td")
	p.AllowAttrs("data-service").Matching(regexp.MustCompile(`^talent_outsourcing$`)).OnElements("article")
	p.AllowAttrs("data-section-code").Matching(markerPattern).OnElements("section")
	p.AllowAttrs("data-row-index", "data-group-index").Matching(regexp.MustCompile(`^[0-9]{1,3}$`)).OnElements("tr", "td", "th", "div", "li")
	p.AllowAttrs("data-limitation-code").Matching(markerPattern).OnElements("tr", "td", "th", "div", "li")
	p.AllowAttrs("data-value").Matching(regexp.MustCompile(`^[0-9]{1,16}$`)).OnElements("tr", "td", "th", "div", "li")
	// Cloud infrastructure documents use explicit markers so their commercial
	// snapshot can be audited after sanitization without trusting text search.
	p.AllowAttrs("data-cloud-package-row", "data-cloud-setup-row", "data-cloud-total-row", "data-cloud-scope-row", "data-cloud-payment-row").Matching(regexp.MustCompile(`^[0-9]{1,3}$|^(monthly|setup)$`)).OnElements("tr")
	p.AllowAttrs("data-cloud-package-field", "data-cloud-field", "data-cloud-scope-field").Matching(markerPattern).OnElements("td", "th", "p", "span")
	p.AllowAttrs("data-cloud-payment-component").Matching(regexp.MustCompile(`^(setup|monthly_service)$`)).OnElements("tr")
	p.AllowAttrs("data-signature-block").Matching(regexp.MustCompile(`^true$`)).OnElements("section")

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
