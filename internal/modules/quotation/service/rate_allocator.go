package service

import (
	"errors"
	"fmt"
	"math"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

type RateAllocator interface {
	DetermineTermType(projectDurationMonths int64) string
	Calculate(proposal dto.AllocationProposal, rateCardItems []domain.RateCardItem, projectDurationMonths int64, budgetMaximum int64, currency string) (dto.EnrichedAllocation, error)
	CalculateRaw(proposal dto.AllocationProposal, rateCardItems []domain.RateCardItem, projectDurationMonths int64, currency string) (dto.EnrichedAllocation, error)
}

type defaultRateAllocator struct{}

func NewRateAllocator() RateAllocator {
	return &defaultRateAllocator{}
}

func (r *defaultRateAllocator) DetermineTermType(projectDurationMonths int64) string {
	if projectDurationMonths < 3 {
		return "short_term"
	}
	return "long_term"
}

func (r *defaultRateAllocator) CalculateRaw(proposal dto.AllocationProposal, rateCardItems []domain.RateCardItem, projectDurationMonths int64, currency string) (dto.EnrichedAllocation, error) {
	if projectDurationMonths < 1 {
		return dto.EnrichedAllocation{}, errors.New("project_duration_months must be positive")
	}

	term := r.DetermineTermType(projectDurationMonths)

	// Build rate lookup map: "role|seniority|term_type" -> AmountPerMonth
	rateMap := make(map[string]int64)
	for _, item := range rateCardItems {
		if item.AmountPerMonth <= 0 {
			return dto.EnrichedAllocation{}, fmt.Errorf("rate item %s|%s amount_per_month must be positive", item.Role, item.Seniority)
		}
		key := fmt.Sprintf("%s|%s|%s", item.Role, item.Seniority, item.TermType)
		if _, exists := rateMap[key]; exists {
			return dto.EnrichedAllocation{}, fmt.Errorf("duplicate rate item for %s", key)
		}
		rateMap[key] = item.AmountPerMonth
		resolved := resolveRole(item.Role)
		if resolved != item.Role {
			resolvedKey := fmt.Sprintf("%s|%s|%s", resolved, item.Seniority, item.TermType)
			if _, exists := rateMap[resolvedKey]; !exists {
				rateMap[resolvedKey] = item.AmountPerMonth
			}
		}
	}

	result := dto.EnrichedAllocation{
		Resources:             make([]dto.EnrichedResource, 0, len(proposal.Resources)),
		SelectedOptionalScope: append([]string(nil), proposal.SelectedOptionalScope...),
		StandardActivities:    append([]string(nil), proposal.StandardActivities...),
		Currency:              currency,
		TermType:              term,
	}

	for i, res := range proposal.Resources {
		quarters := res.PeriodQuarters
		if quarters <= 0 && res.PeriodMonths > 0 {
			quarters = res.PeriodMonths * 4
		}
		months := res.PeriodMonths
		if months <= 0 && quarters > 0 {
			months = (quarters + 3) / 4
		}

		if res.EmployeeCount <= 0 {
			return dto.EnrichedAllocation{}, fmt.Errorf("resources[%d].employee_count must be positive", i)
		}
		if quarters <= 0 {
			return dto.EnrichedAllocation{}, fmt.Errorf("resources[%d].period_quarters must be positive", i)
		}

		resolvedRole := resolveRole(res.Role)
		key := fmt.Sprintf("%s|%s|%s", resolvedRole, res.Seniority, term)
		amountPerMonth, exists := rateMap[key]
		if !exists {
			legacyKey := fmt.Sprintf("%s|%s|%s", res.Role, res.Seniority, term)
			amountPerMonth, exists = rateMap[legacyKey]
		}
		if !exists {
			return dto.EnrichedAllocation{}, fmt.Errorf("%w: missing rate for %s", dto.ErrMissingRateItem, key)
		}

		lineTotal, err := checkedMul(amountPerMonth, res.EmployeeCount)
		if err != nil {
			return dto.EnrichedAllocation{}, fmt.Errorf("resources[%d] rate * employee_count: %w", i, err)
		}
		lineTotal, err = checkedMul(lineTotal, quarters)
		if err != nil {
			return dto.EnrichedAllocation{}, fmt.Errorf("resources[%d] line total * quarters: %w", i, err)
		}
		lineTotal = lineTotal / 4

		var addErr error
		result.TotalInvestment, addErr = checkedAdd(result.TotalInvestment, lineTotal)
		if addErr != nil {
			return dto.EnrichedAllocation{}, fmt.Errorf("total investment: %w", addErr)
		}

		result.Resources = append(result.Resources, dto.EnrichedResource{
			Role:            res.Role,
			Seniority:       res.Seniority,
			TermType:        term,
			EmployeeCount:   res.EmployeeCount,
			PeriodQuarters:  quarters,
			PeriodMonths:    months,
			AmountPerMonth:  amountPerMonth,
			LineTotal:       lineTotal,
			ScopeReferences: append([]string(nil), res.ScopeReferences...),
			Reason:          res.Reason,
		})
	}

	return result, nil
}

func (r *defaultRateAllocator) Calculate(proposal dto.AllocationProposal, rateCardItems []domain.RateCardItem, projectDurationMonths int64, budgetMaximum int64, currency string) (dto.EnrichedAllocation, error) {
	result, err := r.CalculateRaw(proposal, rateCardItems, projectDurationMonths, currency)
	if err != nil {
		return dto.EnrichedAllocation{}, err
	}

	if budgetMaximum > 0 {
		if result.TotalInvestment > budgetMaximum {
			excess, err := checkedAdd(result.TotalInvestment, -budgetMaximum)
			if err != nil {
				return dto.EnrichedAllocation{}, fmt.Errorf("budget excess calculation: %w", err)
			}
			return dto.EnrichedAllocation{}, &dto.BudgetExceededError{
				Total:    result.TotalInvestment,
				Budget:   budgetMaximum,
				Excess:   excess,
				Currency: currency,
			}
		}

		utilizationNumerator, err := checkedMul(result.TotalInvestment, 100)
		if err != nil {
			return dto.EnrichedAllocation{}, fmt.Errorf("utilization calculation: %w", err)
		}
		result.UtilizationPercent = utilizationNumerator / budgetMaximum
	}

	return result, nil
}

func checkedMul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a == -1 && b == math.MinInt64 || b == -1 && a == math.MinInt64 {
		return 0, errors.New("integer multiplication overflow")
	}
	result := a * b
	if result/b != a {
		return 0, errors.New("integer multiplication overflow")
	}
	return result, nil
}

func checkedAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, errors.New("integer addition overflow")
	}
	return a + b, nil
}
