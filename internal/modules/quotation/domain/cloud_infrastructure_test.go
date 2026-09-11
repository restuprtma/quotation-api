package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustCloudDate(value string) time.Time {
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return d
}
func validCloudInput() CloudInfrastructureInput {
	return CloudInfrastructureInput{DocumentNumber: " CTH/Authen-Cloud/0626/001 ", ServiceName: " Venturo Cloud ", QuotationDate: mustCloudDate("2026-06-24"), TaxTreatment: CloudTaxExclusive, PackageItems: []CloudPackageItemInput{{Category: CloudPackageVPS, Name: " Virtual Machine Small - Kube Master ", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900000}, {Category: CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900000}}, ScopeRows: []CloudScopeRowInput{{Sections: []CloudScopeSectionInput{{Title: " Performance / Load Test ", Activities: []string{" Menyiapkan staging environment ", "Membuat script testing menggunakan tools K6"}}}, Timeline: " 2 Minggu "}}, PaymentSchedule: []CloudPaymentRowInput{{WorkPeriod: " Bulan 1 ", Terms: "Pelunasan Bulanan", InvoiceDateLabel: "30 Jun 2026", DueDateLabel: "14 Jul 2026", Component: CloudPaymentMonthly, Nominal: 1800000}}, PaymentDueDays: 14, LimitationCodes: []CloudLimitationCode{CloudLimitOutOfPocket, CloudLimitOvertime, CloudLimitThirdParty, CloudLimitScopeChange}}
}
func TestCloudInfrastructureNormalizeAndValidate(t *testing.T) {
	original := validCloudInput()
	got := original.Normalize()
	assert.Equal(t, "CTH/Authen-Cloud/0626/001", got.DocumentNumber)
	assert.Equal(t, "Venturo Cloud", got.ServiceName)
	assert.Equal(t, "Virtual Machine Small - Kube Master", got.PackageItems[0].Name)
	assert.Equal(t, "Menyiapkan staging environment", got.ScopeRows[0].Sections[0].Activities[0])
	assert.Equal(t, "2 Minggu", got.ScopeRows[0].Timeline)
	assert.Equal(t, " Virtual Machine Small - Kube Master ", original.PackageItems[0].Name)
	require.NoError(t, got.Validate())
}
func TestCloudInfrastructureRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name, want string
		mutate     func(*CloudInfrastructureInput)
	}{{"empty document", "document_number", func(in *CloudInfrastructureInput) { in.DocumentNumber = " " }}, {"unknown category", "package_items[0].category", func(in *CloudInfrastructureInput) { in.PackageItems[0].Category = "bare_metal" }}, {"zero vcpu", "package_items[0].vcpu", func(in *CloudInfrastructureInput) { in.PackageItems[0].VCPU = 0 }}, {"unsafe money", "package_items[0].unit_rate", func(in *CloudInfrastructureInput) { in.PackageItems[0].UnitRate = MaxCloudMoney + 1 }}, {"html package", "package_items[0].name", func(in *CloudInfrastructureInput) { in.PackageItems[0].Name = "<b>VPS</b>" }}, {"empty activities", "scope_rows[0].sections[0].activities", func(in *CloudInfrastructureInput) { in.ScopeRows[0].Sections[0].Activities = nil }}, {"duplicate limitation", "limitation_codes", func(in *CloudInfrastructureInput) {
		in.LimitationCodes = append(in.LimitationCodes, CloudLimitOutOfPocket)
	}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validCloudInput()
			tt.mutate(&in)
			require.ErrorContains(t, in.Validate(), tt.want)
		})
	}
}
func TestCloudInfrastructureStrictJSON(t *testing.T) {
	var in CloudInfrastructureInput
	require.NoError(t, json.Unmarshal([]byte(`{"document_number":"A","service_name":"Cloud","quotation_date":"2026-06-24","tax_treatment":"exclusive_ppn"}`), &in))
	assert.Equal(t, mustCloudDate("2026-06-24"), in.QuotationDate)
	require.ErrorContains(t, json.Unmarshal([]byte(`{"quotation_date":"2026-02-30"}`), &in), "quotation_date")
	require.ErrorContains(t, json.Unmarshal([]byte(`{"quotation_date":123}`), &in), "quotation_date")
	require.ErrorContains(t, json.Unmarshal([]byte(`{"quotation_date":"2026-06-24","unknown":true}`), &in), "unknown field")
	require.Error(t, json.Unmarshal([]byte(`{"quotation_date":"2026-06-24"}{}`), &in))
	var req struct {
		Cloud *CloudInfrastructureInput `json:"cloud_infrastructure"`
	}
	err := json.Unmarshal([]byte(`{"cloud_infrastructure":{"payment_schedule":[{"nominal":"#REF!"}]}}`), &req)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "cannot unmarshal"))
}
