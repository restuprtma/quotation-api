package rnd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

var (
	ErrBudgetExceeded   = errors.New("allocation exceeds budget")
	ErrBudgetUnfeasible = errors.New("budget is unfeasible")
)

type BudgetExceededError struct {
	Total  int64
	Budget int64
	Excess int64
}

func (e *BudgetExceededError) Error() string {
	return fmt.Sprintf("%s by %d IDR", ErrBudgetExceeded, e.Excess)
}

func (e *BudgetExceededError) Unwrap() error {
	return ErrBudgetExceeded
}

func DecodeAllocation(raw []byte) (AllocationProposal, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var proposal AllocationProposal
	if err := decoder.Decode(&proposal); err != nil {
		return AllocationProposal{}, fmt.Errorf("decode allocation: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return AllocationProposal{}, errors.New("decode allocation: expected a single JSON value")
		}
		return AllocationProposal{}, fmt.Errorf("decode allocation: trailing data after JSON value: %w", err)
	}
	return proposal, nil
}

func ValidateAndEnrich(input GenerationInput, proposal AllocationProposal, cards RateCardFixture) (EnrichedAllocation, error) {
	if input.ProjectDurationMonths < 1 || input.ProjectDurationMonths > 24 {
		return EnrichedAllocation{}, errors.New("project_duration_months must be between 1 and 24")
	}
	if input.BudgetMaximum < 1 {
		return EnrichedAllocation{}, errors.New("budget_maximum must be positive")
	}
	if len(proposal.Resources) < 1 || len(proposal.Resources) > 30 {
		return EnrichedAllocation{}, errors.New("resources must contain between 1 and 30 items")
	}
	if err := validateSelection("selected_optional_scope", proposal.SelectedOptionalScope, input.OptionalScope); err != nil {
		return EnrichedAllocation{}, err
	}
	if err := validateSelection("standard_activities", proposal.StandardActivities, input.AllowedStandardActivities); err != nil {
		return EnrichedAllocation{}, err
	}

	allowedReferences := make(map[string]struct{}, len(input.MandatoryScope)+len(proposal.SelectedOptionalScope)+len(proposal.StandardActivities))
	for _, scope := range input.MandatoryScope {
		allowedReferences[scope] = struct{}{}
	}
	for _, scope := range proposal.SelectedOptionalScope {
		allowedReferences[scope] = struct{}{}
	}
	for _, activity := range proposal.StandardActivities {
		allowedReferences[activity] = struct{}{}
	}
	coveredMandatory := make(map[string]struct{}, len(input.MandatoryScope))

	term := termType(input.ProjectDurationMonths)
	rates, currency, err := buildRateLookup(cards, input.RateCardCode)
	if err != nil {
		return EnrichedAllocation{}, err
	}

	result := EnrichedAllocation{
		Resources:             make([]EnrichedResource, 0, len(proposal.Resources)),
		SelectedOptionalScope: append([]string(nil), proposal.SelectedOptionalScope...),
		StandardActivities:    append([]string(nil), proposal.StandardActivities...),
		Currency:              currency,
		TermType:              term,
	}

	for resourceIndex, resource := range proposal.Resources {
		if !containsString([]string{"business_analyst", "ui_ux_designer", "programmer", "quality_assurance", "devops"}, resource.Role) {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].role %q is invalid", resourceIndex, resource.Role)
		}
		if !containsString([]string{"associate", "intermediate", "senior"}, resource.Seniority) {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].seniority %q is invalid", resourceIndex, resource.Seniority)
		}
		if resource.EmployeeCount < 1 || resource.EmployeeCount > 10 {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].employee_count must be between 1 and 10", resourceIndex)
		}
		if resource.PeriodMonths < 1 || resource.PeriodMonths > 24 {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].period_months must be between 1 and 24", resourceIndex)
		}
		if resource.PeriodMonths > input.ProjectDurationMonths {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].period_months must not exceed project_duration_months", resourceIndex)
		}
		if len(resource.ScopeReferences) < 1 || len(resource.ScopeReferences) > 50 {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].scope_references must contain between 1 and 50 items", resourceIndex)
		}
		seenReferences := make(map[string]struct{}, len(resource.ScopeReferences))
		for referenceIndex, reference := range resource.ScopeReferences {
			if strings.TrimSpace(reference) == "" || utf8.RuneCountInString(reference) > 200 {
				return EnrichedAllocation{}, fmt.Errorf("resources[%d].scope_references[%d] must be non-empty and at most 200 characters", resourceIndex, referenceIndex)
			}
			if _, exists := seenReferences[reference]; exists {
				return EnrichedAllocation{}, fmt.Errorf("resources[%d].scope_references contains duplicate value %q", resourceIndex, reference)
			}
			seenReferences[reference] = struct{}{}
			if _, exists := allowedReferences[reference]; !exists {
				return EnrichedAllocation{}, fmt.Errorf("resources[%d] has unknown scope reference %q", resourceIndex, reference)
			}
			if containsString(input.MandatoryScope, reference) {
				coveredMandatory[reference] = struct{}{}
			}
		}
		if strings.TrimSpace(resource.Reason) == "" || utf8.RuneCountInString(resource.Reason) > 500 {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d].reason must be non-empty and at most 500 characters", resourceIndex)
		}

		key := rateLookupKey(resource.Role, resource.Seniority, term)
		rateMatches := rates[key]
		if len(rateMatches) == 0 {
			return EnrichedAllocation{}, fmt.Errorf("missing rate for %s|%s", input.RateCardCode, key)
		}
		if len(rateMatches) > 1 {
			return EnrichedAllocation{}, fmt.Errorf("duplicate rate for %s|%s", input.RateCardCode, key)
		}
		amountPerMonth := rateMatches[0]
		lineTotal, err := checkedMul(amountPerMonth, resource.EmployeeCount)
		if err != nil {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d] rate * employee_count: %w", resourceIndex, err)
		}
		lineTotal, err = checkedMul(lineTotal, resource.PeriodMonths)
		if err != nil {
			return EnrichedAllocation{}, fmt.Errorf("resources[%d] line total: %w", resourceIndex, err)
		}
		result.TotalInvestment, err = checkedAdd(result.TotalInvestment, lineTotal)
		if err != nil {
			return EnrichedAllocation{}, fmt.Errorf("total investment: %w", err)
		}
		result.Resources = append(result.Resources, EnrichedResource{
			Role: resource.Role, Seniority: resource.Seniority, TermType: term,
			EmployeeCount: resource.EmployeeCount, PeriodMonths: resource.PeriodMonths,
			AmountPerMonth: amountPerMonth, LineTotal: lineTotal,
			ScopeReferences: append([]string(nil), resource.ScopeReferences...), Reason: resource.Reason,
		})
	}

	for _, mandatory := range input.MandatoryScope {
		if _, covered := coveredMandatory[mandatory]; !covered {
			return EnrichedAllocation{}, fmt.Errorf("mandatory scope %q is not covered by any resource", mandatory)
		}
	}
	if result.TotalInvestment > input.BudgetMaximum {
		excess, err := checkedAdd(result.TotalInvestment, -input.BudgetMaximum)
		if err != nil {
			return EnrichedAllocation{}, fmt.Errorf("budget excess: %w", err)
		}
		return EnrichedAllocation{}, &BudgetExceededError{Total: result.TotalInvestment, Budget: input.BudgetMaximum, Excess: excess}
	}
	utilizationNumerator, err := checkedMul(result.TotalInvestment, 100)
	if err != nil {
		return EnrichedAllocation{}, fmt.Errorf("utilization calculation: %w", err)
	}
	result.UtilizationPercent = utilizationNumerator / input.BudgetMaximum
	return result, nil
}

