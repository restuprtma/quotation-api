package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validPercetakanServiceInput() domain.SubscriptionSaaSInput {
	return domain.SubscriptionSaaSInput{
		ProductName:       "Subscription AI Percetakan",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      domain.SubscriptionBillingQuarterly,
		TaxTreatment:      domain.SubscriptionTaxNotStated,
		InitialTermMonths: 12,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Subscription 12 bulan", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_250_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Agustus-Oktober 2026", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Nominal: 3_750_000},
			{WorkPeriod: "November 2026-Januari 2027", StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: "1 Oktober 2026", DueDateLabel: "15 Oktober 2026", Nominal: 3_750_000},
			{WorkPeriod: "Februari-April 2027", StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: "1 Januari 2027", DueDateLabel: "15 Januari 2027", Nominal: 3_750_000},
			{WorkPeriod: "Mei-Juli 2027", StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: "1 April 2027", DueDateLabel: "15 April 2027", Nominal: 3_750_000},
		},
		IsPartnership: true,
		IncomeSharing: &domain.IncomeSharingInput{
			VenturoShareBPS:            5_000,
			PartnerShareBPS:            5_000,
			EligibleRevenueDescription: "Penjualan dari referensi mitra",
			SettlementBusinessDays:     30,
			ProhibitUnilateralDiscount: true,
		},
	}
}

func TestCalculateSubscriptionSaaSPercetakan(t *testing.T) {
	input := validPercetakanServiceInput()
	got, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	assert.Equal(t, int64(15_000_000), got.RecurringTotal)
	assert.Zero(t, got.OneTimeTotal)
	assert.Equal(t, int64(15_000_000), got.TotalContract)
	assert.Len(t, got.PricePeriods, 1)
	assert.Equal(t, int64(15_000_000), got.PricePeriods[0].Subtotal)
	assert.Len(t, got.Payments, 4)
	assert.Empty(t, got.OneTimeItems)
}

func TestCalculateSubscriptionSaaSAnnualRateChange(t *testing.T) {
	input := domain.SubscriptionSaaSInput{
		ProductName:       "Annual SaaS",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutDevelopmentMaintenance,
		BillingCycle:      domain.SubscriptionBillingYearly,
		TaxTreatment:      domain.SubscriptionTaxNotStated,
		InitialTermMonths: 24,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Tahun 1 - Development", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_000_000},
			{Label: "Tahun 2 - Maintenance", StartMonthOffset: 12, DurationMonths: 12, MonthlyRate: 1_500_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Tahun 1", StartMonthOffset: 0, CoveredMonths: 12, InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Nominal: 12_000_000},
			{WorkPeriod: "Tahun 2", StartMonthOffset: 12, CoveredMonths: 12, InvoiceDateLabel: "Awal tahun kedua", DueDateLabel: "H+14", Nominal: 18_000_000},
		},
		FeatureGroups: []domain.SubscriptionFeatureGroupInput{
			{Name: "Core", Features: []string{"Login"}},
		},
	}
	got, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	assert.Equal(t, int64(30_000_000), got.RecurringTotal)
	assert.Zero(t, got.OneTimeTotal)
	assert.Equal(t, int64(30_000_000), got.TotalContract)
	assert.Len(t, got.PricePeriods, 2)
	assert.Equal(t, int64(12_000_000), got.PricePeriods[0].Subtotal)
	assert.Equal(t, int64(18_000_000), got.PricePeriods[1].Subtotal)
	assert.Len(t, got.Payments, 2)
}

func TestCalculateSubscriptionSaaSWithOneTimeItems(t *testing.T) {
	input := validPercetakanServiceInput()
	input.OneTimeItems = []domain.SubscriptionOneTimeItemInput{
		{Name: "Setup and Onboarding", Amount: 2_500_000},
		{Name: "Data Migration", Amount: 1_500_000},
	}
	// Recurring is 15M, OneTime is 4M, TotalContract is 19M.
	// Adjust payment schedule to total 19M (e.g. 4.75M each).
	input.PaymentSchedule[0].Nominal = 4_750_000
	input.PaymentSchedule[1].Nominal = 4_750_000
	input.PaymentSchedule[2].Nominal = 4_750_000
	input.PaymentSchedule[3].Nominal = 4_750_000

	got, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	assert.Equal(t, int64(15_000_000), got.RecurringTotal)
	assert.Equal(t, int64(4_000_000), got.OneTimeTotal)
	assert.Equal(t, int64(19_000_000), got.TotalContract)
	assert.Len(t, got.OneTimeItems, 2)
	assert.Equal(t, "Setup and Onboarding", got.OneTimeItems[0].Name)
	assert.Equal(t, int64(2_500_000), got.OneTimeItems[0].Amount)
}

