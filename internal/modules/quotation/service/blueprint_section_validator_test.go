package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestBlueprintSectionValidatorRejectsInvalidEnvelope(t *testing.T) {
	v := NewBlueprintSectionValidator()

	testCases := []struct {
		name         string
		sectionType  domain.BlueprintSectionType
		payload      json.RawMessage
		expectedPath string
	}{
		{
			name:         "nil_payload",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      nil,
			expectedPath: "",
		},
		{
			name:         "empty_payload",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte(""),
			expectedPath: "",
		},
		{
			name:         "whitespace_payload",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte("   \n\t  "),
			expectedPath: "",
		},
		{
			name:         "malformed_json",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte("{malformed: json"),
			expectedPath: "",
		},
		{
			name:         "trailing_json",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte(`{"journey":{"phases":[],"actors":[],"activities":[]}} extra`),
			expectedPath: "",
		},
		{
			name:         "unknown_top_level_field",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte(`{"journey":{"phases":[],"actors":[],"activities":[]},"unknown_field":"value"}`),
			expectedPath: "/unknown_field",
		},
		{
			name:         "supplied_schema_version_is_unknown_field",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte(`{"journey":{"phases":[],"actors":[],"activities":[]},"schema_version":"1.0"}`),
			expectedPath: "/schema_version",
		},
		{
			name:         "wrong_root_key",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte(`{"architecture":{"groups":[],"nodes":[],"edges":[]}}`),
			expectedPath: "/architecture",
		},
		{
			name:         "workflow_singular_root_key_rejected",
			sectionType:  domain.BlueprintSectionWorkflow,
			payload:      []byte(`{"workflow":[]}`),
			expectedPath: "/workflow",
		},
		{
			name:         "unknown_section_type",
			sectionType:  domain.BlueprintSectionType("unknown_stage"),
			payload:      []byte(`{"unknown":{}}`),
			expectedPath: "",
		},
		{
			name:         "context_section_not_eligible",
			sectionType:  domain.BlueprintSectionContext,
			payload:      []byte(`{"context":{}}`),
			expectedPath: "",
		},
		{
			name:         "cross_validation_section_not_eligible",
			sectionType:  domain.BlueprintSectionCrossValidation,
			payload:      []byte(`{"cross_validation":{}}`),
			expectedPath: "",
		},
		{
			name:         "invalid_utf8",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte("{\"journey\":\xff\xfe}"),
			expectedPath: "",
		},
		{
			name:         "nesting_depth_exceeded",
			sectionType:  domain.BlueprintSectionJourney,
			payload:      []byte(strings.Repeat("{\"a\":", 35) + "1" + strings.Repeat("}", 35)),
			expectedPath: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := v.Validate(tc.sectionType, tc.payload)
			assert.False(t, result.Valid, "expected Valid=false for invalid envelope")
			assert.Equal(t, BlueprintValidationInvalidSchema, result.SafeOutcome)
			assert.Nil(t, result.NormalizedContent, "NormalizedContent must be nil on invalid envelope")
			require.NotEmpty(t, result.Issues, "expected at least one issue")

			found := false
			for _, iss := range result.Issues {
				if iss.Code == BlueprintIssueUnsupportedVersion {
					found = true
					assert.True(t, iss.Blocking)
					if tc.expectedPath != "" {
						assert.Equal(t, tc.expectedPath, iss.Path)
					}
					break
				}
			}
			assert.True(t, found, "expected BlueprintIssueUnsupportedVersion for %s", tc.name)
		})
	}
}