func termType(projectDurationMonths int64) string {
	if projectDurationMonths < 3 {
		return "short_term"
	}
	return "long_term"
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

func validateSelection(field string, selected, allowlist []string) error {
	if len(selected) > 50 {
		return fmt.Errorf("%s must contain at most 50 items", field)
	}
	seen := make(map[string]struct{}, len(selected))
	for i, value := range selected {
		if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 200 {
			return fmt.Errorf("%s[%d] must be non-empty and at most 200 characters", field, i)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains duplicate value %q", field, value)
		}
		seen[value] = struct{}{}
		if !containsString(allowlist, value) {
			return fmt.Errorf("%s contains value outside allowlist: %q", field, value)
		}
	}
	return nil
}

func buildRateLookup(cards RateCardFixture, rateCardCode string) (map[string][]int64, string, error) {
	rates := make(map[string][]int64)
	currency := ""
	matchedCard := false
	for _, card := range cards.RateCards {
		if card.Code != rateCardCode {
			continue
		}
		matchedCard = true
		if card.Currency != "IDR" {
			return nil, "", fmt.Errorf("rate card %q currency must be IDR", rateCardCode)
		}
		if currency != "" && currency != card.Currency {
			return nil, "", fmt.Errorf("rate card %q has inconsistent currency", rateCardCode)
		}
		currency = card.Currency
		for _, item := range card.Items {
			if item.AmountPerMonth <= 0 {
				return nil, "", fmt.Errorf("rate card %q amount_per_month must be positive", rateCardCode)
			}
			key := rateLookupKey(item.Role, item.Seniority, item.TermType)
			rates[key] = append(rates[key], item.AmountPerMonth)
		}
	}
	if !matchedCard {
		return nil, "", fmt.Errorf("missing rate card %q", rateCardCode)
	}
	return rates, currency, nil
}

func rateLookupKey(role, seniority, term string) string {
	return strings.Join([]string{role, seniority, term}, "|")
}
