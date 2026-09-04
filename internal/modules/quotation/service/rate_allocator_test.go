package service_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestDetermineTermType_Boundaries(t *testing.T) {
	allocator := service.NewRateAllocator()

	assert.Equal(t, "short_term", allocator.DetermineTermType(1))
	assert.Equal(t, "short_term", allocator.DetermineTermType(2))
	assert.Equal(t, "long_term", allocator.DetermineTermType(3))
	assert.Equal(t, "long_term", allocator.DetermineTermType(4))
	assert.Equal(t, "long_term", allocator.DetermineTermType(12))
	assert.Equal(t, "long_term", allocator.DetermineTermType(24))
}

func TestRateAllocator_Calculate_HappyPathMultiRole(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardID := uuid.New()
	companyID := uuid.New()

	rateCardItems := []domain.RateCardItem{
		{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000,
		},
		{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           "quality_assurance",
			Seniority:      "intermediate",
			TermType:       "long_term",
			AmountPerMonth: 8_000_000,
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "Lead backend development",
			},
			{
				Role:            "quality_assurance",
				Seniority:       "intermediate",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "Testing and QA",
			},
		},
		SelectedOptionalScope: []string{"Audit log"},
		StandardActivities:    []string{"documentation"},
	}

	enriched, err := allocator.Calculate(proposal, rateCardItems, 3, 100_000_000, "IDR")
	require.NoError(t, err)

	assert.Equal(t, "IDR", enriched.Currency)
	assert.Equal(t, "long_term", enriched.TermType)
	assert.Equal(t, int64(60_000_000), enriched.TotalInvestment)
	assert.Equal(t, int64(60), enriched.UtilizationPercent)
	assert.Equal(t, []string{"Audit log"}, enriched.SelectedOptionalScope)
	assert.Equal(t, []string{"documentation"}, enriched.StandardActivities)
	require.Len(t, enriched.Resources, 2)

	// Resource 1: 12M * 1 * 3 = 36M
	assert.Equal(t, "programmer", enriched.Resources[0].Role)
	assert.Equal(t, "senior", enriched.Resources[0].Seniority)
	assert.Equal(t, "long_term", enriched.Resources[0].TermType)
	assert.Equal(t, int64(12_000_000), enriched.Resources[0].AmountPerMonth)
	assert.Equal(t, int64(36_000_000), enriched.Resources[0].LineTotal)

	// Resource 2: 8M * 1 * 3 = 24M
	assert.Equal(t, "quality_assurance", enriched.Resources[1].Role)
	assert.Equal(t, "intermediate", enriched.Resources[1].Seniority)
	assert.Equal(t, "long_term", enriched.Resources[1].TermType)
	assert.Equal(t, int64(8_000_000), enriched.Resources[1].AmountPerMonth)
	assert.Equal(t, int64(24_000_000), enriched.Resources[1].LineTotal)
}

func TestRateAllocator_Calculate_Exact2Month_ShortTermRate(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardID := uuid.New()
	companyID := uuid.New()

	rateCardItems := []domain.RateCardItem{
		{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "short_term",
			AmountPerMonth: 14_000_000,
		},
		{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000,
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    2,
				ScopeReferences: []string{"Scope A"},
				Reason:          "Sprint development",
			},
		},
	}

	// 2 months project duration -> should pick short_term rate (14M), not long_term (12M)
	enriched, err := allocator.Calculate(proposal, rateCardItems, 2, 50_000_000, "IDR")
	require.NoError(t, err)

	assert.Equal(t, "short_term", enriched.TermType)
	assert.Equal(t, int64(28_000_000), enriched.TotalInvestment)
	require.Len(t, enriched.Resources, 1)
	assert.Equal(t, int64(14_000_000), enriched.Resources[0].AmountPerMonth)
	assert.Equal(t, int64(28_000_000), enriched.Resources[0].LineTotal)
}

func TestRateAllocator_Calculate_MissingRateItem(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000,
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "devops",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"CI/CD"},
				Reason:          "Infrastructure",
			},
		},
	}

	_, err := allocator.Calculate(proposal, rateCardItems, 3, 50_000_000, "IDR")
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrMissingRateItem) || strings.Contains(err.Error(), "rate_item_missing"))
}

