package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func validPonpesWhiteLabelInput() domain.WhiteLabelInput {
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
		TimelineRows: []domain.WhiteLabelTimelineRowInput{
			{Description: "Implementasi Sistem", Timeline: "Bulan 1-3"},
		},
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 2", Terms: "Bulanan", InvoiceDateLabel: "Bulan 2", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 3", Terms: "Bulanan", InvoiceDateLabel: "Bulan 3", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
		},
		RightsStatements: []string{"Hak cipta source code dan re-branding disepakati."},
		LimitationCodes:  []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func TestResolveWhiteLabelProfile_PonpesLayout(t *testing.T) {
	input := validPonpesWhiteLabelInput()
	profile, err := service.ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	assert.True(t, profile.ShowMaintenanceHosting)
	assert.True(t, profile.ShowScopeTimeline)
	assert.True(t, profile.ShowRights)

	expectedSections := []domain.DocumentSection{
		{Number: 1, Code: "white_label_cost", Title: "Biaya White Label"},
		{Number: 2, Code: "maintenance_hosting", Title: "Biaya Maintenance & Hosting"},
		{Number: 3, Code: "scope_timeline", Title: "Lingkup Pekerjaan & Timeline"},
		{Number: 4, Code: "payment", Title: "Pembayaran"},
		{Number: 5, Code: "limitations", Title: "Batasan dan Pengecualian"},
	}
	assert.Equal(t, expectedSections, profile.Sections)
}

func TestResolveWhiteLabelProfile_LicenseOnlyLayout(t *testing.T) {
	input := validPonpesWhiteLabelInput()
	input.MaintenanceItems = nil
	input.HostingItems = nil
	input.ModuleGroups = nil
	input.TimelineRows = nil
	input.RightsStatements = nil
	input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
		{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
	}

	profile, err := service.ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	assert.False(t, profile.ShowMaintenanceHosting)
	assert.False(t, profile.ShowScopeTimeline)
	assert.False(t, profile.ShowRights)

	expectedSections := []domain.DocumentSection{
		{Number: 1, Code: "white_label_cost", Title: "Biaya White Label"},
		{Number: 2, Code: "payment", Title: "Pembayaran"},
		{Number: 3, Code: "limitations", Title: "Batasan dan Pengecualian"},
	}
	assert.Equal(t, expectedSections, profile.Sections)
}

func TestResolveWhiteLabelProfile_MaintenanceOnlyLayout(t *testing.T) {
	input := validPonpesWhiteLabelInput()
	input.ModuleGroups = nil
	input.TimelineRows = nil

	profile, err := service.ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	assert.True(t, profile.ShowMaintenanceHosting)
	assert.False(t, profile.ShowScopeTimeline)

	expectedSections := []domain.DocumentSection{
		{Number: 1, Code: "white_label_cost", Title: "Biaya White Label"},
		{Number: 2, Code: "maintenance_hosting", Title: "Biaya Maintenance & Hosting"},
		{Number: 3, Code: "payment", Title: "Pembayaran"},
		{Number: 4, Code: "limitations", Title: "Batasan dan Pengecualian"},
	}
	assert.Equal(t, expectedSections, profile.Sections)
}

func TestResolveWhiteLabelProfile_ScopeOnlyLayout(t *testing.T) {
	input := validPonpesWhiteLabelInput()
	input.MaintenanceItems = nil
	input.HostingItems = nil
	input.PaymentSchedule = []domain.WhiteLabelPaymentRowInput{
		{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
	}

	profile, err := service.ResolveWhiteLabelProfile(input)
	require.NoError(t, err)

	assert.False(t, profile.ShowMaintenanceHosting)
	assert.True(t, profile.ShowScopeTimeline)

	expectedSections := []domain.DocumentSection{
		{Number: 1, Code: "white_label_cost", Title: "Biaya White Label"},
		{Number: 2, Code: "scope_timeline", Title: "Lingkup Pekerjaan & Timeline"},
		{Number: 3, Code: "payment", Title: "Pembayaran"},
		{Number: 4, Code: "limitations", Title: "Batasan dan Pengecualian"},
	}
	assert.Equal(t, expectedSections, profile.Sections)
}

func TestResolveWhiteLabelProfile_RightsVisibility(t *testing.T) {
	t.Run("rights empty", func(t *testing.T) {
		input := validPonpesWhiteLabelInput()
		input.RightsStatements = nil

		profile, err := service.ResolveWhiteLabelProfile(input)
		require.NoError(t, err)
		assert.False(t, profile.ShowRights)
	})

	t.Run("rights populated", func(t *testing.T) {
		input := validPonpesWhiteLabelInput()
		input.RightsStatements = []string{"Aplikasi adalah milik Venturo.", "Klien berhak melakukan re-branding."}

		profile, err := service.ResolveWhiteLabelProfile(input)
		require.NoError(t, err)
		assert.True(t, profile.ShowRights)
	})
}

func TestResolveWhiteLabelProfile_TaxTreatments(t *testing.T) {
	tests := []struct {
		name         string
		taxTreatment domain.WhiteLabelTaxTreatment
		expectedText string
		expectErr    bool
	}{
		{
			name:         "not_stated",
			taxTreatment: domain.WhiteLabelTaxNotStated,
			expectedText: "",
		},
		{
			name:         "exclusive_ppn",
			taxTreatment: domain.WhiteLabelTaxExclusive,
			expectedText: "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.",
		},
		{
			name:         "inclusive_ppn",
			taxTreatment: domain.WhiteLabelTaxInclusive,
			expectedText: "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini.",
		},
		{
			name:         "unsupported",
			taxTreatment: domain.WhiteLabelTaxTreatment("custom_ppn"),
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validPonpesWhiteLabelInput()
			input.TaxTreatment = tt.taxTreatment

			profile, err := service.ResolveWhiteLabelProfile(input)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedText, profile.TaxClauseText)
				assert.Equal(t, tt.taxTreatment, profile.TaxTreatment)
			}
		})
	}
}

func TestResolveWhiteLabelProfile_InvalidInput(t *testing.T) {
	input := validPonpesWhiteLabelInput()
	input.ProductName = ""

	_, err := service.ResolveWhiteLabelProfile(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "product_name")
}
