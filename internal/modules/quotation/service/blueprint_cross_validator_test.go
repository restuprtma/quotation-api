package service

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleValidBundleForSymbols() BlueprintBundle {
	return BlueprintBundle{
		SchemaVersion: BlueprintSchemaVersion,
		Title:         "Sample System Architecture",
		Summary:       "Executive summary for architecture",
		Journey: BlueprintJourney{
			Phases: []BlueprintJourneyPhase{
				{ID: "phase-1", Label: "Onboarding", Order: 1},
			},
			Actors: []BlueprintJourneyActor{
				{ID: "actor-1", Label: "Customer", Order: 1},
			},
			Activities: []BlueprintJourneyActivity{
				{
					ID:          "act-1",
					PhaseID:     "phase-1",
					ActorID:     "actor-1",
					Label:       "Sign Up",
					Description: "Registers customer account",
					Touchpoints: []string{"web"},
					References:  []string{"workflow.wf-1.step.wnode-1"},
				},
			},
		},
		Architecture: BlueprintArchitecture{
			Groups: []BlueprintArchitectureGroup{
				{ID: "grp-1", Label: "Backend", Order: 1},
			},
			Nodes: []BlueprintArchitectureNode{
				{
					ID:          "node-1",
					GroupID:     "grp-1",
					Type:        "service",
					Label:       "Auth Service",
					Description: "Authentication API",
					Layout:      BlueprintLayout{X: 100, Y: 100},
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
			Entity:       "Order",
			InitialState: "state-1",
			States: []BlueprintState{
				{ID: "state-1", Label: "Draft", Type: "initial", Layout: BlueprintLayout{X: 50, Y: 50}},
			},
			Transitions: []BlueprintTransition{
				{
					ID:      "trans-1",
					Source:  "state-1",
					Target:  "state-1",
					Event:   "SUBMIT",
					Actor:   "Customer",
					Guard:   "valid",
					Actions: []string{"notify"},
				},
			},
		},
		Workflows: []BlueprintWorkflow{
			{
				ID:          "wf-1",
				Name:        "Order Flow",
				Description: "End to end flow",
				Lanes: []BlueprintWorkflowLane{
					{ID: "lane-1", Label: "Customer", Order: 1},
				},
				Nodes: []BlueprintWorkflowNode{
					{
						ID:          "wnode-1",
						LaneID:      "lane-1",
						Type:        "action",
						Label:       "Start Order",
						Description: "Click start",
						Layout:      BlueprintLayout{X: 10, Y: 10},
					},
				},
				Edges: []BlueprintWorkflowEdge{
					{
						ID:     "wedge-1",
						Source: "wnode-1",
						Target: "wnode-1",
						Label:  "loop",
					},
				},
			},
		},
		WBS: BlueprintWBS{
			Modules: []BlueprintWBSModule{
				{
					ID:           "mod-1",
					Name:         "Auth Module",
					Description:  "Auth implementation",
					Priority:     "high",
					Dependencies: []string{},
					References:   []string{"architecture.node.node-1"},
					Features: []BlueprintWBSFeature{
						{
							ID:                 "feat-1",
							Name:               "Login API",
							Description:        "POST /login",
							AcceptanceCriteria: []string{"Returns 200 on success"},
						},
					},
				},
			},
		},
		Assumptions: []string{"Cloud hosted"},
		OpenQuestions: []BlueprintOpenQuestion{
			{
				ID:            "oq-1",
				Question:      "Support OAuth?",
				TargetSection: "architecture",
				Impact:        "Medium",
				Status:        "open",
			},
		},
		CrossReferences: []BlueprintCrossReference{
			{
				ID:           "xref-1",
				SourceRef:    "journey.activity.act-1",
				TargetRef:    "workflow.wf-1.step.wnode-1",
				RelationType: "triggers",
				Description:  "Sign up triggers start order",
			},
		},
	}
}

func TestBlueprintCrossValidatorCanonicalReferences(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()

	t.Run("resolves_full_canonical_names", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		// References already use full canonical:
		// act-1 references workflow.wf-1.step.wnode-1
		// mod-1 references architecture.node.node-1
		// xref-1 connects journey.activity.act-1 to workflow.wf-1.step.wnode-1
		res := v.Validate(bundle)
		assert.True(t, res.Valid, "Bundle with valid canonical references should be valid")
		assert.Empty(t, res.SafeOutcome)

		for _, iss := range res.Issues {
			assert.False(t, iss.Blocking, "No blocking issues should exist for resolvable canonical references: %v", iss)
		}
	})

	t.Run("resolves_unique_short_aliases", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		// Use short aliases instead of full canonical
		bundle.Journey.Activities[0].References = []string{"wnode-1"}
		bundle.WBS.Modules[0].References = []string{"node-1"}
		bundle.CrossReferences[0].SourceRef = "act-1"
		bundle.CrossReferences[0].TargetRef = "state-1"

		res := v.Validate(bundle)
		assert.True(t, res.Valid, "Bundle with unique short aliases should resolve without blocking errors")
		assert.Empty(t, res.SafeOutcome)

		for _, iss := range res.Issues {
			assert.False(t, iss.Blocking, "Unique short aliases must not produce blocking issues: %v", iss)
		}
	})

	t.Run("resolves_intermediate_aliases", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		bundle.Journey.Activities[0].References = []string{"workflow.step.wnode-1"}
		bundle.WBS.Modules[0].References = []string{"node-1"}
		bundle.CrossReferences[0].SourceRef = "act-1"
		bundle.CrossReferences[0].TargetRef = "state.state-1"

		res := v.Validate(bundle)
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)

		for _, iss := range res.Issues {
			assert.False(t, iss.Blocking, "Intermediate aliases must not produce blocking errors: %v", iss)
		}
	})
}

