package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validCloudInfrastructureInput() domain.CloudInfrastructureInput {
	return domain.CloudInfrastructureInput{
		DocumentNumber: "CTH/Authen-Cloud/0626/001",
		ServiceName:    "Venturo Cloud",
		QuotationDate:  time.Date(2026, time.June, 24, 0, 0, 0, 0, time.UTC),
		TaxTreatment:   domain.CloudTaxExclusive,
		PackageItems: []domain.CloudPackageItemInput{
			{Category: domain.CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900_000},
			{Category: domain.CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900_000},
		},
		ScopeRows: []domain.CloudScopeRowInput{{
			Sections: []domain.CloudScopeSectionInput{{Title: "Performance / Load Test", Activities: []string{"Load test"}}},
			Timeline: "2 Minggu",
		}},
		PaymentSchedule: []domain.CloudPaymentRowInput{{
			WorkPeriod: "Bulan 1", Terms: "Pelunasan Bulanan", InvoiceDateLabel: "30 Jun 2026", DueDateLabel: "14 Jul 2026", Component: domain.CloudPaymentMonthly, Nominal: 1_800_000,
		}},
		PaymentDueDays:  14,
		LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitOutOfPocket},
	}
}

func TestCloudInfrastructureCalculator(t *testing.T) {
	t.Run("calculates Authen monthly quotation", func(t *testing.T) {
		calc, err := CalculateCloudInfrastructure(validCloudInfrastructureInput().Normalize())
		require.NoError(t, err)
		require.Len(t, calc.Packages, 2)
		assert.Equal(t, int64(900_000), calc.Packages[0].LineTotal)
		assert.Equal(t, int64(1_800_000), calc.MonthlySubtotal)
		assert.Zero(t, calc.SetupSubtotal)
		assert.Zero(t, calc.SetupPaymentTotal)
		assert.Equal(t, int32(1), calc.MonthlyPaymentRows)
		assert.Equal(t, int64(1_800_000), calc.ScheduledRecurringTotal)
		assert.Equal(t, int64(1_800_000), calc.PaymentTotal)
	})

	t.Run("reconciles setup payment", func(t *testing.T) {
		in := validCloudInfrastructureInput()
		in.SetupItems = []domain.CloudSetupItemInput{{Name: "Setup Fee Server", Amount: 500_000}}
		in.PaymentSchedule = append([]domain.CloudPaymentRowInput{{
			WorkPeriod: "Setup", Terms: "Pelunasan", InvoiceDateLabel: "24 Jun 2026", DueDateLabel: "8 Jul 2026", Component: domain.CloudPaymentSetup, Nominal: 500_000,
		}}, in.PaymentSchedule...)

		calc, err := CalculateCloudInfrastructure(in.Normalize())
		require.NoError(t, err)
		assert.Equal(t, int64(500_000), calc.SetupSubtotal)
		assert.Equal(t, int64(500_000), calc.SetupPaymentTotal)
		assert.Equal(t, int64(2_300_000), calc.PaymentTotal)
	})

	t.Run("tracks recurring rows separately", func(t *testing.T) {
		in := validCloudInfrastructureInput()
		in.PaymentSchedule = append(in.PaymentSchedule, domain.CloudPaymentRowInput{
			WorkPeriod: "Bulan 2", Terms: "Pelunasan Bulanan", InvoiceDateLabel: "30 Jul 2026", DueDateLabel: "13 Agu 2026", Component: domain.CloudPaymentMonthly, Nominal: 1_800_000,
		})

		calc, err := CalculateCloudInfrastructure(in.Normalize())
		require.NoError(t, err)
		assert.Equal(t, int32(2), calc.MonthlyPaymentRows)
		assert.Equal(t, int64(3_600_000), calc.ScheduledRecurringTotal)
		assert.Equal(t, int64(3_600_000), calc.PaymentTotal)
	})
}

