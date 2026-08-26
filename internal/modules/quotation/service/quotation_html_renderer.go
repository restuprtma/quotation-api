package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var (
	ErrUnsafeHTML          = errors.New("unsafe_html")
	ErrInvalidPaymentTerms = errors.New("invalid_payment_terms")
)

type QuotationMetadata struct {
	QuotationNumber string
	Subject         string
	ClientName      string
	IssueDate       string
}

type CriticalBlocksInput struct {
	Metadata   QuotationMetadata
	Allocation dto.EnrichedAllocation
	Payment    dto.PaymentTermsInput
	LegalHTML  string
}

type HTMLRenderer interface {
	RenderCriticalBlocks(in CriticalBlocksInput) (string, error)
	Assemble(criticalHTML, narrativeHTML string) (string, error)
}

type defaultHTMLRenderer struct{}

func NewHTMLRenderer() HTMLRenderer {
	return &defaultHTMLRenderer{}
}

func (r *defaultHTMLRenderer) RenderCriticalBlocks(in CriticalBlocksInput) (string, error) {
	if len(in.Allocation.Resources) == 0 {
		return "", fmt.Errorf("%w: allocation has no resources", ErrInvalidPaymentTerms)
	}
	if err := validatePaymentTerms(in.Payment); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.LegalHTML) == "" {
		return "", fmt.Errorf("%w: legal template snapshot is empty", ErrInvalidPaymentTerms)
	}

	var b strings.Builder
	b.WriteString(r.renderMetadata(in.Metadata))
	b.WriteString(r.renderInvestmentTable(in.Allocation))
	b.WriteString(r.renderPaymentSchedule(in.Payment))
	b.WriteString(r.renderLegalBlock(in.LegalHTML))
	return b.String(), nil
}

func validatePaymentTerms(p dto.PaymentTermsInput) error {
	if p.Mode != "milestone" {
		return fmt.Errorf("%w: unsupported payment mode %q", ErrInvalidPaymentTerms, p.Mode)
	}
	if len(p.Items) == 0 {
		return fmt.Errorf("%w: payment items are empty", ErrInvalidPaymentTerms)
	}
	sum := int64(0)
	for _, item := range p.Items {
		if item.Percentage <= 0 {
			return fmt.Errorf("%w: percentage must be positive", ErrInvalidPaymentTerms)
		}
		sum += item.Percentage
	}
	if sum != 100 {
		return fmt.Errorf("%w: percentages sum to %d, want 100", ErrInvalidPaymentTerms, sum)
	}
	return nil
}

func (r *defaultHTMLRenderer) renderMetadata(m QuotationMetadata) string {
	var b strings.Builder
	b.WriteString(`<div class="qtr-block qtr-metadata" data-block="metadata">`)
	b.WriteString("<p><strong>Nomor Quotation:</strong> " + htmlEscape(m.QuotationNumber) + "</p>")
	b.WriteString("<p><strong>Subjek:</strong> " + htmlEscape(m.Subject) + "</p>")
	b.WriteString("<p><strong>Klien:</strong> " + htmlEscape(m.ClientName) + "</p>")
	if m.IssueDate != "" {
		b.WriteString("<p><strong>Tanggal:</strong> " + htmlEscape(m.IssueDate) + "</p>")
	}
	b.WriteString("</div>")
	return b.String()
}

func (r *defaultHTMLRenderer) renderInvestmentTable(a dto.EnrichedAllocation) string {
	var b strings.Builder
	b.WriteString(`<div class="qtr-block qtr-investment" data-block="investment">`)
	b.WriteString(`<table>`)
	b.WriteString(`<thead><tr><th>Peran</th><th>Senioritas</th><th>Jumlah SDM</th><th>Durasi (bulan)</th><th>Tarif/Bulan</th><th>Total</th></tr></thead>`)
	b.WriteString(`<tbody>`)
	for _, res := range a.Resources {
		rowKey := res.Role + "_" + res.Seniority
		fmt.Fprintf(&b, `<tr data-role="%s">`, htmlEscape(rowKey))
		b.WriteString("<td>" + htmlEscape(humanizeLabel(res.Role)) + "</td>")
		b.WriteString("<td>" + htmlEscape(humanizeLabel(res.Seniority)) + "</td>")
		fmt.Fprintf(&b, "<td>%d</td>", res.EmployeeCount)
		fmt.Fprintf(&b, "<td>%d</td>", res.PeriodMonths)
		b.WriteString("<td>" + formatNumber(res.AmountPerMonth) + "</td>")
		fmt.Fprintf(&b, `<td data-field="line_total">%s</td>`, formatNumber(res.LineTotal))
		b.WriteString("</tr>")
	}
	totalRow := fmt.Sprintf(
		`<tr class="qtr-total-row"><td colspan="5"><strong>Total Investasi</strong></td>`+
			`<td data-field="total_investment"><strong>%s</strong></td></tr>`,
		formatCurrency(a.TotalInvestment, a.Currency),
	)
	b.WriteString(totalRow)
	b.WriteString(`</tbody></table>`)
	b.WriteString("</div>")
	return b.String()
}

func (r *defaultHTMLRenderer) renderPaymentSchedule(p dto.PaymentTermsInput) string {
	var b strings.Builder
	b.WriteString(`<div class="qtr-block qtr-payment" data-block="payment_schedule">`)
	b.WriteString(`<table><thead><tr><th>Milestone</th><th>Persentase</th><th>Keterangan</th></tr></thead><tbody>`)
	for _, item := range p.Items {
		b.WriteString("<tr>")
		b.WriteString("<td>" + htmlEscape(item.MilestoneName) + "</td>")
		fmt.Fprintf(&b, "<td>%d%%</td>", item.Percentage)
		b.WriteString("<td>" + htmlEscape(item.Description) + "</td>")
		b.WriteString("</tr>")
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

func (r *defaultHTMLRenderer) renderLegalBlock(legalHTML string) string {
	return `<div class="qtr-block qtr-legal" data-block="legal">` + legalHTML + `</div><!-- /qtr-legal -->`
}

func htmlEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	).Replace(s)
}

func humanizeLabel(raw string) string {
	parts := strings.Split(raw, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func formatNumber(amount int64) string {
	digits := strconv.FormatInt(amount, 10)
	neg := strings.HasPrefix(digits, "-")
	if neg {
		digits = digits[1:]
	}
	var groups []string
	for len(digits) > 3 {
		groups = append([]string{digits[len(digits)-3:]}, groups...)
		digits = digits[:len(digits)-3]
	}
	groups = append([]string{digits}, groups...)
	out := strings.Join(groups, ".")
	if neg {
		return "-" + out
	}
	return out
}

func formatCurrency(amount int64, currency string) string {
	if currency == "IDR" {
		return "Rp " + formatNumber(amount)
	}
	return currency + " " + formatNumber(amount)
}

func (r *defaultHTMLRenderer) Assemble(criticalHTML, narrativeHTML string) (string, error) {
	panic("not implemented")
}
