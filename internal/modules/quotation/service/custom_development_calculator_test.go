package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func kopiStudio24Input() domain.CustomDevelopmentInput {
	return domain.CustomDevelopmentInput{
		SystemName:           "Sistem Kopi Studio24",
		QuotationDate:        time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		DevelopmentTableMode: domain.CustomDevelopmentMonthlyPeriod,
		TaxTreatment:         domain.CustomDevelopmentTaxExclusive,
		DevelopmentGroups: []domain.CustomDevelopmentCostGroupInput{
			{
				Title: "Fase 1: Backend & Core Engine",
				Items: []domain.CustomDevelopmentCostItemInput{
					{
						Name:            "Senior Backend Developer",
						DisplayRate:     ptrInt64(15_000_000),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64(15_000_000),
					},
					{
						Name:            "Frontend Developer",
						DisplayRate:     ptrInt64(5_062_500),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64(5_062_500),
					},
				},
			},
		},
		DiscountLabel:   "DISCOUNT",
		DiscountAmount:  2_062_500,
		ShowDiscountRow: true,
		MonthlyItems: []domain.CustomDevelopmentMonthlyItemInput{
			{
				Name:          "Venturo Cloud Mini",
				Specification: "Cloud Server Standard",
				UnitRate:      800_000,
				Quantity:      1,
			},
			{
				Name:          "Support",
				Specification: "4 Jam Maintenance Support",
				UnitRate:      93_000,
				Quantity:      4,
			},
			{
				Name:          "WhatsApp Connect",
				Specification: "Integrasi Notifikasi WA",
				UnitRate:      100_000,
				Quantity:      1,
			},
		},
		MonthlyNotes: []string{
			"Biaya API Gemini ditandai out-of-pocket.",
		},
		ModuleGroups: []domain.CustomDevelopmentModuleGroupInput{
			{
				Name: "Modul Transaksi & Kasir",
				Features: []string{
					"Pencatatan order dan invoice",
					"Integrasi payment gateway",
				},
			},
		},
		TimelineRows: []domain.CustomDevelopmentTimelineRowInput{
			{
				Description: "Fase 1: Pengembangan dan Testing",
				Timeline:    "4 Minggu",
			},
		},
		PaymentSchedule: []domain.CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Down Payment 50%",
				Terms:            "Termin 1",
				InvoiceDateLabel: "Saat tanda tangan kontrak",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          9_000_000,
			},
			{
				WorkPeriod:       "Pelunasan 50%",
				Terms:            "Termin 2",
				InvoiceDateLabel: "Setelah serah terima",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          9_000_000,
			},
			{
				WorkPeriod:       "Bulan 1",
				Terms:            "Layanan Bulanan Cloud & Support",
				InvoiceDateLabel: "Awal bulan berjalan",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentMonthly,
				Nominal:          1_272_000,
			},
		},
		LimitationCodes: []string{
			"out_of_pocket",
			"overtime",
			"third_party",
			"scope_change",
		},
	}
}

