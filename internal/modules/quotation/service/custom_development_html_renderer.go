package service

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	nethtml "golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const CustomDevelopmentOpeningParagraph = "Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek. Dokumen ini memuat ruang lingkup pekerjaan, output, timeline, dan rincian biaya sebagai acuan kesepakatan bersama kedua belah pihak."

var CustomDevelopmentLimitationCatalog = map[string]string{
	"out_of_pocket": "Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.",
	"overtime":      "Percepatan pekerjaan di luar jam kerja reguler (malam hari, Sabtu, Minggu, atau hari libur nasional) dikenakan biaya lembur tersendiri.",
	"third_party":   "Lisensi pihak ketiga, credit API AI (OpenAI/Anthropic), Payment Gateway, WhatsApp API / SMS Gateway, domain, dan server cloud di luar paket yang disepakati.",
	"scope_change":  "Penambahan fitur baru, perubahan alur proses yang telah di-approve, atau penundaan approval dari klien yang mengakibatkan masa kontrak molor.",
}

type CustomDevelopmentDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.CustomDevelopmentInput
	Calculation domain.CustomDevelopmentCalculation
	Profile     domain.ResolvedCustomDevelopmentProfile
	LegalHTML   string
}

type CustomDevelopmentRenderer interface {
	Render(CustomDevelopmentDocumentInput) (string, error)
}

type CustomDevelopmentHTMLRenderer = CustomDevelopmentRenderer

type customDevelopmentRenderer struct{}

func NewCustomDevelopmentRenderer() CustomDevelopmentRenderer {
	return &customDevelopmentRenderer{}
}

func NewCustomDevelopmentHTMLRenderer() CustomDevelopmentRenderer {
	return NewCustomDevelopmentRenderer()
}

