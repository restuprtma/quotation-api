package service

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func talentBPJSInput() domain.TalentOutsourcingInput {
	return domain.TalentOutsourcingInput{EngagementMode: domain.TalentModeManpowerSupply, DocumentNumber: "A", ServiceName: "A", QuotationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), TaxTreatment: domain.TalentTaxBPJSDPP12, CostGroups: []domain.TalentCostGroupInput{{Category: domain.TalentCategoryPersonnel, Title: "Personil", Items: []domain.TalentCostItemInput{{Name: "PM", Quantity: 1, QuantityUnit: "Orang", PeriodCount: 32, PeriodUnit: domain.TalentPeriodWorkday, UnitRate: 25_000_000}}}}, ScopeBlocks: []domain.TalentScopeBlockInput{{Title: "Scope", Description: "Description", Timeline: "Jan"}}, PaymentSchedule: []domain.TalentPaymentRowInput{{WorkPeriod: "Jan", Terms: "T1", InvoiceDateLabel: "Feb", DueDateLabel: "Feb", Nominal: 44_400_000}}, LimitationCodes: []domain.TalentLimitationCode{domain.TalentLimitOutOfPocket}}
}
func TestCalculateTalentOutsourcing_BPJS(t *testing.T) {
	in := talentBPJSInput()
	calc, err := CalculateTalentOutsourcing(in)
	require.NoError(t, err)
	require.Equal(t, int64(40_000_000), calc.PersonnelSubtotal)
	require.Equal(t, int64(36_666_666), calc.DPPAmount)
	require.Equal(t, int64(4_400_000), calc.TaxAmount)
	require.Equal(t, int64(44_400_000), calc.GrandTotal)
	require.Equal(t, calc.GrandTotal, calc.PaymentTotal)
}
