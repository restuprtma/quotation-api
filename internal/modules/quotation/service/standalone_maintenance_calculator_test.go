package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func nexRetainerInputForCalculator() domain.StandaloneMaintenanceInput {
	return domain.StandaloneMaintenanceInput{
		SystemName:    "Aplikasi Nex Health",
		QuotationDate: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		Mode:          domain.StandaloneMaintenanceRetainer,
		TaxTreatment:  domain.StandaloneTaxExclusive,
		RetainerItems: []domain.MaintenanceRetainerItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 16_050_000, FTEQuarters: 4, PeriodMonths: 1},
			{Name: "Senior Quality Assurance", MonthlyRate: 14_070_000, FTEQuarters: 2, PeriodMonths: 1},
		},
		DefectGroups: []domain.MaintenanceDefectGroupInput{{
			Title: "1. Detail Pasien",
			Items: []domain.MaintenanceDefectItemInput{{
				ItemNumber:  "1.1",
				Priority:    domain.StandalonePriorityP0,
				Function:    "Melihat riwayat medis pasien di luar sesi konsultasi.",
				ScreenFlow:  "Pasien > Data Pasien dan Riwayat > Lihat Detail Pasien > Medical History.",
				Existing:    "Hanya mencantumkan catatan registrasi awal.",
				Improvement: "Mencantumkan seluruh riwayat SOAP (Dokter, Terapis, dan Perawat).",
				Reference:   "Ref 1.01",
			}},
		}},
		ShowDiscountRow: true,
		DiscountAmount:  0,
		PaymentSchedule: []domain.MaintenancePaymentRowInput{{
			WorkPeriod:       "13 Juli - 13 Agustus 2026",
			Terms:            "Maintenance",
			InvoiceDateLabel: "13 Agustus 2026",
			DueDateLabel:     "27 Agustus 2026",
			Nominal:          23_085_000,
		}},
	}
}

func liputan6HourlyInputForCalculator() domain.StandaloneMaintenanceInput {
	return domain.StandaloneMaintenanceInput{
		SystemName:    "Portal Berita Liputan6",
		QuotationDate: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		Mode:          domain.StandaloneMaintenanceHourlyTask,
		TaxTreatment:  domain.StandaloneTaxNotStated,
		HourlyCostItems: []domain.MaintenanceHourlyCostItemInput{
			{Name: "Senior Mobile Programmer", DisplayMonthlyRate: 16_050_000, Headcount: 1, HoursHalfUnits: 16, QuotedLineTotal: 730_000},
			{Name: "Senior QA", DisplayMonthlyRate: 12_500_000, Headcount: 1, HoursHalfUnits: 4, QuotedLineTotal: 140_000},
		},
		HourlyTasks: []domain.MaintenanceHourlyTaskInput{
			{Description: "Perbaikan crash login", Platform: "Android", HoursHalfUnits: 16},
			{Description: "Regression testing", Platform: "QA Test", HoursHalfUnits: 4},
		},
		ShowDiscountRow: false,
		DiscountAmount:  0,
		PaymentSchedule: []domain.MaintenancePaymentRowInput{{
			WorkPeriod:       "Support Juli 2026",
			Terms:            "Pelunasan",
			InvoiceDateLabel: "31 Juli 2026",
			DueDateLabel:     "14 Agustus 2026",
			Nominal:          870_000,
		}},
		RightsStatements: []string{"Source code dan modul merupakan hak milik bersama."},
	}
}

// Step 1: Tests for Nex example
func TestStandaloneMaintenanceCalculator_NexRetainer(t *testing.T) {
	input := nexRetainerInputForCalculator()
	calc, err := service.CalculateStandaloneMaintenance(input)
	require.NoError(t, err)

	assert.Equal(t, domain.StandaloneMaintenanceRetainer, calc.Mode)
	require.Len(t, calc.Retainer, 2)
	assert.Equal(t, int64(16_050_000), calc.Retainer[0].LineTotal)
	assert.Equal(t, int64(7_035_000), calc.Retainer[1].LineTotal)
	assert.Equal(t, int64(23_085_000), calc.Subtotal)
	assert.Equal(t, int64(0), calc.DiscountAmount)
	assert.Equal(t, int64(23_085_000), calc.TotalInvestment)
	assert.Equal(t, int64(23_085_000), calc.PaymentTotal)
	assert.Nil(t, calc.Hourly)
}