func (r *customDevelopmentRenderer) Render(in CustomDevelopmentDocumentInput) (string, error) {
	if len(in.Input.DevelopmentGroups) == 0 {
		return "", fmt.Errorf("development_groups: groups are empty")
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
		profile, err = ResolveCustomDevelopmentProfile(in.Input)
		if err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString(`<article class="quotation-document">`)
	b.WriteString(renderCustomDevelopmentHeader(in.Metadata))
	b.WriteString(`<section data-block-type="opening"><p>` + CustomDevelopmentOpeningParagraph + `</p></section>`)

	for _, section := range profile.Sections {
		switch section.Code {
		case "development":
			b.WriteString(renderCustomDevelopmentCommercial(section, in.Input, in.Calculation, profile))
		case "monthly":
			if profile.ShowMonthly && len(in.Input.MonthlyItems) > 0 {
				b.WriteString(renderCustomDevelopmentMonthly(section, in.Input, in.Calculation))
			}
		case "scope":
			b.WriteString(renderCustomDevelopmentScope(section, in.Input))
		case "payment":
			b.WriteString(renderCustomDevelopmentPayment(section, in.Input, in.Calculation, profile))
		case "rights":
			if profile.ShowRights && len(in.Input.RightsStatements) > 0 {
				b.WriteString(renderCustomDevelopmentRights(section, in.Input.RightsStatements))
			}
		case "limitations":
			b.WriteString(renderCustomDevelopmentLimitations(section, in.Input))
		}
	}

	b.WriteString(renderCustomDevelopmentLegal(in.LegalHTML, in.Input.TaxTreatment))
	b.WriteString(renderCustomDevelopmentSignatures(in.Metadata))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderCustomDevelopmentHeader(m QuotationMetadata) string {
	return renderOfficialQuotationHeader("PENAWARAN LAYANAN CUSTOM SOFTWARE DEVELOPMENT", m)
}

func renderCustomDevelopmentCommercial(section domain.DocumentSection, in domain.CustomDevelopmentInput, calc domain.CustomDevelopmentCalculation, profile domain.ResolvedCustomDevelopmentProfile) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section data-quotation-section="commercial-development" data-quotation-section-number="%d">`, section.Number)
	fmt.Fprintf(&b, `<h2>%d. %s</h2>`, section.Number, html.EscapeString(section.Title))
	b.WriteString(`<table><thead><tr>`)
	b.WriteString(`<th>Deskripsi</th>`)
	if profile.ShowRateColumn {
		fmt.Fprintf(&b, `<th>%s</th>`, html.EscapeString(profile.RateHeader))
	}
	b.WriteString(`<th>SDM</th>`)
	fmt.Fprintf(&b, `<th>%s</th>`, html.EscapeString(profile.PeriodHeader))
	b.WriteString(`<th>Total</th>`)
	b.WriteString(`</tr></thead><tbody>`)

	colspanHeading := 5
	colspanSummary := 4
	if !profile.ShowRateColumn {
		colspanHeading = 4
		colspanSummary = 3
	}

	for gIdx, group := range in.DevelopmentGroups {
		fmt.Fprintf(&b, `<tr><td colspan="%d"><strong>%s</strong></td></tr>`, colspanHeading, html.EscapeString(group.Title))
		for iIdx, item := range group.Items {
			fmt.Fprintf(&b, `<tr data-role="development_line_%d_%d">`, gIdx, iIdx)
			fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(item.Name))
			if profile.ShowRateColumn {
				if item.DisplayRate != nil {
					fmt.Fprintf(&b, `<td>Rp %s</td>`, formatNumber(*item.DisplayRate))
				} else {
					b.WriteString(`<td>-</td>`)
				}
			}
			fmt.Fprintf(&b, `<td>%d</td>`, item.Headcount)
			fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(item.PeriodLabel))
			if item.QuotedLineTotal != nil {
				fmt.Fprintf(&b, `<td data-quotation-field="development_line_%d_%d">Rp %s</td>`, gIdx, iIdx, formatNumber(*item.QuotedLineTotal))
			} else {
				fmt.Fprintf(&b, `<td data-quotation-field="development_line_%d_%d"></td>`, gIdx, iIdx)
			}
			b.WriteString(`</tr>`)
		}
	}

	// Subtotal
	fmt.Fprintf(&b, `<tr><td colspan="%d"><strong>Subtotal</strong></td><td data-quotation-field="development_subtotal"><strong>Rp %s</strong></td></tr>`,
		colspanSummary, formatNumber(calc.DevelopmentSubtotal))

	// Discount if showDiscountRow
	if profile.ShowDiscountRow {
		discountLabel := strings.TrimSpace(in.DiscountLabel)
		if discountLabel == "" {
			discountLabel = "DISCOUNT"
		}
		fmt.Fprintf(&b, `<tr><td colspan="%d"><strong>%s</strong></td><td data-quotation-field="discount_amount"><strong>Rp %s</strong></td></tr>`,
			colspanSummary, html.EscapeString(discountLabel), formatNumber(calc.DiscountAmount))
	}

	// Total Investment
	fmt.Fprintf(&b, `<tr><td colspan="%d"><strong>TOTAL INVESTMENT</strong></td><td data-quotation-field="total_investment"><strong>Rp %s</strong></td></tr>`,
		colspanSummary, formatNumber(calc.TotalInvestment))

	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderCustomDevelopmentMonthly(section domain.DocumentSection, in domain.CustomDevelopmentInput, calc domain.CustomDevelopmentCalculation) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section data-quotation-section="commercial-monthly" data-quotation-section-number="%d">`, section.Number)
	fmt.Fprintf(&b, `<h2>%d. %s</h2>`, section.Number, html.EscapeString(section.Title))
	b.WriteString(`<table><thead><tr><th>Deskripsi</th><th>Spesifikasi</th><th>Biaya / Unit</th><th>Jumlah</th><th>Total</th></tr></thead><tbody>`)

	for i, item := range calc.MonthlyLines {
		fmt.Fprintf(&b, `<tr data-role="monthly_%d">`, i)
		fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(item.Name))
		fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(item.Specification))
		fmt.Fprintf(&b, `<td>Rp %s</td>`, formatNumber(item.UnitRate))
		fmt.Fprintf(&b, `<td>%d</td>`, item.Quantity)
		fmt.Fprintf(&b, `<td data-quotation-field="monthly_line_%d">Rp %s</td>`, i, formatNumber(item.LineTotal))
		b.WriteString(`</tr>`)
	}

	fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-quotation-field="monthly_total"><strong>Rp %s</strong></td></tr>`,
		formatNumber(calc.MonthlyTotal))
	b.WriteString(`</tbody></table>`)

	if len(in.MonthlyNotes) > 0 {
		b.WriteString(`<ul data-quotation-component="monthly-notes">`)
		for _, note := range in.MonthlyNotes {
			fmt.Fprintf(&b, `<li>%s</li>`, html.EscapeString(note))
		}
		b.WriteString(`</ul>`)
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderCustomDevelopmentScope(section domain.DocumentSection, in domain.CustomDevelopmentInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section data-quotation-section="scope" data-quotation-section-number="%d">`, section.Number)
	fmt.Fprintf(&b, `<h2>%d. %s</h2>`, section.Number, html.EscapeString(section.Title))

	for _, group := range in.ModuleGroups {
		fmt.Fprintf(&b, `<h3>%s</h3>`, html.EscapeString(group.Name))
		if len(group.Features) > 0 {
			b.WriteString(`<ul>`)
			for _, feat := range group.Features {
				fmt.Fprintf(&b, `<li>%s</li>`, html.EscapeString(feat))
			}
			b.WriteString(`</ul>`)
		}
	}

	if len(in.TimelineRows) > 0 {
		b.WriteString(`<section data-quotation-section="timeline">`)
		b.WriteString(`<table><thead><tr><th data-column-width="75">Description</th><th data-column-width="25">Timeline</th></tr></thead><tbody>`)
		for _, row := range in.TimelineRows {
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td></tr>`, html.EscapeString(row.Description), html.EscapeString(row.Timeline))
		}
		b.WriteString(`</tbody></table></section>`)
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderCustomDevelopmentPayment(section domain.DocumentSection, in domain.CustomDevelopmentInput, calc domain.CustomDevelopmentCalculation, profile domain.ResolvedCustomDevelopmentProfile) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section data-quotation-section="payment" data-quotation-section-number="%d">`, section.Number)
	fmt.Fprintf(&b, `<h2>%d. %s</h2>`, section.Number, html.EscapeString(section.Title))
	b.WriteString(`<table><thead><tr><th>Work Period</th><th>Terms</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th></tr></thead><tbody>`)

	for i, row := range in.PaymentSchedule {
		fmt.Fprintf(&b, `<tr data-role="payment_%d" data-quotation-component="%s">`, i, html.EscapeString(string(row.Component)))
		fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(row.WorkPeriod))
		fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(row.Terms))
		fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(row.InvoiceDateLabel))
		fmt.Fprintf(&b, `<td>%s</td>`, html.EscapeString(row.DueDateLabel))
		fmt.Fprintf(&b, `<td data-quotation-field="payment_nominal_%d">Rp %s</td>`, i, formatNumber(row.Nominal))
		b.WriteString(`</tr>`)
	}

	b.WriteString(`</tbody></table>`)
	if profile.TaxClauseText != "" {
		fmt.Fprintf(&b, `<p data-quotation-field="tax_clause">%s</p>`, html.EscapeString(profile.TaxClauseText))
	}
	b.WriteString(`</section>`)
	return b.String()
}

