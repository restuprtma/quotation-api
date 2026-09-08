package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func validWhiteLabelInputForService() domain.WhiteLabelInput {
	return domain.WhiteLabelInput{
		ProductName:   "POS Mini Market",
		QuotationDate: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		TaxTreatment:  domain.WhiteLabelTaxExclusive,
		LicenseItems: []domain.WhiteLabelLicenseItemInput{
			{Name: "POS Mini Market", UnitRate: 35_000_000, Quantity: 1},
		},
		MaintenanceItems: []domain.WhiteLabelMaintenanceItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 16_170_000, FTEQuarters: 2, PeriodMonths: 3},
		},
		HostingItems: []domain.WhiteLabelHostingItemInput{
			{Name: "Shared Hosting Small", MonthlyRate: 350_000, Quantity: 1, PeriodMonths: 3},
		},
		ModuleGroups: []domain.WhiteLabelModuleGroupInput{
			{Name: "Inventory", Features: []string{"Stok masuk", "Stok keluar"}},
		},
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 2", Terms: "Bulanan", InvoiceDateLabel: "Bulan 2", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 3", Terms: "Bulanan", InvoiceDateLabel: "Bulan 3", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func TestCalculateWhiteLabel_PonpesAmounts(t *testing.T) {
	got, err := service.CalculateWhiteLabel(validWhiteLabelInputForService())
	require.NoError(t, err)

	assert.Equal(t, int64(35_000_000), got.LicenseSubtotal)
	assert.Equal(t, int64(24_255_000), got.MaintenanceSubtotal)
	assert.Equal(t, int64(1_050_000), got.HostingSubtotal)
	assert.Equal(t, int64(25_305_000), got.MaintenanceHostingSubtotal)
	assert.Equal(t, int64(60_305_000), got.TotalInvestment)
	assert.Equal(t, got.TotalInvestment, got.PaymentTotal)

	assert.Equal(t, int64(35_000_000), got.LicensePaymentTotal)
	assert.Equal(t, int64(25_305_000), got.MaintenanceHostingPaymentTotal)

	require.Len(t, got.License, 1)
	assert.Equal(t, "POS Mini Market", got.License[0].Name)
	assert.Equal(t, int64(35_000_000), got.License[0].LineTotal)

	require.Len(t, got.Maintenance, 1)
	assert.Equal(t, "Senior Web Programmer", got.Maintenance[0].Name)
	assert.Equal(t, int64(24_255_000), got.Maintenance[0].LineTotal)

	require.Len(t, got.Hosting, 1)
	assert.Equal(t, "Shared Hosting Small", got.Hosting[0].Name)
	assert.Equal(t, int64(1_050_000), got.Hosting[0].LineTotal)
}

func TestCalculateWhiteLabel_LicensePaymentMismatch(t *testing.T) {
	t.Run("license payment kurang Rp1", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.PaymentSchedule[0].Nominal -= 1
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule.license: total 34999999 must equal license subtotal 35000000", err.Error())
	})

	t.Run("license payment lebih Rp1", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.PaymentSchedule[0].Nominal += 1
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule.license: total 35000001 must equal license subtotal 35000000", err.Error())
	})
}

func TestCalculateWhiteLabel_MaintenanceHostingPaymentMismatch(t *testing.T) {
	t.Run("maintenance hosting payment kurang", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.PaymentSchedule[3].Nominal -= 10_000
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule.maintenance_hosting: total 25295000 must equal maintenance and hosting subtotal 25305000", err.Error())
	})

	t.Run("maintenance hosting payment lebih", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.PaymentSchedule[3].Nominal += 10_000
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule.maintenance_hosting: total 25315000 must equal maintenance and hosting subtotal 25305000", err.Error())
	})

	t.Run("zero maintenance and hosting subtotal with unexpected maintenance_hosting payment", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.MaintenanceItems = nil
		input.HostingItems = nil
		input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance Tambahan", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 1_000_000},
		}
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule.maintenance_hosting: total 1000000 must equal maintenance and hosting subtotal 0", err.Error())
	})

	t.Run("zero maintenance and hosting subtotal with license payment only succeeds", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.MaintenanceItems = nil
		input.HostingItems = nil
		input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
		}
		got, err := service.CalculateWhiteLabel(input)
		require.NoError(t, err)
		assert.Equal(t, int64(35_000_000), got.LicenseSubtotal)
		assert.Zero(t, got.MaintenanceSubtotal)
		assert.Zero(t, got.HostingSubtotal)
		assert.Zero(t, got.MaintenanceHostingSubtotal)
		assert.Equal(t, int64(35_000_000), got.TotalInvestment)
		assert.Equal(t, int64(35_000_000), got.LicensePaymentTotal)
		assert.Zero(t, got.MaintenanceHostingPaymentTotal)
		assert.Equal(t, int64(35_000_000), got.PaymentTotal)
		assert.Empty(t, got.Maintenance)
		assert.Empty(t, got.Hosting)
	})
}