// Step 2: Tests for Liputan6 hourly
func TestStandaloneMaintenanceCalculator_Liputan6Hourly(t *testing.T) {
	t.Run("valid liputan6 calculation", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)

		assert.Equal(t, domain.StandaloneMaintenanceHourlyTask, calc.Mode)
		require.Len(t, calc.Hourly, 2)
		assert.Equal(t, int64(730_000), calc.Hourly[0].LineTotal)
		assert.Equal(t, int64(140_000), calc.Hourly[1].LineTotal)
		assert.Equal(t, int64(870_000), calc.Subtotal)
		assert.Equal(t, int64(0), calc.DiscountAmount)
		assert.Equal(t, int64(870_000), calc.TotalInvestment)
		assert.Equal(t, int64(870_000), calc.PaymentTotal)
		assert.Nil(t, calc.Retainer)
	})

	t.Run("half-hour fractional hours (1.5h = 3 half units) preserves fractional units without float", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.HourlyCostItems = []domain.MaintenanceHourlyCostItemInput{
			{Name: "Junior Dev", DisplayMonthlyRate: 8_000_000, Headcount: 1, HoursHalfUnits: 3, QuotedLineTotal: 150_000},
		}
		input.HourlyTasks = []domain.MaintenanceHourlyTaskInput{
			{Description: "Quick bugfix", Platform: "Web", HoursHalfUnits: 3},
		}
		input.PaymentSchedule = []domain.MaintenancePaymentRowInput{
			{WorkPeriod: "Period 1", Terms: "Pelunasan", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Nominal: 150_000},
		}

		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)
		require.Len(t, calc.Hourly, 1)
		assert.Equal(t, int32(3), calc.Hourly[0].HoursHalfUnits)
		assert.Equal(t, int64(150_000), calc.Hourly[0].LineTotal)
		assert.Equal(t, int64(150_000), calc.Subtotal)
		assert.Equal(t, int64(150_000), calc.TotalInvestment)
	})

	t.Run("quoted_line_total zero is rejected", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.HourlyCostItems[0].QuotedLineTotal = 0
		input.PaymentSchedule[0].Nominal = 140_000
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quoted_line_total")
	})

	t.Run("quoted_line_total negative is rejected", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.HourlyCostItems[0].QuotedLineTotal = -50_000
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quoted_line_total")
	})

	t.Run("quoted_line_total exceeding limit is rejected", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.HourlyCostItems[0].QuotedLineTotal = domain.MaxStandaloneMaintenanceMoney + 1
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum safe amount")
	})
}

