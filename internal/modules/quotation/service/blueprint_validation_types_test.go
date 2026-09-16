package service

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlueprintValidationTypesSerializeCanonicalJSON(t *testing.T) {
	bundle := BlueprintBundle{
		SchemaVersion: BlueprintSchemaVersion,
		Title:         "Sample System Architecture",
		Summary:       "Executive summary for architecture",
		Journey: BlueprintJourney{
			Phases: []BlueprintJourneyPhase{
				{ID: "phase-1", Label: "Onboarding", Order: 1},
			},
			Actors: []BlueprintJourneyActor{
				{ID: "actor-1", Label: "End User", Order: 1},
			},
			Activities: []BlueprintJourneyActivity{
				{
					ID:          "act-1",
					PhaseID:     "phase-1",
					ActorID:     "actor-1",
					Label:       "Sign Up",
					Description: "Registers an account",
					Touchpoints: []string{"web-app"},
					References:  []string{"journey:phase-1"},
				},
			},
		},
		Architecture: BlueprintArchitecture{
			Groups: []BlueprintArchitectureGroup{
				{ID: "grp-1", Label: "Backend Services", Order: 1},
			},
			Nodes: []BlueprintArchitectureNode{
				{
					ID:          "node-1",
					GroupID:     "grp-1",
					Type:        "service",
					Label:       "Auth Service",
					Description: "Handles authentication",
					Layout:      BlueprintLayout{X: 100.5, Y: 200.0},
				},
			},
			Edges: []BlueprintArchitectureEdge{
				{
					ID:     "edge-1",
					Source: "node-1",
					Target: "node-1",
					Label:  "self-check",
					Type:   "sync",
				},
			},
		},
		StateMachine: BlueprintStateMachine{
			Entity:       "Account",
			InitialState: "state-pending",
			States: []BlueprintState{
				{
					ID:     "state-pending",
					Label:  "Pending",
					Type:   "normal",
					Layout: BlueprintLayout{X: 50.0, Y: 50.0},
				},
			},
			Transitions: []BlueprintTransition{
				{
					ID:      "trans-1",
					Source:  "state-pending",
					Target:  "state-pending",
					Event:   "VERIFY",
					Actor:   "system",
					Guard:   "token_valid",
					Actions: []string{"send_welcome_email"},
				},
			},
		},
		Workflows: []BlueprintWorkflow{
			{
				ID:          "wf-1",
				Name:        "User Registration Workflow",
				Description: "Flow from registration to activation",
				Lanes: []BlueprintWorkflowLane{
					{ID: "lane-1", Label: "User", Order: 1},
				},
				Nodes: []BlueprintWorkflowNode{
					{
						ID:          "wnode-1",
						LaneID:      "lane-1",
						Type:        "start",
						Label:       "Start",
						Description: "Start registration",
						Layout:      BlueprintLayout{X: 10.0, Y: 10.0},
					},
				},
				Edges: []BlueprintWorkflowEdge{
					{
						ID:     "wedge-1",
						Source: "wnode-1",
						Target: "wnode-1",
						Label:  "step",
					},
				},
			},
		},
		WBS: BlueprintWBS{
			Modules: []BlueprintWBSModule{
				{
					ID:           "mod-1",
					Name:         "User Management",
					Description:  "User CRUD and Auth",
					Priority:     "high",
					Dependencies: []string{},
					References:   []string{"architecture:node-1"},
					Features: []BlueprintWBSFeature{
						{
							ID:                 "feat-1",
							Name:               "Registration API",
							Description:        "Endpoint to register user",
							AcceptanceCriteria: []string{"Returns 201 Created on valid payload"},
						},
					},
				},
			},
		},
		Assumptions: []string{"Standard internet connectivity"},
		OpenQuestions: []BlueprintOpenQuestion{
			{
				ID:            "oq-1",
				Question:      "Will SMS 2FA be supported in Phase 1?",
				TargetSection: "general",
				Impact:        "Scope expansion",
				Status:        "open",
			},
		},
		CrossReferences: []BlueprintCrossReference{
			{
				ID:           "xref-1",
				SourceRef:    "wbs:mod-1",
				TargetRef:    "architecture:node-1",
				RelationType: "implements",
				Description:  "User Management module implements Auth Service",
			},
		},
	}

	marshaled, err := json.Marshal(bundle)
	require.NoError(t, err)

	marshaledStr := string(marshaled)
	require.Contains(t, marshaledStr, `"schema_version":"1.0"`)
	require.Contains(t, marshaledStr, `"state_machine":`)
	require.Contains(t, marshaledStr, `"workflows":`)

	// Must reject singular workflow key at top level
	var rawMap map[string]json.RawMessage
	err = json.Unmarshal(marshaled, &rawMap)
	require.NoError(t, err)
	_, hasSingularWorkflow := rawMap["workflow"]
	require.False(t, hasSingularWorkflow, "BlueprintBundle JSON must not serialize top-level 'workflow' singular")
	_, hasPluralWorkflows := rawMap["workflows"]
	require.True(t, hasPluralWorkflows, "BlueprintBundle JSON must serialize top-level 'workflows' plural")

	// Must reject any R&D / provider-specific leaks
	forbiddenStrings := []string{
		"prompt_tokens",
		"candidate_tokens",
		"provider",
		"model",
		"estimated_cost_usd",
		"raw_response",
		"prompt",
	}
	for _, forbidden := range forbiddenStrings {
		require.NotContains(t, marshaledStr, `"`+forbidden+`":`, "JSON should not contain forbidden key: %s", forbidden)
	}
}

