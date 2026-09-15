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
