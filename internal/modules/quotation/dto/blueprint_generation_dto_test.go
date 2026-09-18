package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBlueprintGenerationRequestJSONContract(t *testing.T) {
	req := CreateBlueprintGenerationRequest{
		IdempotencyKey:            uuid.New(),
		QuotationID:               uuid.New(),
		SourceQuotationRevisionID: uuid.New(),
		Title:                     "Enterprise Portal Blueprint",
		BusinessGoals:             []string{"Streamline partner onboarding"},
		Actors:                    []string{"Admin", "Partner"},
		Channels:                  []string{"Web", "Mobile"},
		BusinessPhases:            []string{"Phase 1 Discovery", "Phase 2 Core MVP"},
		BusinessRules:             []string{"Only verified partners can access sandbox"},
		KnownStates:               []string{"Draft", "Under Review", "Active"},
		Integrations:              []string{"Payment Gateway", "ERP"},
		TechnologyPreferences:     []string{"Go", "PostgreSQL", "React"},
		OperationalConstraints:    []string{"99.9% uptime SLA"},
		AdditionalNotes:           "Target Q4 launch",
	}

	b, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(b, &raw)
	require.NoError(t, err)

	expectedKeys := []string{
		"idempotency_key",
		"quotation_id",
		"source_quotation_revision_id",
		"title",
		"business_goals",
		"actors",
		"channels",
		"business_phases",
		"business_rules",
		"known_states",
		"integrations",
		"technology_preferences",
		"operational_constraints",
		"additional_notes",
	}

	for _, key := range expectedKeys {
		assert.Contains(t, raw, key, "expected request JSON to contain key %s", key)
	}

	_, hasSkillID := raw["skill_id"]
	assert.False(t, hasSkillID, "skill_id must not exist in production create request")
}

func TestBlueprintStatusResponseOmitsSensitiveFields(t *testing.T) {
	now := time.Now().UTC()
	errCode := "E_SAMPLE"
	resp := BlueprintStatusResponse{
		BlueprintID:        uuid.New(),
		RevisionID:         uuid.New(),
		Title:              "Sample Blueprint",
		GenerationStatus:   "in_progress",
		PublicationStatus:  "draft",
		CompletenessStatus: "partial",
		ContentVersion:     1,
		Sections: []BlueprintSectionStatusResponse{
			{
				Type:        "context",
				Status:      "validated",
				Attempts:    1,
				ErrorCode:   &errCode,
				StartedAt:   &now,
				CompletedAt: &now,
				UpdatedAt:   now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)
	jsonStr := string(b)

	forbidden := []string{
		"prompt_payload", "system_instructions", "ai_raw_response",
		"source_context_snapshot", "skill_snapshot", "budget", "rate_card",
		"payment_terms", "legal", "credential",
	}

	for _, f := range forbidden {
		assert.NotContains(t, jsonStr, f, "forbidden field %q must not appear in BlueprintStatusResponse JSON", f)
	}
}

func TestBlueprintListResponseSerializesEmptySlice(t *testing.T) {
	resp := BlueprintListResponse{
		Items: []BlueprintListItemResponse{},
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[]}`, string(b))

	// Also when Items is nil, MarshalJSON should enforce []
	respNil := BlueprintListResponse{
		Items: nil,
	}
	bNil, err := json.Marshal(respNil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[]}`, string(bNil))
}

func TestBlueprintDetailResponseJSONContract(t *testing.T) {
	now := time.Now().UTC()
	qID := uuid.New()
	sourceRevID := uuid.New()
	detail := BlueprintDetailResponse{
		Status: BlueprintStatusResponse{
			BlueprintID:        uuid.New(),
			RevisionID:         uuid.New(),
			Title:              "Detail Test",
			GenerationStatus:   "completed",
			PublicationStatus:  "draft",
			CompletenessStatus: "complete",
			ContentVersion:     2,
			Sections:           make([]BlueprintSectionStatusResponse, 0),
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		QuotationID:               qID,
		SourceQuotationRevisionID: &sourceRevID,
		ContentJSON:               json.RawMessage(`{"sections":[]}`),
		ValidationResult:          json.RawMessage(`{"valid":true}`),
		CrossValidationResult:     json.RawMessage(`{"consistent":true}`),
	}

	b, err := json.Marshal(detail)
	require.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(b, &raw)
	require.NoError(t, err)

	assert.Contains(t, raw, "status")
	assert.Contains(t, raw, "quotation_id")
	assert.Contains(t, raw, "source_quotation_revision_id")
	assert.Contains(t, raw, "content_json")
	assert.Contains(t, raw, "validation_result")
	assert.Contains(t, raw, "cross_validation_result")

	forbidden := []string{
		"skill_snapshot", "source_context_snapshot", "ai_provider", "ai_model",
	}
	jsonStr := string(b)
	for _, f := range forbidden {
		assert.NotContains(t, jsonStr, f)
	}
}
