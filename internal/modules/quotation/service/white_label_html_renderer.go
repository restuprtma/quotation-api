package service

import (
	"fmt"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const WhiteLabelOpeningParagraph = "Melalui penawaran ini, Venturo menyampaikan skema penyediaan lisensi software White Label Product beserta rincian biaya, pemeliharaan, hosting, lingkup modul, dan ketentuan komersial yang disepakati."

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
			b.WriteString(renderWhiteLabelLegal(section, in.LegalHTML))
		}
	}

	b.WriteString(renderWhiteLabelSignatures(in.Metadata))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderWhiteLabelHeader(m QuotationMetadata) string {
	var b strings.Builder
	b.WriteString(`<header data-block-type="metadata">`)
	b.WriteString(`<h1>PENAWARAN LAYANAN WHITE LABEL PRODUCT</h1>`)
	b.WriteString(`<p><strong>Nomor:</strong> <span data-field="quotation_number">` + htmlEscape(m.QuotationNumber) + `</span></p>`)
	b.WriteString(`<p><strong>Perihal:</strong> <span data-field="subject">` + htmlEscape(m.Subject) + `</span></p>`)
	b.WriteString(`<p><strong>Ditujukan kepada:</strong> <span data-field="client_pic">` + htmlEscape(m.ClientName) + `</span></p>`)
	if m.ClientCompanyName != "" {
		b.WriteString(`<p data-field="client_company">` + htmlEscape(m.ClientCompanyName) + `</p>`)
	}
	b.WriteString(`</header>`)
	return b.String()
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
	for _, row := range rows {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`,
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

func renderWhiteLabelLegal(section domain.DocumentSection, legalHTML string) string {
	var b strings.Builder
	writeSectionStart(&b, section, "legal")
	b.WriteString(legalHTML)
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
