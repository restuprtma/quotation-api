package service

import (
	"fmt"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const KioskQueueOpeningParagraph = "Melalui penawaran ini, kami menegaskan kesiapan tim IT Profesional kami yang berjumlah lebih dari 100 orang untuk mendukung layanan dan keberhasilan proyek. Dokumen ini memuat ruang lingkup pekerjaan, output, timeline, dan rincian biaya sebagai acuan kesepakatan bersama kedua belah pihak."

type KioskQueueDocumentInput struct {
	Metadata    QuotationMetadata
	Input       domain.KioskQueueSystemInput
	Calculation domain.KioskQueueCalculation
	Profile     domain.ResolvedKioskQueueProfile
	LegalHTML   string
}

type KioskQueueHTMLRenderer interface {
	Render(KioskQueueDocumentInput) (string, error)
}

type kioskQueueHTMLRenderer struct{}

func NewKioskQueueHTMLRenderer() KioskQueueHTMLRenderer {
	return &kioskQueueHTMLRenderer{}
}

func (r *kioskQueueHTMLRenderer) Render(in KioskQueueDocumentInput) (string, error) {
	if len(in.Input.ServiceGroups) == 0 {
		return "", fmt.Errorf("service_groups: groups are empty")
	}
	switch in.Input.Variant {
	case domain.KioskQueueGovernmentMPP:
		if len(in.Input.GovernmentScopePhases) == 0 {
			return "", fmt.Errorf("government_scope_phases: phases are empty")
		}
		if len(in.Input.ExpertRequirements) == 0 {
			return "", fmt.Errorf("expert_requirements: experts are empty")
		}
		if len(in.Input.GovernmentPaymentSchedule) == 0 {
			return "", fmt.Errorf("government_payment_schedule: payment schedule is empty")
		}
	case domain.KioskQueuePrivateFoodCourt:
		if len(in.Input.PrivateTimelineRows) == 0 {
			return "", fmt.Errorf("private_timeline_rows: rows are empty")
		}
		if len(in.Input.PrivatePaymentSchedule) == 0 {
			return "", fmt.Errorf("private_payment_schedule: payment schedule is empty")
		}
	default:
		return "", fmt.Errorf("variant: unsupported value %q", in.Input.Variant)
	}

	if strings.TrimSpace(in.LegalHTML) == "" {
		return "", fmt.Errorf("legal_html: legal template snapshot is empty")
	}

	profile := in.Profile
	if len(profile.Sections) == 0 {
		var err error
		profile, err = ResolveKioskQueueProfile(in.Input)
		if err != nil {
			return "", err
		}
	}

	calc := in.Calculation
	if calc.Variant == "" {
		var err error
		calc, err = CalculateKioskQueueSystem(in.Input)
		if err != nil {
			return "", err
		}
	}

	var b strings.Builder
	b.WriteString(`<article class="quotation-document kiosk-queue-document">`)
	renderKioskQueueHeader(&b, in.Metadata, in.Input)
	b.WriteString(`<section data-block-type="opening"><p>` + KioskQueueOpeningParagraph + `</p></section>`)

	for _, sec := range profile.Sections {
		switch sec.Code {
		case "scope":
			renderGovernmentScope(&b, sec, in.Input.GovernmentScopePhases)
		case "experts":
			renderGovernmentExperts(&b, sec, in.Input.ExpertRequirements)
		case "costs":
			renderGovernmentCosts(&b, sec, calc, profile)
		case "development":
			renderPrivateDevelopment(&b, sec, calc)
		case "maintenance":
			if profile.ShowMaintenance && len(calc.Maintenance) > 0 {
				renderPrivateMaintenance(&b, sec, calc)
			}
		case "timeline":
			renderPrivateTimeline(&b, sec, in.Input.PrivateTimelineRows)
		case "payment":
			renderKioskPayments(&b, sec, in.Input, calc, profile)
		case "limitations":
			renderKioskLimitations(&b, sec, profile, in.Input.AdditionalLimitations)
		}
	}

	if profile.ShowRights && len(in.Input.RightsStatements) > 0 {
		renderKioskRights(&b, in.Input.RightsStatements)
	}

	b.WriteString(`<section data-block-type="legal">` + in.LegalHTML + `</section>`)
	renderKioskSignatures(&b, in.Metadata, in.Input.QuotationDate)
	b.WriteString(`</article>`)
	return b.String(), nil
}

func renderKioskQueueHeader(b *strings.Builder, m QuotationMetadata, input domain.KioskQueueSystemInput) {
	docNum := input.DocumentNumber
	if docNum == "" {
		docNum = m.QuotationNumber
	}
	dateStr := m.IssueDate
	if dateStr == "" && !input.QuotationDate.IsZero() {
		dateStr = indonesianDate(input.QuotationDate)
	}

	b.WriteString(`<header data-block-type="metadata">`)
	b.WriteString(`<h1>PENAWARAN LAYANAN KIOSK &amp; SISTEM ANTRIAN</h1>`)
	b.WriteString(`<p><strong>Nomor:</strong> <span data-field="quotation_number">` + htmlEscape(docNum) + `</span></p>`)
	b.WriteString(`<p><strong>Perihal:</strong> <span data-field="subject">` + htmlEscape(m.Subject) + `</span></p>`)
	if dateStr != "" {
		b.WriteString(`<p><strong>Tanggal:</strong> <span data-field="issue_date_header">` + htmlEscape(dateStr) + `</span></p>`)
	}
	b.WriteString(`<p><strong>Ditujukan kepada:</strong> <span data-field="client_pic">` + htmlEscape(m.ClientName) + `</span></p>`)
	if m.ClientCompanyName != "" {
		b.WriteString(`<p data-field="client_company">` + htmlEscape(m.ClientCompanyName) + `</p>`)
	}
	if input.SystemName != "" {
		b.WriteString(`<p><strong>Sistem:</strong> <span data-field="system_name">` + htmlEscape(input.SystemName) + `</span></p>`)
	}
	b.WriteString(`</header>`)
}

func renderGovernmentScope(b *strings.Builder, sec domain.DocumentSection, phases []domain.KioskQueueScopePhaseInput) {
	fmt.Fprintf(b, `<section data-block-type="scope" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	for _, phase := range phases {
		fmt.Fprintf(b, `<h3>%s</h3>`, htmlEscape(phase.Title))
		if len(phase.Deliverables) > 0 {
			b.WriteString(`<ul>`)
			for _, d := range phase.Deliverables {
				fmt.Fprintf(b, `<li>%s</li>`, htmlEscape(d))
			}
			b.WriteString(`</ul>`)
		}
	}
	b.WriteString(`</section>`)
}

func renderGovernmentExperts(b *strings.Builder, sec domain.DocumentSection, experts []domain.KioskQueueExpertInput) {
	fmt.Fprintf(b, `<section data-block-type="experts" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	b.WriteString(`<table><thead><tr><th>Posisi / Peran</th><th>Jumlah SDM</th><th>Durasi</th><th>Kualifikasi Minimal</th></tr></thead><tbody>`)
	for i, exp := range experts {
		fmt.Fprintf(b, `<tr data-role="expert_%d"><td>%s</td><td>%d</td><td>%s</td><td>%s</td></tr>`,
			i,
			htmlEscape(exp.Role),
			exp.Headcount,
			htmlEscape(exp.DurationLabel),
			renderMultiline(exp.MinimumQualification),
		)
	}
	b.WriteString(`</tbody></table></section>`)
}

func renderGovernmentCosts(b *strings.Builder, sec domain.DocumentSection, calc domain.KioskQueueCalculation, profile domain.ResolvedKioskQueueProfile) {
	fmt.Fprintf(b, `<section data-block-type="commercial" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	b.WriteString(`<table data-role="service_cost_table"><thead><tr><th>Deskripsi</th><th>Rate (Bulanan)</th><th>Jumlah SDM</th><th>Durasi</th><th>Total</th></tr></thead><tbody>`)
	for gIdx, group := range calc.ServiceGroups {
		fmt.Fprintf(b, `<tr><td colspan="5"><strong>%s</strong></td></tr>`, htmlEscape(group.Title))
		for iIdx, item := range group.Items {
			fmt.Fprintf(b, `<tr data-role="service_%d_%d"><td>%s</td><td>Rp %s</td><td>%d</td><td>%s</td><td data-field="service_line_total">Rp %s</td></tr>`,
				gIdx, iIdx,
				htmlEscape(item.Name),
				formatNumber(item.MonthlyRate),
				item.Headcount,
				formatQuarterDuration(int64(item.PeriodQuarters), 0),
				formatNumber(item.LineTotal),
			)
		}
	}
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>Subtotal Jasa</strong></td><td data-field="service_subtotal">Rp %s</td></tr>`, formatNumber(calc.ServiceSubtotal))
	if calc.ServiceDiscountAmount > 0 {
		fmt.Fprintf(b, `<tr><td colspan="4"><strong>DISCOUNT</strong></td><td data-field="service_discount_amount">Rp %s</td></tr>`, formatNumber(calc.ServiceDiscountAmount))
	}
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>PPN 11%%</strong></td><td data-field="service_tax_amount">Rp %s</td></tr>`, formatNumber(calc.ServiceTaxAmount))
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>Total Biaya Jasa</strong></td><td data-field="service_total">Rp %s</td></tr>`, formatNumber(calc.ServiceTotal))
	b.WriteString(`</tbody></table>`)

	if profile.ShowHardware && len(calc.Hardware) > 0 {
		b.WriteString(`<table data-role="hardware_cost_table"><thead><tr><th>Perangkat Keras</th><th>Harga Satuan</th><th>Jumlah</th><th>Total</th></tr></thead><tbody>`)
		for hIdx, item := range calc.Hardware {
			fmt.Fprintf(b, `<tr data-role="hardware_%d"><td>%s</td><td>Rp %s</td><td>%d</td><td data-field="hardware_line_total">Rp %s</td></tr>`,
				hIdx,
				htmlEscape(item.Name),
				formatNumber(item.UnitPrice),
				item.Quantity,
				formatNumber(item.LineTotal),
			)
		}
		fmt.Fprintf(b, `<tr><td colspan="3"><strong>Subtotal Perangkat Keras</strong></td><td data-field="hardware_subtotal">Rp %s</td></tr>`, formatNumber(calc.HardwareSubtotal))
		fmt.Fprintf(b, `<tr><td colspan="3"><strong>PPN 11%%</strong></td><td data-field="hardware_tax_amount">Rp %s</td></tr>`, formatNumber(calc.HardwareTaxAmount))
		fmt.Fprintf(b, `<tr><td colspan="3"><strong>Total Biaya Perangkat Keras</strong></td><td data-field="hardware_total">Rp %s</td></tr>`, formatNumber(calc.HardwareTotal))
		b.WriteString(`</tbody></table>`)
	}

	b.WriteString(`</section>`)
}

func renderPrivateDevelopment(b *strings.Builder, sec domain.DocumentSection, calc domain.KioskQueueCalculation) {
	fmt.Fprintf(b, `<section data-block-type="commercial" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	b.WriteString(`<table><thead><tr><th>Deskripsi</th><th>Rate (Bulanan)</th><th>Jumlah SDM</th><th>Durasi</th><th>Total</th></tr></thead><tbody>`)
	for gIdx, group := range calc.ServiceGroups {
		if len(calc.ServiceGroups) > 1 || group.Title != "" {
			fmt.Fprintf(b, `<tr><td colspan="5"><strong>%s</strong></td></tr>`, htmlEscape(group.Title))
		}
		for iIdx, item := range group.Items {
			fmt.Fprintf(b, `<tr data-role="service_%d_%d"><td>%s</td><td>Rp %s</td><td>%d</td><td>%s</td><td data-field="service_line_total">Rp %s</td></tr>`,
				gIdx, iIdx,
				htmlEscape(item.Name),
				formatNumber(item.MonthlyRate),
				item.Headcount,
				formatQuarterDuration(int64(item.PeriodQuarters), 0),
				formatNumber(item.LineTotal),
			)
		}
	}
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>Subtotal</strong></td><td data-field="service_subtotal">Rp %s</td></tr>`, formatNumber(calc.ServiceSubtotal))
	if calc.ServiceDiscountAmount > 0 {
		fmt.Fprintf(b, `<tr><td colspan="4"><strong>DISCOUNT</strong></td><td data-field="service_discount_amount">Rp %s</td></tr>`, formatNumber(calc.ServiceDiscountAmount))
	}
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>TOTAL INVESTMENT</strong></td><td data-field="service_total">Rp %s</td></tr>`, formatNumber(calc.ServiceTotal))
	b.WriteString(`</tbody></table></section>`)
}

func renderPrivateMaintenance(b *strings.Builder, sec domain.DocumentSection, calc domain.KioskQueueCalculation) {
	fmt.Fprintf(b, `<section data-block-type="commercial-maintenance" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	b.WriteString(`<table><thead><tr><th>Maintenance Squad</th><th>Rate (Bulanan)</th><th>Jumlah SDM</th><th>Durasi</th><th>Total</th></tr></thead><tbody>`)
	for mIdx, item := range calc.Maintenance {
		fmt.Fprintf(b, `<tr data-role="maintenance_%d"><td>%s</td><td>Rp %s</td><td>%d</td><td>%s</td><td data-field="maintenance_line_total">Rp %s</td></tr>`,
			mIdx,
			htmlEscape(item.Name),
			formatNumber(item.MonthlyRate),
			item.Headcount,
			formatQuarterDuration(int64(item.PeriodQuarters), 0),
			formatNumber(item.LineTotal),
		)
	}
	fmt.Fprintf(b, `<tr><td colspan="4"><strong>TOTAL MAINTENANCE</strong></td><td data-field="maintenance_total">Rp %s</td></tr>`, formatNumber(calc.MaintenanceTotal))
	b.WriteString(`</tbody></table></section>`)
}

func renderPrivateTimeline(b *strings.Builder, sec domain.DocumentSection, rows []domain.KioskQueueTimelineRowInput) {
	fmt.Fprintf(b, `<section data-block-type="scope" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	b.WriteString(`<table><thead><tr><th data-column-width="75">Description</th><th data-column-width="25">Timeline</th></tr></thead><tbody>`)
	for rIdx, row := range rows {
		fmt.Fprintf(b, `<tr data-role="timeline_%d"><td>%s</td><td>%s</td></tr>`,
			rIdx,
			renderMultiline(row.Description),
			htmlEscape(row.Timeline),
		)
	}
	b.WriteString(`</tbody></table></section>`)
}

func renderKioskPayments(b *strings.Builder, sec domain.DocumentSection, input domain.KioskQueueSystemInput, calc domain.KioskQueueCalculation, profile domain.ResolvedKioskQueueProfile) {
	fmt.Fprintf(b, `<section data-block-type="payment" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	switch input.Variant {
	case domain.KioskQueueGovernmentMPP:
		b.WriteString(`<table><thead><tr><th>Tahap Pembayaran</th><th>Dasar Penagihan</th><th>Nominal</th></tr></thead><tbody>`)
		for pIdx, p := range input.GovernmentPaymentSchedule {
			fmt.Fprintf(b, `<tr data-role="government_payment_%d"><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`,
				pIdx,
				htmlEscape(p.Stage),
				renderMultiline(p.BillingBasis),
				formatNumber(p.Nominal),
			)
		}
		fmt.Fprintf(b, `<tr><td colspan="2"><strong>TOTAL</strong></td><td data-field="payment_total">Rp %s</td></tr>`, formatNumber(calc.PaymentTotal))
		b.WriteString(`</tbody></table>`)

	case domain.KioskQueuePrivateFoodCourt:
		b.WriteString(`<table><thead><tr><th>Work Period</th><th>Terms</th><th>Invoice Date</th><th>Due Date</th><th>Nominal</th></tr></thead><tbody>`)
		for pIdx, p := range input.PrivatePaymentSchedule {
			fmt.Fprintf(b, `<tr data-role="private_payment_%d"><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td data-field="payment_nominal">Rp %s</td></tr>`,
				pIdx,
				htmlEscape(p.WorkPeriod),
				htmlEscape(p.Terms),
				htmlEscape(p.InvoiceDateLabel),
				htmlEscape(p.DueDateLabel),
				formatNumber(p.Nominal),
			)
		}
		fmt.Fprintf(b, `<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="payment_total">Rp %s</td></tr>`, formatNumber(calc.PaymentTotal))
		b.WriteString(`</tbody></table>`)
	}
	if profile.TaxClauseText != "" {
		fmt.Fprintf(b, `<p data-field="tax_clause">%s</p>`, htmlEscape(profile.TaxClauseText))
	}
	if profile.PaymentDueDays > 0 {
		fmt.Fprintf(b, `<p data-field="payment_due_clause">Pembayaran dilakukan maksimal H+%d hari kalender setelah invoice diterbitkan.</p>`, profile.PaymentDueDays)
	}
	b.WriteString(`</section>`)
}

func renderKioskLimitations(b *strings.Builder, sec domain.DocumentSection, profile domain.ResolvedKioskQueueProfile, additionalLimitations []string) {
	fmt.Fprintf(b, `<section data-block-type="limitations" data-section-number="%d"><h2>%d. %s</h2>`, sec.Number, sec.Number, htmlEscape(sec.Title))
	b.WriteString(`<ol>`)
	texts := profile.LimitationTexts
	if len(texts) == 0 && len(additionalLimitations) > 0 {
		texts = additionalLimitations
	}
	for _, text := range texts {
		fmt.Fprintf(b, `<li>%s</li>`, htmlEscape(text))
	}
	b.WriteString(`</ol></section>`)
}

func renderKioskRights(b *strings.Builder, statements []string) {
	b.WriteString(`<div data-block-type="rights">`)
	for _, stmt := range statements {
		fmt.Fprintf(b, `<p>%s</p>`, htmlEscape(stmt))
	}
	b.WriteString(`</div>`)
}

func renderKioskSignatures(b *strings.Builder, m QuotationMetadata, qDate time.Time) {
	dateStr := m.IssueDate
	if dateStr == "" && !qDate.IsZero() {
		dateStr = indonesianDate(qDate)
	}

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
}