func TestBlueprintSectionValidatorRejectsForbiddenContent(t *testing.T) {
	v := NewBlueprintSectionValidator()

	testCases := []struct {
		name        string
		sectionType domain.BlueprintSectionType
		jsonSnippet string
	}{
		// Commercial keys
		{"commercial_budget", domain.BlueprintSectionJourney, `{"journey":{"phases":[],"actors":[],"activities":[],"budget":1000}}`},
		{"commercial_maximum_budget", domain.BlueprintSectionJourney, `{"journey":{"phases":[],"actors":[],"activities":[],"maximum_budget":5000}}`},
		{"commercial_rate_card", domain.BlueprintSectionArchitecture, `{"architecture":{"groups":[],"nodes":[],"edges":[],"rate_card":{"hourly":100}}}`},
		{"commercial_rates", domain.BlueprintSectionStateMachine, `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"L","type":"normal","layout":{"x":0,"y":0}}],"transitions":[],"rates":[1,2]}}`},
		{"commercial_payment_terms", domain.BlueprintSectionWorkflow, `{"workflows":[],"payment_terms":"net30"}`},
		{"commercial_legal", domain.BlueprintSectionWBS, `{"wbs":{"modules":[]},"cross_references":[],"legal":"confidential"}`},
		{"commercial_signature", domain.BlueprintSectionJourney, `{"journey":{"phases":[],"actors":[],"activities":[],"signature":"sign_here"}}`},

		// Secret and provider keys
		{"secret_api_key", domain.BlueprintSectionJourney, `{"journey":{"phases":[],"actors":[],"activities":[],"api_key":"sk-secret"}}`},
		{"secret_credential", domain.BlueprintSectionArchitecture, `{"architecture":{"groups":[],"nodes":[],"edges":[],"credential":"pass"}}`},
		{"secret_access_token", domain.BlueprintSectionStateMachine, `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"L","type":"normal","layout":{"x":0,"y":0}}],"transitions":[],"access_token":"token"}}`},
		{"provider_system_prompt", domain.BlueprintSectionWorkflow, `{"workflows":[],"system_prompt":"act as assistant"}`},
		{"provider_raw_response", domain.BlueprintSectionWBS, `{"wbs":{"modules":[]},"cross_references":[],"raw_response":"resp"}`},
		{"provider_field", domain.BlueprintSectionJourney, `{"journey":{"phases":[],"actors":[],"activities":[],"provider":"anthropic"}}`},
		{"model_field", domain.BlueprintSectionJourney, `{"journey":{"phases":[],"actors":[],"activities":[],"model":"claude-3-opus"}}`},

		// Forbidden string patterns in string values
		{"string_script_tag", domain.BlueprintSectionJourney, `{"journey":{"phases":[{"id":"phase-1","label":"<script>alert('xss')</script>","order":1}],"actors":[],"activities":[]}}`},
		{"string_onerror_handler", domain.BlueprintSectionJourney, `{"journey":{"phases":[{"id":"phase-1","label":"Normal Label","order":1}],"actors":[],"activities":[{"id":"act-1","phase_id":"phase-1","actor_id":"actor-1","label":"<img src=x onerror=alert(1)>","description":"D","touchpoints":[],"references":[]}]}}`},
		{"string_javascript_uri", domain.BlueprintSectionArchitecture, `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-1","type":"service","label":"N","description":"javascript:evil()","layout":{"x":0,"y":0}}],"edges":[]}}`},
		{"string_data_text_html", domain.BlueprintSectionStateMachine, `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"L","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"t1","source":"s1","target":"s1","event":"data:text/html;base64,PHNjcmlwdD4=","actor":"a","guard":"g","actions":[]}]}}`},
		{"string_triple_backticks", domain.BlueprintSectionWorkflow, "{\"workflows\":[{\"id\":\"wf-1\",\"name\":\"W\",\"description\":\"```code block```\",\"lanes\":[],\"nodes\":[],\"edges\":[]}]}"},
		{"string_template_dollar_brace", domain.BlueprintSectionWBS, `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"${process.env.SECRET}","priority":"high","dependencies":[],"references":[],"features":[]}]},"cross_references":[]}`},
		{"string_template_double_brace", domain.BlueprintSectionWBS, `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"{{ config.token }}","priority":"high","dependencies":[],"references":[],"features":[]}]},"cross_references":[]}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := v.Validate(tc.sectionType, json.RawMessage(tc.jsonSnippet))
			assert.False(t, result.Valid, "expected Valid=false for forbidden content")
			assert.Equal(t, BlueprintValidationSecurityFailure, result.SafeOutcome)
			assert.Nil(t, result.NormalizedContent, "NormalizedContent must be nil on security failure")
			require.NotEmpty(t, result.Issues, "expected at least one issue")

			foundSecurityIssue := false
			for _, iss := range result.Issues {
				if iss.Code == BlueprintIssueForbiddenContent {
					foundSecurityIssue = true
					assert.True(t, iss.Blocking)
					assert.Equal(t, "section payload contains forbidden content", iss.Message)
					// Verify no leakage of rejected string content in Message or Remediation
					assert.NotContains(t, iss.Message, "sk-secret")
					assert.NotContains(t, iss.Message, "alert")
					assert.NotContains(t, iss.Message, "process.env")
					assert.NotContains(t, iss.Remediation, "sk-secret")
					assert.NotContains(t, iss.Remediation, "alert")
				}
			}
			assert.True(t, foundSecurityIssue, "expected BlueprintIssueForbiddenContent for %s", tc.name)
		})
	}

	// Benign keys / strings must not be falsely rejected
	t.Run("benign_tokenization_not_rejected", func(t *testing.T) {
		benign := `{"journey":{"phases":[{"id":"phase-1","label":"Tokenization Service Phase","order":1}],"actors":[],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(benign))
		for _, iss := range res.Issues {
			assert.NotEqual(t, BlueprintIssueForbiddenContent, iss.Code, "tokenization should not be rejected as token")
		}
	})
}

func TestBlueprintSectionValidatorProducesDeterministicIssues(t *testing.T) {
	v := NewBlueprintSectionValidator()

	// Payload with multiple intentional defects across different sections and paths
	multiIssueJourney := `{"journey":{
		"phases":[
			{"id":"phase-1","label":"Phase 1","order":1},
			{"id":"phase-1","label":"Phase 1 Duplicate","order":2},
			{"id":"","label":"Phase Missing ID","order":3}
		],
		"actors":[
			{"id":"actor-1","label":"Actor 1","order":1}
		],
		"activities":[
			{"id":"act-1","phase_id":"phase-unknown","actor_id":"actor-unknown","label":"Activity with dangling refs","description":"D","touchpoints":[],"references":["bad canonical ref"]}
		]
	}}`

	var firstRunJSON string
	for i := 0; i < 25; i++ {
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(multiIssueJourney))
		assert.False(t, res.Valid)
		assert.NotEmpty(t, res.Issues)

		// Verify ordering: Section -> Path -> Code -> TargetID
		for j := 1; j < len(res.Issues); j++ {
			prev := res.Issues[j-1]
			curr := res.Issues[j]
			if prev.Section == curr.Section {
				if prev.Path == curr.Path {
					if prev.Code == curr.Code {
						assert.True(t, prev.TargetID <= curr.TargetID, "TargetID order violation")
					} else {
						assert.True(t, prev.Code <= curr.Code, "Code order violation")
					}
				} else {
					assert.True(t, prev.Path <= curr.Path, "Path order violation: %s > %s", prev.Path, curr.Path)
				}
			} else {
				assert.True(t, prev.Section <= curr.Section, "Section order violation")
			}
		}

		bytes, err := json.Marshal(res.Issues)
		require.NoError(t, err)
		if i == 0 {
			firstRunJSON = string(bytes)
		} else {
			assert.Equal(t, firstRunJSON, string(bytes), "Issue serialization must be byte-equivalent on run %d", i)
		}
	}
}

