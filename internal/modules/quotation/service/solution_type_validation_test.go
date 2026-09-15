package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func validKioskRequestWithSolutionType(solutionType string) dto.GenerateQuotationRequest {
	kioskInput := domain.KioskQueueSystemInput{
		Variant:           domain.KioskQueueGovernmentMPP,
		DocumentNumber:    "LANDA/MPP-Antrian/0926/001",
		SystemName:        "Sistem Antrian Terpadu MPP",
		QuotationDate:     time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC),
		HardwareProvision: domain.HardwareClientSupplied,
		TaxTreatment:      domain.KioskTaxInclusivePPN11,
		GovernmentScopePhases: []domain.KioskQueueScopePhaseInput{
			{
				Title:        "Fase 1: Persiapan & Desain",
				Deliverables: []string{"Dokumen Analisis Kebutuhan", "UI/UX Design Mockup"},
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
			{Stage: "Uang Muka 50%", BillingBasis: "SPMK", Nominal: 31_000_000},
			{Stage: "Pelunasan 50%", BillingBasis: "BAST", Nominal: 31_000_000},
		},
		PaymentDueDays: 14,
		LimitationCodes: []domain.KioskQueueLimitationCode{
			domain.KioskLimitCivilElectrical,
			domain.KioskLimitClientHardware,
			domain.KioskLimitHardwareWarranty,
		},
	}

	return dto.GenerateQuotationRequest{
		Subject:          "Penawaran Pengadaan Sistem Antrian Terpadu MPP",
		SolutionType:     solutionType,
		CommercialScheme: domain.CommercialSchemeKioskQueueSystem,
		KioskQueueSystem: &kioskInput,
	}
}

func TestSolutionTypeValidation_100RunesAccepted(t *testing.T) {
	// Exactly 100 ASCII runes
	req100ASCII := validKioskRequestWithSolutionType(strings.Repeat("A", 100))
	err := ValidateGenerateInput(req100ASCII)
	require.NoError(t, err, "100-rune ASCII solution_type must be accepted")

	// Exactly 100 multi-byte Unicode runes
	req100Unicode := validKioskRequestWithSolutionType(strings.Repeat("★", 100))
	errUnicode := ValidateGenerateInput(req100Unicode)
	require.NoError(t, errUnicode, "100-rune multi-byte Unicode solution_type must be accepted")
}

func TestSolutionTypeValidation_101RunesRejected(t *testing.T) {
	// 101 ASCII runes
	req101ASCII := validKioskRequestWithSolutionType(strings.Repeat("A", 101))
	err := ValidateGenerateInput(req101ASCII)
	require.Error(t, err, "101-rune solution_type must be rejected")

	var ge *GenerationError
	require.True(t, errors.As(err, &ge))
	assert.Equal(t, ErrCodeGenerationFailed, ge.Code)
	assert.Equal(t, "solution_type must be between 1 and 100 characters", ge.Message())

	// 101 multi-byte Unicode runes
	req101Unicode := validKioskRequestWithSolutionType(strings.Repeat("★", 101))
	errUnicode := ValidateGenerateInput(req101Unicode)
	require.Error(t, errUnicode, "101-rune multi-byte Unicode solution_type must be rejected")
	var geUni *GenerationError
	require.True(t, errors.As(errUnicode, &geUni))
	assert.Equal(t, ErrCodeGenerationFailed, geUni.Code)
	assert.Equal(t, "solution_type must be between 1 and 100 characters", geUni.Message())
}

func TestSolutionTypeValidation_SistemAntrianQueueManagementAccepted(t *testing.T) {
	// "Sistem Antrian (Queue Management)" has length 34 runes, must be accepted
	solType := "Sistem Antrian (Queue Management)"
	req := validKioskRequestWithSolutionType(solType)
	err := ValidateGenerateInput(req)
	require.NoError(t, err, "Sistem Antrian (Queue Management) must be accepted")
}

func TestSolutionTypeValidation_CommercialSchemeNotUsedAsSubstitute(t *testing.T) {
	// Empty solution_type must be rejected even though commercial_scheme is provided
	reqEmpty := validKioskRequestWithSolutionType("")
	errEmpty := ValidateGenerateInput(reqEmpty)
	require.Error(t, errEmpty)
	var geEmpty *GenerationError
	require.True(t, errors.As(errEmpty, &geEmpty))
	assert.Equal(t, ErrCodeGenerationFailed, geEmpty.Code)
	assert.Equal(t, "solution_type must be between 1 and 100 characters", geEmpty.Message())

	// Whitespace-only solution_type must also be rejected
	reqSpaces := validKioskRequestWithSolutionType("   ")
	errSpaces := ValidateGenerateInput(reqSpaces)
	require.Error(t, errSpaces)
	var geSpaces *GenerationError
	require.True(t, errors.As(errSpaces, &geSpaces))
	assert.Equal(t, ErrCodeGenerationFailed, geSpaces.Code)
	assert.Equal(t, "solution_type must be between 1 and 100 characters", geSpaces.Message())

	// Both solution_type and commercial_scheme are distinct and retained
	req := validKioskRequestWithSolutionType("Sistem Antrian (Queue Management)")
	require.Equal(t, "Sistem Antrian (Queue Management)", req.SolutionType)
	require.Equal(t, domain.CommercialSchemeKioskQueueSystem, req.CommercialScheme)
	assert.NotEqual(t, string(req.CommercialScheme), req.SolutionType)
	require.NoError(t, ValidateGenerateInput(req))
}