func renderCustomDevelopmentRights(section domain.DocumentSection, statements []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section data-quotation-section="rights" data-quotation-section-number="%d">`, section.Number)
	fmt.Fprintf(&b, `<h2>%d. %s</h2>`, section.Number, html.EscapeString(section.Title))
	for _, stmt := range statements {
		fmt.Fprintf(&b, `<p>%s</p>`, html.EscapeString(stmt))
	}
	b.WriteString(`</section>`)
	return b.String()
}

func renderCustomDevelopmentLimitations(section domain.DocumentSection, in domain.CustomDevelopmentInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<section data-quotation-section="limitations" data-quotation-section-number="%d">`, section.Number)
	fmt.Fprintf(&b, `<h2>%d. %s</h2>`, section.Number, html.EscapeString(section.Title))
	b.WriteString(`<ol>`)

	for _, code := range in.LimitationCodes {
		if text, ok := CustomDevelopmentLimitationCatalog[code]; ok {
			fmt.Fprintf(&b, `<li>%s</li>`, html.EscapeString(text))
		}
	}
	for _, add := range in.AdditionalLimitations {
		trimmed := strings.TrimSpace(add)
		if trimmed != "" {
			fmt.Fprintf(&b, `<li>%s</li>`, html.EscapeString(trimmed))
		}
	}

	b.WriteString(`</ol></section>`)
	return b.String()
}

