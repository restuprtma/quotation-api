package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func governmentHardwareFixtureInput() domain.KioskQueueSystemInput {
	return domain.KioskQueueSystemInput{
		Variant:           domain.KioskQueueGovernmentMPP,
		DocumentNumber:    "LANDA/MPP-Antrian/0726/001",
		SystemName:        "Sistem Antrian Terpadu MPP",
		QuotationDate:     time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC),
		HardwareProvision: domain.HardwareVenturoSupplied,
		TaxTreatment:      domain.KioskTaxInclusivePPN11,
		GovernmentScopePhases: []domain.KioskQueueScopePhaseInput{
			{
				Title: "Fase 1: Persiapan & Desain",
				Deliverables: []string{
					"Dokumen Analisis Kebutuhan",
					"UI/UX Design Mockup",
				},
			},
		},
		ExpertRequirements: []domain.KioskQueueExpertInput{
			{
				Role:                 "Project Manager",
				Headcount:            1,
				DurationLabel:        "3 Bulan",
				MinimumQualification: "S1 Sistem Informasi, pengalaman 5 tahun",
			},
		},
		ServiceGroups: []domain.KioskQueueServiceGroupInput{
			{
				Title: "Requirement, Design & Development",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "UI UX", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 1},
					{Name: "Senior Programmer", MonthlyRate: 16_170_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Quality Assurance", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 3},
				},
			},
			{
				Title: "SIT, UAT & Go-Live Beta Release",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "Senior Programmer", MonthlyRate: 16_170_000, Headcount: 1, PeriodQuarters: 2},
					{Name: "Quality Assurance", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 2},
				},
			},
		},
		ServiceDiscountAmount:  799_100,
		ShowServiceDiscountRow: true,
		HardwareItems: []domain.KioskQueueHardwareItemInput{
			{Name: "Mesin Kiosk Touchscreen 21.5 inch", UnitPrice: 24_500_000, Quantity: 1},
			{Name: "Printer Thermal & Audio Master", UnitPrice: 4_500_000, Quantity: 1},
			{Name: "TV Display 43 Inch", UnitPrice: 3_500_000, Quantity: 3},
			{Name: "Bracket TV", UnitPrice: 300_000, Quantity: 3},
			{Name: "HDMI Switch", UnitPrice: 100_000, Quantity: 1},
			{Name: "Mini PC Core i5", UnitPrice: 6_000_000, Quantity: 1},
		},
		GovernmentPaymentSchedule: []domain.KioskQueueGovernmentPaymentInput{
			{
				Stage:        "Uang Muka 50%",
				BillingBasis: "Kontrak ditandatangani & SPMK terbit",
				Nominal:      51_615_000,
			},
			{
				Stage:        "Pelunasan 50%",
				BillingBasis: "Pekerjaan selesai 100%\nBerita Acara Serah Terima (BAST)",
				Nominal:      48_385_000,
			},
		},
		PaymentDueDays: 14,
		LimitationCodes: []domain.KioskQueueLimitationCode{
			domain.KioskLimitCivilElectrical,
		},
		AdditionalLimitations: []string{
			"Pekerjaan sipil dan perkabelan tidak termasuk penawaran ini.",
		},
	}
}

func governmentServiceOnlyFixtureInput() domain.KioskQueueSystemInput {
	return domain.KioskQueueSystemInput{
		Variant:           domain.KioskQueueGovernmentMPP,
		DocumentNumber:    "LANDA/MPP-Antrian/0726/002",
		SystemName:        "Sistem Antrian Terpadu MPP (Jasa Saja)",
		QuotationDate:     time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC),
		HardwareProvision: domain.HardwareClientSupplied,
		TaxTreatment:      domain.KioskTaxInclusivePPN11,
		GovernmentScopePhases: []domain.KioskQueueScopePhaseInput{
			{
				Title: "Fase 1: Persiapan & Desain",
				Deliverables: []string{
					"Dokumen Analisis Kebutuhan",
					"UI/UX Design Mockup",
				},
			},
		},
		ExpertRequirements: []domain.KioskQueueExpertInput{
			{
				Role:                 "Project Manager",
				Headcount:            1,
				DurationLabel:        "3 Bulan",
				MinimumQualification: "S1 Sistem Informasi, pengalaman 5 tahun",
			},
		},
		ServiceGroups: []domain.KioskQueueServiceGroupInput{
			{
				Title: "Requirement, Design & Development",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "UI UX", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 1},
					{Name: "Senior Programmer", MonthlyRate: 16_170_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Intermediate Web Programmer", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Quality Assurance", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 3},
				},
			},
			{
				Title: "SIT, UAT & Go-Live Beta Release",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "Senior Programmer", MonthlyRate: 16_170_000, Headcount: 1, PeriodQuarters: 2},
					{Name: "Quality Assurance", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 2},
				},
			},
		},
		ServiceDiscountAmount:  2_024_800,
		ShowServiceDiscountRow: true,
		GovernmentPaymentSchedule: []domain.KioskQueueGovernmentPaymentInput{
			{
				Stage:        "Uang Muka 50%",
				BillingBasis: "Kontrak ditandatangani & SPMK terbit",
				Nominal:      31_000_000,
			},
			{
				Stage:        "Pelunasan 50%",
				BillingBasis: "Pekerjaan selesai 100%\nBerita Acara Serah Terima (BAST)",
				Nominal:      31_000_000,
			},
		},
		PaymentDueDays: 14,
		LimitationCodes: []domain.KioskQueueLimitationCode{
			domain.KioskLimitCivilElectrical,
			domain.KioskLimitClientHardware,
			domain.KioskLimitHardwareWarranty,
		},
	}
}