func TestBlueprintSectionValidatorJourney(t *testing.T) {
	v := NewBlueprintSectionValidator()

	validJourneyPayload := `{"journey":{
		"phases":[
			{"id":"phase-onboarding","label":"Onboarding Phase","order":1}
		],
		"actors":[
			{"id":"actor-user","label":"Regular User","order":1}
		],
		"activities":[
			{
				"id":"act-register",
				"phase_id":"phase-onboarding",
				"actor_id":"actor-user",
				"label":"Register User Account",
				"description":"User registers their credentials",
				"touchpoints":["Web Browser"],
				"references":["architecture.node.node-auth","wbs.module.mod-auth"]
			}
		]
	}}`

	t.Run("valid_journey", func(t *testing.T) {
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(validJourneyPayload))
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)
		assert.NotNil(t, res.Issues)
		assert.Len(t, res.Issues, 0)
		assert.NotNil(t, res.NormalizedContent)

		var normalized struct {
			Journey BlueprintJourney `json:"journey"`
		}
		err := json.Unmarshal(res.NormalizedContent, &normalized)
		require.NoError(t, err)
		assert.Equal(t, "phase-onboarding", normalized.Journey.Phases[0].ID)
	})

	t.Run("missing_phase_id", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"   ","label":"L","order":1}],"actors":[],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/journey/phases/0/id", "")
	})

	t.Run("duplicate_phase_id", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"phase-1","label":"L1","order":1},{"id":"phase-1","label":"L2","order":2}],"actors":[],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/journey/phases/1/id", "phase-1")
	})

	t.Run("blank_phase_label", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"phase-1","label":"   ","order":1}],"actors":[],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/journey/phases/0/label", "phase-1")
	})

	t.Run("missing_actor_id", func(t *testing.T) {
		p := `{"journey":{"phases":[],"actors":[{"id":"","label":"A","order":1}],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/journey/actors/0/id", "")
	})

	t.Run("duplicate_actor_id", func(t *testing.T) {
		p := `{"journey":{"phases":[],"actors":[{"id":"actor-1","label":"A1","order":1},{"id":"actor-1","label":"A2","order":2}],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/journey/actors/1/id", "actor-1")
	})

	t.Run("blank_actor_label", func(t *testing.T) {
		p := `{"journey":{"phases":[],"actors":[{"id":"actor-1","label":"","order":1}],"activities":[]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/journey/actors/0/label", "actor-1")
	})

	t.Run("missing_activity_id", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"p-1","label":"P","order":1}],"actors":[{"id":"a-1","label":"A","order":1}],"activities":[{"id":"","phase_id":"p-1","actor_id":"a-1","label":"Act","description":"D","touchpoints":[],"references":[]}]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/journey/activities/0/id", "")
	})

	t.Run("duplicate_activity_id", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"p-1","label":"P","order":1}],"actors":[{"id":"a-1","label":"A","order":1}],"activities":[{"id":"act-1","phase_id":"p-1","actor_id":"a-1","label":"Act1","description":"D","touchpoints":[],"references":[]},{"id":"act-1","phase_id":"p-1","actor_id":"a-1","label":"Act2","description":"D","touchpoints":[],"references":[]}]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/journey/activities/1/id", "act-1")
	})

	t.Run("blank_activity_label", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"p-1","label":"P","order":1}],"actors":[{"id":"a-1","label":"A","order":1}],"activities":[{"id":"act-1","phase_id":"p-1","actor_id":"a-1","label":"  ","description":"D","touchpoints":[],"references":[]}]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/journey/activities/0/label", "act-1")
	})

	t.Run("dangling_phase_id", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"phase-1","label":"P","order":1}],"actors":[{"id":"actor-1","label":"A","order":1}],"activities":[{"id":"act-1","phase_id":"phase-unknown","actor_id":"actor-1","label":"Act","description":"D","touchpoints":[],"references":[]}]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/journey/activities/0/phase_id", "act-1")
	})

	t.Run("dangling_actor_id", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"phase-1","label":"P","order":1}],"actors":[{"id":"actor-1","label":"A","order":1}],"activities":[{"id":"act-1","phase_id":"phase-1","actor_id":"actor-unknown","label":"Act","description":"D","touchpoints":[],"references":[]}]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/journey/activities/0/actor_id", "act-1")
	})

	t.Run("malformed_canonical_reference", func(t *testing.T) {
		p := `{"journey":{"phases":[{"id":"phase-1","label":"P","order":1}],"actors":[{"id":"actor-1","label":"A","order":1}],"activities":[{"id":"act-1","phase_id":"phase-1","actor_id":"actor-1","label":"Act","description":"D","touchpoints":[],"references":["invalid://uri#ref"]}]}}`
		res := v.Validate(domain.BlueprintSectionJourney, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueInvalidCanonicalRef, "/journey/activities/0/references/0", "act-1")
	})
}

func TestBlueprintSectionValidatorArchitecture(t *testing.T) {
	v := NewBlueprintSectionValidator()

	validArchPayload := `{"architecture":{
		"groups":[{"id":"grp-1","label":"Group 1","order":1}],
		"nodes":[{"id":"node-1","group_id":"grp-1","type":"service","label":"Service 1","description":"Desc","layout":{"x":100.5,"y":200.5}}],
		"edges":[{"id":"edge-1","source":"node-1","target":"node-1","label":"Loopback","type":"sync"}]
	}}`

	t.Run("valid_architecture", func(t *testing.T) {
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(validArchPayload))
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)
		assert.NotNil(t, res.Issues)
		assert.Len(t, res.Issues, 0)
		assert.NotNil(t, res.NormalizedContent)
	})

	t.Run("missing_group_id", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"","label":"G","order":1}],"nodes":[],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/architecture/groups/0/id", "")
	})

	t.Run("duplicate_group_id", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G1","order":1},{"id":"grp-1","label":"G2","order":2}],"nodes":[],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/architecture/groups/1/id", "grp-1")
	})

	t.Run("missing_group_label", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"  ","order":1}],"nodes":[],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/architecture/groups/0/label", "grp-1")
	})

	t.Run("missing_node_id", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/architecture/nodes/0/id", "")
	})

	t.Run("duplicate_node_id", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-1","type":"service","label":"L1","description":"","layout":{"x":0,"y":0}},{"id":"node-1","group_id":"grp-1","type":"service","label":"L2","description":"","layout":{"x":10,"y":10}}],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/architecture/nodes/1/id", "node-1")
	})

	t.Run("missing_node_label", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-1","type":"service","label":"","description":"","layout":{"x":0,"y":0}}],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/architecture/nodes/0/label", "node-1")
	})

	t.Run("dangling_node_group", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-unknown","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/architecture/nodes/0/group_id", "node-1")
	})

	t.Run("unsupported_node_type", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-1","type":"quantum_computing","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueUnsupportedVersion, "/architecture/nodes/0/type", "node-1")
	})

	t.Run("layout_coordinates_nan_inf_or_out_of_bounds", func(t *testing.T) {
		// Out of bounds (> 100000)
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":150000,"y":0}}],"edges":[]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMalformedLayout, "/architecture/nodes/0/layout", "node-1")

		// Out of bounds (< -100000)
		p2 := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"node-1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":-120000}}],"edges":[]}}`
		res2 := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p2))
		assert.False(t, res2.Valid)
		assertIssuePresent(t, res2.Issues, BlueprintIssueMalformedLayout, "/architecture/nodes/0/layout", "node-1")
	})

	t.Run("missing_edge_id", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"n1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"","source":"n1","target":"n1","label":"","type":"sync"}]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/architecture/edges/0/id", "")
	})

	t.Run("duplicate_edge_id", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"n1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"e1","source":"n1","target":"n1","label":"","type":"sync"},{"id":"e1","source":"n1","target":"n1","label":"","type":"sync"}]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/architecture/edges/1/id", "e1")
	})

	t.Run("unsupported_edge_type", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"n1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"e1","source":"n1","target":"n1","label":"","type":"telepathy"}]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueUnsupportedVersion, "/architecture/edges/0/type", "e1")
	})

	t.Run("dangling_edge_source", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"n1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"e1","source":"node-unknown","target":"n1","label":"","type":"sync"}]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/architecture/edges/0/source", "e1")
	})

	t.Run("dangling_edge_target", func(t *testing.T) {
		p := `{"architecture":{"groups":[{"id":"grp-1","label":"G","order":1}],"nodes":[{"id":"n1","group_id":"grp-1","type":"service","label":"L","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"e1","source":"n1","target":"node-unknown","label":"","type":"sync"}]}}`
		res := v.Validate(domain.BlueprintSectionArchitecture, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/architecture/edges/0/target", "e1")
	})
}