func TestCalculateSubscriptionSaaSPaymentMismatch(t *testing.T) {
	t.Run("payment kurang Rp1", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.PaymentSchedule[3].Nominal -= 1 // 14_999_999 vs 15_000_000
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "payment_schedule: nominal total must equal total contract")
	})

	t.Run("payment lebih Rp1", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.PaymentSchedule[3].Nominal += 1 // 15_000_001 vs 15_000_000
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "payment_schedule: nominal total must equal total contract")
	})
}

func TestCalculateSubscriptionSaaSExceedsSafeIntegerLimit(t *testing.T) {
	input := domain.SubscriptionSaaSInput{
		ProductName:       "Huge SaaS",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      domain.SubscriptionBillingMonthly,
		TaxTreatment:      domain.SubscriptionTaxNotStated,
		InitialTermMonths: 2,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			// Each month is 5_000_000_000_000_000 (<= MaxSubscriptionSafeAmount: 9_000_000_000_000_000)
			// Total for 2 months is 10_000_000_000_000_000 (> MaxSubscriptionSafeAmount)
			{Label: "Month 1-2", StartMonthOffset: 0, DurationMonths: 2, MonthlyRate: 5_000_000_000_000_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Month 1", StartMonthOffset: 0, CoveredMonths: 1, InvoiceDateLabel: "Start", DueDateLabel: "H+14", Nominal: 5_000_000_000_000_000},
			{WorkPeriod: "Month 2", StartMonthOffset: 1, CoveredMonths: 1, InvoiceDateLabel: "Month 2", DueDateLabel: "H+14", Nominal: 5_000_000_000_000_000},
		},
	}
	_, err := CalculateSubscriptionSaaS(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "total_contract: exceeds JSON safe integer limit")
}

func TestCalculateSubscriptionSaaSValidationErrors(t *testing.T) {
	t.Run("zero monthly rate", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.PricePeriods[0].MonthlyRate = 0
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "monthly_rate: must be greater than zero")
	})

	t.Run("negative monthly rate", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.PricePeriods[0].MonthlyRate = -1_000_000
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "monthly_rate: must be greater than zero")
	})

	t.Run("negative one-time item amount", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.OneTimeItems = []domain.SubscriptionOneTimeItemInput{
			{Name: "Discount Item", Amount: -500_000},
		}
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "amount: must be greater than zero")
	})

	t.Run("negative payment nominal", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.PaymentSchedule[0].Nominal = -1_000_000
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nominal: must be greater than zero")
	})

	t.Run("payment coverage gap", func(t *testing.T) {
		input := validPercetakanServiceInput()
		input.PaymentSchedule[1].StartMonthOffset = 4 // gap between month 3 and 4
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start_month_offset: must be contiguous")
	})

	t.Run("payment row duration does not match billing cycle", func(t *testing.T) {
		input := validPercetakanServiceInput()
		// Quarterly expects 3 months per row (except optionally the last one)
		input.PaymentSchedule[0].CoveredMonths = 2
		input.PaymentSchedule[1].StartMonthOffset = 2
		_, err := CalculateSubscriptionSaaS(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "covered_months: must be 3 for quarterly billing cycle")
	})
}

func TestCalculateSubscriptionSaaSImmutability(t *testing.T) {
	input := validPercetakanServiceInput()
	input.OneTimeItems = []domain.SubscriptionOneTimeItemInput{
		{Name: "Initial Setup", Amount: 1_000_000},
	}
	input.PaymentSchedule[0].Nominal += 1_000_000 // total 16M

	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)

	// Mutate input
	input.PricePeriods[0].MonthlyRate = 999_999
	input.OneTimeItems[0].Amount = 999_999
	input.PaymentSchedule[0].Nominal = 999_999

	// Output must remain unchanged
	assert.Equal(t, int64(1_250_000), calc.PricePeriods[0].MonthlyRate)
	assert.Equal(t, int64(15_000_000), calc.PricePeriods[0].Subtotal)
	assert.Equal(t, int64(1_000_000), calc.OneTimeItems[0].Amount)
	assert.Equal(t, int64(4_750_000), calc.Payments[0].Nominal)

	// Mutate calculation output slice
	calc.OneTimeItems[0].Amount = 888_888
	calc.Payments[0].Nominal = 888_888
	assert.Equal(t, int64(999_999), input.OneTimeItems[0].Amount)
	assert.Equal(t, int64(999_999), input.PaymentSchedule[0].Nominal)
}