func TestBlueprintCrossValidatorRejectsAmbiguousShortAlias(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()

	t.Run("ambiguous_raw_id_in_journey_and_architecture", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		// Re-use "shared-id" in journey activity AND architecture node
		bundle.Journey.Activities[0].ID = "shared-id"
		bundle.Architecture.Nodes[0].ID = "shared-id"
		// Reference the ambiguous short alias in WBS module
		bundle.WBS.Modules[0].References = []string{"shared-id"}

		res := v.Validate(bundle)
		assert.False(t, res.Valid)
		assert.Equal(t, BlueprintValidationCrossValidationError, res.SafeOutcome)

		foundAmbiguous := false
		for _, iss := range res.Issues {
			if iss.Code == BlueprintIssueAmbiguousReference {
				foundAmbiguous = true
				assert.Equal(t, "BLOCKING_ERROR", iss.Severity)
				assert.True(t, iss.Blocking)
				assert.Equal(t, "/wbs/modules/0/references/0", iss.Path)
				assert.Equal(t, "mod-1", iss.TargetID)
				// Message must NOT leak/echo reference text
				assert.NotContains(t, iss.Message, "shared-id")
				assert.NotEmpty(t, iss.Message)
				assert.NotEmpty(t, iss.Remediation)
			}
		}
		assert.True(t, foundAmbiguous, "Expected ERR_AMBIGUOUS_REFERENCE for ambiguous alias")
	})

	t.Run("ambiguous_raw_id_in_cross_references", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		// Re-use "shared-item" in actor and open question
		bundle.Journey.Actors[0].ID = "shared-item"
		bundle.OpenQuestions[0].ID = "shared-item"
		// Reference ambiguous alias in cross reference target_ref
		bundle.CrossReferences[0].TargetRef = "shared-item"

		res := v.Validate(bundle)
		assert.False(t, res.Valid)
		assert.Equal(t, BlueprintValidationCrossValidationError, res.SafeOutcome)

		foundAmbiguous := false
		for _, iss := range res.Issues {
			if iss.Code == BlueprintIssueAmbiguousReference && iss.Path == "/cross_references/0/target_ref" {
				foundAmbiguous = true
				assert.True(t, iss.Blocking)
				assert.Equal(t, "xref-1", iss.TargetID)
				assert.NotContains(t, iss.Message, "shared-item")
			}
		}
		assert.True(t, foundAmbiguous)
	})
}

