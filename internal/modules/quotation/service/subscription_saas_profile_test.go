package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validSubscriptionDevelopmentInput() domain.SubscriptionSaaSInput {
	return domain.SubscriptionSaaSInput{
		ProductName:       "Subscription AI Percetakan",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      domain.SubscriptionBillingQuarterly,
		TaxTreatment:      domain.SubscriptionTaxNotStated,
		InitialTermMonths: 12,
		AutoRenewal:       true,
		RightsClauseCode:  domain.RightsClauseVenturoSoftwareOwnershipV1,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Subscription 12 bulan", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_250_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Agustus-Oktober 2026", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Nominal: 3_750_000},
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

func validDevelopmentMaintenanceInput() domain.SubscriptionSaaSInput {
	return domain.SubscriptionSaaSInput{
		ProductName:       "Annual SaaS Officia",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutDevelopmentMaintenance,
		BillingCycle:      domain.SubscriptionBillingYearly,
		TaxTreatment:      domain.SubscriptionTaxExclusive,
		InitialTermMonths: 24,
		AutoRenewal:       false,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Tahun 1 - Development", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_000_000},
			{Label: "Tahun 2 - Maintenance", StartMonthOffset: 12, DurationMonths: 12, MonthlyRate: 1_500_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Tahun 1", StartMonthOffset: 0, CoveredMonths: 12, InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Nominal: 12_000_000},
			{WorkPeriod: "Tahun 2", StartMonthOffset: 12, CoveredMonths: 12, InvoiceDateLabel: "Awal tahun kedua", DueDateLabel: "H+14", Nominal: 18_000_000},
		},
		FeatureGroups: []domain.SubscriptionFeatureGroupInput{
			{Name: "Core Modules", Features: []string{"User Management", "Audit Log"}},
		},
		IsPartnership: false,
	}
}

func TestResolveSubscriptionSaaSProfile(t *testing.T) {
	t.Run("subscription_development with ownership, auto-renewal, and partnership", func(t *testing.T) {
		input := validSubscriptionDevelopmentInput()
		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)

		assert.Equal(t, SubscriptionLegalCatalogVersion, profile.LegalCatalogVersion)
		assert.Equal(t, domain.SubscriptionLayoutSubscriptionDevelopment, profile.SectionLayout)
		assert.Equal(t, domain.SubscriptionBillingQuarterly, profile.BillingCycle)
		assert.Equal(t, domain.SubscriptionTaxNotStated, profile.TaxTreatment)
		assert.Empty(t, profile.TaxClauseText)

		assert.Equal(t, domain.RightsClauseVenturoSoftwareOwnershipV1, profile.RightsClauseCode)
		assert.Equal(t, "Aplikasi dan source code merupakan milik Venturo.", profile.RightsClauseText)

		assert.Equal(t, "Masa berlangganan awal adalah 12 bulan dan diperpanjang otomatis untuk periode yang sama kecuali disepakati lain secara tertulis.", profile.AutoRenewalText)

		expectedIncomeSharing := "Pendapatan dari Penjualan dari referensi mitra dibagi sebesar 50% untuk Venturo dan 50% untuk Mitra. Pembayaran diselesaikan paling lambat 30 hari kerja. Diskon tidak dapat diberikan secara sepihak."
		assert.Equal(t, expectedIncomeSharing, profile.IncomeSharingText)
	})

	t.Run("income sharing without unilateral discount prohibition and non 50:50 share", func(t *testing.T) {
		input := validSubscriptionDevelopmentInput()
		input.IncomeSharing.VenturoShareBPS = 6_000
		input.IncomeSharing.PartnerShareBPS = 4_000
		input.IncomeSharing.EligibleRevenueDescription = "Semua transaksi via platform"
		input.IncomeSharing.SettlementBusinessDays = 14
		input.IncomeSharing.ProhibitUnilateralDiscount = false

		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)

		expected := "Pendapatan dari Semua transaksi via platform dibagi sebesar 60% untuk Venturo dan 40% untuk Mitra. Pembayaran diselesaikan paling lambat 14 hari kerja."
		assert.Equal(t, expected, profile.IncomeSharingText)
	})

	t.Run("tax clauses", func(t *testing.T) {
		input := validSubscriptionDevelopmentInput()

		input.TaxTreatment = domain.SubscriptionTaxNotStated
		p1, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)
		assert.Empty(t, p1.TaxClauseText)

		input.TaxTreatment = domain.SubscriptionTaxExclusive
		p2, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)
		assert.Equal(t, "Harga belum termasuk PPN.", p2.TaxClauseText)

		input.TaxTreatment = domain.SubscriptionTaxInclusive
		p3, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)
		assert.Equal(t, "Harga sudah termasuk PPN.", p3.TaxClauseText)
	})

	t.Run("non-partnership has empty income sharing text", func(t *testing.T) {
		input := validDevelopmentMaintenanceInput()
		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)
		assert.Empty(t, profile.IncomeSharingText)
	})

	t.Run("no auto-renewal has empty auto renewal text", func(t *testing.T) {
		input := validDevelopmentMaintenanceInput()
		input.AutoRenewal = false
		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)
		assert.Empty(t, profile.AutoRenewalText)
	})

	t.Run("empty rights clause produces empty rights clause text", func(t *testing.T) {
		input := validDevelopmentMaintenanceInput()
		input.RightsClauseCode = ""
		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)
		assert.Empty(t, profile.RightsClauseCode)
		assert.Empty(t, profile.RightsClauseText)
	})

	t.Run("invalid input fails validation", func(t *testing.T) {
		input := validSubscriptionDevelopmentInput()
		input.ProductName = ""
		_, err := ResolveSubscriptionSaaSProfile(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "product_name: is required")
	})
}

