package service

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type TalentOutsourcingDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.TalentOutsourcingInput
	Calculation domain.TalentOutsourcingCalculation
	Profile     domain.ResolvedTalentOutsourcingProfile
	LegalHTML   string
}
type TalentOutsourcingRenderer interface {
	Render(TalentOutsourcingDocumentInput) (string, error)
}
type talentOutsourcingRenderer struct{}

func NewTalentOutsourcingRenderer() TalentOutsourcingRenderer { return &talentOutsourcingRenderer{} }
func formatTalentIDR(v int64) string {
	s := strconv.FormatInt(v, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return "Rp " + s
}
func moneyCell(f string, v int64) string {
	return fmt.Sprintf(`<td data-field="%s" data-value="%d">%s</td>`, html.EscapeString(f), v, formatTalentIDR(v))
}
func (r *talentOutsourcingRenderer) Render(raw TalentOutsourcingDocumentInput) (string, error) {
	in := domain.NormalizeTalentOutsourcingInput(raw.Input)
	if err := in.Validate(); err != nil {
		return "", err
	}
	calc := raw.Calculation
	if calc.GrandTotal == 0 {
		var err error
		calc, err = CalculateTalentOutsourcing(in)
		if err != nil {
			return "", err
		}
	}
	p := raw.Profile
	if len(p.Sections) == 0 {
		var err error
		p, err = ResolveTalentOutsourcingProfile(in)
		if err != nil {
			return "", err
		}
	}
	var b strings.Builder
	b.WriteString(`<article class="quotation-document talent-outsourcing-document" data-service="talent_outsourcing">`)
	b.WriteString(`<header><h1>PENAWARAN LAYANAN TALENT OUTSOURCING</h1><p>` + html.EscapeString(in.DocumentNumber) + `</p><p>` + html.EscapeString(in.ServiceName) + `</p></header>`)
	for _, s := range p.Sections {
		switch s.Code {
		case "development_cost":
			b.WriteString(renderTalentCommercial(s, in, calc))
		case "scope_timeline":
			b.WriteString(renderTalentScope(s, in))
		case "payment":
			b.WriteString(renderTalentPayment(s, in, calc))
		case "limitations":
			b.WriteString(renderTalentLegal(s, in, p, raw.LegalHTML))
		}
	}
	b.WriteString(`</article>`)
	return b.String(), nil
}
func talentSection(s domain.DocumentSection, typ string) string {
	return fmt.Sprintf(`<section data-block-type="%s" data-section-code="%s"><h2>%d. %s</h2>`, typ, s.Code, s.Number, html.EscapeString(s.Title))
}
func renderTalentCommercial(s domain.DocumentSection, in domain.TalentOutsourcingInput, c domain.TalentOutsourcingCalculation) string {
	var b strings.Builder
	b.WriteString(talentSection(s, "commercial"))
	b.WriteString(`<table><tbody>`)
	if in.EngagementMode == domain.TalentModeManpowerSupply {
		for gi, g := range c.CostGroups {
			b.WriteString(`<tr data-group-index="` + strconv.Itoa(gi) + `"><th colspan="2">` + html.EscapeString(g.Title) + `</th></tr>`)
			for ii, x := range g.Items {
				b.WriteString(`<tr data-row-index="` + strconv.Itoa(ii) + `"><td>` + html.EscapeString(x.Name) + `</td>` + moneyCell(fmt.Sprintf("cost_groups.%d.items.%d.line_total", gi, ii), x.LineTotal) + `</tr>`)
			}
		}
		b.WriteString(`<tr><th>Subtotal Personil</th>` + moneyCell("personnel_subtotal", c.PersonnelSubtotal) + `</tr>`)
		if c.NonPersonnelSubtotal > 0 {
			b.WriteString(`<tr><th>Subtotal Non Personil</th>` + moneyCell("non_personnel_subtotal", c.NonPersonnelSubtotal) + `</tr>`)
		}
	} else {
		if len(c.Preparation) > 0 {
			b.WriteString(`<tr><th colspan="2">Hiring Fee</th></tr>`)
			for i, x := range c.Preparation {
				b.WriteString(`<tr data-row-index="` + strconv.Itoa(i) + `"><td>` + html.EscapeString(x.Name) + `</td>` + moneyCell(fmt.Sprintf("preparation.%d.line_total", i), x.LineTotal) + `</tr>`)
			}
			b.WriteString(`<tr><th>Subtotal Hiring Fee</th>` + moneyCell("preparation_subtotal", c.PreparationSubtotal) + `</tr>`)
			if p := c.PreparationDiscountAmount; p > 0 {
				b.WriteString(`<tr><th>Diskon Hiring Fee</th>` + moneyCell("preparation_discount_amount", p) + `</tr>`)
			}
		}
		for i, x := range c.Squad {
			b.WriteString(`<tr data-row-index="` + strconv.Itoa(i) + `"><td>` + html.EscapeString(x.Role) + `</td>` + moneyCell(fmt.Sprintf("squad.%d.line_total", i), x.LineTotal) + `</tr>`)
		}
		b.WriteString(`<tr><th>Subtotal Squad</th>` + moneyCell("squad_subtotal", c.SquadSubtotal) + `</tr>`)
	}
	b.WriteString(`<tr><th>Total Sebelum Pajak</th>` + moneyCell("total_before_tax", c.TotalBeforeTax) + `</tr>`)
	if c.DPPAmount > 0 {
		b.WriteString(`<tr><th>DPP PPN</th>` + moneyCell("dpp_amount", c.DPPAmount) + `</tr><tr><th>PPN</th>` + moneyCell("tax_amount", c.TaxAmount) + `</tr>`)
	}
	b.WriteString(`<tr><th>Grand Total</th>` + moneyCell("grand_total", c.GrandTotal) + `</tr></tbody></table>`)
	return b.String() + `</section>`
}
func renderTalentScope(s domain.DocumentSection, in domain.TalentOutsourcingInput) string {
	var b strings.Builder
	b.WriteString(talentSection(s, "scope"))
	for _, x := range in.ScopeBlocks {
		b.WriteString(`<div data-block-type="scope"><h3>` + html.EscapeString(x.Title) + `</h3><p>` + html.EscapeString(x.Description) + `</p>`)
		for title, list := range map[string][]string{"Responsibilities": x.Responsibilities, "Qualifications": x.Qualifications, "Deliverables": x.Deliverables} {
			if len(list) > 0 {
				b.WriteString(`<h4>` + title + `</h4><ul>`)
				for _, v := range list {
					b.WriteString(`<li>` + html.EscapeString(v) + `</li>`)
				}
				b.WriteString(`</ul>`)
			}
		}
		if x.Timeline != "" {
			b.WriteString(`<p>` + html.EscapeString(x.Timeline) + `</p>`)
		}
		b.WriteString(`</div>`)
	}
	return b.String() + `</section>`
}
func renderTalentPayment(s domain.DocumentSection, in domain.TalentOutsourcingInput, c domain.TalentOutsourcingCalculation) string {
	var b strings.Builder
	b.WriteString(talentSection(s, "payment") + `<table><tbody>`)
	for i, x := range in.PaymentSchedule {
		b.WriteString(`<tr data-row-index="` + strconv.Itoa(i) + `"><td>` + html.EscapeString(x.WorkPeriod) + `</td><td>` + html.EscapeString(x.Terms) + `</td>` + moneyCell(fmt.Sprintf("payments.%d.nominal", i), x.Nominal) + `</tr>`)
	}
	b.WriteString(`<tr><th colspan="2">Total Pembayaran</th>` + moneyCell("payment_total", c.PaymentTotal) + `</tr></tbody></table></section>`)
	return b.String()
}
func renderTalentLegal(s domain.DocumentSection, in domain.TalentOutsourcingInput, p domain.ResolvedTalentOutsourcingProfile, legal string) string {
	var b strings.Builder
	b.WriteString(talentSection(s, "limitations") + `<ul>`)
	for i, text := range p.LimitationTexts {
		code := "additional"
		if i < len(in.LimitationCodes) {
			code = string(in.LimitationCodes[i])
		}
		b.WriteString(`<li data-limitation-code="` + html.EscapeString(code) + `">` + html.EscapeString(text) + `</li>`)
	}
	b.WriteString(`</ul>`)
	if p.ShowWarranty {
		b.WriteString(`<div data-block-type="warranty"><h3>Garansi</h3>`)
		for _, x := range in.WarrantyStatements {
			b.WriteString(`<p>` + html.EscapeString(x) + `</p>`)
		}
		b.WriteString(`</div>`)
	}
	if p.ShowRights {
		b.WriteString(`<div data-block-type="rights"><h3>Hak</h3>`)
		for _, x := range in.RightsStatements {
			b.WriteString(`<p>` + html.EscapeString(x) + `</p>`)
		}
		b.WriteString(`</div>`)
	}
	if p.TaxClauseText != "" {
		b.WriteString(`<p>` + html.EscapeString(p.TaxClauseText) + `</p>`)
	}
	if strings.TrimSpace(legal) != "" {
		b.WriteString(`<div data-block-type="legal">` + legal + `</div>`)
	}
	return b.String() + `</section>`
}