func TestBlueprintCrossValidatorRejectsDanglingReferences(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()

	t.Run("dangling_in_cross_reference_source_and_target", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		bundle.CrossReferences[0].SourceRef = "journey.activity.ghost-activity"
		bundle.CrossReferences[0].TargetRef = "workflow.ghost-node"

		res := v.Validate(bundle)
		assert.False(t, res.Valid)
		assert.Equal(t, BlueprintValidationCrossValidationError, res.SafeOutcome)

		foundSourceDangling := false
		foundTargetDangling := false
		for _, iss := range res.Issues {
			if iss.Code == BlueprintIssueDanglingReference {
				if iss.Path == "/cross_references/0/source_ref" {
					foundSourceDangling = true
					assert.Equal(t, "xref-1", iss.TargetID)
					assert.NotContains(t, iss.Message, "ghost-activity")
				}
				if iss.Path == "/cross_references/0/target_ref" {
					foundTargetDangling = true
					assert.Equal(t, "xref-1", iss.TargetID)
					assert.NotContains(t, iss.Message, "ghost-node")
				}
			}
		}
		assert.True(t, foundSourceDangling, "Expected dangling reference for source_ref")
		assert.True(t, foundTargetDangling, "Expected dangling reference for target_ref")
	})

	t.Run("dangling_in_activity_references", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		bundle.Journey.Activities[0].References = []string{"nonexistent-ref"}

		res := v.Validate(bundle)
		assert.False(t, res.Valid)
		assert.Equal(t, BlueprintValidationCrossValidationError, res.SafeOutcome)

		found := false
		for _, iss := range res.Issues {
			if iss.Code == BlueprintIssueDanglingReference && iss.Path == "/journey/activities/0/references/0" {
				found = true
				assert.Equal(t, "act-1", iss.TargetID)
				assert.NotContains(t, iss.Message, "nonexistent-ref")
			}
		}
		assert.True(t, found)
	})

	t.Run("dangling_in_wbs_references", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()
		bundle.WBS.Modules[0].References = []string{"architecture.node.ghost-service"}

		res := v.Validate(bundle)
		assert.False(t, res.Valid)
		assert.Equal(t, BlueprintValidationCrossValidationError, res.SafeOutcome)

		found := false
		for _, iss := range res.Issues {
			if iss.Code == BlueprintIssueDanglingReference && iss.Path == "/wbs/modules/0/references/0" {
				found = true
				assert.Equal(t, "mod-1", iss.TargetID)
				assert.NotContains(t, iss.Message, "ghost-service")
			}
		}
		assert.True(t, found)
	})
}

func TestBlueprintCrossValidatorRejectsUnsupportedSchemaVersion(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()

	testCases := []struct {
		name    string
		version string
	}{
		{"empty_version", ""},
		{"unsupported_v2", "2.0"},
		{"v1_beta", "1.0-beta"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := sampleValidBundleForSymbols()
			bundle.SchemaVersion = tc.version

			res := v.Validate(bundle)
			assert.False(t, res.Valid)
			assert.Equal(t, BlueprintValidationCrossValidationError, res.SafeOutcome)
			require.NotEmpty(t, res.Issues)

			found := false
			for _, iss := range res.Issues {
				if iss.Code == BlueprintIssueUnsupportedVersion {
					found = true
					assert.Equal(t, "BLOCKING_ERROR", iss.Severity)
					assert.True(t, iss.Blocking)
					assert.Equal(t, "/schema_version", iss.Path)
					if tc.version != "" {
						assert.NotContains(t, iss.Message, tc.version)
					}
				}
			}
			assert.True(t, found, "Expected ERR_UNSUPPORTED_SCHEMA_VERSION for version: %s", tc.version)
		})
	}
}