func privateFoodCourtFixtureInput() domain.KioskQueueSystemInput {
	return domain.KioskQueueSystemInput{
		Variant:           domain.KioskQueuePrivateFoodCourt,
		DocumentNumber:    "VEN/Kiosk-FoodCourt/0826/007",
		SystemName:        "Self-Ordering Kiosk Food Court",
		QuotationDate:     time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		HardwareProvision: domain.HardwareClientSupplied,
		TaxTreatment:      domain.KioskTaxExclusivePPN,
		ServiceGroups: []domain.KioskQueueServiceGroupInput{
			{
				Title: "Engineering Kick-off",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "Senior Web Developer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Senior Mobile Developer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "UI/UX Designer", MonthlyRate: 9_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Quality Assurance", MonthlyRate: 9_550_000, Headcount: 1, PeriodQuarters: 4},
				},
			},
			{
				Title: "Requirement & Design",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "System Analyst", MonthlyRate: 13_370_000, Headcount: 1, PeriodQuarters: 4},
				},
			},
			{
				Title: "Development",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "Senior Web Developer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Quality Assurance", MonthlyRate: 9_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Senior Mobile Developer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Frontend Developer", MonthlyRate: 9_550_000, Headcount: 1, PeriodQuarters: 4},
				},
			},
			{
				Title: "SIT, UAT & Go-Live",
				Items: []domain.KioskQueueServiceItemInput{
					{Name: "Senior Web Developer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Senior Mobile Developer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "UI/UX Designer", MonthlyRate: 9_550_000, Headcount: 1, PeriodQuarters: 4},
					{Name: "Quality Assurance", MonthlyRate: 16_170_000, Headcount: 1, PeriodQuarters: 2},
				},
			},
		},
		ServiceDiscountAmount:  42_200_000,
		ShowServiceDiscountRow: true,
		MaintenanceItems: []domain.KioskQueueServiceItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 12},
			{Name: "Senior Mobile Programmer", MonthlyRate: 11_550_000, Headcount: 1, PeriodQuarters: 12},
			{Name: "Quality Assurance", MonthlyRate: 9_550_000, Headcount: 1, PeriodQuarters: 6},
		},
		PrivateTimelineRows: []domain.KioskQueueTimelineRowInput{
			{Description: "Kick Off & Design", Timeline: "Bulan 1"},
			{Description: "Development Core", Timeline: "Bulan 2"},
			{Description: "Integration & Testing", Timeline: "Bulan 3"},
		},
		PrivatePaymentSchedule: []domain.KioskQueuePrivatePaymentInput{
			{WorkPeriod: "Termin 1", Terms: "DP Kick Off", InvoiceDateLabel: "01/08/2026", DueDateLabel: "15/08/2026", Nominal: 6_685_000},
			{WorkPeriod: "Termin 2", Terms: "Desain Selesai", InvoiceDateLabel: "01/09/2026", DueDateLabel: "15/09/2026", Nominal: 6_685_000},
			{WorkPeriod: "Termin 3", Terms: "Development", InvoiceDateLabel: "01/10/2026", DueDateLabel: "15/10/2026", Nominal: 42_200_000},
			{WorkPeriod: "Termin 4", Terms: "Pelunasan UAT", InvoiceDateLabel: "01/11/2026", DueDateLabel: "15/11/2026", Nominal: 40_735_000},
		},
		PaymentDueDays: 0,
		LimitationCodes: []domain.KioskQueueLimitationCode{
			domain.KioskLimitClientHardware,
		},
	}
}

func privateWithoutMaintenanceInput() domain.KioskQueueSystemInput {
	base := privateFoodCourtFixtureInput()
	base.MaintenanceItems = nil
	return base
}

