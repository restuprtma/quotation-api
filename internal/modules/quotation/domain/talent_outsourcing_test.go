package domain

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func validTalentManpowerInput() TalentOutsourcingInput {
	return TalentOutsourcingInput{EngagementMode: TalentModeManpowerSupply, DocumentNumber: "LANDA/BPJS/0626/001", ServiceName: "Outsourcing Talent BPJS", QuotationDate: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), TaxTreatment: TalentTaxBPJSDPP12, CostGroups: []TalentCostGroupInput{{Category: TalentCategoryPersonnel, Title: "Personil", Items: []TalentCostItemInput{{Name: "Project Manager", Quantity: 1, QuantityUnit: "Orang", PeriodCount: 32, PeriodUnit: TalentPeriodWorkday, UnitRate: 25_000_000}}}}, ScopeBlocks: []TalentScopeBlockInput{{Title: "Scope", Description: "Pekerjaan", Timeline: "Juli 2026"}}, PaymentSchedule: []TalentPaymentRowInput{{WorkPeriod: "Juli", Terms: "Termin 1", InvoiceDateLabel: "1 Agustus", DueDateLabel: "14 Agustus", Nominal: 44_400_000}}, LimitationCodes: []TalentLimitationCode{TalentLimitOutOfPocket}}
}
func TestTalentOutsourcingValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TalentOutsourcingInput)
		want   string
	}{{"unknown mode", func(v *TalentOutsourcingInput) { v.EngagementMode = "other" }, "engagement_mode"}, {"missing document", func(v *TalentOutsourcingInput) { v.DocumentNumber = " " }, "document_number"}, {"html", func(v *TalentOutsourcingInput) { v.ScopeBlocks[0].Description = "<script>" }, "HTML-like"}, {"non integral", func(v *TalentOutsourcingInput) { v.CostGroups[0].Items[0].UnitRate = 1 }, "whole rupiah"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validTalentManpowerInput()
			tt.mutate(&v)
			require.ErrorContains(t, v.Validate(), tt.want)
		})
	}
}
func TestTalentOutsourcingStrictJSON(t *testing.T) {
	raw := `{"engagement_mode":"manpower_supply","document_number":"A","service_name":"A","quotation_date":"2026-06-10","tax_treatment":"bpjs_dpp_ppn_12","cost_groups":[],"scope_blocks":[],"payment_schedule":[],"limitation_codes":[],"unexpected":1}`
	var in TalentOutsourcingInput
	err := json.Unmarshal([]byte(raw), &in)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unknown field"))
}