func TestBlueprintCrossValidatorWarningsAndCoverage(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()

	t.Run("unreferenced_arch_nodes_and_unlinked_activities", func(t *testing.T) {
		bundle := sampleValidBundleForSymbols()

		// Add eligible unreferenced architecture node
		bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, BlueprintArchitectureNode{
			ID:          "node-db",
			GroupID:     "grp-1",
			Type:        "database", // eligible type
			Label:       "Postgres DB",
			Description: "Main database",
			Layout:      BlueprintLayout{X: 200, Y: 200},
		})

		// Add non-eligible unreferenced node (e.g. cache)
		bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, BlueprintArchitectureNode{
			ID:          "node-cache",
			GroupID:     "grp-1",
			Type:        "cache", // non-eligible type
			Label:       "Redis Cache",
			Description: "Session cache",
			Layout:      BlueprintLayout{X: 300, Y: 300},
		})

		// Add 2 more journey activities: 1 linked via xref, 1 unlinked
		bundle.Journey.Activities = append(bundle.Journey.Activities,
			BlueprintJourneyActivity{
				ID:          "act-2",
				PhaseID:     "phase-1",
				ActorID:     "actor-1",
				Label:       "Place Order",
				Description: "Submit order details",
				Touchpoints: []string{"web"},
				References:  []string{},
			},
			BlueprintJourneyActivity{
				ID:          "act-3",
				PhaseID:     "phase-1",
				ActorID:     "actor-1",
				Label:       "Offline Feedback",
				Description: "Physical feedback form",
				Touchpoints: []string{"physical"},
				References:  []string{},
			},
		)

		// Link act-2 to state machine state via cross references
		bundle.CrossReferences = append(bundle.CrossReferences, BlueprintCrossReference{
			ID:           "xref-2",
			SourceRef:    "journey.activity.act-2",
			TargetRef:    "state.Order.state-1",
			RelationType: "transitions_on",
			Description:  "Place Order transitions to state-1",
		})

		res := v.Validate(bundle)
		// Non-blocking warnings must keep Valid == true
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)

		// Check warnings
		foundArchWarn := false
		foundCacheWarn := false
		foundAct3Warn := false
		foundAct1Warn := false
		foundAct2Warn := false

		for _, iss := range res.Issues {
			if iss.Code == BlueprintWarningArchitectureUncovered {
				if iss.TargetID == "node-db" {
					foundArchWarn = true
					assert.False(t, iss.Blocking)
					assert.Equal(t, "/architecture/nodes/1", iss.Path)
				}
				if iss.TargetID == "node-cache" {
					foundCacheWarn = true
				}
			}
			if iss.Code == BlueprintWarningJourneyUnlinked {
				if iss.TargetID == "act-3" {
					foundAct3Warn = true
					assert.False(t, iss.Blocking)
					assert.Equal(t, "/journey/activities/2", iss.Path)
				}
				if iss.TargetID == "act-1" {
					foundAct1Warn = true
				}
				if iss.TargetID == "act-2" {
					foundAct2Warn = true
				}
			}
		}

		assert.True(t, foundArchWarn, "node-db must produce WARN_ARCH_NODE_UNREFERENCED_IN_WBS")
		assert.False(t, foundCacheWarn, "node-cache (type: cache) must NOT produce WARN_ARCH_NODE_UNREFERENCED_IN_WBS")
		assert.True(t, foundAct3Warn, "act-3 must produce WARN_JOURNEY_ACTIVITY_UNLINKED")
		assert.False(t, foundAct1Warn, "act-1 is linked to workflow step, should not warn")
		assert.False(t, foundAct2Warn, "act-2 is linked to state, should not warn")

		// Coverage assertions:
		// Eligible arch nodes: node-1 (service, referenced), node-db (database, unreferenced) = 1 / 2 = 0.5000
		assert.Equal(t, 0.5, res.Coverage.ArchitectureToWBS)

		// Journey activities: act-1 (linked), act-2 (linked), act-3 (unlinked) = 2 / 3 = 0.6667
		assert.Equal(t, 0.6667, res.Coverage.JourneyToWorkflow)
	})

	t.Run("empty_core_sections_produce_warn_empty_section_in_canonical_order", func(t *testing.T) {
		emptyBundle := BlueprintBundle{
			SchemaVersion: BlueprintSchemaVersion,
			Title:         "Empty Bundle",
			Summary:       "All core sections empty",
		}

		res := v.Validate(emptyBundle)
		assert.True(t, res.Valid, "Empty section warnings must be non-blocking")
		assert.Empty(t, res.SafeOutcome)

		var emptySectionIssues []BlueprintValidationIssue
		for _, iss := range res.Issues {
			if iss.Code == BlueprintWarningEmptySection {
				emptySectionIssues = append(emptySectionIssues, iss)
			}
		}

		require.Len(t, emptySectionIssues, 5, "Must produce WARN_EMPTY_SECTION for all 5 core sections")

		// Verify that all 5 core sections are represented
		expectedSections := []string{"journey", "architecture", "state_machine", "workflow", "wbs"}
		foundSections := make(map[string]bool)
		for _, iss := range emptySectionIssues {
			foundSections[iss.Section] = true
			assert.False(t, iss.Blocking)
			assert.NotEmpty(t, iss.Message)
		}
		for _, expected := range expectedSections {
			assert.True(t, foundSections[expected], "Missing empty section warning for: %s", expected)
		}
	})
}