func TestBlueprintSectionValidatorStateMachine(t *testing.T) {
	v := NewBlueprintSectionValidator()

	validSMPayload := `{"state_machine":{
		"entity":"Order",
		"initial_state":"state-created",
		"states":[
			{"id":"state-created","label":"Created","type":"normal","layout":{"x":0,"y":0}},
			{"id":"state-completed","label":"Completed","type":"terminal","layout":{"x":100,"y":100}}
		],
		"transitions":[
			{"id":"trans-1","source":"state-created","target":"state-completed","event":"COMPLETE","actor":"admin","guard":"paid","actions":["notify"]}
		]
	}}`

	t.Run("valid_state_machine", func(t *testing.T) {
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(validSMPayload))
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)
		assert.NotNil(t, res.Issues)
		assert.Len(t, res.Issues, 0)
	})

	t.Run("blank_initial_state", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"   ","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueStateInvalidInitial, "/state_machine/initial_state", "")
	})

	t.Run("initial_state_not_found", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"state-missing","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueStateInvalidInitial, "/state_machine/initial_state", "state-missing")
	})

	t.Run("missing_state_id", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/state_machine/states/0/id", "")
	})

	t.Run("duplicate_state_id", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S1","type":"normal","layout":{"x":0,"y":0}},{"id":"s1","label":"S2","type":"normal","layout":{"x":1,"y":1}}],"transitions":[]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/state_machine/states/1/id", "s1")
	})

	t.Run("missing_transition_id", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"","source":"s1","target":"s1","event":"E","actor":"","guard":"","actions":[]}]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/state_machine/transitions/0/id", "")
	})

	t.Run("duplicate_transition_id", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"t1","source":"s1","target":"s1","event":"E","actor":"","guard":"","actions":[]},{"id":"t1","source":"s1","target":"s1","event":"E","actor":"","guard":"","actions":[]}]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/state_machine/transitions/1/id", "t1")
	})

	t.Run("dangling_transition_source", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"t1","source":"state-unknown","target":"s1","event":"E","actor":"","guard":"","actions":[]}]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueStateDanglingTransition, "/state_machine/transitions/0/source", "t1")
	})

	t.Run("dangling_transition_target", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"t1","source":"s1","target":"state-unknown","event":"E","actor":"","guard":"","actions":[]}]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueStateDanglingTransition, "/state_machine/transitions/0/target", "t1")
	})

	t.Run("self_transition_blank_event_rejected", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"t1","source":"s1","target":"s1","event":"   ","actor":"","guard":"","actions":[]}]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueStateDanglingTransition, "/state_machine/transitions/0/event", "t1")
	})

	t.Run("self_transition_with_event_allowed", func(t *testing.T) {
		p := `{"state_machine":{"entity":"E","initial_state":"s1","states":[{"id":"s1","label":"S","type":"normal","layout":{"x":0,"y":0}}],"transitions":[{"id":"t1","source":"s1","target":"s1","event":"RETRY","actor":"","guard":"","actions":[]}]}}`
		res := v.Validate(domain.BlueprintSectionStateMachine, json.RawMessage(p))
		assert.True(t, res.Valid)
		assert.Empty(t, res.Issues)
	})
}

