package service_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestGenerateShareToken_EntropyAndHashing(t *testing.T) {
	rawToken, tokenHash, err := service.GenerateShareToken()
	require.NoError(t, err)

	// Entropy: 32 bytes base64.RawURLEncoding produces 43 URL-safe characters
	assert.GreaterOrEqual(t, len(rawToken), 43, "token must be at least 43 chars (32 bytes url-safe base64)")
	assert.False(t, strings.ContainsAny(rawToken, "+/="), "token must be url-safe without +, /, or =")

	// Lowercase hex SHA-256 hash validation
	expectedHashBytes := sha256.Sum256([]byte(rawToken))
	expectedHash := hex.EncodeToString(expectedHashBytes[:])

	assert.Equal(t, 64, len(tokenHash), "sha256 hex must be 64 characters")
	assert.Equal(t, expectedHash, tokenHash, "hash must match sha256 sum of raw token")
	assert.Equal(t, strings.ToLower(tokenHash), tokenHash, "hash must be lowercase")

	// Distinct successive tokens test
	raw2, hash2, err := service.GenerateShareToken()
	require.NoError(t, err)
	assert.NotEqual(t, rawToken, raw2, "successive tokens must be unique")
	assert.NotEqual(t, tokenHash, hash2, "successive hashes must be unique")
}

func TestHashShareToken(t *testing.T) {
	input := "test-sample-raw-token-12345"
	expectedSum := sha256.Sum256([]byte(input))
	expected := hex.EncodeToString(expectedSum[:])

	res := service.HashShareToken(input)
	assert.Equal(t, expected, res)
}