func TestBuildSubscriptionSectionPlan(t *testing.T) {
	t.Run("subscription_development + ownership + partnership", func(t *testing.T) {
		input := validSubscriptionDevelopmentInput()
		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)

		sections := BuildSubscriptionSectionPlan(profile)
		expectedTitles := []string{
			"Biaya Subscription dan Development Aplikasi",
			"Pembayaran",
			"Hak Para Pihak",
			"Batasan dan Pengecualian",
		}
		assert.Equal(t, expectedTitles, sectionTitles(sections))

		for i, s := range sections {
			assert.Equal(t, i+1, s.Number)
			assert.NotEmpty(t, s.Code)
		}
	})

	t.Run("development_maintenance tanpa rights/partnership", func(t *testing.T) {
		input := validDevelopmentMaintenanceInput()
		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)

		sections := BuildSubscriptionSectionPlan(profile)
		expectedTitles := []string{
			"Biaya Development & Maintenance",
			"List Features",
			"Pembayaran",
			"Batasan dan Pengecualian",
		}
		assert.Equal(t, expectedTitles, sectionTitles(sections))

		for i, s := range sections {
			assert.Equal(t, i+1, s.Number)
			assert.NotEmpty(t, s.Code)
		}
	})

	t.Run("subscription_development without rights or partnership", func(t *testing.T) {
		input := validSubscriptionDevelopmentInput()
		input.RightsClauseCode = ""
		input.IsPartnership = false
		input.IncomeSharing = nil

		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)

		sections := BuildSubscriptionSectionPlan(profile)
		expectedTitles := []string{
			"Biaya Subscription dan Development Aplikasi",
			"Pembayaran",
			"Batasan dan Pengecualian",
		}
		assert.Equal(t, expectedTitles, sectionTitles(sections))
	})

	t.Run("development_maintenance with rights clause and partnership", func(t *testing.T) {
		input := validDevelopmentMaintenanceInput()
		input.RightsClauseCode = domain.RightsClauseVenturoSoftwareOwnershipV1
		input.IsPartnership = true
		input.IncomeSharing = &domain.IncomeSharingInput{
			VenturoShareBPS:            7_000,
			PartnerShareBPS:            3_000,
			EligibleRevenueDescription: "SaaS revenue",
			SettlementBusinessDays:     15,
		}

		profile, err := ResolveSubscriptionSaaSProfile(input)
		require.NoError(t, err)

		sections := BuildSubscriptionSectionPlan(profile)
		expectedTitles := []string{
			"Biaya Development & Maintenance",
			"List Features",
			"Pembayaran",
			"Hak Para Pihak",
			"Batasan dan Pengecualian",
		}
		assert.Equal(t, expectedTitles, sectionTitles(sections))
	})
}