func TestRateAllocator_Calculate_BudgetExceeded(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000,
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   2,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "2 senior devs for 3 months = 72M",
			},
		},
	}

	// Budget is 50M, total is 72M -> Excess is 22M
	_, err := allocator.Calculate(proposal, rateCardItems, 3, 50_000_000, "IDR")
	require.Error(t, err)

	var budgetErr *dto.BudgetExceededError
	require.True(t, errors.As(err, &budgetErr))
	assert.Equal(t, int64(72_000_000), budgetErr.Total)
	assert.Equal(t, int64(50_000_000), budgetErr.Budget)
	assert.Equal(t, int64(22_000_000), budgetErr.Excess)
}

func TestRateAllocator_Calculate_OverflowProtection(t *testing.T) {
	allocator := service.NewRateAllocator()

	t.Run("multiplication overflow", func(t *testing.T) {
		rateCardItems := []domain.RateCardItem{
			{
				Role:           "programmer",
				Seniority:      "senior",
				TermType:       "long_term",
				AmountPerMonth: math.MaxInt64,
			},
		}

		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   2,
					PeriodMonths:    1,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Overflow test",
				},
			},
		}

		_, err := allocator.Calculate(proposal, rateCardItems, 3, math.MaxInt64, "IDR")
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "overflow")
	})

	t.Run("addition overflow", func(t *testing.T) {
		rateCardItems := []domain.RateCardItem{
			{
				Role:           "programmer",
				Seniority:      "senior",
				TermType:       "long_term",
				AmountPerMonth: math.MaxInt64/2 + 10,
			},
			{
				Role:           "quality_assurance",
				Seniority:      "senior",
				TermType:       "long_term",
				AmountPerMonth: math.MaxInt64/2 + 10,
			},
		}

		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    1,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
				{
					Role:            "quality_assurance",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    1,
					ScopeReferences: []string{"Scope A"},
					Reason:          "QA",
				},
			},
		}

		_, err := allocator.Calculate(proposal, rateCardItems, 3, math.MaxInt64, "IDR")
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "overflow")
	})
}

func TestRateAllocator_Calculate_InvalidCounts(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000,
		},
	}

	tests := []struct {
		name          string
		employeeCount int64
		periodMonths  int64
	}{
		{"zero employee count", 0, 3},
		{"negative employee count", -1, 3},
		{"zero period months", 1, 0},
		{"negative period months", 1, -2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposal := dto.AllocationProposal{
				Resources: []dto.AllocationResource{
					{
						Role:            "programmer",
						Seniority:       "senior",
						EmployeeCount:   tt.employeeCount,
						PeriodMonths:    tt.periodMonths,
						ScopeReferences: []string{"Scope A"},
						Reason:          "Invalid count test",
					},
				},
			}

			_, err := allocator.Calculate(proposal, rateCardItems, 3, 50_000_000, "IDR")
			require.Error(t, err)
		})
	}
}

func TestRateAllocator_Calculate_DuplicateRateItem(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000,
		},
		{
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 13_000_000,
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "Duplicate test",
			},
		},
	}

	_, err := allocator.Calculate(proposal, rateCardItems, 3, 50_000_000, "IDR")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "duplicate")
}

func TestRateAllocator_Calculate_NationalVsInternationalNotMixed(t *testing.T) {
	allocator := service.NewRateAllocator()

	// National card items only (doesn't contain international rate)
	nationalItems := []domain.RateCardItem{
		{
			Role:           "programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 12_000_000, // IDR rate
		},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "National card test",
			},
		},
	}

	enriched, err := allocator.Calculate(proposal, nationalItems, 3, 50_000_000, "IDR")
	require.NoError(t, err)
	assert.Equal(t, int64(36_000_000), enriched.TotalInvestment)
	assert.Equal(t, int64(12_000_000), enriched.Resources[0].AmountPerMonth)
}

