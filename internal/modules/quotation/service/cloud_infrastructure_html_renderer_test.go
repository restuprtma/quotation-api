package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func cloudRendererInput() domain.CloudInfrastructureInput {
	return domain.CloudInfrastructureInput{DocumentNumber: "CTH/Authen-Cloud/0626/001", ServiceName: "Venturo Cloud", QuotationDate: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), TaxTreatment: domain.CloudTaxExclusive, PackageItems: []domain.CloudPackageItemInput{{Category: domain.CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900000}, {Category: domain.CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900000}}, ScopeRows: []domain.CloudScopeRowInput{{Sections: []domain.CloudScopeSectionInput{{Title: "Performance / Load Test", Activities: []string{"Membuat script testing menggunakan tools K6"}}, {Title: "Penetration Test", Activities: []string{"Melakukan vulnerability assessment"}}}, Timeline: "2 Minggu"}}, PaymentSchedule: []domain.CloudPaymentRowInput{{WorkPeriod: "Bulan 1", Terms: "Pelunasan Bulanan", InvoiceDateLabel: "30 Jun 2026", DueDateLabel: "14 Jul 2026", Component: domain.CloudPaymentMonthly, Nominal: 1800000}}, PaymentDueDays: 14, LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitOutOfPocket}}
}

func TestCloudInfrastructureHTMLRendererCanonicalAuthen(t *testing.T) {
	in := cloudRendererInput()
	calc, err := CalculateCloudInfrastructure(in)
	require.NoError(t, err)
	profile, err := ResolveCloudInfrastructureProfile(in)
	require.NoError(t, err)
	doc, err := NewCloudInfrastructureRenderer().Render(CloudInfrastructureDocumentInput{Metadata: QuotationMetadata{QuotationNumber: in.DocumentNumber, ClientName: "Authen", IssueDate: "24 Juni 2026"}, Input: in, Calculation: calc, Profile: profile, LegalHTML: `<p>Ketentuan legal</p>`})
	require.NoError(t, err)
	assert.Contains(t, doc, `class="quotation-document quotation-document--cloud-infrastructure"`)
	assert.Contains(t, doc, `data-section-code="cloud_cost"`)
	assert.Contains(t, doc, `<th>Nama Paket</th><th>VCPU</th><th>RAM</th><th>Disk / Storage</th><th>Unit</th><th>Total</th>`)
	assert.Equal(t, 2, strings.Count(doc, `data-cloud-package-row=`))
	assert.Contains(t, doc, `data-cloud-field="monthly_subtotal">Rp 1.800.000`)
	assert.Contains(t, doc, "Performance / Load Test")
	assert.Contains(t, doc, "Penetration Test")
	assert.Contains(t, doc, "2 Minggu")
	assert.NotContains(t, doc, "99,5%")
	assert.NotContains(t, doc, "backup harian")
	assert.NotContains(t, doc, "Setup Fee Server (One Time Only)")
	assert.Contains(t, doc, "Malang,")
}
