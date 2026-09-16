package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func testSampleJourneyNormalized() json.RawMessage {
	return json.RawMessage(`{
		"journey": {
			"phases": [{"id": "phase-1", "label": "Discovery", "order": 1}],
			"actors": [{"id": "actor-1", "label": "Client", "order": 1}],
			"activities": [{"id": "act-1", "phase_id": "phase-1", "actor_id": "actor-1", "label": "Initial Meeting", "description": "Discuss project scope"}]
		}
	}`)
}

func testSampleArchitectureNormalized() json.RawMessage {
	return json.RawMessage(`{
		"architecture": {
			"groups": [{"id": "grp-1", "label": "Group 1", "order": 1}],
			"nodes": [{"id": "node-1", "group_id": "grp-1", "type": "service", "label": "Service 1", "description": "Desc", "layout": {"x": 100.5, "y": 200.5}}],
			"edges": [{"id": "edge-1", "source": "node-1", "target": "node-1", "label": "Loopback", "type": "sync"}]
		}
	}`)
}

func testSampleStateMachineNormalized() json.RawMessage {
	return json.RawMessage(`{
		"state_machine": {
			"entity": "Order",
			"initial_state": "state-created",
			"states": [
				{"id": "state-created", "label": "Created", "type": "normal", "layout": {"x": 0, "y": 0}},
				{"id": "state-completed", "label": "Completed", "type": "terminal", "layout": {"x": 100, "y": 100}}
			],
			"transitions": [
				{"id": "trans-1", "source": "state-created", "target": "state-completed", "event": "COMPLETE", "actor": "admin", "guard": "paid", "actions": ["notify"]}
			]
		}
	}`)
}

func testSampleWorkflowNormalized() json.RawMessage {
	return json.RawMessage(`{
		"workflows": [{
			"id": "wf-main",
			"name": "Main Flow",
			"description": "Primary process workflow",
			"lanes": [{"id": "lane-1", "label": "Customer", "order": 1}],
			"nodes": [
				{"id": "node-start", "lane_id": "lane-1", "type": "start", "label": "Start", "description": "", "layout": {"x": 0, "y": 0}}
			],
			"edges": []
		}]
	}`)
}

func testSampleWBSNormalized() json.RawMessage {
	return json.RawMessage(`{
		"wbs": {
			"modules": [
				{
					"id": "mod-auth",
					"name": "Auth Module",
					"description": "Authentication and authorization",
					"priority": "high",
					"dependencies": [],
					"references": ["architecture.node.node-1"],
					"features": [
						{
							"id": "feat-login",
							"name": "User Login",
							"description": "Login with password",
							"acceptance_criteria": ["Successful login returns JWT"]
						}
					]
				}
			]
		},
		"cross_references": [
			{
				"id": "xref-1",
				"source_ref": "journey.activity.act-1",
				"target_ref": "wbs.module.mod-auth",
				"relation_type": "implements",
				"description": "Activity implements auth"
			}
		]
	}`)
}

func testCompleteValidBundle() json.RawMessage {
	return json.RawMessage(`{
		"schema_version": "1.0",
		"title": "Banking Blueprint",
		"summary": "Enterprise Core Banking Solution",
		"journey": {
			"phases": [{"id": "phase-1", "label": "Discovery", "order": 1}],
			"actors": [{"id": "actor-1", "label": "Client", "order": 1}],
			"activities": [{"id": "act-1", "phase_id": "phase-1", "actor_id": "actor-1", "label": "Initial Meeting", "description": "Discuss project scope"}]
		},
		"architecture": {
			"groups": [{"id": "grp-1", "label": "Group 1", "order": 1}],
			"nodes": [{"id": "node-1", "group_id": "grp-1", "type": "service", "label": "Service 1", "description": "Desc", "layout": {"x": 100.5, "y": 200.5}}],
			"edges": [{"id": "edge-1", "source": "node-1", "target": "node-1", "label": "Loopback", "type": "sync"}]
		},
		"state_machine": {
			"entity": "Order",
			"initial_state": "state-created",
			"states": [
				{"id": "state-created", "label": "Created", "type": "normal", "layout": {"x": 0, "y": 0}},
				{"id": "state-completed", "label": "Completed", "type": "terminal", "layout": {"x": 100, "y": 100}}
			],
			"transitions": [
				{"id": "trans-1", "source": "state-created", "target": "state-completed", "event": "COMPLETE", "actor": "admin", "guard": "paid", "actions": ["notify"]}
			]
		},
		"workflows": [{
			"id": "wf-main",
			"name": "Main Flow",
			"description": "Primary process workflow",
			"lanes": [{"id": "lane-1", "label": "Customer", "order": 1}],
			"nodes": [
				{"id": "node-start", "lane_id": "lane-1", "type": "start", "label": "Start", "description": "", "layout": {"x": 0, "y": 0}}
			],
			"edges": []
		}],
		"wbs": {
			"modules": [
				{
					"id": "mod-auth",
					"name": "Auth Module",
					"description": "Authentication and authorization",
					"priority": "high",
					"dependencies": [],
					"references": ["architecture.node.node-1"],
					"features": [
						{
							"id": "feat-login",
							"name": "User Login",
							"description": "Login with password",
							"acceptance_criteria": ["Successful login returns JWT"]
						}
					]
				}
			]
		},
		"assumptions": [],
		"open_questions": [],
		"cross_references": [
			{
				"id": "xref-1",
				"source_ref": "journey.activity.act-1",
				"target_ref": "wbs.module.mod-auth",
				"relation_type": "implements",
				"description": "Activity implements auth"
			}
		]
	}`)
}