func ResolveCustomDevelopmentLegalHTML(legalHTML string, _ domain.CustomDevelopmentTaxTreatment) (string, error) {
	if strings.TrimSpace(legalHTML) == "" {
		return "", nil
	}

	root, err := nethtml.Parse(strings.NewReader("<div>" + legalHTML + "</div>"))
	if err != nil {
		return "", fmt.Errorf("failed to parse legal html: %w", err)
	}

	var div *nethtml.Node
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		if n.Type == nethtml.ElementNode && n.Data == "html" {
			for b := n.FirstChild; b != nil; b = b.NextSibling {
				if b.Type == nethtml.ElementNode && b.Data == "body" {
					for d := b.FirstChild; d != nil; d = d.NextSibling {
						if d.Type == nethtml.ElementNode && d.Data == "div" {
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
		if node.Type != nethtml.ElementNode {
			continue
		}
		isTaxField := attributeValue(node, "data-field") == "tax_clause" || attributeValue(node, "data-quotation-field") == "tax_clause"
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
		if err := nethtml.Render(&buf, child); err != nil {
			return "", fmt.Errorf("failed to render resolved legal html: %w", err)
		}
	}

	return buf.String(), nil
}

func renderCustomDevelopmentLegal(legalHTML string, taxTreatment domain.CustomDevelopmentTaxTreatment) string {
	var b strings.Builder
	b.WriteString(`<section data-quotation-section="legal">`)
	resolved, err := ResolveCustomDevelopmentLegalHTML(legalHTML, taxTreatment)
	if err != nil || strings.TrimSpace(resolved) == "" {
		b.WriteString(legalHTML)
	} else {
		b.WriteString(resolved)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func renderCustomDevelopmentSignatures(m QuotationMetadata) string {
	var b strings.Builder
	b.WriteString(`<section data-quotation-section="signature">`)
	if m.IssueDate != "" {
		b.WriteString(`<p>Malang, <span data-field="issue_date">` + html.EscapeString(m.IssueDate) + `</span></p>`)
	}
	b.WriteString(`<table class="signature-table"><tbody><tr>`)
	b.WriteString(`<td><p>Hormat Kami,</p><br><br><p><strong>Yulianto Frandi</strong></p><p>Direksi Venturo Pro</p></td>`)

	clientName := m.ClientName
	if clientName == "" {
		clientName = "(Nama Klien)"
	}
	b.WriteString(`<td><p>Disetujui Oleh,</p><br><br><p><strong>` + html.EscapeString(clientName) + `</strong></p>`)
	if m.ClientCompanyName != "" {
		b.WriteString(`<p>` + html.EscapeString(m.ClientCompanyName) + `</p>`)
	}
	b.WriteString(`</td></tr></tbody></table></section>`)
	return b.String()
}
