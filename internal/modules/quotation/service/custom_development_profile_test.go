package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func baseProfileInput() domain.CustomDevelopmentInput {
	return domain.CustomDevelopmentInput{
		SystemName:           "Sistem HRIS Terpadu",
		QuotationDate:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		DevelopmentTableMode: domain.CustomDevelopmentMonthlyPeriod,
		TaxTreatment:         domain.CustomDevelopmentTaxExclusive,
		DevelopmentGroups: []domain.CustomDevelopmentCostGroupInput{
			{
				Title: "Modul Payroll & Kehadiran",
				Items: []domain.CustomDevelopmentCostItemInput{
					{
						Name:            "Backend Engineer",
						DisplayRate:     ptrInt64(15_000_000),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64(15_000_000),
					},
				},
			},
		},
		DiscountLabel:   "DISCOUNT",
		DiscountAmount:  1_000_000,
		ShowDiscountRow: true,
		ModuleGroups: []domain.CustomDevelopmentModuleGroupInput{
			{
				Name:     "Modul Presensi",
				Features: []string{"GPS Geofencing", "Face Recognition"},
			},
		},
		TimelineRows: []domain.CustomDevelopmentTimelineRowInput{
			{
				Description: "Fase 1: Backend",
				Timeline:    "1 Bulan",
			},
		},
		PaymentSchedule: []domain.CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Kickoff Proyek",
				Terms:            "DP 100%",
				InvoiceDateLabel: "Saat tanda tangan kontrak",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          14_000_000,
			},
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func TestResolveCustomDevelopmentProfile_SectionMatrix(t *testing.T) {
	t.Run("without monthly, without rights produces 4 sections", func(t *testing.T) {
		in := baseProfileInput()
		in.MonthlyItems = nil
		in.MonthlyNotes = nil
		in.RightsStatements = nil

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.False(t, profile.ShowMonthly)
		assert.False(t, profile.ShowRights)

		expectedSections := []domain.DocumentSection{
			{Number: 1, Code: "development", Title: "Biaya Development"},
			{Number: 2, Code: "scope", Title: "Lingkup Pekerjaan & Timeline"},
			{Number: 3, Code: "payment", Title: "Pembayaran"},
			{Number: 4, Code: "limitations", Title: "Batasan dan Pengecualian"},
		}
		assert.Equal(t, expectedSections, profile.Sections)

		for i, s := range profile.Sections {
			assert.Equal(t, i+1, s.Number)
		}
	})

	t.Run("with monthly, without rights produces 5 sections", func(t *testing.T) {
		in := baseProfileInput()
		in.MonthlyItems = []domain.CustomDevelopmentMonthlyItemInput{
			{Name: "Cloud Server", UnitRate: 1_000_000, Quantity: 1},
		}
		in.MonthlyNotes = []string{"Biaya dibayar per bulan"}
		in.RightsStatements = nil

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.True(t, profile.ShowMonthly)
		assert.False(t, profile.ShowRights)

		expectedSections := []domain.DocumentSection{
			{Number: 1, Code: "development", Title: "Biaya Development"},
			{Number: 2, Code: "monthly", Title: "Biaya Monthly"},
			{Number: 3, Code: "scope", Title: "Lingkup Pekerjaan & Timeline"},
			{Number: 4, Code: "payment", Title: "Pembayaran"},
			{Number: 5, Code: "limitations", Title: "Batasan dan Pengecualian"},
		}
		assert.Equal(t, expectedSections, profile.Sections)

		for i, s := range profile.Sections {
			assert.Equal(t, i+1, s.Number)
		}
	})

	t.Run("without monthly, with rights produces 5 sections", func(t *testing.T) {
		in := baseProfileInput()
		in.MonthlyItems = nil
		in.MonthlyNotes = nil
		in.RightsStatements = []string{"Source code diserahkan kepada klien."}

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.False(t, profile.ShowMonthly)
		assert.True(t, profile.ShowRights)

		expectedSections := []domain.DocumentSection{
			{Number: 1, Code: "development", Title: "Biaya Development"},
			{Number: 2, Code: "scope", Title: "Lingkup Pekerjaan & Timeline"},
			{Number: 3, Code: "payment", Title: "Pembayaran"},
			{Number: 4, Code: "rights", Title: "Hak Para Pihak"},
			{Number: 5, Code: "limitations", Title: "Batasan dan Pengecualian"},
		}
		assert.Equal(t, expectedSections, profile.Sections)

		for i, s := range profile.Sections {
			assert.Equal(t, i+1, s.Number)
		}
	})

	t.Run("with monthly, with rights produces 6 sections", func(t *testing.T) {
		in := baseProfileInput()
		in.MonthlyItems = []domain.CustomDevelopmentMonthlyItemInput{
			{Name: "Cloud Hosting", UnitRate: 1_200_000, Quantity: 1},
		}
		in.RightsStatements = []string{"Hak cipta dipegang klien setelah pelunasan."}

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.True(t, profile.ShowMonthly)
		assert.True(t, profile.ShowRights)

		expectedSections := []domain.DocumentSection{
			{Number: 1, Code: "development", Title: "Biaya Development"},
			{Number: 2, Code: "monthly", Title: "Biaya Monthly"},
			{Number: 3, Code: "scope", Title: "Lingkup Pekerjaan & Timeline"},
			{Number: 4, Code: "payment", Title: "Pembayaran"},
			{Number: 5, Code: "rights", Title: "Hak Para Pihak"},
			{Number: 6, Code: "limitations", Title: "Batasan dan Pengecualian"},
		}
		assert.Equal(t, expectedSections, profile.Sections)

		for i, s := range profile.Sections {
			assert.Equal(t, i+1, s.Number)
		}
	})
}

func TestResolveCustomDevelopmentProfile_TableModeHeaders(t *testing.T) {
	t.Run("monthly_period table mode headers", func(t *testing.T) {
		in := baseProfileInput()
		in.DevelopmentTableMode = domain.CustomDevelopmentMonthlyPeriod

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.Equal(t, domain.CustomDevelopmentMonthlyPeriod, profile.TableMode)
		assert.Equal(t, "Rate (Monthly)", profile.RateHeader)
		assert.Equal(t, "Period (Month)", profile.PeriodHeader)
		assert.True(t, profile.ShowRateColumn)
	})

	t.Run("daily_period table mode headers", func(t *testing.T) {
		in := baseProfileInput()
		in.DevelopmentTableMode = domain.CustomDevelopmentDailyPeriod

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.Equal(t, domain.CustomDevelopmentDailyPeriod, profile.TableMode)
		assert.Equal(t, "Rate (Monthly)", profile.RateHeader)
		assert.Equal(t, "Period (Day)", profile.PeriodHeader)
		assert.True(t, profile.ShowRateColumn)
	})

	t.Run("fixed_days table mode headers", func(t *testing.T) {
		in := baseProfileInput()
		in.DevelopmentTableMode = domain.CustomDevelopmentFixedDays
		in.DevelopmentGroups[0].Items[0].DisplayRate = nil

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.Equal(t, domain.CustomDevelopmentFixedDays, profile.TableMode)
		assert.Equal(t, "", profile.RateHeader)
		assert.Equal(t, "Period (Days)", profile.PeriodHeader)
		assert.False(t, profile.ShowRateColumn)
	})
}

func TestResolveCustomDevelopmentProfile_TaxTreatment(t *testing.T) {
	t.Run("exclusive_ppn tax text", func(t *testing.T) {
		in := baseProfileInput()
		in.TaxTreatment = domain.CustomDevelopmentTaxExclusive

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.Equal(t, "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.", profile.TaxClauseText)
	})

	t.Run("inclusive_ppn tax text", func(t *testing.T) {
		in := baseProfileInput()
		in.TaxTreatment = domain.CustomDevelopmentTaxInclusive

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.Equal(t, "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini.", profile.TaxClauseText)
	})

	t.Run("not_stated tax text", func(t *testing.T) {
		in := baseProfileInput()
		in.TaxTreatment = domain.CustomDevelopmentTaxNotStated

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.Equal(t, "", profile.TaxClauseText)
	})
}

func TestResolveCustomDevelopmentProfile_ShowDiscountRow(t *testing.T) {
	t.Run("show discount row true", func(t *testing.T) {
		in := baseProfileInput()
		in.DiscountAmount = 500_000
		in.ShowDiscountRow = true

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.True(t, profile.ShowDiscountRow)
	})

	t.Run("show discount row false when zero discount", func(t *testing.T) {
		in := baseProfileInput()
		in.DiscountAmount = 0
		in.ShowDiscountRow = false

		profile, err := service.ResolveCustomDevelopmentProfile(in)
		require.NoError(t, err)

		assert.False(t, profile.ShowDiscountRow)
	})
}

func TestResolveCustomDevelopmentProfile_InvalidInput(t *testing.T) {
	in := baseProfileInput()
	in.SystemName = "" // invalid

	_, err := service.ResolveCustomDevelopmentProfile(in)
	require.Error(t, err)
}

func TestNormalizeCustomDevelopmentModules(t *testing.T) {
	t.Run("trims names and features preserving order and casing", func(t *testing.T) {
		input := []domain.CustomDevelopmentModuleGroupInput{
			{
				Name: "  Modul Transaksi & POS  ",
				Features: []string{
					"  Integrasi Payment Gateway (QRIS & VA)  ",
					"Rekonsiliasi Kas Harian   ",
				},
			},
			{
				Name: "Modul Inventori",
				Features: []string{
					"  Manajemen Stok Multi-Gudang",
				},
			},
		}

		got := service.NormalizeCustomDevelopmentModules(input)
		require.Len(t, got, 2)

		assert.Equal(t, "Modul Transaksi & POS", got[0].Name)
		assert.Equal(t, []string{
			"Integrasi Payment Gateway (QRIS & VA)",
			"Rekonsiliasi Kas Harian",
		}, got[0].Features)

		assert.Equal(t, "Modul Inventori", got[1].Name)
		assert.Equal(t, []string{
			"Manajemen Stok Multi-Gudang",
		}, got[1].Features)
	})

	t.Run("nil or empty input returns non-nil empty slice", func(t *testing.T) {
		got := service.NormalizeCustomDevelopmentModules(nil)
		assert.NotNil(t, got)
		assert.Empty(t, got)

		gotEmpty := service.NormalizeCustomDevelopmentModules([]domain.CustomDevelopmentModuleGroupInput{})
		assert.NotNil(t, gotEmpty)
		assert.Empty(t, gotEmpty)
	})
}

func TestNormalizeCustomDevelopmentTimeline(t *testing.T) {
	t.Run("trims description and timeline preserving order and casing", func(t *testing.T) {
		input := []domain.CustomDevelopmentTimelineRowInput{
			{
				Description: "  Fase 1: Backend Architecture & DB  ",
				Timeline:    "  4 Minggu  ",
			},
			{
				Description: "Fase 2: Frontend & Mobile UI  ",
				Timeline:    "3 Minggu Kerja",
			},
		}

		got := service.NormalizeCustomDevelopmentTimeline(input)
		require.Len(t, got, 2)

		assert.Equal(t, "Fase 1: Backend Architecture & DB", got[0].Description)
		assert.Equal(t, "4 Minggu", got[0].Timeline)

		assert.Equal(t, "Fase 2: Frontend & Mobile UI", got[1].Description)
		assert.Equal(t, "3 Minggu Kerja", got[1].Timeline)
	})

	t.Run("nil or empty input returns non-nil empty slice", func(t *testing.T) {
		got := service.NormalizeCustomDevelopmentTimeline(nil)
		assert.NotNil(t, got)
		assert.Empty(t, got)

		gotEmpty := service.NormalizeCustomDevelopmentTimeline([]domain.CustomDevelopmentTimelineRowInput{})
		assert.NotNil(t, gotEmpty)
		assert.Empty(t, gotEmpty)
	})
}
