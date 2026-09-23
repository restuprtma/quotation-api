package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const StandaloneMaintenanceOpeningParagraph = "Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek. Dokumen ini memuat ruang lingkup pekerjaan, output, timeline, dan rincian biaya sebagai acuan kesepakatan bersama kedua belah pihak."

type StandaloneMaintenanceDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.StandaloneMaintenanceInput
	Calculation domain.StandaloneMaintenanceCalculation
	Profile     domain.ResolvedStandaloneMaintenanceProfile
	LegalHTML   string
}

type StandaloneMaintenanceHTMLRenderer interface {
	Render(StandaloneMaintenanceDocumentInput) (string, error)
}

type standaloneMaintenanceHTMLRenderer struct{}

func NewStandaloneMaintenanceHTMLRenderer() StandaloneMaintenanceHTMLRenderer {
	return &standaloneMaintenanceHTMLRenderer{}
}

func (r *standaloneMaintenanceHTMLRenderer) Render(in StandaloneMaintenanceDocumentInput) (string, error) {
	switch in.Input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		if len(in.Calculation.Retainer) == 0 {
			return "", fmt.Errorf("retainer: calculation lines are empty")
		}
	case domain.StandaloneMaintenanceHourlyTask:
		if len(in.Calculation.Hourly) == 0 {
			return "", fmt.Errorf("hourly: calculation lines are empty")
		}
	default:
		return "", fmt.Errorf("mode: unsupported value %q", in.Input.Mode)
	}

	if len(in.Input.PaymentSchedule) == 0 {
		return "", fmt.Errorf("payment_schedule: payment schedule rows are empty")
	}
	if strings.TrimSpace(in.LegalHTML) == "" {
		return "", fmt.Errorf("legal_html: legal template snapshot is empty")
	}

	profile := in.Profile
	if len(profile.Sections) == 0 {
		profile = ResolveStandaloneMaintenanceProfile(in.Input)
	}

	var b strings.Builder
	b.WriteString(`<article class="quotation-document">`)
	b.WriteString(renderStandaloneMaintenanceHeader(in.Metadata, in.Input.QuotationDate))
	b.WriteString(`<section data-block-type="opening"><p>` + StandaloneMaintenanceOpeningParagraph + `</p></section>`)

	for _, section := range profile.Sections {
		switch section.Code {
		case "maintenance_cost":
			b.WriteString(renderStandaloneMaintenanceRetainerCost(section, in.Calculation.Retainer, in.Calculation, profile.ShowDiscountRow))
		case "development_cost":
			b.WriteString(renderStandaloneMaintenanceHourlyCost(section, in.Calculation.Hourly, in.Calculation, profile.ShowDiscountRow))
		case "maintenance_scope":
			b.WriteString(renderStandaloneMaintenanceScope(section, in.Input))
		case "payment":
			b.WriteString(renderStandaloneMaintenancePayments(section, in.Input.PaymentSchedule, in.Calculation.PaymentTotal, profile.TaxClauseText))
		case "rights":
			if profile.ShowRights && len(in.Input.RightsStatements) > 0 {
				b.WriteString(renderStandaloneMaintenanceRights(section, in.Input.RightsStatements))
			}
		case "limitations":
			b.WriteString(renderStandaloneMaintenanceLegal(section, in.LegalHTML))
		}
	}

	b.WriteString(renderStandaloneMaintenanceSignatures(in.Metadata, in.Input.QuotationDate))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderStandaloneMaintenanceHeader(m QuotationMetadata, qDate time.Time) string {
	dateStr := m.IssueDate
	if dateStr == "" && !qDate.IsZero() {
		dateStr = indonesianDate(qDate)
	}

	m.IssueDate = dateStr
	return renderOfficialQuotationHeader("PENAWARAN LAYANAN STANDALONE MAINTENANCE & SUPPORT", m)
}

