package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestDecodeAllocationJSON_RejectsForbiddenFields(t *testing.T) {
	validator := service.NewAllocationValidator()

	t.Run("rejects rate field", func(t *testing.T) {
		raw := []byte(`{
			"resources": [
				{
					"role": "programmer",
					"seniority": "senior",
					"employee_count": 1,
					"period_months": 3,
					"scope_references": ["Scope A"],
					"reason": "Build",
					"rate": 12000000
				}
			],
			"selected_optional_scope": [],
			"standard_activities": []
		}`)

		_, err := validator.DecodeAllocationJSON(raw)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "unknown field")
	})

	t.Run("rejects total field", func(t *testing.T) {
		raw := []byte(`{
			"resources": [
				{
					"role": "programmer",
					"seniority": "senior",
					"employee_count": 1,
					"period_months": 3,
					"scope_references": ["Scope A"],
					"reason": "Build",
					"total": 36000000
				}
			],
			"selected_optional_scope": [],
			"standard_activities": []
		}`)

		_, err := validator.DecodeAllocationJSON(raw)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "unknown field")
	})
}

func TestDecodeAllocationJSON_RejectsTrailingJSON(t *testing.T) {
	validator := service.NewAllocationValidator()

	raw := []byte(`{
		"resources": [
			{
				"role": "programmer",
				"seniority": "senior",
				"employee_count": 1,
				"period_months": 3,
				"scope_references": ["Scope A"],
				"reason": "Build"
			}
		],
		"selected_optional_scope": [],
		"standard_activities": []
	} {"extra": true}`)

	_, err := validator.DecodeAllocationJSON(raw)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "single json")
}

func TestValidateProposal_HappyPath(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths:     3,
		BudgetMaximum:             50_000_000,
		MandatoryScope:            []string{"Scope A", "Scope B"},
		OptionalScope:             []string{"Scope C"},
		AllowedStandardActivities: []string{"documentation"},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A", "Scope C"},
				Reason:          "Lead developer for Scope A and Scope C",
			},
			{
				Role:            "quality_assurance",
				Seniority:       "intermediate",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope B", "documentation"},
				Reason:          "Testing Scope B and doc",
			},
		},
		SelectedOptionalScope: []string{"Scope C"},
		StandardActivities:    []string{"documentation"},
	}

	err := validator.ValidateProposal(input, proposal)
	require.NoError(t, err)
}

func TestValidateProposal_RejectsUnknownRoleOrSeniority(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths: 3,
		BudgetMaximum:         50_000_000,
		MandatoryScope:        []string{"Scope A"},
	}

	t.Run("unknown role", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "product_manager",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Manage",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "role")
	})

	t.Run("unknown seniority", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "lead",
					EmployeeCount:   1,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "seniority")
	})
}

func TestValidateProposal_RejectsExceededPeriods(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths: 3,
		BudgetMaximum:         50_000_000,
		MandatoryScope:        []string{"Scope A"},
	}

	t.Run("period exceeds project duration", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    4, // 4 > 3
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "project_duration_months")
	})

	t.Run("period is zero or negative", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    0,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "period_months")
	})
}

func TestValidateProposal_RejectsExceededEmployeeCounts(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths: 3,
		BudgetMaximum:         50_000_000,
		MandatoryScope:        []string{"Scope A"},
	}

	t.Run("zero count", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   0,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "employee_count")
	})

	t.Run("exceeds max 10 count", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   11,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "employee_count")
	})
}

func TestValidateProposal_RejectsUnknownScopeReferences(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths: 3,
		BudgetMaximum:         50_000_000,
		MandatoryScope:        []string{"Scope A"},
	}

	t.Run("invented business feature", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A", "Invented Blockchain Feature"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "unknown scope reference")
	})

	t.Run("duplicate scope reference", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A", "Scope A"},
					Reason:          "Dev",
				},
			},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "duplicate")
	})
}

func TestValidateProposal_RejectsMissingMandatoryScopeCoverage(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths: 3,
		BudgetMaximum:         50_000_000,
		MandatoryScope:        []string{"Scope A", "Scope B (Missing)"},
		OptionalScope:         []string{"Scope C"},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "Dev Scope A only",
			},
		},
	}

	err := validator.ValidateProposal(input, proposal)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "mandatory scope")
	assert.Contains(t, strings.ToLower(err.Error()), "is not covered")
}

func TestValidateProposal_RejectsUnknownOptionalOrStandardSelection(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths:     3,
		BudgetMaximum:             50_000_000,
		MandatoryScope:            []string{"Scope A"},
		OptionalScope:             []string{"Scope B"},
		AllowedStandardActivities: []string{"documentation"},
	}

	t.Run("unapproved optional scope selected", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
			SelectedOptionalScope: []string{"Scope Unapproved"},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "selected_optional_scope")
	})

	t.Run("unapproved standard activity", func(t *testing.T) {
		proposal := dto.AllocationProposal{
			Resources: []dto.AllocationResource{
				{
					Role:            "programmer",
					Seniority:       "senior",
					EmployeeCount:   1,
					PeriodMonths:    3,
					ScopeReferences: []string{"Scope A"},
					Reason:          "Dev",
				},
			},
			StandardActivities: []string{"arbitrary_activity"},
		}

		err := validator.ValidateProposal(input, proposal)
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "standard_activities")
	})
}

func TestValidateProposal_RejectsEmptyReason(t *testing.T) {
	validator := service.NewAllocationValidator()

	input := dto.GenerateQuotationRequest{
		ProjectDurationMonths: 3,
		BudgetMaximum:         50_000_000,
		MandatoryScope:        []string{"Scope A"},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{
				Role:            "programmer",
				Seniority:       "senior",
				EmployeeCount:   1,
				PeriodMonths:    3,
				ScopeReferences: []string{"Scope A"},
				Reason:          "   ",
			},
		},
	}

	err := validator.ValidateProposal(input, proposal)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "reason")
}
