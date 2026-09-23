package service

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const WhiteLabelOpeningParagraph = "Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek. Dokumen ini memuat ruang lingkup pekerjaan, output, timeline, dan rincian biaya sebagai acuan kesepakatan bersama kedua belah pihak."

type WhiteLabelDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.WhiteLabelInput
	Calculation domain.WhiteLabelCalculation
	Profile     domain.ResolvedWhiteLabelProfile
	LegalHTML   string
}

type WhiteLabelHTMLRenderer interface {
	Render(WhiteLabelDocumentInput) (string, error)
}

type whiteLabelHTMLRenderer struct{}

func NewWhiteLabelHTMLRenderer() WhiteLabelHTMLRenderer {
	return &whiteLabelHTMLRenderer{}
}

func (r *whiteLabelHTMLRenderer) Render(in WhiteLabelDocumentInput) (string, error) {
	if len(in.Calculation.License) == 0 {
		return "", fmt.Errorf("license: calculation lines are empty")
	}
	if len(in.Input.PaymentSchedule) == 0 {
		return "", fmt.Errorf("payment_schedule: payment schedule rows are empty")
	}
	if strings.TrimSpace(in.LegalHTML) == "" {
		return "", fmt.Errorf("legal_html: legal template snapshot is empty")
	}

	profile := in.Profile
	if len(profile.Sections) == 0 {
		var err error
		profile, err = ResolveWhiteLabelProfile(in.Input)
		if err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString(`<article class="quotation-document">`)
	b.WriteString(renderWhiteLabelHeader(in.Metadata))
	b.WriteString(`<section data-block-type="opening"><p>` + WhiteLabelOpeningParagraph + `</p></section>`)

	for _, section := range profile.Sections {
		switch section.Code {
		case "white_label_cost":
			b.WriteString(renderWhiteLabelCommercial(section, in.Calculation.License, in.Calculation.LicenseSubtotal))
		case "maintenance_hosting":
			b.WriteString(renderWhiteLabelMaintenanceHosting(section, in.Calculation.Maintenance, in.Calculation.Hosting, in.Calculation.MaintenanceHostingSubtotal))
		case "scope_timeline":
			b.WriteString(renderWhiteLabelScopeTimeline(section, in.Input.ModuleGroups, in.Input.TimelineRows))
		case "payment":
			b.WriteString(renderWhiteLabelPayments(section, in.Input.PaymentSchedule, in.Calculation.PaymentTotal, profile.TaxClauseText))
			if profile.ShowRights && len(in.Input.RightsStatements) > 0 {
				b.WriteString(renderWhiteLabelRights(in.Input.RightsStatements))
			}
		case "limitations":
			b.WriteString(renderWhiteLabelLegal(section, in.LegalHTML, profile.TaxTreatment))
		}
	}

	b.WriteString(renderWhiteLabelSignatures(in.Metadata))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderWhiteLabelHeader(m QuotationMetadata) string {
	return renderOfficialQuotationHeader("PENAWARAN LAYANAN WHITE LABEL PRODUCT", m)
}

func renderWhiteLabelCommercial(section domain.DocumentSection, items []domain.WhiteLabelLicenseLineCalculation, subtotal int64) string {
	var b strings.Builder
	writeSectionStart(&b, section, "commercial")
	b.WriteString(`<table><thead><tr><th>Nama Paket</th><th>Rate</th><th>Jumlah</th><th>Total</th></tr></thead><tbody>`)
	for i, item := range items {
		fmt.Fprintf(&b, `<tr data-role="license_%d"><td>%s</td><td>Rp %s</td><td>%d</td><td data-field="license_line_total">Rp %s</td></tr>`,
			i,
			htmlEscape(item.Name),
			formatNumber(item.UnitRate),
			item.Quantity,
			formatNumber(item.LineTotal),
		)
	}
	fmt.Fprintf(&b, `<tr><td colspan="3"><strong>TOTAL INVESTMENT</strong></td><td data-field="total_license"><strong>Rp %s</strong></td></tr>`,
		formatNumber(subtotal),
	)
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderWhiteLabelMaintenanceHosting(section domain.DocumentSection, maintenance []domain.WhiteLabelMaintenanceLineCalculation, hosting []domain.WhiteLabelHostingLineCalculation, totalMH int64) string {
	var b strings.Builder
	writeSectionStart(&b, section, "commercial-maintenance-hosting")
	b.WriteString(`<table><thead><tr><th>Nama Paket / Komponen</th><th>Rate (Monthly)</th><th>Employee</th><th>Period (Month)</th><th>Total</th></tr></thead><tbody>`)

	if len(maintenance) > 0 {
		b.WriteString(`<tr><td colspan="5"><strong>Maintenance</strong></td></tr>`)
		for i, item := range maintenance {
			fmt.Fprintf(&b, `<tr data-role="maintenance_%d"><td>%s</td><td>Rp %s</td><td>%s</td><td>%d Bulan</td><td data-field="maintenance_line_total">Rp %s</td></tr>`,
				i,
				htmlEscape(item.Name),
				formatNumber(item.MonthlyRate),
				formatFTE(item.FTEQuarters),
				item.PeriodMonths,
				formatNumber(item.LineTotal),
			)
		}
	}

	if len(hosting) > 0 {
		b.WriteString(`<tr><td colspan="5"><strong>Hosting</strong></td></tr>`)
		for i, item := range hosting {
			empLabel := "-"
			if item.Quantity > 1 {
				empLabel = fmt.Sprintf("%d Unit", item.Quantity)
			}
			fmt.Fprintf(&b, `<tr data-role="hosting_%d"><td>%s</td><td>Rp %s</td><td>%s</td><td>%d Bulan</td><td data-field="hosting_line_total">Rp %s</td></tr>`,
				i,
				htmlEscape(item.Name),
				formatNumber(item.MonthlyRate),
				empLabel,
				item.PeriodMonths,
				formatNumber(item.LineTotal),
			)
		}
	}

	fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="maintenance_hosting_total"><strong>Rp %s</strong></td></tr>`,
		formatNumber(totalMH),
	)
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderWhiteLabelScopeTimeline(section domain.DocumentSection, groups []domain.WhiteLabelModuleGroupInput, timeline []domain.WhiteLabelTimelineRowInput) string {
	var b strings.Builder
	writeSectionStart(&b, section, "scope")

	for _, g := range groups {
		fmt.Fprintf(&b, `<h3>%s</h3>`, htmlEscape(g.Name))
		if len(g.Features) > 0 {
			b.WriteString(`<ul>`)
			for _, f := range g.Features {
				fmt.Fprintf(&b, `<li>%s</li>`, htmlEscape(f))
			}
			b.WriteString(`</ul>`)
		}
	}

	if len(timeline) > 0 {
		b.WriteString(`<section data-block-type="timeline">`)
		b.WriteString(`<table><thead><tr><th data-column-width="75">Description</th><th data-column-width="25">Timeline</th></tr></thead><tbody>`)
		for _, row := range timeline {
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td></tr>`, htmlEscape(row.Description), htmlEscape(row.Timeline))
		}
		b.WriteString(`</tbody></table></section>`)
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderWhiteLabelPayments(section domain.DocumentSection, rows []domain.WhiteLabelPaymentRowInput, paymentTotal int64, taxClauseText string) string {
	var b strings.Builder
	writeSectionStart(&b, section, "payment")
	b.WriteString(`<table><thead><tr><th>Work Period</th><th>Terms</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th></tr></thead><tbody>`)
	for i, row := range rows {
		fmt.Fprintf(&b, `<tr data-role="payment_%d" data-component="%s" data-payment-index="%d"><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`,
			i,
			htmlEscape(string(row.Component)),
			i,
			htmlEscape(row.WorkPeriod),
			htmlEscape(row.Terms),
			htmlEscape(row.InvoiceDateLabel),
			htmlEscape(row.DueDateLabel),
			formatNumber(row.Nominal),
		)
	}
	fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="payment_total"><strong>Rp %s</strong></td></tr>`,
		formatNumber(paymentTotal),
	)
	b.WriteString(`</tbody></table>`)
	if taxClauseText != "" {
		fmt.Fprintf(&b, `<p data-field="tax_clause">%s</p>`, htmlEscape(taxClauseText))
	}
	b.WriteString(`</section>`)
	return b.String()
}

func renderWhiteLabelRights(statements []string) string {
	var b strings.Builder
	b.WriteString(`<div data-block-type="rights">`)
	for _, stmt := range statements {
		fmt.Fprintf(&b, `<p>%s</p>`, htmlEscape(stmt))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// ResolveWhiteLabelLegalHTML normalizes legal template HTML for White Label documents.
// Since White Label explicitly manages its tax clause in the Payment section
// according to tax_treatment, any duplicate or default tax clause paragraph
// in the legal template (marked via data-field="tax_clause" or fallback matching
// standard PPN disclaimer phrases) is stripped so the document contains no duplicate
// or contradictory tax clauses. Custom non-tax legal clauses are strictly preserved.
func ResolveWhiteLabelLegalHTML(legalHTML string, _ domain.WhiteLabelTaxTreatment) (string, error) {
	if strings.TrimSpace(legalHTML) == "" {
		return "", nil
	}

	root, err := html.Parse(strings.NewReader("<div>" + legalHTML + "</div>"))
	if err != nil {
		return "", fmt.Errorf("failed to parse legal html: %w", err)
	}

	var div *html.Node
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		if n.Type == html.ElementNode && n.Data == "html" {
			for b := n.FirstChild; b != nil; b = b.NextSibling {
				if b.Type == html.ElementNode && b.Data == "body" {
					for d := b.FirstChild; d != nil; d = d.NextSibling {
						if d.Type == html.ElementNode && d.Data == "div" {
							div = d
							break
						}
					}
				}
			}
		}
	}
	if div == nil {
		div = root
	}

	nodes := collectNodes(div)
	for _, node := range nodes {
		if node.Type != html.ElementNode {
			continue
		}
		isTaxField := attributeValue(node, "data-field") == "tax_clause"
		isLegacyTaxPara := false
		if node.Data == "p" && !isTaxField {
			text := strings.TrimSpace(nodeText(node))
			if strings.Contains(text, "belum termasuk Pajak Pertambahan Nilai") ||
				strings.Contains(text, "belum termasuk PPN") ||
				strings.Contains(text, "telah termasuk Pajak Pertambahan Nilai") ||
				strings.Contains(text, "telah termasuk PPN") ||
				strings.Contains(text, "termasuk Pajak Pertambahan Nilai (PPN)") {
				isLegacyTaxPara = true
			}
		}

		if isTaxField || isLegacyTaxPara {
			if node.Parent != nil {
				node.Parent.RemoveChild(node)
			}
		}
	}

	var buf bytes.Buffer
	for child := div.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&buf, child); err != nil {
			return "", fmt.Errorf("failed to render resolved legal html: %w", err)
		}
	}

	return buf.String(), nil
}

func renderWhiteLabelLegal(section domain.DocumentSection, legalHTML string, taxTreatment domain.WhiteLabelTaxTreatment) string {
	var b strings.Builder
	writeSectionStart(&b, section, "legal")
	resolved, err := ResolveWhiteLabelLegalHTML(legalHTML, taxTreatment)
	if err != nil {
		b.WriteString(legalHTML)
	} else {
		b.WriteString(resolved)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func renderWhiteLabelSignatures(m QuotationMetadata) string {
	var b strings.Builder
	b.WriteString(`<section data-block-type="signature">`)
	if m.IssueDate != "" {
		b.WriteString(`<p>Malang, <span data-field="issue_date">` + htmlEscape(m.IssueDate) + `</span></p>`)
	}
	b.WriteString(`<table class="signature-table"><tbody><tr>`)
	b.WriteString(`<td><p>Hormat Kami,</p><br><br><p><strong>Yulianto Frandi</strong></p><p>Direksi Venturo Pro</p></td>`)

	clientName := m.ClientName
	if clientName == "" {
		clientName = "(Nama Klien)"
	}
	b.WriteString(`<td><p>Disetujui Oleh,</p><br><br><p><strong>` + htmlEscape(clientName) + `</strong></p>`)
	if m.ClientCompanyName != "" {
		b.WriteString(`<p>` + htmlEscape(m.ClientCompanyName) + `</p>`)
	}
	b.WriteString(`</td></tr></tbody></table></section>`)
	return b.String()
}

func formatFTE(fteQuarters int32) string {
	switch fteQuarters {
	case 1:
		return "0.25"
	case 2:
		return "0.5"
	case 3:
		return "0.75"
	case 4:
		return "1"
	default:
		return fmt.Sprintf("%.2f", float64(fteQuarters)/4.0)
	}
}
