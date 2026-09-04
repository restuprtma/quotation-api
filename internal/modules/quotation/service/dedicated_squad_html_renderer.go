package service

import (
	"fmt"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const DedicatedSquadOpeningParagraph = "Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek. Dokumen ini memuat ruang lingkup pekerjaan, output, timeline, dan rincian biaya sebagai acuan kesepakatan bersama kedua belah pihak."

type DedicatedSquadDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.DedicatedSquadInput
	Calculation domain.DedicatedSquadCalculation
	Rules       DedicatedSquadRules
	Timeline    []domain.TimelineRow
}

type DedicatedSquadHTMLRenderer interface {
	Render(in DedicatedSquadDocumentInput) (string, error)
}

type dedicatedSquadHTMLRenderer struct{}

func NewDedicatedSquadHTMLRenderer() DedicatedSquadHTMLRenderer { return &dedicatedSquadHTMLRenderer{} }

func (r *dedicatedSquadHTMLRenderer) Render(in DedicatedSquadDocumentInput) (string, error) {
	if len(in.Calculation.Development) == 0 {
		return "", fmt.Errorf("development: calculation lines are empty")
	}
	sections := BuildSectionPlan(in.Rules)
	var b strings.Builder
	b.WriteString(`<article class="quotation-document">`)
	b.WriteString(renderDedicatedHeader(in.Metadata))
	b.WriteString(`<section data-block-type="opening"><p>` + DedicatedSquadOpeningParagraph + `</p></section>`)
	for _, section := range sections {
		switch section.Code {
		case "development":
			b.WriteString(renderDedicatedCostSection(section, in.Calculation.Development, in.Calculation, in.Input.Model == domain.ModelSprint))
		case "maintenance":
			b.WriteString(renderMaintenanceSection(section, in.Calculation.Maintenance, in.Calculation.MaintenanceSubtotal))
		case "timeline":
			b.WriteString(renderDedicatedTimeline(section, in.Timeline))
		case "payment":
			b.WriteString(renderDedicatedPayments(section, in.Calculation.Payments))
		case "limitations":
			b.WriteString(renderDedicatedLegal(section))
		}
	}
	b.WriteString(renderDedicatedSignatures(in.Metadata))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderDedicatedHeader(m QuotationMetadata) string {
	return `<header data-block-type="metadata"><h1>PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD</h1>` +
		`<p><strong>Nomor:</strong> <span data-field="quotation_number">` + htmlEscape(m.QuotationNumber) + `</span></p>` +
		`<p><strong>Perihal:</strong> <span data-field="subject">` + htmlEscape(m.Subject) + `</span></p>` +
		`<p><strong>Ditujukan kepada:</strong> <span data-field="client_pic">` + htmlEscape(m.ClientName) + `</span></p>` +
		`<p data-field="client_company">` + htmlEscape(m.ClientCompanyName) + `</p></header>`
}

func renderDedicatedCostSection(section domain.DocumentSection, lines []domain.SquadLineCalculation, calc domain.DedicatedSquadCalculation, daily bool) string {
	unit, period := "Rate (Monthly)", "Period (Month)"
	if daily {
		unit, period = "Rate (Hari)", "Period (Hari)"
	}
	var b strings.Builder
	writeSectionStart(&b, section, "commercial")
	b.WriteString(`<table><thead><tr><th>Development Squad</th><th>` + unit + `</th><th>Employee</th><th>` + period + `</th><th>Total</th></tr></thead><tbody>`)
	renderDedicatedLines(&b, lines, daily)
	if calc.Preparation.Gross > 0 {
		fmt.Fprintf(&b, `<tr data-field="preparation"><td>%s</td><td>-</td><td>-</td><td>-</td><td>Rp %s</td></tr>`, htmlEscape(preparationLabel(calc.Preparation.Kind)), formatNumber(calc.Preparation.Gross))
	}
	writeMoneyRow(&b, "total", "TOTAL", calc.Total)
	writeMoneyRow(&b, "discount", "DISCOUNT", calc.Discount)
	writeMoneyRow(&b, "total_investment", "TOTAL INVESTMENT", calc.TotalInvestment)
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderMaintenanceSection(section domain.DocumentSection, lines []domain.SquadLineCalculation, subtotal int64) string {
	var b strings.Builder
	writeSectionStart(&b, section, "commercial-maintenance")
	b.WriteString(`<table><thead><tr><th>Maintenance Squad</th><th>Rate (Monthly)</th><th>Employee</th><th>Period (Month)</th><th>Total</th></tr></thead><tbody>`)
	renderDedicatedLines(&b, lines, false)
	writeMoneyRow(&b, "maintenance_subtotal", "TOTAL MAINTENANCE", subtotal)
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderDedicatedLines(b *strings.Builder, lines []domain.SquadLineCalculation, daily bool) {
	for _, line := range lines {
		period := formatQuarters32(line.PeriodQuarters)
		if daily {
			period = fmt.Sprintf("%d", line.WorkdayCount)
		}
		fmt.Fprintf(b, `<tr data-role="%s_%s"><td>%s %s</td><td>Rp %s</td><td>%s</td><td>%s</td><td data-field="line_total">Rp %s</td></tr>`, htmlEscape(line.Role), htmlEscape(line.Seniority), htmlEscape(humanizeLabel(line.Seniority)), htmlEscape(humanizeLabel(line.Role)), formatNumber(line.AmountPerUnit), formatQuarters32(line.HeadcountQuarters), period, formatNumber(line.LineTotal))
	}
}

func renderDedicatedTimeline(section domain.DocumentSection, rows []domain.TimelineRow) string {
	var b strings.Builder
	writeSectionStart(&b, section, "timeline")
	b.WriteString(`<table><thead><tr><th data-column-width="75">Description</th><th data-column-width="25">Timeline</th></tr></thead><tbody>`)
	for _, row := range rows {
		b.WriteString(`<tr><td><strong>` + htmlEscape(row.Description) + `</strong><ul>`)
		for _, item := range row.Deliverables {
			b.WriteString(`<li>` + htmlEscape(item) + `</li>`)
		}
		b.WriteString(`</ul></td><td>` + htmlEscape(row.Timeline) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderDedicatedPayments(section domain.DocumentSection, rows []domain.PaymentRow) string {
	var b strings.Builder
	writeSectionStart(&b, section, "payment")
	b.WriteString(`<table><thead><tr><th>Work Period</th><th>Terms</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th></tr></thead><tbody>`)
	for _, row := range rows {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`, htmlEscape(row.WorkPeriod), htmlEscape(row.Terms), formatIndonesianDate(row.InvoiceDate), formatIndonesianDate(row.DueDate), formatNumber(row.Nominal))
	}
	b.WriteString(`</tbody></table><p>` + DedicatedTaxClause + `</p><p>` + DedicatedInvoiceClause + `</p><p>` + DedicatedSuspensionClause + `</p></section>`)
	return b.String()
}

func renderDedicatedLegal(section domain.DocumentSection) string {
	var b strings.Builder
	writeSectionStart(&b, section, "legal")
	b.WriteString(`<ol data-field="exclusions">`)
	for _, clause := range DedicatedExclusionClauses {
		b.WriteString(`<li>` + htmlEscape(clause) + `</li>`)
	}
	b.WriteString(`</ol><p>` + DedicatedBindingClause + `</p></section>`)
	return b.String()
}

func renderDedicatedSignatures(m QuotationMetadata) string {
	return `<section data-block-type="signature"><p>Malang, <span data-field="issue_date">` + htmlEscape(m.IssueDate) + `</span></p><table class="signature-table"><tbody><tr>` +
		`<td><p>Hormat Kami,</p><br><br><p><strong>Yulianto Frandi</strong></p><p>Direksi Venturo Pro</p></td>` +
		`<td><p>Disetujui Oleh,</p><br><br><p><strong>` + htmlEscape(m.ClientName) + `</strong></p><p>` + htmlEscape(m.ClientCompanyName) + `</p></td>` +
		`</tr></tbody></table></section>`
}

func writeSectionStart(b *strings.Builder, section domain.DocumentSection, blockType string) {
	fmt.Fprintf(b, `<section data-block-type="%s" data-section-number="%d"><h2>%d. %s</h2>`, blockType, section.Number, section.Number, htmlEscape(section.Title))
}

func writeMoneyRow(b *strings.Builder, field, label string, value int64) {
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>%s</strong></td><td data-field="%s"><strong>Rp %s</strong></td></tr>`, label, field, formatNumber(value))
}

func formatQuarters32(value int32) string {
	return strings.TrimSuffix(formatQuarterDuration(int64(value), 0), " bln")
}
func preparationLabel(kind string) string {
	if kind == string(PreparationHiring) {
		return "Hiring Fee (Recruitment & Onboarding)"
	}
	return "Engineering Kick-off (One Time Only)"
}

func formatIndonesianDate(value time.Time) string {
	months := [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("%d %s %d", value.Day(), months[value.Month()], value.Year())
}
