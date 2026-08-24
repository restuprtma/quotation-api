package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialResponse_JSONSerialization(t *testing.T) {
	resp := CredentialResponse{
		ID:           uuid.New(),
		CompanyID:    uuid.New(),
		ProviderType: "openai",
		KeyHint:      "sk-...abcd",
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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

	assert.Equal(t, "openai", raw["provider_type"])
	assert.Equal(t, "sk-...abcd", raw["key_hint"])
}