func TestCalculateKioskQueueSystem_GovernmentHardware(t *testing.T) {
	calc, err := service.CalculateKioskQueueSystem(governmentHardwareFixtureInput())
	require.NoError(t, err)
	assert.EqualValues(t, 44_310_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 799_100, calc.ServiceDiscountAmount)
	assert.EqualValues(t, 4_874_100, calc.ServiceTaxAmount)
	assert.EqualValues(t, 48_385_000, calc.ServiceTotal)
	assert.EqualValues(t, 46_500_000, calc.HardwareSubtotal)
	assert.EqualValues(t, 5_115_000, calc.HardwareTaxAmount)
	assert.EqualValues(t, 51_615_000, calc.HardwareTotal)
	assert.EqualValues(t, 100_000_000, calc.PaymentTargetTotal)
	assert.EqualValues(t, 100_000_000, calc.PaymentTotal)
}

func TestCalculateKioskQueueSystem_GovernmentServiceOnly(t *testing.T) {
	calc, err := service.CalculateKioskQueueSystem(governmentServiceOnlyFixtureInput())
	require.NoError(t, err)
	assert.EqualValues(t, 57_680_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 2_024_800, calc.ServiceDiscountAmount)
	assert.EqualValues(t, 6_344_800, calc.ServiceTaxAmount)
	assert.EqualValues(t, 62_000_000, calc.PaymentTargetTotal)
	assert.Zero(t, calc.HardwareTotal)
}

func TestCalculateKioskQueueSystem_PrivateFoodCourt(t *testing.T) {
	calc, err := service.CalculateKioskQueueSystem(privateFoodCourtFixtureInput())
	require.NoError(t, err)
	assert.EqualValues(t, 138_505_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 42_200_000, calc.ServiceDiscountAmount)
	assert.EqualValues(t, 96_305_000, calc.ServiceTotal)
	assert.EqualValues(t, 83_625_000, calc.MaintenanceTotal)
	assert.EqualValues(t, 96_305_000, calc.PaymentTargetTotal)
	assert.EqualValues(t, 96_305_000, calc.PaymentTotal)
}

func TestCalculateKioskQueueSystemRejectsPaymentMismatch(t *testing.T) {
	in := governmentHardwareFixtureInput()
	in.GovernmentPaymentSchedule[0].Nominal = 50_000_000
	_, err := service.CalculateKioskQueueSystem(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payment")
}

func TestCalculateKioskQueueSystemRejectsFractionalRupiah(t *testing.T) {
	in := privateFoodCourtFixtureInput()
	in.ServiceGroups[0].Items[0].MonthlyRate = 11_550_001
	in.ServiceGroups[0].Items[0].Headcount = 1
	in.ServiceGroups[0].Items[0].PeriodQuarters = 1
	_, err := service.CalculateKioskQueueSystem(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integer rupiah")

	govIn := governmentHardwareFixtureInput()
	govIn.ServiceGroups = []domain.KioskQueueServiceGroupInput{
		{
			Title: "Testing",
			Items: []domain.KioskQueueServiceItemInput{
				{Name: "Item", MonthlyRate: 4004, Headcount: 1, PeriodQuarters: 1},
			},
		},
	}
	govIn.HardwareItems = nil
	govIn.HardwareProvision = domain.HardwareClientSupplied
	govIn.LimitationCodes = []domain.KioskQueueLimitationCode{domain.KioskLimitCivilElectrical, domain.KioskLimitClientHardware}
	govIn.ServiceDiscountAmount = 0
	govIn.GovernmentPaymentSchedule = []domain.KioskQueueGovernmentPaymentInput{
		{Stage: "Stage", BillingBasis: "Basis", Nominal: 1111},
	}
	_, err = service.CalculateKioskQueueSystem(govIn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fractional rupiah")
}

func TestCalculateKioskQueueSystemRejectsOverflow(t *testing.T) {
	in := governmentHardwareFixtureInput()
	in.ServiceGroups[0].Items[0].MonthlyRate = domain.KioskQueueMaxMoney
	in.ServiceGroups[0].Items[0].PeriodQuarters = 4
	in.ServiceGroups[0].Items[0].Headcount = 2
	_, err := service.CalculateKioskQueueSystem(in)
	require.Error(t, err)
}

func TestCalculateKioskQueueSystemDoesNotAddMaintenanceToPrivatePaymentTarget(t *testing.T) {
	calc, err := service.CalculateKioskQueueSystem(privateFoodCourtFixtureInput())
	require.NoError(t, err)
	assert.EqualValues(t, 96_305_000, calc.PaymentTargetTotal)
	assert.EqualValues(t, 83_625_000, calc.MaintenanceTotal)
	assert.Equal(t, calc.ServiceTotal, calc.PaymentTargetTotal)
	assert.NotEqual(t, calc.ServiceTotal+calc.MaintenanceTotal, calc.PaymentTargetTotal)
}

func TestCalculateKioskQueueSystemDoesNotDerivePaymentFromStageLabel(t *testing.T) {
	calc, err := service.CalculateKioskQueueSystem(governmentHardwareFixtureInput())
	require.NoError(t, err)
	assert.EqualValues(t, 100_000_000, calc.PaymentTotal)
	assert.EqualValues(t, 100_000_000, calc.PaymentTargetTotal)
}