func TestRateAllocator_Calculate_QuarterMonthsPhasing(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardItems := []domain.RateCardItem{
		{Role: "programmer", Seniority: "intermediate", TermType: "short_term", AmountPerMonth: 12_000_000},
		{Role: "business_analyst", Seniority: "associate", TermType: "short_term", AmountPerMonth: 8_400_000},
		{Role: "ui_ux_designer", Seniority: "associate", TermType: "short_term", AmountPerMonth: 7_800_000},
		{Role: "quality_assurance", Seniority: "associate", TermType: "short_term", AmountPerMonth: 7_200_000},
		{Role: "devops", Seniority: "associate", TermType: "short_term", AmountPerMonth: 10_200_000},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{Role: "programmer", Seniority: "intermediate", EmployeeCount: 1, PeriodQuarters: 8, Reason: "Full 2 mo"},
			{Role: "business_analyst", Seniority: "associate", EmployeeCount: 1, PeriodQuarters: 2, Reason: "0.5 mo"},
			{Role: "ui_ux_designer", Seniority: "associate", EmployeeCount: 1, PeriodQuarters: 1, Reason: "0.25 mo"},
			{Role: "quality_assurance", Seniority: "associate", EmployeeCount: 1, PeriodQuarters: 2, Reason: "0.5 mo"},
			{Role: "devops", Seniority: "associate", EmployeeCount: 1, PeriodQuarters: 1, Reason: "0.25 mo"},
		},
	}

	// CalculateRaw test
	raw, err := allocator.CalculateRaw(proposal, rateCardItems, 2, "IDR")
	require.NoError(t, err)
	assert.Equal(t, int64(24_000_000), raw.Resources[0].LineTotal) // 12M * 8 / 4
	assert.Equal(t, int64(4_200_000), raw.Resources[1].LineTotal)  // 8.4M * 2 / 4
	assert.Equal(t, int64(1_950_000), raw.Resources[2].LineTotal)  // 7.8M * 1 / 4
	assert.Equal(t, int64(3_600_000), raw.Resources[3].LineTotal)  // 7.2M * 2 / 4
	assert.Equal(t, int64(2_550_000), raw.Resources[4].LineTotal)  // 10.2M * 1 / 4
	assert.Equal(t, int64(36_300_000), raw.TotalInvestment)

	// Calculate with budget check
	enriched, err := allocator.Calculate(proposal, rateCardItems, 2, 50_000_000, "IDR")
	require.NoError(t, err)
	assert.Equal(t, int64(36_300_000), enriched.TotalInvestment)
}

func TestRateAllocator_Calculate_Venturo14StandardRoles(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardID := uuid.New()
	companyID := uuid.New()

	roles := []string{
		"business_analyst", "system_analyst", "frontend_programmer", "backend_programmer",
		"fullstack_developer", "mobile_developer", "devops", "data_engineer",
		"data_analyst", "ui_ux_designer", "automated_tester", "functional_tester",
		"technical_writer", "pentester",
	}

	var rateCardItems []domain.RateCardItem
	var proposalResources []dto.AllocationResource

	for _, role := range roles {
		rateCardItems = append(rateCardItems, domain.RateCardItem{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           role,
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 10_000_000,
		})
		proposalResources = append(proposalResources, dto.AllocationResource{
			Role:            role,
			Seniority:       "senior",
			EmployeeCount:   1,
			PeriodMonths:    3,
			ScopeReferences: []string{"Scope A"},
			Reason:          "Role execution",
		})
	}

	proposal := dto.AllocationProposal{Resources: proposalResources}
	enriched, err := allocator.Calculate(proposal, rateCardItems, 3, 500_000_000, "IDR")
	require.NoError(t, err)

	// 14 roles * 10M * 1 * 3 = 420M
	assert.Equal(t, int64(420_000_000), enriched.TotalInvestment)
	assert.Len(t, enriched.Resources, 14)
}

func TestRateAllocator_Calculate_RoleAliases(t *testing.T) {
	allocator := service.NewRateAllocator()

	rateCardID := uuid.New()
	companyID := uuid.New()

	// Rate card contains canonical names
	rateCardItems := []domain.RateCardItem{
		{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           "backend_programmer",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 11_550_000,
		},
		{
			ID:             uuid.New(),
			CompanyID:      companyID,
			RateCardID:     rateCardID,
			Role:           "automated_tester",
			Seniority:      "senior",
			TermType:       "long_term",
			AmountPerMonth: 11_550_000,
		},
	}

	// Proposal uses legacy aliases: programmer, quality_assurance
	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "Backend lead",
			},
			{
				Role:            "quality_assurance",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "QA Automation",
			},
		},
	}

	enriched, err := allocator.Calculate(proposal, rateCardItems, 3, 100_000_000, "IDR")
	require.NoError(t, err)

	// 11.55M * 3 = 34.65M each -> 69.3M
	assert.Equal(t, int64(69_300_000), enriched.TotalInvestment)
	require.Len(t, enriched.Resources, 2)
	assert.Equal(t, int64(34_650_000), enriched.Resources[0].LineTotal)
	assert.Equal(t, int64(34_650_000), enriched.Resources[1].LineTotal)
}