func TestMergeBlueprintSectionBuildsCanonicalSkeleton(t *testing.T) {
	current := json.RawMessage(`{}`)
	title := "Banking Blueprint"
	contextContent := json.RawMessage(`{"title":"Banking Blueprint","summary":"Enterprise Core Banking Solution"}`)

	merged, err := MergeBlueprintSection(
		current,
		title,
		contextContent,
		domain.BlueprintSectionJourney,
		testSampleJourneyNormalized(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, merged)

	var rawMap map[string]any
	err = json.Unmarshal(merged, &rawMap)
	require.NoError(t, err)

	assert.Equal(t, "1.0", rawMap["schema_version"])
	assert.Equal(t, "Banking Blueprint", rawMap["title"])
	assert.Equal(t, "Enterprise Core Banking Solution", rawMap["summary"])
	assert.NotNil(t, rawMap["assumptions"])
	assert.NotNil(t, rawMap["open_questions"])
	assert.NotNil(t, rawMap["journey"])

	// Assert other generated sections are not present
	assert.Nil(t, rawMap["architecture"])
	assert.Nil(t, rawMap["state_machine"])
	assert.Nil(t, rawMap["workflows"])
	assert.Nil(t, rawMap["wbs"])
	assert.Nil(t, rawMap["cross_references"])

	// Test with nil/empty current
	mergedFromEmpty, err := MergeBlueprintSection(
		nil,
		title,
		contextContent,
		domain.BlueprintSectionJourney,
		testSampleJourneyNormalized(),
	)
	require.NoError(t, err)
	assert.JSONEq(t, string(merged), string(mergedFromEmpty))
}

func TestMergeBlueprintSectionReplacesOnlyTarget(t *testing.T) {
	title := "Banking Blueprint"
	contextContent := json.RawMessage(`{"title":"Banking Blueprint","summary":"Enterprise Core Banking Solution"}`)

	// First merge Journey
	mergedJourney, err := MergeBlueprintSection(
		json.RawMessage(`{}`),
		title,
		contextContent,
		domain.BlueprintSectionJourney,
		testSampleJourneyNormalized(),
	)
	require.NoError(t, err)

	// Next merge Architecture
	mergedArch, err := MergeBlueprintSection(
		mergedJourney,
		title,
		contextContent,
		domain.BlueprintSectionArchitecture,
		testSampleArchitectureNormalized(),
	)
	require.NoError(t, err)

	var rawMap map[string]any
	err = json.Unmarshal(mergedArch, &rawMap)
	require.NoError(t, err)

	// Both journey and architecture must exist
	assert.NotNil(t, rawMap["journey"])
	assert.NotNil(t, rawMap["architecture"])

	// Later sections must still be absent
	assert.Nil(t, rawMap["state_machine"])
	assert.Nil(t, rawMap["workflows"])
	assert.Nil(t, rawMap["wbs"])
	assert.Nil(t, rawMap["cross_references"])

	// Verify Journey was preserved semantically
	var jEnv struct {
		Journey BlueprintJourney `json:"journey"`
	}
	err = json.Unmarshal(mergedArch, &jEnv)
	require.NoError(t, err)
	assert.Equal(t, "phase-1", jEnv.Journey.Phases[0].ID)
}

func TestMergeBlueprintSectionMapsWorkflowToWorkflows(t *testing.T) {
	title := "Banking Blueprint"
	contextContent := json.RawMessage(`{"title":"Banking Blueprint","summary":"Enterprise Core Banking Solution"}`)

	merged, err := MergeBlueprintSection(
		json.RawMessage(`{}`),
		title,
		contextContent,
		domain.BlueprintSectionWorkflow,
		testSampleWorkflowNormalized(),
	)
	require.NoError(t, err)

	mergedStr := string(merged)
	assert.True(t, strings.Contains(mergedStr, `"workflows"`), "must contain 'workflows' plural")
	assert.False(t, strings.Contains(mergedStr, `"workflow"`), "must not contain 'workflow' singular")

	var rawMap map[string]any
	err = json.Unmarshal(merged, &rawMap)
	require.NoError(t, err)

	wfs, ok := rawMap["workflows"].([]any)
	require.True(t, ok)
	assert.Len(t, wfs, 1)
}

func TestMergeBlueprintSectionMapsWBSAndCrossReferencesTogether(t *testing.T) {
	title := "Banking Blueprint"
	contextContent := json.RawMessage(`{"title":"Banking Blueprint","summary":"Enterprise Core Banking Solution"}`)

	merged, err := MergeBlueprintSection(
		json.RawMessage(`{}`),
		title,
		contextContent,
		domain.BlueprintSectionWBS,
		testSampleWBSNormalized(),
	)
	require.NoError(t, err)

	var rawMap map[string]any
	err = json.Unmarshal(merged, &rawMap)
	require.NoError(t, err)

	assert.NotNil(t, rawMap["wbs"], "wbs must be present")
	assert.NotNil(t, rawMap["cross_references"], "cross_references must be present")

	xrefs, ok := rawMap["cross_references"].([]any)
	require.True(t, ok)
	assert.Len(t, xrefs, 1)
}

func TestMergeBlueprintSectionRejectsMalformedCurrentContent(t *testing.T) {
	title := "Banking Blueprint"
	contextContent := json.RawMessage(`{"title":"Banking Blueprint","summary":"Enterprise Core Banking Solution"}`)

	t.Run("syntax_error", func(t *testing.T) {
		_, err := MergeBlueprintSection(
			json.RawMessage(`{invalid json`),
			title,
			contextContent,
			domain.BlueprintSectionJourney,
			testSampleJourneyNormalized(),
		)
		assert.Error(t, err)
	})

	t.Run("unknown_field_in_current", func(t *testing.T) {
		_, err := MergeBlueprintSection(
			json.RawMessage(`{"schema_version":"1.0","title":"T","summary":"S","unknown_prop":"foo"}`),
			title,
			contextContent,
			domain.BlueprintSectionJourney,
			testSampleJourneyNormalized(),
		)
		assert.Error(t, err)
	})

	t.Run("unsupported_schema_version", func(t *testing.T) {
		_, err := MergeBlueprintSection(
			json.RawMessage(`{"schema_version":"2.0","title":"T","summary":"S"}`),
			title,
			contextContent,
			domain.BlueprintSectionJourney,
			testSampleJourneyNormalized(),
		)
		assert.Error(t, err)
	})

	t.Run("rejects_context_section", func(t *testing.T) {
		_, err := MergeBlueprintSection(
			json.RawMessage(`{}`),
			title,
			contextContent,
			domain.BlueprintSectionContext,
			testSampleJourneyNormalized(),
		)
		assert.Error(t, err)
	})

	t.Run("rejects_cross_validation_section", func(t *testing.T) {
		_, err := MergeBlueprintSection(
			json.RawMessage(`{}`),
			title,
			contextContent,
			domain.BlueprintSectionCrossValidation,
			testSampleJourneyNormalized(),
		)
		assert.Error(t, err)
	})

	t.Run("malformed_normalized_section", func(t *testing.T) {
		_, err := MergeBlueprintSection(
			json.RawMessage(`{}`),
			title,
			contextContent,
			domain.BlueprintSectionJourney,
			json.RawMessage(`{bad json`),
		)
		assert.Error(t, err)
	})
}

func TestInvalidateBlueprintDownstreamRemovesOnlyDerivedSections(t *testing.T) {
	complete := testCompleteValidBundle()

	t.Run("invalidate_from_architecture", func(t *testing.T) {
		newContent, invalidated, err := InvalidateBlueprintDownstream(complete, domain.BlueprintSectionArchitecture)
		require.NoError(t, err)

		expectedInvalidated := []domain.BlueprintSectionType{
			domain.BlueprintSectionStateMachine,
			domain.BlueprintSectionWorkflow,
			domain.BlueprintSectionWBS,
			domain.BlueprintSectionCrossValidation,
		}
		assert.Equal(t, expectedInvalidated, invalidated)

		var rawMap map[string]any
		err = json.Unmarshal(newContent, &rawMap)
		require.NoError(t, err)

		// Preserved
		assert.NotNil(t, rawMap["journey"])
		assert.NotNil(t, rawMap["architecture"])
		assert.Equal(t, "Banking Blueprint", rawMap["title"])

		// Removed
		assert.Nil(t, rawMap["state_machine"])
		assert.Nil(t, rawMap["workflows"])
		assert.Nil(t, rawMap["wbs"])
		assert.Nil(t, rawMap["cross_references"])
	})

	t.Run("invalidate_from_workflow", func(t *testing.T) {
		newContent, invalidated, err := InvalidateBlueprintDownstream(complete, domain.BlueprintSectionWorkflow)
		require.NoError(t, err)

		expectedInvalidated := []domain.BlueprintSectionType{
			domain.BlueprintSectionWBS,
			domain.BlueprintSectionCrossValidation,
		}
		assert.Equal(t, expectedInvalidated, invalidated)

		var rawMap map[string]any
		err = json.Unmarshal(newContent, &rawMap)
		require.NoError(t, err)

		// Preserved
		assert.NotNil(t, rawMap["journey"])
		assert.NotNil(t, rawMap["architecture"])
		assert.NotNil(t, rawMap["state_machine"])
		assert.NotNil(t, rawMap["workflows"])

		// Removed
		assert.Nil(t, rawMap["wbs"])
		assert.Nil(t, rawMap["cross_references"])
	})

	t.Run("invalidate_from_wbs", func(t *testing.T) {
		newContent, invalidated, err := InvalidateBlueprintDownstream(complete, domain.BlueprintSectionWBS)
		require.NoError(t, err)

		expectedInvalidated := []domain.BlueprintSectionType{
			domain.BlueprintSectionCrossValidation,
		}
		assert.Equal(t, expectedInvalidated, invalidated)

		var rawMap map[string]any
		err = json.Unmarshal(newContent, &rawMap)
		require.NoError(t, err)

		// WBS and its cross_references are target, so they are preserved
		assert.NotNil(t, rawMap["wbs"])
		assert.NotNil(t, rawMap["cross_references"])
	})

	t.Run("rejects_invalid_section_type", func(t *testing.T) {
		_, _, err := InvalidateBlueprintDownstream(complete, domain.BlueprintSectionContext)
		assert.Error(t, err)

		_, _, err = InvalidateBlueprintDownstream(complete, domain.BlueprintSectionCrossValidation)
		assert.Error(t, err)
	})
}

func TestDecodeBlueprintBundleIsStrictAndDeterministic(t *testing.T) {
	complete := testCompleteValidBundle()

	t.Run("valid_bundle", func(t *testing.T) {
		bundle, err := DecodeBlueprintBundle(complete)
		require.NoError(t, err)
		assert.Equal(t, "1.0", bundle.SchemaVersion)
		assert.Equal(t, "Banking Blueprint", bundle.Title)
		assert.Len(t, bundle.Journey.Phases, 1)
		assert.Len(t, bundle.Architecture.Nodes, 1)
		assert.Len(t, bundle.StateMachine.States, 2)
		assert.Len(t, bundle.Workflows, 1)
		assert.Len(t, bundle.WBS.Modules, 1)
		assert.Len(t, bundle.CrossReferences, 1)
	})

	t.Run("unknown_field_rejected", func(t *testing.T) {
		extra := `{"schema_version":"1.0","title":"T","summary":"S","journey":{"phases":[],"actors":[],"activities":[]},"architecture":{"groups":[],"nodes":[],"edges":[]},"state_machine":{"entity":"E","initial_state":"s","states":[],"transitions":[]},"workflows":[],"wbs":{"modules":[]},"assumptions":[],"open_questions":[],"cross_references":[],"extra_field":"bad"}`
		_, err := DecodeBlueprintBundle(json.RawMessage(extra))
		assert.Error(t, err)
	})

	t.Run("unsupported_schema_version", func(t *testing.T) {
		badVer := `{"schema_version":"2.0","title":"T","summary":"S","journey":{"phases":[],"actors":[],"activities":[]},"architecture":{"groups":[],"nodes":[],"edges":[]},"state_machine":{"entity":"E","initial_state":"s","states":[],"transitions":[]},"workflows":[],"wbs":{"modules":[]},"assumptions":[],"open_questions":[],"cross_references":[]}`
		_, err := DecodeBlueprintBundle(json.RawMessage(badVer))
		assert.Error(t, err)
	})

	t.Run("trailing_tokens_rejected", func(t *testing.T) {
		trailing := string(complete) + " trailing"
		_, err := DecodeBlueprintBundle(json.RawMessage(trailing))
		assert.Error(t, err)
	})
}