func TestBlueprintCrossValidatorZeroDenominatorCoverage(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()

	bundle := BlueprintBundle{
		SchemaVersion: BlueprintSchemaVersion,
		Title:         "Zero Denominator Bundle",
		Summary:       "No eligible arch nodes and no journey activities",
		Journey: BlueprintJourney{
			Phases:     []BlueprintJourneyPhase{{ID: "p1", Label: "P", Order: 1}},
			Actors:     []BlueprintJourneyActor{{ID: "a1", Label: "A", Order: 1}},
			Activities: []BlueprintJourneyActivity{}, // 0 activities
		},
		Architecture: BlueprintArchitecture{
			Groups: []BlueprintArchitectureGroup{{ID: "g1", Label: "G", Order: 1}},
			Nodes: []BlueprintArchitectureNode{
				// Only cache node, which is NOT eligible for coverage
				{ID: "n1", GroupID: "g1", Type: "cache", Label: "C", Description: "", Layout: BlueprintLayout{X: 0, Y: 0}},
			},
			Edges: []BlueprintArchitectureEdge{},
		},
		StateMachine: BlueprintStateMachine{
			Entity:       "E",
			InitialState: "s1",
			States:       []BlueprintState{{ID: "s1", Label: "S", Type: "initial", Layout: BlueprintLayout{X: 0, Y: 0}}},
			Transitions:  []BlueprintTransition{},
		},
		Workflows: []BlueprintWorkflow{
			{
				ID:    "wf-1",
				Name:  "W",
				Lanes: []BlueprintWorkflowLane{{ID: "l1", Label: "L", Order: 1}},
				Nodes: []BlueprintWorkflowNode{{ID: "wn1", LaneID: "l1", Type: "action", Label: "A", Layout: BlueprintLayout{X: 0, Y: 0}}},
				Edges: []BlueprintWorkflowEdge{},
			},
		},
		WBS: BlueprintWBS{
			Modules: []BlueprintWBSModule{
				{ID: "m1", Name: "M", Priority: "high", Dependencies: []string{}, References: []string{}, Features: []BlueprintWBSFeature{}},
			},
		},
	}

	res := v.Validate(bundle)
	assert.True(t, res.Valid)

	// Zero denominator must result in numeric 0.0 (semantically 0.0000, not NaN or Inf)
	assert.False(t, math.IsNaN(res.Coverage.JourneyToWorkflow))
	assert.False(t, math.IsInf(res.Coverage.JourneyToWorkflow, 0))
	assert.Equal(t, 0.0, res.Coverage.JourneyToWorkflow)

	assert.False(t, math.IsNaN(res.Coverage.ArchitectureToWBS))
	assert.False(t, math.IsInf(res.Coverage.ArchitectureToWBS, 0))
	assert.Equal(t, 0.0, res.Coverage.ArchitectureToWBS)

	// Verify JSON marshaling does not output "NaN" or "Infinity"
	marshaled, err := json.Marshal(res.Coverage)
	require.NoError(t, err)
	marshaledStr := string(marshaled)
	assert.Contains(t, marshaledStr, `"journey_to_workflow":0`)
	assert.Contains(t, marshaledStr, `"architecture_to_wbs":0`)
	assert.NotContains(t, marshaledStr, "NaN")
	assert.NotContains(t, marshaledStr, "null")
}

func TestBlueprintCrossValidatorDeterministicJSON(t *testing.T) {
	v := NewProductionBlueprintCrossValidator()
	bundle := sampleValidBundleForSymbols()

	// Add complex defects and warnings
	bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, BlueprintArchitectureNode{
		ID:          "node-db",
		GroupID:     "grp-1",
		Type:        "database",
		Label:       "DB",
		Description: "Unreferenced DB",
		Layout:      BlueprintLayout{X: 10, Y: 10},
	})
	bundle.Journey.Activities = append(bundle.Journey.Activities, BlueprintJourneyActivity{
		ID:          "act-unlinked",
		PhaseID:     "phase-1",
		ActorID:     "actor-1",
		Label:       "Unlinked Act",
		Description: "No link",
		Touchpoints: []string{"web"},
		References:  []string{},
	})

	var referenceJSON string
	for i := 0; i < 25; i++ {
		res := v.Validate(bundle)
		bytesOut, err := json.Marshal(res)
		require.NoError(t, err)
		strOut := string(bytesOut)

		if i == 0 {
			referenceJSON = strOut
		} else {
			require.Equal(t, referenceJSON, strOut, "Validation result JSON must be byte-identical on iteration %d", i)
		}
	}
}