// Step 3: Tests for reconciliation and overflow
func TestStandaloneMaintenanceCalculator_ReconciliationAndOverflow(t *testing.T) {
	t.Run("payment kurang Rp1", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.PaymentSchedule[0].Nominal = 869_999
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule: total 869999 must equal total investment 870000", err.Error())
	})

	t.Run("payment lebih Rp1", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.PaymentSchedule[0].Nominal = 870_001
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Equal(t, "payment_schedule: total 870001 must equal total investment 870000", err.Error())
	})

	t.Run("dua payment yang jumlahnya tepat", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.PaymentSchedule = []domain.MaintenancePaymentRowInput{
			{WorkPeriod: "Termin 1", Terms: "DP", InvoiceDateLabel: "15 Juli 2026", DueDateLabel: "22 Juli 2026", Nominal: 500_000},
			{WorkPeriod: "Termin 2", Terms: "Pelunasan", InvoiceDateLabel: "31 Juli 2026", DueDateLabel: "14 Agustus 2026", Nominal: 370_000},
		}
		calc, err := service.CalculateStandaloneMaintenance(input)
		require.NoError(t, err)
		assert.Equal(t, int64(870_000), calc.PaymentTotal)
		assert.Equal(t, int64(870_000), calc.TotalInvestment)
	})

	t.Run("discount melebihi subtotal", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.DiscountAmount = 870_001
		input.ShowDiscountRow = true
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "discount_amount: cannot exceed subtotal")
	})

	t.Run("retainer line multiplication overflow", func(t *testing.T) {
		input := nexRetainerInputForCalculator()
		input.RetainerItems[0].MonthlyRate = 5_000_000_000_000_000
		input.RetainerItems[0].FTEQuarters = 2
		input.PaymentSchedule[0].Nominal = domain.MaxStandaloneMaintenanceMoney
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line calculation exceeds maximum safe amount")
	})

	t.Run("retainer subtotal addition overflow", func(t *testing.T) {
		input := nexRetainerInputForCalculator()
		input.RetainerItems = []domain.MaintenanceRetainerItemInput{
			{Name: "Item A", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
			{Name: "Item B", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
			{Name: "Item C", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
			{Name: "Item D", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
			{Name: "Item E", MonthlyRate: 8_000_000_000_000_000, FTEQuarters: 1, PeriodMonths: 1},
		}
		input.PaymentSchedule[0].Nominal = domain.MaxStandaloneMaintenanceMoney
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subtotal exceeds maximum safe amount")
	})

	t.Run("hourly subtotal addition overflow", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.HourlyCostItems = []domain.MaintenanceHourlyCostItemInput{
			{Name: "Task 1", DisplayMonthlyRate: 10_000_000, Headcount: 1, HoursHalfUnits: 2, QuotedLineTotal: 5_000_000_000_000_000},
			{Name: "Task 2", DisplayMonthlyRate: 10_000_000, Headcount: 1, HoursHalfUnits: 2, QuotedLineTotal: 5_000_000_000_000_000},
		}
		input.PaymentSchedule[0].Nominal = domain.MaxStandaloneMaintenanceMoney
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subtotal exceeds maximum safe amount")
	})

	t.Run("payment schedule addition overflow", func(t *testing.T) {
		input := liputan6HourlyInputForCalculator()
		input.HourlyCostItems = []domain.MaintenanceHourlyCostItemInput{
			{Name: "Task 1", DisplayMonthlyRate: 10_000_000, Headcount: 1, HoursHalfUnits: 2, QuotedLineTotal: 9_000_000_000_000_000},
		}
		input.PaymentSchedule = []domain.MaintenancePaymentRowInput{
			{WorkPeriod: "Pay 1", Terms: "Termin 1", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Nominal: 5_000_000_000_000_000},
			{WorkPeriod: "Pay 2", Terms: "Termin 2", InvoiceDateLabel: "Now", DueDateLabel: "H+14", Nominal: 5_000_000_000_000_000},
		}
		_, err := service.CalculateStandaloneMaintenance(input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "payment_schedule: total exceeds maximum safe amount")
	})
}

func TestStandaloneMaintenanceCalculator_CheckedMathHelpers(t *testing.T) {
	t.Run("checkedMulStandaloneMaintenance normal", func(t *testing.T) {
		res, err := service.CheckedMulStandaloneMaintenance(100, 20, 5)
		require.NoError(t, err)
		assert.Equal(t, int64(10_000), res)
	})

	t.Run("checkedMulStandaloneMaintenance non-positive", func(t *testing.T) {
		_, err := service.CheckedMulStandaloneMaintenance(100, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money multiplication exceeds limit")

		_, err = service.CheckedMulStandaloneMaintenance(100, -5)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money multiplication exceeds limit")
	})

	t.Run("checkedMulStandaloneMaintenance overflow", func(t *testing.T) {
		_, err := service.CheckedMulStandaloneMaintenance(domain.MaxStandaloneMaintenanceMoney, 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money multiplication exceeds limit")
	})

	t.Run("checkedAddStandaloneMaintenance normal", func(t *testing.T) {
		res, err := service.CheckedAddStandaloneMaintenance(100, 200)
		require.NoError(t, err)
		assert.Equal(t, int64(300), res)
	})

	t.Run("checkedAddStandaloneMaintenance negative", func(t *testing.T) {
		_, err := service.CheckedAddStandaloneMaintenance(-1, 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money addition exceeds limit")

		_, err = service.CheckedAddStandaloneMaintenance(100, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money addition exceeds limit")
	})

	t.Run("checkedAddStandaloneMaintenance overflow", func(t *testing.T) {
		_, err := service.CheckedAddStandaloneMaintenance(domain.MaxStandaloneMaintenanceMoney, 1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "money addition exceeds limit")
	})
}
