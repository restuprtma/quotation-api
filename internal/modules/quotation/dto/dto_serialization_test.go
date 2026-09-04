package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestCredentialResponse_JSONSerialization(t *testing.T) {
	resp := CredentialResponse{
		ID:        uuid.New(),
		CompanyID: uuid.New(),
		Provider:  "openai",
		KeyHint:   "sk-...abcd",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(b, &raw)
	require.NoError(t, err)

	_, hasEncryptedAPIKey := raw["encrypted_api_key"]
	assert.False(t, hasEncryptedAPIKey, "JSON should not contain encrypted_api_key")

	_, hasAPIKey := raw["api_key"]
	assert.False(t, hasAPIKey, "JSON should not contain api_key")

	assert.Equal(t, "openai", raw["provider"])
	assert.Equal(t, "sk-...abcd", raw["key_hint"])
}

func TestGenerateQuotationRequest_DedicatedSquadJSONSerialization(t *testing.T) {
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	req := GenerateQuotationRequest{
		DedicatedSquad: &domain.DedicatedSquadInput{
			Model:             domain.ModelSOWLarge,
			Requirement:       "Build quotation engine",
			DevelopmentSquad:  []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 8}},
			PaymentCount:      2,
			PaymentAnchorDate: &anchor,
			Timeline:          domain.TimelineInput{Mode: domain.TimelineRelative},
		},
	}

	b, err := json.Marshal(req)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	require.Contains(t, raw, "dedicated_squad")

	var decoded GenerateQuotationRequest
	require.NoError(t, json.Unmarshal(b, &decoded))
	require.NotNil(t, decoded.DedicatedSquad)
	require.Equal(t, domain.ModelSOWLarge, decoded.DedicatedSquad.Model)
	require.Equal(t, anchor, *decoded.DedicatedSquad.PaymentAnchorDate)
	require.NoError(t, decoded.DedicatedSquad.Validate())
}
