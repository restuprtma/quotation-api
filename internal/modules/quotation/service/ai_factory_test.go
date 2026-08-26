package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIFactory_MissingCredential_Ollama(t *testing.T) {
	factory := NewProviderFactory()
	cfg := ProviderConfig{
		ProviderName: "ollama",
	}

	provider, err := factory.New(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestAIFactory_StrictMissingCredential(t *testing.T) {
	factory := NewProviderFactory()
	cfg := ProviderConfig{
		ProviderName: "gemini",
	}

	provider, err := factory.New(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, provider)

	var providerErr *ProviderError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, ErrCredentialNotConfigured, providerErr.Code)
}

func TestAIErrorMapping(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		expectedCode string
	}{
		{"Unauthorized", http.StatusUnauthorized, ErrInvalidCredential},
		{"Forbidden", http.StatusForbidden, ErrInvalidCredential},
		{"NotFound", http.StatusNotFound, ErrUnsupportedModel},
		{"TooManyRequests", http.StatusTooManyRequests, ErrRateLimited},
		{"ServiceUnavailable", http.StatusServiceUnavailable, ErrTimeout},
		{"InternalServerError", http.StatusInternalServerError, ErrProviderUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			factory := NewProviderFactory()
			cfg := ProviderConfig{
				ProviderName:    "ollama", // Testing with ollama as we can easily override BaseURL
				DecryptedAPIKey: "test_key",
				BaseURL:         server.URL,
				Timeout:         1 * time.Second,
			}

			provider, err := factory.New(context.Background(), cfg)
			require.NoError(t, err)

			_, err = provider.Generate(context.Background(), GenerateRequest{Prompt: "test"})
			require.Error(t, err)

			var providerErr *ProviderError
			require.ErrorAs(t, err, &providerErr)
			assert.Equal(t, tt.expectedCode, providerErr.Code)
		})
	}
}