func TestBlueprintSectionValidatorWorkflow(t *testing.T) {
	v := NewBlueprintSectionValidator()

	validWorkflowPayload := `{"workflows":[{
		"id":"wf-main",
		"name":"Main Flow",
		"description":"Primary process workflow",
		"lanes":[{"id":"lane-1","label":"Customer","order":1}],
		"nodes":[
			{"id":"node-start","lane_id":"lane-1","type":"start","label":"Start","description":"","layout":{"x":0,"y":0}},
			{"id":"node-dec","lane_id":"lane-1","type":"decision","label":"Decision","description":"","layout":{"x":50,"y":0}},
			{"id":"node-act1","lane_id":"lane-1","type":"action","label":"Yes Path","description":"","layout":{"x":100,"y":0}},
			{"id":"node-act2","lane_id":"lane-1","type":"action","label":"No Path","description":"","layout":{"x":100,"y":50}}
		],
		"edges":[
			{"id":"edge-1","source":"node-start","target":"node-dec","label":"init"},
			{"id":"edge-2","source":"node-dec","target":"node-act1","label":"Yes"},
			{"id":"edge-3","source":"node-dec","target":"node-act2","label":"No"}
		]
	}]}`

	t.Run("valid_workflow", func(t *testing.T) {
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(validWorkflowPayload))
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)
		assert.NotNil(t, res.Issues)
		assert.Len(t, res.Issues, 0)
	})

	t.Run("single_node_workflow_zero_edges_allowed", func(t *testing.T) {
		singleNode := `{"workflows":[{
			"id":"wf-single",
			"name":"Single Node Flow",
			"description":"One node only",
			"lanes":[{"id":"lane-1","label":"Customer","order":1}],
			"nodes":[
				{"id":"node-only","lane_id":"lane-1","type":"action","label":"Only Node","description":"","layout":{"x":0,"y":0}}
			],
			"edges":[]
		}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(singleNode))
		assert.True(t, res.Valid)
		assert.Empty(t, res.Issues)
	})

	t.Run("missing_workflow_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"","name":"W","description":"","lanes":[],"nodes":[],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/workflows/0/id", "")
	})

	t.Run("duplicate_workflow_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W1","description":"","lanes":[],"nodes":[],"edges":[]},{"id":"wf-1","name":"W2","description":"","lanes":[],"nodes":[],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/workflows/1/id", "wf-1")
	})

	t.Run("missing_lane_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"","label":"L","order":1}],"nodes":[],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/workflows/0/lanes/0/id", "")
	})

	t.Run("duplicate_lane_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L1","order":1},{"id":"lane-1","label":"L2","order":2}],"nodes":[],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/workflows/0/lanes/1/id", "lane-1")
	})

	t.Run("missing_node_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"","lane_id":"lane-1","type":"action","label":"A","description":"","layout":{"x":0,"y":0}}],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/workflows/0/nodes/0/id", "")
	})

	t.Run("duplicate_node_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-1","lane_id":"lane-1","type":"action","label":"A1","description":"","layout":{"x":0,"y":0}},{"id":"node-1","lane_id":"lane-1","type":"action","label":"A2","description":"","layout":{"x":10,"y":10}}],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/workflows/0/nodes/1/id", "node-1")
	})

	t.Run("dangling_lane_id", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-1","lane_id":"lane-unknown","type":"action","label":"A","description":"","layout":{"x":0,"y":0}}],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/workflows/0/nodes/0/lane_id", "node-1")
	})

	t.Run("dangling_edge_source", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-1","lane_id":"lane-1","type":"action","label":"A","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"edge-1","source":"node-unknown","target":"node-1","label":""}]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/workflows/0/edges/0/source", "edge-1")
	})

	t.Run("dangling_edge_target", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-1","lane_id":"lane-1","type":"action","label":"A","description":"","layout":{"x":0,"y":0}}],"edges":[{"id":"edge-1","source":"node-1","target":"node-unknown","label":""}]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDanglingReference, "/workflows/0/edges/0/target", "edge-1")
	})

	t.Run("orphan_node_in_multi_node_workflow", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-1","lane_id":"lane-1","type":"action","label":"A1","description":"","layout":{"x":0,"y":0}},{"id":"node-orphan","lane_id":"lane-1","type":"action","label":"Orphan","description":"","layout":{"x":50,"y":50}}],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueWorkflowOrphanNode, "/workflows/0/nodes/1", "node-orphan")
	})

	t.Run("decision_node_underbranched", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-dec","lane_id":"lane-1","type":"decision","label":"Decide","description":"","layout":{"x":0,"y":0}},{"id":"node-target","lane_id":"lane-1","type":"action","label":"Action","description":"","layout":{"x":50,"y":0}}],"edges":[{"id":"edge-1","source":"node-dec","target":"node-target","label":"Only One"}]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueWorkflowUnderbranched, "/workflows/0/nodes/0", "node-dec")
	})

	t.Run("malformed_layout", func(t *testing.T) {
		p := `{"workflows":[{"id":"wf-1","name":"W","description":"","lanes":[{"id":"lane-1","label":"L","order":1}],"nodes":[{"id":"node-1","lane_id":"lane-1","type":"action","label":"A","description":"","layout":{"x":-200000,"y":0}}],"edges":[]}]}`
		res := v.Validate(domain.BlueprintSectionWorkflow, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMalformedLayout, "/workflows/0/nodes/0/layout", "node-1")
	})
}

func TestBlueprintSectionValidatorWBS(t *testing.T) {
	v := NewBlueprintSectionValidator()

	validWBSPayload := `{"wbs":{
		"modules":[
			{
				"id":"mod-auth",
				"name":"Auth Module",
				"description":"Authentication and authorization",
				"priority":"high",
				"dependencies":[],
				"references":["architecture.node.node-auth"],
				"features":[
					{
						"id":"feat-login",
						"name":"User Login",
						"description":"Login with password",
						"acceptance_criteria":["Successful login returns JWT"]
					}
				]
			},
			{
				"id":"mod-billing",
				"name":"Billing Module",
				"description":"Invoicing and payments",
				"priority":"medium",
				"dependencies":["mod-auth"],
				"references":["architecture.node.node-billing"],
				"features":[
					{
						"id":"feat-invoice",
						"name":"Generate Invoice",
						"description":"Generates monthly invoice",
						"acceptance_criteria":["PDF invoice generated"]
					}
				]
			}
		]
	},"cross_references":[
		{
			"id":"xref-1",
			"source_ref":"journey.activity.act-login",
			"target_ref":"wbs.module.mod-auth",
			"relation_type":"implements",
			"description":"Login activity implements auth module"
		}
	]}`

	t.Run("valid_wbs", func(t *testing.T) {
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(validWBSPayload))
		assert.True(t, res.Valid)
		assert.Empty(t, res.SafeOutcome)
		assert.NotNil(t, res.Issues)
		assert.Len(t, res.Issues, 0)
	})

	t.Run("missing_module_id", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"","name":"M","description":"","priority":"high","dependencies":[],"references":[],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/wbs/modules/0/id", "")
	})

	t.Run("duplicate_module_id", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M1","description":"","priority":"high","dependencies":[],"references":[],"features":[]},{"id":"mod-1","name":"M2","description":"","priority":"high","dependencies":[],"references":[],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/wbs/modules/1/id", "mod-1")
	})

	t.Run("missing_feature_id", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"high","dependencies":[],"references":[],"features":[{"id":"","name":"F","description":"","acceptance_criteria":["ok"]}]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueMissingID, "/wbs/modules/0/features/0/id", "")
	})

	t.Run("duplicate_feature_id", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"high","dependencies":[],"references":[],"features":[{"id":"feat-1","name":"F1","description":"","acceptance_criteria":["ok"]},{"id":"feat-1","name":"F2","description":"","acceptance_criteria":["ok"]}]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/wbs/modules/0/features/1/id", "feat-1")
	})

	t.Run("self_dependency", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"high","dependencies":["mod-1"],"references":[],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueWBSInvalidDependency, "/wbs/modules/0/dependencies/0", "mod-1")
	})

	t.Run("missing_unknown_dependency", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"high","dependencies":["mod-unknown"],"references":[],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueWBSInvalidDependency, "/wbs/modules/0/dependencies/0", "mod-1")
	})

	t.Run("duplicate_dependency", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M1","description":"","priority":"high","dependencies":[],"references":[],"features":[]},{"id":"mod-2","name":"M2","description":"","priority":"high","dependencies":["mod-1","mod-1"],"references":[],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueDuplicateID, "/wbs/modules/1/dependencies/1", "mod-1")
	})

	t.Run("deterministic_cycle_path", func(t *testing.T) {
		p := `{"wbs":{"modules":[
			{"id":"mod-a","name":"A","description":"","priority":"high","dependencies":["mod-b"],"references":[],"features":[]},
			{"id":"mod-b","name":"B","description":"","priority":"high","dependencies":["mod-a"],"references":[],"features":[]}
		]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueWBSDependencyCycle, "/wbs/modules", "")
		// Assert message contains deterministic cycle path
		for _, iss := range res.Issues {
			if iss.Code == BlueprintIssueWBSDependencyCycle {
				assert.Contains(t, iss.Message, "mod-a -> mod-b -> mod-a")
			}
		}
	})

	t.Run("invalid_priority", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"urgent_critical","dependencies":[],"references":[],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueUnsupportedVersion, "/wbs/modules/0/priority", "mod-1")
	})

	t.Run("empty_acceptance_criteria", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"high","dependencies":[],"references":[],"features":[{"id":"feat-1","name":"F","description":"","acceptance_criteria":[]}]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueUnsupportedVersion, "/wbs/modules/0/features/0/acceptance_criteria", "feat-1")
	})

	t.Run("malformed_canonical_reference", func(t *testing.T) {
		p := `{"wbs":{"modules":[{"id":"mod-1","name":"M","description":"","priority":"high","dependencies":[],"references":["bad ref without dot notation"],"features":[]}]},"cross_references":[]}`
		res := v.Validate(domain.BlueprintSectionWBS, json.RawMessage(p))
		assert.False(t, res.Valid)
		assertIssuePresent(t, res.Issues, BlueprintIssueInvalidCanonicalRef, "/wbs/modules/0/references/0", "mod-1")
	})
}

func assertIssuePresent(t *testing.T, issues []BlueprintValidationIssue, expectedCode, expectedPath, expectedTargetID string) {
	t.Helper()
	for _, iss := range issues {
		if iss.Code == expectedCode && iss.Path == expectedPath {
			if expectedTargetID != "" {
				assert.Equal(t, expectedTargetID, iss.TargetID, "TargetID mismatch for code %s at path %s", expectedCode, expectedPath)
			}
			return
		}
	}
	var foundDescriptions []string
	for _, iss := range issues {
		foundDescriptions = append(foundDescriptions, iss.Code+" at "+iss.Path+" (target: "+iss.TargetID+")")
	}
	t.Fatalf("expected issue %s at %s (target %s), but got: %s", expectedCode, expectedPath, expectedTargetID, strings.Join(foundDescriptions, "; "))
}
