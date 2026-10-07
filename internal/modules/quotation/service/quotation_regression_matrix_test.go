package service_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestQuotationRegression_Boundary2vs3Months(t *testing.T) {
	allocator := service.NewRateAllocator()

	tests := []struct {
		months       int64
		expectedTerm string
	}{
		{months: 1, expectedTerm: "short_term"},
		{months: 2, expectedTerm: "short_term"},
		{months: 3, expectedTerm: "long_term"},
		{months: 4, expectedTerm: "long_term"},
		{months: 6, expectedTerm: "long_term"},
		{months: 12, expectedTerm: "long_term"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedTerm, func(t *testing.T) {
			assert.Equal(t, tt.expectedTerm, allocator.DetermineTermType(tt.months))
		})
	}
}

func TestQuotationRegression_NationalVsInternationalRates(t *testing.T) {
	now := time.Now()
	companyID := uuid.New()

	cardNat := &domain.RateCard{
		ID:                    uuid.New(),
		CompanyID:             companyID,
		Code:                  "VENTURO-STD-2026",
		Name:                  "National Standard",
		Market:                "national",
		ClientTier:            "commercial_national",
		Currency:              "IDR",
		ShortTermSurchargePct: 40,
		EnterpriseMarkupPct:   35,
		EffectiveFrom:         now,
		IsActive:              true,
	}

	cardIntl := &domain.RateCard{
		ID:                    uuid.New(),
		CompanyID:             companyID,
		Code:                  "VENTURO-INTL-2026",
		Name:                  "International Tier",
		Market:                "international",
		ClientTier:            "international",
		Currency:              "USD",
		ShortTermSurchargePct: 50,
		EnterpriseMarkupPct:   40,
		EffectiveFrom:         now,
		IsActive:              true,
	}

	assert.Equal(t, "national", cardNat.Market)
	assert.Equal(t, "IDR", cardNat.Currency)
	assert.Equal(t, "international", cardIntl.Market)
	assert.Equal(t, "USD", cardIntl.Currency)
	assert.NotEqual(t, cardNat.Code, cardIntl.Code)
}

func TestQuotationRegression_IntegerOverflowPrevention(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			ID:             uuid.New(),
			CompanyID:      uuid.New(),
			RateCardID:     uuid.New(),
			Role:           "backend_programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: math.MaxInt64 / 2, // Near max
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:           "backend_programmer",
				Seniority:      "senior",
				EmployeeCount:  10, // Multiplication will overflow MaxInt64
				PeriodQuarters: 4,
				PeriodMonths:   1,
			},
		},
	}

	_, err := allocator.Calculate(proposal, rateCardItems, 3, 100_000_000_000, "IDR")
	assert.Error(t, err, "must return integer overflow error when calculating large values")
	assert.True(t, strings.Contains(err.Error(), "overflow"), "error message must mention overflow")
}

func TestQuotationRegression_BudgetHandling(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			ID:             uuid.New(),
			CompanyID:      uuid.New(),
			RateCardID:     uuid.New(),
			Role:           "backend_programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 10_000_000,
		},
	}

	// 1 senior for 3 months = 30M
	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:           "backend_programmer",
				Seniority:      "senior",
				EmployeeCount:  1,
				PeriodQuarters: 12, // 3 months
				PeriodMonths:   3,
			},
		},
	}

	t.Run("Under-budget utilization", func(t *testing.T) {
		res, err := allocator.Calculate(proposal, rateCardItems, 3, 50_000_000, "IDR")
		require.NoError(t, err)
		assert.Equal(t, int64(30_000_000), res.TotalInvestment)
		assert.Equal(t, int64(60), res.UtilizationPercent) // 30M / 50M = 60%
	})

	t.Run("Exact budget utilization", func(t *testing.T) {
		res, err := allocator.Calculate(proposal, rateCardItems, 3, 30_000_000, "IDR")
		require.NoError(t, err)
		assert.Equal(t, int64(30_000_000), res.TotalInvestment)
		assert.Equal(t, int64(100), res.UtilizationPercent)
	})

	t.Run("Exceeded budget error", func(t *testing.T) {
		_, err := allocator.Calculate(proposal, rateCardItems, 3, 20_000_000, "IDR")
		assert.Error(t, err)
		assert.ErrorIs(t, err, dto.ErrBudgetExceeded)
	})
}

func TestQuotationRegression_MandatoryAndOptionalScopes(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			ID:             uuid.New(),
			CompanyID:      uuid.New(),
			RateCardID:     uuid.New(),
			Role:           "backend_programmer",
			Seniority:      "intermediate",
			TermType:       "long_term",
			AmountPerMonth: 8_000_000,
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:           "backend_programmer",
				Seniority:      "intermediate",
				EmployeeCount:  1,
				PeriodQuarters: 4,
				PeriodMonths:   1,
			},
		},
		SelectedOptionalScope: []string{"Payment Gateway Integration", "Push Notification"},
		StandardActivities:    []string{"Unit Testing", "Code Review", "CI/CD Setup"},
	}

	res, err := allocator.Calculate(proposal, rateCardItems, 3, 50_000_000, "IDR")
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.SelectedOptionalScope))
	assert.Equal(t, 3, len(res.StandardActivities))
	assert.Contains(t, res.SelectedOptionalScope, "Payment Gateway Integration")
	assert.Contains(t, res.StandardActivities, "Unit Testing")
}
