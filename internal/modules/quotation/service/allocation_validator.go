package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var (
	validRoles = []string{
		"business_analyst", "system_analyst", "frontend_programmer", "backend_programmer",
		"fullstack_developer", "mobile_developer", "devops", "data_engineer",
		"data_analyst", "ui_ux_designer", "automated_tester", "functional_tester",
		"technical_writer", "pentester",
	}
	validSeniorities = []string{"associate", "intermediate", "senior"}

	roleAliases = map[string]string{
		"programmer":        "backend_programmer",
		"quality_assurance": "automated_tester",
		"qa":                "automated_tester",
	}
)

func resolveRole(role string) string {
	if canonical, ok := roleAliases[role]; ok {
		return canonical
	}
	return role
}

type AllocationValidator interface {
	DecodeAllocationJSON(raw []byte) (dto.AllocationProposal, error)
	ValidateProposal(input dto.GenerateQuotationRequest, proposal dto.AllocationProposal) error
}

type defaultAllocationValidator struct{}

func NewAllocationValidator() AllocationValidator {
	return &defaultAllocationValidator{}
}

func (v *defaultAllocationValidator) DecodeAllocationJSON(raw []byte) (dto.AllocationProposal, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var proposal dto.AllocationProposal
	if err := decoder.Decode(&proposal); err != nil {
		return dto.AllocationProposal{}, fmt.Errorf("%w: decode allocation: %v", dto.ErrAllocationInvalid, err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return dto.AllocationProposal{}, fmt.Errorf("%w: decode allocation: expected a single JSON value", dto.ErrAllocationInvalid)
		}
		return dto.AllocationProposal{}, fmt.Errorf("%w: decode allocation: trailing data after JSON value: %v", dto.ErrAllocationInvalid, err)
	}

	for i := range proposal.Resources {
		if proposal.Resources[i].PeriodQuarters <= 0 && proposal.Resources[i].PeriodMonths > 0 {
			proposal.Resources[i].PeriodQuarters = proposal.Resources[i].PeriodMonths * 4
		}
		if proposal.Resources[i].PeriodMonths <= 0 && proposal.Resources[i].PeriodQuarters > 0 {
			proposal.Resources[i].PeriodMonths = (proposal.Resources[i].PeriodQuarters + 3) / 4
		}
	}

	return proposal, nil
}

func (v *defaultAllocationValidator) ValidateProposal(input dto.GenerateQuotationRequest, proposal dto.AllocationProposal) error {
	if input.ProjectDurationMonths < 1 || input.ProjectDurationMonths > 24 {
		return fmt.Errorf("%w: project_duration_months must be between 1 and 24", dto.ErrAllocationInvalid)
	}
	if input.BudgetMaximum < 1 {
		return fmt.Errorf("%w: budget_maximum must be positive", dto.ErrAllocationInvalid)
	}
	if len(proposal.Resources) < 1 || len(proposal.Resources) > 30 {
		return fmt.Errorf("%w: resources must contain between 1 and 30 items", dto.ErrAllocationInvalid)
	}

	if err := validateSelection("selected_optional_scope", proposal.SelectedOptionalScope, input.OptionalScope); err != nil {
		return fmt.Errorf("%w: %v", dto.ErrAllocationInvalid, err)
	}
	if err := validateSelection("standard_activities", proposal.StandardActivities, input.AllowedStandardActivities); err != nil {
		return fmt.Errorf("%w: %v", dto.ErrAllocationInvalid, err)
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

	for i := range proposal.Resources {
		res := &proposal.Resources[i]
		res.Role = resolveRole(res.Role)
		if res.PeriodQuarters <= 0 && res.PeriodMonths > 0 {
			res.PeriodQuarters = res.PeriodMonths * 4
		}
		if res.PeriodMonths <= 0 && res.PeriodQuarters > 0 {
			res.PeriodMonths = (res.PeriodQuarters + 3) / 4
		}

		if !containsString(validRoles, res.Role) {
			return fmt.Errorf("%w: resources[%d].role %q is invalid", dto.ErrAllocationInvalid, i, res.Role)
		}
		if !containsString(validSeniorities, res.Seniority) {
			return fmt.Errorf("%w: resources[%d].seniority %q is invalid", dto.ErrAllocationInvalid, i, res.Seniority)
		}
		if res.EmployeeCount < 1 || res.EmployeeCount > 10 {
			return fmt.Errorf("%w: resources[%d].employee_count must be between 1 and 10", dto.ErrAllocationInvalid, i)
		}
		maxQuarters := input.ProjectDurationMonths * 4
		if res.PeriodQuarters < 1 {
			return fmt.Errorf("%w: resources[%d].period_quarters (or period_months) must be between 1 and %d", dto.ErrAllocationInvalid, i, maxQuarters)
		}
		if res.PeriodQuarters > maxQuarters {
			return fmt.Errorf("%w: resources[%d].period_quarters must not exceed project_duration_months * 4", dto.ErrAllocationInvalid, i)
		}
		if len(res.ScopeReferences) < 1 || len(res.ScopeReferences) > 50 {
			return fmt.Errorf("%w: resources[%d].scope_references must contain between 1 and 50 items", dto.ErrAllocationInvalid, i)
		}

		seenReferences := make(map[string]struct{}, len(res.ScopeReferences))
		for refIdx, ref := range res.ScopeReferences {
			trimmed := strings.TrimSpace(ref)
			if trimmed == "" || utf8.RuneCountInString(trimmed) > 200 {
				return fmt.Errorf("%w: resources[%d].scope_references[%d] must be non-empty and at most 200 characters", dto.ErrAllocationInvalid, i, refIdx)
			}
			if _, exists := seenReferences[ref]; exists {
				return fmt.Errorf("%w: resources[%d].scope_references contains duplicate value %q", dto.ErrAllocationInvalid, i, ref)
			}
			seenReferences[ref] = struct{}{}

			if _, exists := allowedReferences[ref]; !exists {
				return fmt.Errorf("%w: resources[%d] has unknown scope reference %q", dto.ErrAllocationInvalid, i, ref)
			}
			if containsString(input.MandatoryScope, ref) {
				coveredMandatory[ref] = struct{}{}
			}
		}

		trimmedReason := strings.TrimSpace(res.Reason)
		if trimmedReason == "" || utf8.RuneCountInString(trimmedReason) > 500 {
			return fmt.Errorf("%w: resources[%d].reason must be non-empty and at most 500 characters", dto.ErrAllocationInvalid, i)
		}
	}

	for _, mandatory := range input.MandatoryScope {
		if _, covered := coveredMandatory[mandatory]; !covered {
			return fmt.Errorf("%w: mandatory scope %q is not covered by any resource", dto.ErrAllocationInvalid, mandatory)
		}
	}

	return nil
}

func validateSelection(field string, selected, allowlist []string) error {
	if len(selected) > 50 {
		return fmt.Errorf("%s must contain at most 50 items", field)
	}
	seen := make(map[string]struct{}, len(selected))
	for i, val := range selected {
		trimmed := strings.TrimSpace(val)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > 200 {
			return fmt.Errorf("%s[%d] must be non-empty and at most 200 characters", field, i)
		}
		if _, exists := seen[val]; exists {
			return fmt.Errorf("%s contains duplicate value %q", field, val)
		}
		seen[val] = struct{}{}
		if !containsString(allowlist, val) {
			return fmt.Errorf("%s contains value outside allowlist: %q", field, val)
		}
	}
	return nil
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
