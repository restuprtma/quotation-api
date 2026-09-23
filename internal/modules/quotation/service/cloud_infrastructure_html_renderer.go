package service

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const cloudInfrastructureOpeningParagraph = "Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek."

type CloudInfrastructureDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.CloudInfrastructureInput
	Calculation domain.CloudInfrastructureCalculation
	Profile     domain.ResolvedCloudInfrastructureProfile
	LegalHTML   string
}

type CloudInfrastructureRenderer interface {
	Render(CloudInfrastructureDocumentInput) (string, error)
}
type cloudInfrastructureRenderer struct{}

func NewCloudInfrastructureRenderer() CloudInfrastructureRenderer {
	return &cloudInfrastructureRenderer{}
}

func (r *cloudInfrastructureRenderer) Render(document CloudInfrastructureDocumentInput) (string, error) {
	in := document.Input.Normalize()
	if err := in.Validate(); err != nil {
		return "", err
	}
	calc := document.Calculation
	if len(calc.Packages) == 0 {
		var err error
		calc, err = CalculateCloudInfrastructure(in)
		if err != nil {
			return "", err
		}
	}
	profile := document.Profile
	if len(profile.Sections) == 0 {
		var err error
		profile, err = ResolveCloudInfrastructureProfile(in)
		if err != nil {
			return "", err
		}
	}
	var b strings.Builder
	b.WriteString(`<article class="quotation-document quotation-document--cloud-infrastructure">`)
	b.WriteString(renderOfficialQuotationHeader("PENAWARAN LAYANAN CLOUD INFRASTRUCTURE", document.Metadata))
	b.WriteString(`<section data-block-type="opening"><p>` + cloudInfrastructureOpeningParagraph + `</p></section>`)
	for _, sec := range profile.Sections {
		switch sec.Code {
		case "cloud_cost":
			b.WriteString(renderCloudCost(sec, in, calc, profile))
		case "scope_timeline":
			b.WriteString(renderCloudScope(sec, in))
		case "payment":
			b.WriteString(renderCloudPayment(sec, in, profile))
		case "limitations":
			b.WriteString(renderCloudLimitations(sec, in, profile, document.LegalHTML))
		}
	}
	b.WriteString(renderCloudSignatures(document.Metadata))
	b.WriteString(`</article>`)
	return b.String(), nil
}

func cloudSection(sec domain.DocumentSection, block string) string {
	return fmt.Sprintf(`<section data-block-type="%s" data-section-code="%s" data-section-number="%d"><h2>%d. %s</h2>`, block, sec.Code, sec.Number, sec.Number, html.EscapeString(sec.Title))
}
func cloudMoney(v int64) string { return "Rp " + formatNumber(v) }