func renderStandaloneMaintenanceRetainerCost(section domain.DocumentSection, items []domain.MaintenanceRetainerLineCalculation, calc domain.StandaloneMaintenanceCalculation, showDiscountRow bool) string {
	var b strings.Builder
	writeSectionStart(&b, section, "commercial")
	b.WriteString(`<table><thead><tr><th>Maintenance Squad</th><th>t(Monthly)</th><th>Employee</th><th>t(Month)</th><th>Total</th></tr></thead><tbody>`)
	for i, item := range items {
		fmt.Fprintf(&b, `<tr data-role="retainer_cost_%d"><td>%s</td><td>Rp %s</td><td>%s</td><td>%d Bulan</td><td data-field="cost_line_total">Rp %s</td></tr>`,
			i,
			htmlEscape(item.Name),
			formatNumber(item.MonthlyRate),
			formatFTE(item.FTEQuarters),
			item.PeriodMonths,
			formatNumber(item.LineTotal),
		)
	}
	if showDiscountRow {
		fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="cost_subtotal"><strong>Rp %s</strong></td></tr>`, formatNumber(calc.Subtotal))
		fmt.Fprintf(&b, `<tr><td colspan="4"><strong>DISCOUNT</strong></td><td data-field="discount_amount"><strong>Rp %s</strong></td></tr>`, formatNumber(calc.DiscountAmount))
	}
	fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL INVESTMENT</strong></td><td data-field="total_investment"><strong>Rp %s</strong></td></tr>`, formatNumber(calc.TotalInvestment))
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderStandaloneMaintenanceHourlyCost(section domain.DocumentSection, items []domain.MaintenanceHourlyLineCalculation, calc domain.StandaloneMaintenanceCalculation, showDiscountRow bool) string {
	var b strings.Builder
	writeSectionStart(&b, section, "commercial")
	b.WriteString(`<table><thead><tr><th>Development</th><th>t(Monthly)</th><th>Employee</th><th>t(Hours)</th><th>Total</th></tr></thead><tbody>`)
	for i, item := range items {
		fmt.Fprintf(&b, `<tr data-role="hourly_cost_%d"><td>%s</td><td>Rp %s</td><td>%d</td><td>%s</td><td data-field="cost_line_total">Rp %s</td></tr>`,
			i,
			htmlEscape(item.Name),
			formatNumber(item.DisplayMonthlyRate),
			item.Headcount,
			formatHoursHalfUnits(item.HoursHalfUnits),
			formatNumber(item.LineTotal),
		)
	}
	if showDiscountRow {
		fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="cost_subtotal"><strong>Rp %s</strong></td></tr>`, formatNumber(calc.Subtotal))
		fmt.Fprintf(&b, `<tr><td colspan="4"><strong>DISCOUNT</strong></td><td data-field="discount_amount"><strong>Rp %s</strong></td></tr>`, formatNumber(calc.DiscountAmount))
	}
	fmt.Fprintf(&b, `<tr><td colspan="4"><strong>TOTAL INVESTMENT</strong></td><td data-field="total_investment"><strong>Rp %s</strong></td></tr>`, formatNumber(calc.TotalInvestment))
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderStandaloneMaintenanceScope(section domain.DocumentSection, input domain.StandaloneMaintenanceInput) string {
	var b strings.Builder
	writeSectionStart(&b, section, "scope")

	switch input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		b.WriteString(`<table><thead><tr><th># / Priority</th><th>Function</th><th>Screen / Flow</th><th>Existing</th><th>Improvement</th><th>Reference</th></tr></thead><tbody>`)
		for gIdx, group := range input.DefectGroups {
			fmt.Fprintf(&b, `<tr data-role="defect_group_%d"><td colspan="6"><strong>%s</strong></td></tr>`, gIdx, htmlEscape(group.Title))
			for iIdx, item := range group.Items {
				refContent := ""
				if strings.TrimSpace(item.Reference) != "" {
					refContent = htmlEscape(strings.TrimSpace(item.Reference))
				}
				fmt.Fprintf(&b, `<tr data-role="defect_%d_%d"><td>%s<br/>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
					gIdx, iIdx,
					htmlEscape(item.ItemNumber),
					htmlEscape(string(item.Priority)),
					renderMultiline(item.Function),
					renderMultiline(item.ScreenFlow),
					renderMultiline(item.Existing),
					renderMultiline(item.Improvement),
					refContent,
				)
			}
		}
		b.WriteString(`</tbody></table>`)

	case domain.StandaloneMaintenanceHourlyTask:
		b.WriteString(`<table><thead><tr><th>Description</th><th>Platform</th><th>Hours</th></tr></thead><tbody>`)
		for i, task := range input.HourlyTasks {
			fmt.Fprintf(&b, `<tr data-role="hourly_task_%d"><td>%s</td><td>%s</td><td>%s</td></tr>`,
				i,
				renderMultiline(task.Description),
				htmlEscape(task.Platform),
				formatHoursHalfUnits(task.HoursHalfUnits),
			)
		}
		b.WriteString(`</tbody></table>`)
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderStandaloneMaintenancePayments(section domain.DocumentSection, rows []domain.MaintenancePaymentRowInput, paymentTotal int64, taxClauseText string) string {
	var b strings.Builder
	writeSectionStart(&b, section, "payment")
	b.WriteString(`<table><thead><tr><th>Work Period</th><th>Terms</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th></tr></thead><tbody>`)
	for i, row := range rows {
		fmt.Fprintf(&b, `<tr data-role="payment_%d"><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`,
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

func renderStandaloneMaintenanceRights(section domain.DocumentSection, statements []string) string {
	var b strings.Builder
	writeSectionStart(&b, section, "rights")
	for _, stmt := range statements {
		fmt.Fprintf(&b, `<p>%s</p>`, htmlEscape(stmt))
	}
	b.WriteString(`</section>`)
	return b.String()
}

func renderStandaloneMaintenanceLegal(section domain.DocumentSection, legalHTML string) string {
	var b strings.Builder
	writeSectionStart(&b, section, "legal")
	b.WriteString(legalHTML)
	b.WriteString(`</section>`)
	return b.String()
}

func renderStandaloneMaintenanceSignatures(m QuotationMetadata, qDate time.Time) string {
	dateStr := m.IssueDate
	if dateStr == "" && !qDate.IsZero() {
		dateStr = indonesianDate(qDate)
	}

	var b strings.Builder
	b.WriteString(`<section data-block-type="signature">`)
	if dateStr != "" {
		b.WriteString(`<p>Malang, <span data-field="signature_date">` + htmlEscape(dateStr) + `</span></p>`)
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

func formatHoursHalfUnits(units int32) string {
	if units%2 == 0 {
		return strconv.Itoa(int(units / 2))
	}
	return fmt.Sprintf("%d,5", units/2)
}

func renderMultiline(s string) string {
	escaped := htmlEscape(s)
	escaped = strings.ReplaceAll(escaped, "\r\n", "<br/>")
	return strings.ReplaceAll(escaped, "\n", "<br/>")
}