func TestCalculateWhiteLabel_MultiplicationOverflow(t *testing.T) {
	t.Run("license line item multiplication overflow", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems[0].UnitRate = 5_000_000_000_000_000
		input.LicenseItems[0].Quantity = 2
		input.PaymentSchedule[0].Nominal = domain.MaxWhiteLabelMoney
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money multiplication exceeds limit")
	})

	t.Run("hosting line item multiplication overflow", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.HostingItems[0].MonthlyRate = 4_000_000_000_000_000
		input.HostingItems[0].Quantity = 2
		input.HostingItems[0].PeriodMonths = 2
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money multiplication exceeds limit")
	})
}

func TestCalculateWhiteLabel_AdditionOverflow(t *testing.T) {
	t.Run("license items addition overflow", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems = []domain.WhiteLabelLicenseItemInput{
			{Name: "Item A", UnitRate: 5_000_000_000_000_000, Quantity: 1},
			{Name: "Item B", UnitRate: 5_000_000_000_000_000, Quantity: 1},
		}
		input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "License Pay", Terms: "Pelunasan", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: domain.MaxWhiteLabelMoney},
		}
		input.MaintenanceItems = nil
		input.HostingItems = nil
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "license_subtotal: money addition exceeds limit")
	})

	t.Run("maintenance and hosting subtotal addition overflow", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems = []domain.WhiteLabelLicenseItemInput{
			{Name: "License", UnitRate: 10_000, Quantity: 1},
		}
		input.MaintenanceItems = []domain.WhiteLabelMaintenanceItemInput{
			{Name: "Maint A", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
		}
		input.HostingItems = []domain.WhiteLabelHostingItemInput{
			{Name: "Host A", MonthlyRate: 8_000_000_000_000_000, Quantity: 1, PeriodMonths: 1},
		}
		input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "License", Terms: "Pelunasan", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 10_000},
		}
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maintenance_hosting_subtotal: money addition exceeds limit")
	})

	t.Run("total investment addition overflow", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems = []domain.WhiteLabelLicenseItemInput{
			{Name: "License Huge", UnitRate: 8_000_000_000_000_000, Quantity: 1},
		}
		input.MaintenanceItems = []domain.WhiteLabelMaintenanceItemInput{
			{Name: "Maint Huge", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
		}
		input.HostingItems = nil
		input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "License", Terms: "Pelunasan", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 8_000_000_000_000_000},
		}
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "total_investment: money addition exceeds limit")
	})

	t.Run("payment schedule addition overflow", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems = []domain.WhiteLabelLicenseItemInput{
			{Name: "License", UnitRate: 9_000_000_000_000_000, Quantity: 1},
		}
		input.MaintenanceItems = nil
		input.HostingItems = nil
		input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "License 1", Terms: "Termin 1", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 5_000_000_000_000_000},
			{WorkPeriod: "License 2", Terms: "Termin 2", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 5_000_000_000_000_000},
		}
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money addition exceeds limit")
	})
}

func TestCalculateWhiteLabel_FTEFractionNotWholeRupiah(t *testing.T) {
	input := validWhiteLabelInputForService()
	input.MaintenanceItems = []domain.WhiteLabelMaintenanceItemInput{
		{Name: "Programmer", MonthlyRate: 10_000_001, FTEQuarters: 1, PeriodMonths: 1},
	}
	_, err := service.CalculateWhiteLabel(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a whole rupiah amount (divisible by 4)")
}

func TestCalculateWhiteLabel_ValidationErrors(t *testing.T) {
	t.Run("empty product name", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.ProductName = ""
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "product_name: is required")
	})

	t.Run("zero quotation date", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.QuotationDate = time.Time{}
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date: is required")
	})

	t.Run("unit rate zero", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems[0].UnitRate = 0
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unit_rate: must be greater than zero")
	})

	t.Run("quantity zero", func(t *testing.T) {
		input := validWhiteLabelInputForService()
		input.LicenseItems[0].Quantity = 0
		_, err := service.CalculateWhiteLabel(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quantity: must be greater than zero")
	})
}

func TestCalculateWhiteLabel_Immutability(t *testing.T) {
	input := validWhiteLabelInputForService()
	got, err := service.CalculateWhiteLabel(input)
	require.NoError(t, err)

	// Mutate input
	input.LicenseItems[0].UnitRate = 999
	input.MaintenanceItems[0].MonthlyRate = 888
	input.HostingItems[0].MonthlyRate = 777

	// Calculation result should be unchanged
	assert.Equal(t, int64(35_000_000), got.License[0].UnitRate)
	assert.Equal(t, int64(35_000_000), got.License[0].LineTotal)
	assert.Equal(t, int64(16_170_000), got.Maintenance[0].MonthlyRate)
	assert.Equal(t, int64(24_255_000), got.Maintenance[0].LineTotal)
	assert.Equal(t, int64(350_000), got.Hosting[0].MonthlyRate)
	assert.Equal(t, int64(1_050_000), got.Hosting[0].LineTotal)

	// Mutate calc
	got.License[0].UnitRate = 123
	assert.Equal(t, int64(999), input.LicenseItems[0].UnitRate)
}
