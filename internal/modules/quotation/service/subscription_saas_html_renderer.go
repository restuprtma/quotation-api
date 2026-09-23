package service

import (
	"fmt"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const SubscriptionSaaSOpeningParagraph = "Melalui penawaran ini, Venturo menyampaikan skema penggunaan layanan software berbasis subscription beserta biaya, periode layanan, ketentuan pembayaran, dan hak para pihak yang disepakati."

type SubscriptionSaaSDocumentInput struct {
	Metadata      QuotationMetadata
	Input         domain.SubscriptionSaaSInput
	Calculation   domain.SubscriptionSaaSCalculation
	Profile       domain.ResolvedSubscriptionSaaSProfile
	FeatureGroups []domain.SubscriptionFeatureGroupInput
	LegalHTML     string
}

type SubscriptionSaaSHTMLRenderer interface {
	Render(SubscriptionSaaSDocumentInput) (string, error)
}

type subscriptionSaaSHTMLRenderer struct{}

func NewSubscriptionSaaSHTMLRenderer() SubscriptionSaaSHTMLRenderer {
	return &subscriptionSaaSHTMLRenderer{}
}

func (r *subscriptionSaaSHTMLRenderer) Render(in SubscriptionSaaSDocumentInput) (string, error) {
	if len(in.Calculation.PricePeriods) == 0 {
		return "", fmt.Errorf("price_periods: calculation periods are empty")
	}
	if len(in.Calculation.Payments) == 0 {
		return "", fmt.Errorf("payments: calculation payments are empty")
	}
	if strings.TrimSpace(in.LegalHTML) == "" {
		return "", fmt.Errorf("legal_html: legal template snapshot is empty")
	}

	sections := BuildSubscriptionSectionPlan(in.Profile)

	var b strings.Builder
	b.WriteString(`<article class="quotation-document">`)
	b.WriteString(renderSubscriptionHeader(in.Metadata))
	b.WriteString(`<section data-block-type="opening"><p>` + SubscriptionSaaSOpeningParagraph + `</p></section>`)

	for _, section := range sections {
		switch section.Code {
		case string(domain.SubscriptionLayoutSubscriptionDevelopment),
			string(domain.SubscriptionLayoutDevelopmentMaintenance),
			"commercial":
			b.WriteString(renderSubscriptionCommercial(section, in.Profile.SectionLayout, in.Calculation))
		case "features":
			b.WriteString(renderSubscriptionFeatures(section, in.FeatureGroups))
		case "payment":
			b.WriteString(renderSubscriptionPayments(section, in.Calculation.Payments, in.Profile))
		case "rights":
			b.WriteString(renderSubscriptionRights(section, in.Profile))
		case "limitations":
			b.WriteString(renderSubscriptionLegal(section, in.LegalHTML))
		}
	}

	b.WriteString(renderSubscriptionSignatures(in.Metadata))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderSubscriptionHeader(m QuotationMetadata) string {
	return renderOfficialQuotationHeader("PENAWARAN LAYANAN SOFTWARE SUBSCRIPTION / SAAS", m)
}

func renderSubscriptionCommercial(section domain.DocumentSection, layout domain.SubscriptionSectionLayout, calc domain.SubscriptionSaaSCalculation) string {
	var b strings.Builder
	writeSubscriptionSectionStart(&b, section, "commercial")

	firstColHeader := "Nama Paket / Komponen"
	if layout == domain.SubscriptionLayoutDevelopmentMaintenance {
		firstColHeader = "Fase / Tahun"
	}

	b.WriteString(`<table><thead><tr><th>` + firstColHeader + `</th><th>Rate (Monthly)</th><th>Period (Month)</th><th>Total</th></tr></thead><tbody>`)

	for i, period := range calc.PricePeriods {
		fmt.Fprintf(&b, `<tr data-role="subscription_period_%d"><td>%s</td><td>Rp %s</td><td>%d Bulan</td><td data-field="subscription_line_total">Rp %s</td></tr>`,
			i,
			htmlEscape(period.Label),
			formatNumber(period.MonthlyRate),
			period.DurationMonths,
			formatNumber(period.Subtotal),
		)
	}

	for i, item := range calc.OneTimeItems {
		fmt.Fprintf(&b, `<tr data-role="subscription_one_time_%d"><td>%s</td><td>-</td><td>-</td><td data-field="subscription_line_total">Rp %s</td></tr>`,
			i,
			htmlEscape(item.Name),
			formatNumber(item.Amount),
		)
	}

	fmt.Fprintf(&b, `<tr><td colspan="3"><strong>TOTAL</strong></td><td data-field="total_contract"><strong>Rp %s</strong></td></tr>`,
		formatNumber(calc.TotalContract),
	)

	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func renderSubscriptionFeatures(section domain.DocumentSection, groups []domain.SubscriptionFeatureGroupInput) string {
	var b strings.Builder
	writeSubscriptionSectionStart(&b, section, "features")

	for _, g := range groups {
		fmt.Fprintf(&b, `<h3>%s</h3><ul>`, htmlEscape(g.Name))
		for _, f := range g.Features {
			fmt.Fprintf(&b, `<li>%s</li>`, htmlEscape(f))
		}
		b.WriteString(`</ul>`)
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderSubscriptionPayments(section domain.DocumentSection, rows []domain.SubscriptionPaymentRowInput, profile domain.ResolvedSubscriptionSaaSProfile) string {
	var b strings.Builder
	writeSubscriptionSectionStart(&b, section, "payment")

	b.WriteString(`<table><thead><tr><th>Work Period</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th></tr></thead><tbody>`)

	for _, row := range rows {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`,
			htmlEscape(row.WorkPeriod),
			htmlEscape(row.InvoiceDateLabel),
			htmlEscape(row.DueDateLabel),
			formatNumber(row.Nominal),
		)
	}

	b.WriteString(`</tbody></table>`)

	if profile.AutoRenewalText != "" {
		fmt.Fprintf(&b, `<p data-field="auto_renewal">%s</p>`, htmlEscape(profile.AutoRenewalText))
	}
	if profile.TaxClauseText != "" {
		fmt.Fprintf(&b, `<p data-field="tax_clause">%s</p>`, htmlEscape(profile.TaxClauseText))
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderSubscriptionRights(section domain.DocumentSection, profile domain.ResolvedSubscriptionSaaSProfile) string {
	var b strings.Builder
	writeSubscriptionSectionStart(&b, section, "rights")

	if profile.RightsClauseText != "" {
		fmt.Fprintf(&b, `<p data-field="rights_clause">%s</p>`, htmlEscape(profile.RightsClauseText))
	}
	if profile.IncomeSharingText != "" {
		fmt.Fprintf(&b, `<p data-field="income_sharing">%s</p>`, htmlEscape(profile.IncomeSharingText))
	}

	b.WriteString(`</section>`)
	return b.String()
}

func renderSubscriptionLegal(section domain.DocumentSection, legalHTML string) string {
	var b strings.Builder
	writeSubscriptionSectionStart(&b, section, "legal")
	b.WriteString(legalHTML)
	b.WriteString(`</section>`)
	return b.String()
}

func renderSubscriptionSignatures(m QuotationMetadata) string {
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

func writeSubscriptionSectionStart(b *strings.Builder, section domain.DocumentSection, blockType string) {
	fmt.Fprintf(b, `<section data-block-type="%s" data-section-number="%d"><h2>%d. %s</h2>`, blockType, section.Number, section.Number, htmlEscape(section.Title))
}
