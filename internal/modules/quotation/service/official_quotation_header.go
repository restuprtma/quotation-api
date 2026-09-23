package service

import (
	"html"
	"strings"
)

type officialHeaderRow struct {
	Field string
	Label string
	Value string
}

var officialHeaderFields = []string{
	"quotation_number",
	"subject",
	"issue_date_header",
	"client_pic",
	"client_company",
}

const (
	OfficialHeaderBlockType = "metadata"

	OfficialHeaderRoleBrand            = "quotation-brand"
	OfficialHeaderRoleLayout           = "metadata-layout"
	OfficialHeaderRoleFacts            = "metadata-facts"
	OfficialHeaderRoleRecipient        = "metadata-recipient"
	OfficialHeaderRoleRecipientHeading = "recipient-heading"
	OfficialHeaderRoleRecipientLine    = "recipient-line"
	OfficialHeaderRoleRecipientPrefix  = "recipient-prefix"
	OfficialHeaderRoleRow              = "metadata-row"
	OfficialHeaderRoleLabel            = "metadata-label"
	OfficialHeaderRoleSeparator        = "metadata-separator"
	OfficialHeaderRoleValue            = "metadata-value"

	OfficialHeaderLogoSrc = "/logo/venturo-logo.png"
	OfficialHeaderLogoAlt = "Venturo — Expert Programmers"

	OfficialHeaderFieldQuotationNumber = "quotation_number"
	OfficialHeaderFieldSubject         = "subject"
	OfficialHeaderFieldIssueDate       = "issue_date_header"
	OfficialHeaderFieldClientPic       = "client_pic"
	OfficialHeaderFieldClientCompany   = "client_company"
	OfficialHeaderFieldSystemName      = "system_name"

	OfficialHeaderRecipientHeadingText = "Kepada"
	OfficialHeaderRecipientPrefixYth   = "Yth. "
)

// RenderOfficialQuotationHeader renders the canonical editable quotation metadata
// header with brand logo, centered title, facts column, and recipient column.
func RenderOfficialQuotationHeader(
	title string,
	m QuotationMetadata,
	optionalRows ...officialHeaderRow,
) string {
	return renderOfficialQuotationHeader(title, m, optionalRows...)
}

// renderOfficialQuotationHeader renders the canonical editable quotation metadata
// header with brand logo, centered title, facts column, and recipient column.
func renderOfficialQuotationHeader(
	title string,
	m QuotationMetadata,
	optionalRows ...officialHeaderRow,
) string {
	var b strings.Builder
	b.WriteString(`<section data-block-type="metadata">`)
	b.WriteString(`<div data-role="quotation-brand"><img src="/logo/venturo-logo.png" alt="Venturo — Expert Programmers"></div>`)
	b.WriteString(`<h1>` + html.EscapeString(title) + `</h1>`)
	b.WriteString(`<div data-role="metadata-layout">`)
	b.WriteString(`<div data-role="metadata-facts">`)
	writeOfficialFactRow(&b, "quotation_number", "No", m.QuotationNumber)
	writeOfficialFactRow(&b, "subject", "Perihal", m.Subject)
	writeOfficialFactRow(&b, "issue_date_header", "Tanggal", m.IssueDate)
	for _, row := range optionalRows {
		writeOfficialFactRow(&b, row.Field, row.Label, row.Value)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div data-role="metadata-recipient">`)
	b.WriteString(`<div data-role="recipient-heading">Kepada</div>`)
	writeOfficialRecipientLine(&b, "client_pic", "Yth. ", m.ClientName)
	writeOfficialRecipientLine(&b, "client_company", "", m.ClientCompanyName)
	b.WriteString(`</div></div></section>`)
	return b.String()
}

func writeOfficialFactRow(b *strings.Builder, field, label, value string) {
	b.WriteString(`<div data-role="metadata-row" data-field="` + field + `">`)
	b.WriteString(`<span data-role="metadata-label">` + html.EscapeString(label) + `</span>`)
	b.WriteString(`<span data-role="metadata-separator">:</span>`)
	b.WriteString(`<span data-role="metadata-value">` + html.EscapeString(value) + `</span></div>`)
}

func writeOfficialRecipientLine(b *strings.Builder, field, prefix, value string) {
	b.WriteString(`<div data-role="recipient-line" data-field="` + field + `">`)
	if prefix != "" {
		b.WriteString(`<span data-role="recipient-prefix">` + html.EscapeString(prefix) + `</span>`)
	}
	b.WriteString(`<span data-role="metadata-value">` + html.EscapeString(value) + `</span></div>`)
}