func TestBlueprintValidationResultHidesNormalizedContent(t *testing.T) {
	result := BlueprintValidationResult{
		Valid:       false,
		SafeOutcome: BlueprintValidationInvalidSchema,
		Issues: []BlueprintValidationIssue{
			{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     "journey",
				Path:        "/journey/phases/0/id",
				TargetID:    "",
				Message:     "Phase is missing required ID",
				Blocking:    true,
				Remediation: "Provide a unique phase-ID",
			},
		},
		NormalizedContent: json.RawMessage(`{"secret_internal_key":"do_not_leak_in_json"}`),
	}

	marshaled, err := json.Marshal(result)
	require.NoError(t, err)

	marshaledStr := string(marshaled)
	require.NotContains(t, marshaledStr, "secret_internal_key")
	require.NotContains(t, marshaledStr, "do_not_leak_in_json")
	require.NotContains(t, strings.ToLower(marshaledStr), "normalizedcontent")
	require.NotContains(t, strings.ToLower(marshaledStr), "normalized_content")
	require.Contains(t, marshaledStr, `"valid":false`)
	require.Contains(t, marshaledStr, `"safe_outcome":"invalid_schema"`)
	require.Contains(t, marshaledStr, `"ERR_MISSING_ID"`)
}

func TestBlueprintValidationContractsExcludeRNDProviderFields(t *testing.T) {
	typesToCheck := []reflect.Type{
		reflect.TypeOf(BlueprintBundle{}),
		reflect.TypeOf(BlueprintValidationResult{}),
		reflect.TypeOf(BlueprintValidationIssue{}),
		reflect.TypeOf(BlueprintCrossValidationResult{}),
		reflect.TypeOf(BlueprintCoverage{}),
		reflect.TypeOf(BlueprintJourney{}),
		reflect.TypeOf(BlueprintArchitecture{}),
		reflect.TypeOf(BlueprintStateMachine{}),
		reflect.TypeOf(BlueprintWorkflow{}),
		reflect.TypeOf(BlueprintWBS{}),
	}

	forbiddenWords := []string{
		"token", "provider", "model", "cost", "rawresponse", "runtime", "rnd",
	}

	for _, typ := range typesToCheck {
		t.Run(typ.Name(), func(t *testing.T) {
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				fieldNameLower := strings.ToLower(field.Name)
				for _, fw := range forbiddenWords {
					require.False(
						t,
						strings.Contains(fieldNameLower, fw),
						"Type %s has forbidden field name: %s (matched %s)",
						typ.Name(), field.Name, fw,
					)
				}

				jsonTag := field.Tag.Get("json")
				jsonTagLower := strings.ToLower(jsonTag)
				for _, fw := range forbiddenWords {
					require.False(
						t,
						strings.Contains(jsonTagLower, fw),
						"Type %s field %s has forbidden json tag: %s (matched %s)",
						typ.Name(), field.Name, jsonTag, fw,
					)
				}
			}
		})
	}
}

func TestBlueprintApprovedFixturesDecodeStrictly(t *testing.T) {
	fixturePaths := []string{
		filepath.Join("testdata", "blueprint", "representative-simple-10.json"),
		filepath.Join("testdata", "blueprint", "representative-complex-01.json"),
	}

	for _, fp := range fixturePaths {
		t.Run(filepath.Base(fp), func(t *testing.T) {
			data, err := os.ReadFile(fp)
			require.NoError(t, err)
			require.NotEmpty(t, data)

			dec := json.NewDecoder(bytes.NewReader(data))
			dec.DisallowUnknownFields()

			var bundle BlueprintBundle
			err = dec.Decode(&bundle)
			require.NoError(t, err, "Fixture %s should decode strictly into BlueprintBundle", fp)

			// Asserts EOF (exactly one value in stream)
			var extra interface{}
			err = dec.Decode(&extra)
			require.Equal(t, io.EOF, err, "Expected EOF after decoding bundle from %s", fp)

			// Verify mandatory fields
			require.Equal(t, BlueprintSchemaVersion, bundle.SchemaVersion)
			require.NotEmpty(t, bundle.Title)
			require.NotEmpty(t, bundle.Summary)
			require.NotEmpty(t, bundle.Journey.Phases)
			require.NotEmpty(t, bundle.Journey.Actors)
			require.NotEmpty(t, bundle.Journey.Activities)
			require.NotEmpty(t, bundle.Architecture.Groups)
			require.NotEmpty(t, bundle.Architecture.Nodes)
			require.NotEmpty(t, bundle.Architecture.Edges)
			require.NotEmpty(t, bundle.StateMachine.Entity)
			require.NotEmpty(t, bundle.StateMachine.InitialState)
			require.NotEmpty(t, bundle.StateMachine.States)
			require.NotEmpty(t, bundle.StateMachine.Transitions)
			require.NotEmpty(t, bundle.Workflows)
			require.NotEmpty(t, bundle.WBS.Modules)
			require.NotEmpty(t, bundle.Assumptions)
			require.NotEmpty(t, bundle.OpenQuestions)
			require.NotEmpty(t, bundle.CrossReferences)
		})
	}
}