func assessmentOnlineInput() domain.CustomDevelopmentInput {
	return domain.CustomDevelopmentInput{
		SystemName:           "Assessment Online - Tes Kognitif",
		QuotationDate:        time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		DevelopmentTableMode: domain.CustomDevelopmentMonthlyPeriod,
		TaxTreatment:         domain.CustomDevelopmentTaxExclusive,
		DevelopmentGroups: []domain.CustomDevelopmentCostGroupInput{
			{
				Title: "Pengembangan Sistem Assessment",
				Items: []domain.CustomDevelopmentCostItemInput{
					{
						Name:            "Developer",
						DisplayRate:     ptrInt64(6_685_000),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64(6_685_000),
					},
					{
						Name:            "Quality Assurance",
						DisplayRate:     ptrInt64(2_005_500),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64(2_005_500),
					},
					{
						Name:            "Hosting & Domain",
						DisplayRate:     ptrInt64(200_000),
						Headcount:       1,
						PeriodLabel:     "12",
						QuotedLineTotal: ptrInt64(2_400_000),
					},
				},
			},
		},
		DiscountLabel:   "DISCOUNT",
		DiscountAmount:  1_090_500,
		ShowDiscountRow: true,
		ModuleGroups: []domain.CustomDevelopmentModuleGroupInput{
			{
				Name: "Modul Tes Kognitif",
				Features: []string{
					"Modul Soal dan Jawaban",
					"Scoring Otomatis",
				},
			},
		},
		TimelineRows: []domain.CustomDevelopmentTimelineRowInput{
			{
				Description: "Pengembangan & UAT",
				Timeline:    "2 Bulan",
			},
		},
		PaymentSchedule: []domain.CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Termin 1 (DP 50%)",
				Terms:            "Down Payment",
				InvoiceDateLabel: "Saat kontrak disepakati",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          5_000_000,
			},
			{
				WorkPeriod:       "Termin 2 (Pelunasan 50%)",
				Terms:            "Pelunasan",
				InvoiceDateLabel: "Setelah UAT selesai",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          5_000_000,
			},
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func ecoflowInput() domain.CustomDevelopmentInput {
	return domain.CustomDevelopmentInput{
		SystemName:           "Company Profile Ecoflow",
		QuotationDate:        time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC),
		DevelopmentTableMode: domain.CustomDevelopmentFixedDays,
		TaxTreatment:         domain.CustomDevelopmentTaxExclusive,
		DevelopmentGroups: []domain.CustomDevelopmentCostGroupInput{
			{
				Title: "Website 1: Landing Page & Katalog",
				Items: []domain.CustomDevelopmentCostItemInput{
					{
						Name:            "Frontend & Backend Developer",
						DisplayRate:     nil,
						Headcount:       1,
						PeriodLabel:     "15 Hari",
						QuotedLineTotal: ptrInt64(15_000_000),
					},
					{
						Name:            "QA Tester (Included)",
						DisplayRate:     nil,
						Headcount:       1,
						PeriodLabel:     "15 Hari",
						QuotedLineTotal: nil, // Included resource, no line total
					},
				},
			},
			{
				Title: "Website 2: Portal Klien & Admin",
				Items: []domain.CustomDevelopmentCostItemInput{
					{
						Name:            "Fullstack Developer",
						DisplayRate:     nil,
						Headcount:       1,
						PeriodLabel:     "10 Hari",
						QuotedLineTotal: ptrInt64(10_000_000),
					},
				},
			},
		},
		DiscountLabel:   "DISCOUNT DEVELOPMENT",
		DiscountAmount:  10_000_000,
		ShowDiscountRow: true,
		ModuleGroups: []domain.CustomDevelopmentModuleGroupInput{
			{
				Name: "Fitur Landing Page",
				Features: []string{
					"Katalog Produk",
					"Kontak dan Form Inquiry",
				},
			},
		},
		TimelineRows: []domain.CustomDevelopmentTimelineRowInput{
			{
				Description: "Pengerjaan Website 1 dan 2",
				Timeline:    "25 Hari Kerja",
			},
		},
		PaymentSchedule: []domain.CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Termin 1 (DP 50%)",
				Terms:            "Down Payment",
				InvoiceDateLabel: "Saat tanda tangan kontrak",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          7_500_000,
			},
			{
				WorkPeriod:       "Termin 2 (Pelunasan 50%)",
				Terms:            "Pelunasan",
				InvoiceDateLabel: "Setelah live deployment",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentProject,
				Nominal:          7_500_000,
			},
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func TestCalculateCustomDevelopment_KopiStudio24(t *testing.T) {
	in := kopiStudio24Input()
	calc, err := service.CalculateCustomDevelopment(in)
	require.NoError(t, err)

	assert.Equal(t, int64(20_062_500), calc.DevelopmentSubtotal)
	assert.Equal(t, int64(2_062_500), calc.DiscountAmount)
	assert.Equal(t, int64(18_000_000), calc.TotalInvestment)
	assert.Equal(t, int64(1_272_000), calc.MonthlyTotal)
	assert.Equal(t, int64(18_000_000), calc.DevelopmentPaymentTotal)
	assert.Equal(t, int32(1), calc.MonthlyPaymentRows)

	require.Len(t, calc.DevelopmentGroups, 1)
	require.Len(t, calc.DevelopmentGroups[0].Items, 2)
	assert.Equal(t, ptrInt64(15_000_000), calc.DevelopmentGroups[0].Items[0].LineTotal)
	assert.Equal(t, ptrInt64(5_062_500), calc.DevelopmentGroups[0].Items[1].LineTotal)

	require.Len(t, calc.MonthlyLines, 3)
	assert.Equal(t, int64(800_000), calc.MonthlyLines[0].LineTotal)
	assert.Equal(t, int64(372_000), calc.MonthlyLines[1].LineTotal)
	assert.Equal(t, int64(100_000), calc.MonthlyLines[2].LineTotal)
}

func TestCalculateCustomDevelopment_AssessmentOnline(t *testing.T) {
	in := assessmentOnlineInput()
	calc, err := service.CalculateCustomDevelopment(in)
	require.NoError(t, err)

	assert.Equal(t, int64(11_090_500), calc.DevelopmentSubtotal)
	assert.Equal(t, int64(1_090_500), calc.DiscountAmount)
	assert.Equal(t, int64(10_000_000), calc.TotalInvestment)
	assert.Equal(t, int64(0), calc.MonthlyTotal)
	assert.Equal(t, int64(10_000_000), calc.DevelopmentPaymentTotal)
	assert.Equal(t, int32(0), calc.MonthlyPaymentRows)
	assert.Empty(t, calc.MonthlyLines)
}

func TestCalculateCustomDevelopment_Ecoflow(t *testing.T) {
	in := ecoflowInput()
	calc, err := service.CalculateCustomDevelopment(in)
	require.NoError(t, err)

	assert.Equal(t, int64(25_000_000), calc.DevelopmentSubtotal)
	assert.Equal(t, int64(10_000_000), calc.DiscountAmount)
	assert.Equal(t, int64(15_000_000), calc.TotalInvestment)
	assert.Equal(t, int64(0), calc.MonthlyTotal)
	assert.Equal(t, int64(15_000_000), calc.DevelopmentPaymentTotal)
	assert.Equal(t, int32(0), calc.MonthlyPaymentRows)
	assert.Empty(t, calc.MonthlyLines)

	// Verify included QA nil totals: sel total nil, tidak ikut subtotal
	require.Len(t, calc.DevelopmentGroups, 2)
	require.Len(t, calc.DevelopmentGroups[0].Items, 2)
	assert.Equal(t, ptrInt64(15_000_000), calc.DevelopmentGroups[0].Items[0].LineTotal)
	assert.Nil(t, calc.DevelopmentGroups[0].Items[1].LineTotal)
	require.Len(t, calc.DevelopmentGroups[1].Items, 1)
	assert.Equal(t, ptrInt64(10_000_000), calc.DevelopmentGroups[1].Items[0].LineTotal)
}

func TestCalculateCustomDevelopment_IncludedQANilTotals(t *testing.T) {
	in := ecoflowInput()
	calc, err := service.CalculateCustomDevelopment(in)
	require.NoError(t, err)

	// Explicit check on item with nil QuotedLineTotal
	qaItem := calc.DevelopmentGroups[0].Items[1]
	assert.Equal(t, "QA Tester (Included)", qaItem.Name)
	assert.Nil(t, qaItem.LineTotal)
	assert.Nil(t, qaItem.QuotedLineTotal)
	// Subtotal is strictly 15jt + 10jt = 25jt, nil does not add anything
	assert.Equal(t, int64(25_000_000), calc.DevelopmentSubtotal)
}

func TestCalculateCustomDevelopment_Overflow(t *testing.T) {
	t.Run("subtotal overflow exceeding MaxCustomDevelopmentMoney", func(t *testing.T) {
		in := assessmentOnlineInput()
		largeVal := domain.MaxCustomDevelopmentMoney
		in.DevelopmentGroups[0].Items = append(in.DevelopmentGroups[0].Items, domain.CustomDevelopmentCostItemInput{
			Name:            "Extra Dev",
			Headcount:       1,
			PeriodLabel:     "1",
			QuotedLineTotal: &largeVal,
		})
		_, err := service.CalculateCustomDevelopment(in)
		require.Error(t, err)
	})

	t.Run("monthly line total multiplication overflow", func(t *testing.T) {
		in := kopiStudio24Input()
		in.MonthlyItems[0].UnitRate = domain.MaxCustomDevelopmentMoney
		in.MonthlyItems[0].Quantity = 2
		_, err := service.CalculateCustomDevelopment(in)
		require.Error(t, err)
	})

	t.Run("payment nominal addition overflow", func(t *testing.T) {
		in := kopiStudio24Input()
		in.PaymentSchedule[0].Nominal = domain.MaxCustomDevelopmentMoney
		in.PaymentSchedule[1].Nominal = 1
		_, err := service.CalculateCustomDevelopment(in)
		require.Error(t, err)
	})
}

func TestCalculateCustomDevelopment_DiscountOverflow(t *testing.T) {
	in := assessmentOnlineInput()
	// Subtotal is 11_090_500, set discount higher than subtotal
	in.DiscountAmount = 12_000_000
	_, err := service.CalculateCustomDevelopment(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discount_amount")
}

func TestCalculateCustomDevelopment_DevelopmentPaymentMismatch(t *testing.T) {
	in := kopiStudio24Input()
	// TotalInvestment is 18_000_000, modify project payment so sum != 18_000_000
	in.PaymentSchedule[0].Nominal = 8_000_000 // now total is 8M + 9M = 17M != 18M
	_, err := service.CalculateCustomDevelopment(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "development payment total")
}

func TestCalculateCustomDevelopment_MissingMonthlyTemplate(t *testing.T) {
	in := kopiStudio24Input()
	// MonthlyTotal is 1_272_000, remove monthly payment row
	in.PaymentSchedule = in.PaymentSchedule[:2] // keep only the 2 project payments
	_, err := service.CalculateCustomDevelopment(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "monthly recurring payment")
}

func TestCalculateCustomDevelopment_UnexpectedMonthlyTemplate(t *testing.T) {
	in := assessmentOnlineInput()
	// MonthlyTotal is 0, but add a monthly recurring payment row
	in.PaymentSchedule = append(in.PaymentSchedule, domain.CustomDevelopmentPaymentRowInput{
		WorkPeriod:       "Bulan 1",
		Terms:            "Tagihan Server",
		InvoiceDateLabel: "Awal bulan",
		DueDateLabel:     "H+7",
		Component:        domain.CustomDevelopmentPaymentMonthly,
		Nominal:          500_000,
	})
	_, err := service.CalculateCustomDevelopment(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected monthly recurring payment")
}

func TestCalculateCustomDevelopment_MonthlyNominalMismatch(t *testing.T) {
	in := kopiStudio24Input()
	// MonthlyTotal is 1_272_000, set monthly payment nominal to 1_000_000
	in.PaymentSchedule[2].Nominal = 1_000_000
	_, err := service.CalculateCustomDevelopment(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "monthly payment nominal")
}
