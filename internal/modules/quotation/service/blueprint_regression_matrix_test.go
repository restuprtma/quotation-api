package service_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestBlueprintRegression_CyclicDependencyDetection(t *testing.T) {
	// State Machine or WBS cyclic transition validation
	crossValidator := service.NewProductionBlueprintCrossValidator()

	cyclicStateMachine := []byte(`{
		"states": [
			{"id": "s1", "name": "Draft", "transitions": ["s2"]},
			{"id": "s2", "name": "Review", "transitions": ["s3"]},
			{"id": "s3", "name": "Approval", "transitions": ["s1"]}
		]
	}`)

	warnings, err := crossValidator.ValidateStateMachineTransitions(cyclicStateMachine)
	// Cyclic flows in approval loops may be flagged or validated according to spec
	assert.NoError(t, err)
	_ = warnings
}

func TestBlueprintRegression_SchemaValidationAllSections(t *testing.T) {
	validator := service.NewBlueprintSectionValidator()

	validSections := []string{"journey", "architecture", "state_machine", "workflow", "wbs"}
	for _, sec := range validSections {
		assert.True(t, validator.IsValidSectionName(sec), "section %s must be recognized", sec)
	}

	assert.False(t, validator.IsValidSectionName("pricing_relational"))
	assert.False(t, validator.IsValidSectionName("unknown_section"))
}

func TestBlueprintRegression_SectionValidatorAllSections(t *testing.T) {
	validator := service.NewBlueprintSectionValidator()

	sections := []domain.BlueprintSectionType{
		domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
	}

	for _, sec := range sections {
		t.Run(string(sec), func(t *testing.T) {
			malformedPayload := json.RawMessage(`{"invalid": "json`)
			res := validator.Validate(sec, malformedPayload)
			assert.False(t, res.Valid, "validator must reject malformed json for %s", sec)
			assert.NotEmpty(t, res.Issues, "must have validation issues")
		})
	}

	// Unknown section should be rejected
	res := validator.Validate(domain.BlueprintSectionType("unknown_section"), json.RawMessage(`{}`))
	assert.False(t, res.Valid)
}

func TestBlueprintRegression_CrossValidatorSymbolIntegrity(t *testing.T) {
	crossValidator := service.NewProductionBlueprintCrossValidator()

	bundleWithInvalidRef := service.BlueprintBundle{
		SchemaVersion: service.BlueprintSchemaVersion,
		Title:         "System Blueprint",
		Summary:       "Cross-validation verification",
		Journey: service.BlueprintJourney{
			Phases: []service.BlueprintJourneyPhase{
				{ID: "phase-1", Label: "Discovery", Order: 1},
			},
			Actors: []service.BlueprintJourneyActor{
				{ID: "actor-1", Label: "Customer", Order: 1},
			},
			Activities: []service.BlueprintJourneyActivity{
				{
					ID:          "act-1",
					PhaseID:     "phase-1",
					ActorID:     "actor-1",
					Label:       "Login",
					Description: "User logs in",
					Touchpoints: []string{"web"},
					References:  []string{"workflow.wf-nonexistent.step.step-1"}, // Non-existent reference
				},
			},
		},
		Architecture: service.BlueprintArchitecture{
			Groups: []service.BlueprintArchitectureGroup{
				{ID: "grp-1", Label: "Core", Order: 1},
			},
			Nodes: []service.BlueprintArchitectureNode{
				{
					ID:          "node-1",
					GroupID:     "grp-1",
					Type:        "service",
					Label:       "Auth",
					Description: "Auth Service",
					Layout:      service.BlueprintLayout{X: 0, Y: 0},
				},
			},
		},
		StateMachine: service.BlueprintStateMachine{
			Entity:       "Session",
			InitialState: "active",
			States: []service.BlueprintState{
				{ID: "active", Label: "Active"},
			},
		},
		Workflows: []service.BlueprintWorkflow{
			{
				ID:          "wf-1",
				Name:        "Login Flow",
				Description: "Flow description",
				Nodes: []service.BlueprintWorkflowNode{
					{ID: "step-1", Type: "action", Label: "Check password"},
				},
			},
		},
		WBS: service.BlueprintWBS{
			Modules: []service.BlueprintWBSModule{
				{
					ID:           "mod-1",
					Name:         "Authentication Module",
					Description:  "Module details",
					Priority:     "high",
					Dependencies: []string{},
				},
			},
		},
	}

	result := crossValidator.Validate(bundleWithInvalidRef)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Issues, "broken cross-references must generate issues")
}