func TestCloudInfrastructureCalculatorRejectsInvalidReconciliation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*domain.CloudInfrastructureInput)
		want string
	}{
		{"monthly mismatch", func(in *domain.CloudInfrastructureInput) { in.PaymentSchedule[0].Nominal = 1_799_999 }, "payment_schedule[0].nominal"},
		{"missing setup payment", func(in *domain.CloudInfrastructureInput) {
			in.SetupItems = []domain.CloudSetupItemInput{{Name: "Setup", Amount: 500_000}}
		}, "setup payment total"},
		{"setup payment without item", func(in *domain.CloudInfrastructureInput) {
			in.PaymentSchedule = append([]domain.CloudPaymentRowInput{{WorkPeriod: "Setup", Terms: "Pelunasan", InvoiceDateLabel: "24 Jun", DueDateLabel: "8 Jul", Component: domain.CloudPaymentSetup, Nominal: 500_000}}, in.PaymentSchedule...)
		}, "setup payment total"},
		{"setup rows do not reconcile", func(in *domain.CloudInfrastructureInput) {
			in.SetupItems = []domain.CloudSetupItemInput{{Name: "Setup", Amount: 500_000}}
			in.PaymentSchedule = append([]domain.CloudPaymentRowInput{{WorkPeriod: "Setup 1", Terms: "Pelunasan", InvoiceDateLabel: "24 Jun", DueDateLabel: "8 Jul", Component: domain.CloudPaymentSetup, Nominal: 200_000}, {WorkPeriod: "Setup 2", Terms: "Pelunasan", InvoiceDateLabel: "25 Jun", DueDateLabel: "9 Jul", Component: domain.CloudPaymentSetup, Nominal: 200_000}}, in.PaymentSchedule...)
		}, "setup payment total"},
		{"missing monthly row", func(in *domain.CloudInfrastructureInput) {
			in.PaymentSchedule = []domain.CloudPaymentRowInput{{WorkPeriod: "Setup", Terms: "Pelunasan", InvoiceDateLabel: "24 Jun", DueDateLabel: "8 Jul", Component: domain.CloudPaymentSetup, Nominal: 1}}
			in.SetupItems = []domain.CloudSetupItemInput{{Name: "Setup", Amount: 1}}
		}, "missing monthly_service"},
		{"package subtotal overflow", func(in *domain.CloudInfrastructureInput) {
			in.PackageItems = []domain.CloudPackageItemInput{{Category: domain.CloudPackageVPS, Name: "One", VCPU: 1, RAMGB: 1, StorageGB: 1, UnitRate: domain.MaxCloudMoney}, {Category: domain.CloudPackageVPS, Name: "Two", VCPU: 1, RAMGB: 1, StorageGB: 1, UnitRate: domain.MaxCloudMoney}}
			in.PaymentSchedule[0].Nominal = domain.MaxCloudMoney
		}, "package_items[1]"},
		{"recurring total overflow", func(in *domain.CloudInfrastructureInput) {
			in.PackageItems = []domain.CloudPackageItemInput{{Category: domain.CloudPackageVPS, Name: "One", VCPU: 1, RAMGB: 1, StorageGB: 1, UnitRate: domain.MaxCloudMoney}}
			in.PaymentSchedule = append(in.PaymentSchedule, domain.CloudPaymentRowInput{WorkPeriod: "Bulan 2", Terms: "Pelunasan", InvoiceDateLabel: "30 Jul", DueDateLabel: "13 Agu", Component: domain.CloudPaymentMonthly, Nominal: domain.MaxCloudMoney})
			in.PaymentSchedule[0].Nominal = domain.MaxCloudMoney
		}, "scheduled recurring total overflow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validCloudInfrastructureInput()
			tt.edit(&in)
			_, err := CalculateCloudInfrastructure(in.Normalize())
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tt.want), "error %q does not contain %q", err, tt.want)
		})
	}
}