func renderCloudCost(sec domain.DocumentSection, in domain.CloudInfrastructureInput, calc domain.CloudInfrastructureCalculation, profile domain.ResolvedCloudInfrastructureProfile) string {
	var b strings.Builder
	b.WriteString(cloudSection(sec, "commercial"))
	if profile.ShowSetup {
		b.WriteString(`<h3>Setup Fee Server (One Time Only)</h3><table><tbody>`)
		for i, x := range calc.Setup {
			fmt.Fprintf(&b, `<tr data-cloud-setup-row="%d"><td>%s</td><td data-cloud-field="setup_line_total">%s</td></tr>`, i, html.EscapeString(x.Name), cloudMoney(x.LineTotal))
		}
		fmt.Fprintf(&b, `<tr data-cloud-total-row="setup"><th>Subtotal Setup</th><td data-cloud-field="setup_subtotal">%s</td></tr></tbody></table>`, cloudMoney(calc.SetupSubtotal))
	}
	b.WriteString(`<table><thead><tr><th>Nama Paket</th><th>VCPU</th><th>RAM</th><th>Disk / Storage</th><th>Unit</th><th>Total</th></tr></thead><tbody>`)
	for i, x := range calc.Packages {
		fmt.Fprintf(&b, `<tr data-cloud-package-row="%d"><td data-cloud-package-field="name">%s</td><td data-cloud-package-field="vcpu">%d vCPU</td><td data-cloud-package-field="ram">%d GB</td><td data-cloud-package-field="storage">%d GB</td><td data-cloud-package-field="unit_rate">%s</td><td data-cloud-package-field="line_total">%s</td></tr>`, i, html.EscapeString(x.Name), x.VCPU, x.RAMGB, x.StorageGB, cloudMoney(x.UnitRate), cloudMoney(x.LineTotal))
	}
	fmt.Fprintf(&b, `<tr data-cloud-total-row="monthly"><th colspan="5">Subtotal Bulanan</th><td data-cloud-field="monthly_subtotal">%s</td></tr></tbody></table></section>`, cloudMoney(calc.MonthlySubtotal))
	return b.String()
}
func renderCloudScope(sec domain.DocumentSection, in domain.CloudInfrastructureInput) string {
	var b strings.Builder
	b.WriteString(cloudSection(sec, "narrative") + `<table><tbody>`)
	for i, row := range in.ScopeRows {
		fmt.Fprintf(&b, `<tr data-cloud-scope-row="%d"><td>`, i)
		for _, s := range row.Sections {
			fmt.Fprintf(&b, `<h3>%s</h3><ul>`, html.EscapeString(s.Title))
			for _, a := range s.Activities {
				fmt.Fprintf(&b, `<li>%s</li>`, html.EscapeString(a))
			}
			b.WriteString(`</ul>`)
		}
		fmt.Fprintf(&b, `</td><td data-cloud-scope-field="timeline">%s</td></tr>`, html.EscapeString(row.Timeline))
	}
	return b.String() + `</tbody></table></section>`
}
func renderCloudPayment(sec domain.DocumentSection, in domain.CloudInfrastructureInput, profile domain.ResolvedCloudInfrastructureProfile) string {
	var b strings.Builder
	b.WriteString(cloudSection(sec, "commercial") + `<table><thead><tr><th>Periode</th><th>Termin</th><th>Tanggal Invoice</th><th>Jatuh Tempo</th><th>Nominal</th></tr></thead><tbody>`)
	for i, p := range in.PaymentSchedule {
		fmt.Fprintf(&b, `<tr data-cloud-payment-row="%d" data-cloud-payment-component="%s"><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td data-cloud-field="payment_nominal_%d">%s</td></tr>`, i, html.EscapeString(string(p.Component)), html.EscapeString(p.WorkPeriod), html.EscapeString(p.Terms), html.EscapeString(p.InvoiceDateLabel), html.EscapeString(p.DueDateLabel), i, cloudMoney(p.Nominal))
	}
	b.WriteString(`</tbody></table>`)
	if profile.TaxClauseText != "" {
		b.WriteString(`<p data-cloud-field="tax_clause">` + html.EscapeString(profile.TaxClauseText) + `</p>`)
	}
	return b.String() + `</section>`
}
func renderCloudLimitations(sec domain.DocumentSection, in domain.CloudInfrastructureInput, profile domain.ResolvedCloudInfrastructureProfile, legal string) string {
	var b strings.Builder
	b.WriteString(cloudSection(sec, "legal") + `<ol>`)
	for i, t := range cloudLimitationTexts(in) {
		code := "additional"
		if i < len(in.LimitationCodes) {
			code = string(in.LimitationCodes[i])
		}
		fmt.Fprintf(&b, `<li data-limitation-code="%s">%s</li>`, html.EscapeString(code), html.EscapeString(t))
	}
	b.WriteString(`</ol>`)
	if strings.TrimSpace(legal) != "" {
		resolved, _ := ResolveCloudInfrastructureLegalHTML(legal, profile.TaxTreatment)
		b.WriteString(`<div data-block-type="legal">` + resolved + `</div>`)
	}
	return b.String() + `</section>`
}
func cloudLimitationTexts(in domain.CloudInfrastructureInput) []string {
	out := make([]string, 0, len(in.LimitationCodes)+len(in.AdditionalLimitations))
	for _, c := range in.LimitationCodes {
		if t, ok := CustomDevelopmentLimitationCatalog[string(c)]; ok {
			out = append(out, t)
		}
	}
	return append(out, in.AdditionalLimitations...)
}
func renderCloudSignatures(m QuotationMetadata) string {
	client := m.ClientName
	if client == "" {
		client = "(Nama Klien)"
	}
	var issue string
	if m.IssueDate != "" {
		issue = html.EscapeString(m.IssueDate)
	}
	return `<section data-block-type="signature" data-signature-block="true"><p>Malang, ` + issue + `</p><table class="signature-table"><tbody><tr><td>Hormat Kami,<br><br><strong>Yulianto Frandi</strong><br>Direksi Venturo Pro</td><td>Disetujui Oleh,<br><br><strong>` + html.EscapeString(client) + `</strong></td></tr></tbody></table></section>`
}

var _ = strconv.Itoa
