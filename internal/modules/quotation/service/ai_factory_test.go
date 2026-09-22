package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIFactory_OpenAIAndAgentRouter(t *testing.T) {
	factory := NewProviderFactory()
	for _, name := range []string{"openai", "agentrouter"} {
		cfg := ProviderConfig{
			ProviderName:    name,
			DecryptedAPIKey: "sk-test",
			Model:           "gpt-4o",
		}
		provider, err := factory.New(context.Background(), cfg)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	}
}

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

func TestOpenAIProvider_UpstreamStatusClassification(t *testing.T) {
	const secretMarker = "TOP_SECRET_INTERNAL_TRACE_12345"

	tests := []struct {
		name              string
		statusCode        int
		expectedCode      string
		expectedRetryable bool
	}{
		{"BadRequest_400", http.StatusBadRequest, ErrRequestRejected, false},
		{"Unauthorized_401", http.StatusUnauthorized, ErrInvalidCredential, false},
		{"Forbidden_403", http.StatusForbidden, ErrInvalidCredential, false},
		{"NotFound_404", http.StatusNotFound, ErrUnsupportedModel, false},
		{"RequestEntityTooLarge_413", http.StatusRequestEntityTooLarge, ErrPayloadTooLarge, false},
		{"UnprocessableEntity_422", http.StatusUnprocessableEntity, ErrRequestRejected, false},
		{"TooManyRequests_429", http.StatusTooManyRequests, ErrRateLimited, true},
		{"RequestTimeout_408", http.StatusRequestTimeout, ErrTimeout, true},
		{"GatewayTimeout_504", http.StatusGatewayTimeout, ErrTimeout, true},
		{"InternalServerError_500", http.StatusInternalServerError, ErrProviderUnavailable, true},
		{"BadGateway_502", http.StatusBadGateway, ErrProviderUnavailable, true},
		{"ServiceUnavailable_503", http.StatusServiceUnavailable, ErrProviderUnavailable, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-request-id", "req-test-12345")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error":{"message":"Failure: ` + secretMarker + `"}}`))
			}))
			defer server.Close()

			provider := NewOpenAIProvider(ProviderConfig{
				ProviderName:    "openai",
				DecryptedAPIKey: "sk-test",
				Model:           "gpt-4o",
				BaseURL:         server.URL,
				Timeout:         2 * time.Second,
			})

			_, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "test prompt"})
			require.Error(t, err)

			var perr *ProviderError
			require.ErrorAs(t, err, &perr)
			assert.Equal(t, tt.expectedCode, perr.Code)
			assert.Equal(t, tt.statusCode, perr.UpstreamStatus)
			assert.Equal(t, tt.expectedRetryable, perr.Retryable)
			assert.Equal(t, "req-test-12345", perr.RequestID)

			// Assert no secret marker leakage
			assert.NotContains(t, err.Error(), secretMarker)
			assert.NotContains(t, perr.Message, secretMarker)
		})
	}
}

func TestOpenAIProvider_HeaderExtraction(t *testing.T) {
	t.Run("Fallback_RequestID_Header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("request-id", "req-fallback-67890")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
		}))
		defer server.Close()

		provider := NewOpenAIProvider(ProviderConfig{
			ProviderName:    "openai",
			DecryptedAPIKey: "sk-test",
			BaseURL:         server.URL,
		})

		_, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "test prompt"})
		require.Error(t, err)

		var perr *ProviderError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, ErrRateLimited, perr.Code)
		assert.Equal(t, "req-fallback-67890", perr.RequestID)
		assert.True(t, perr.Retryable)
	})
}

func TestOpenAIProvider_TransportAndTimeoutFailures(t *testing.T) {
	t.Run("Transport_Failure_ProviderUnavailable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		serverURL := server.URL
		server.Close() // Close immediately to trigger transport error

		provider := NewOpenAIProvider(ProviderConfig{
			ProviderName:    "openai",
			DecryptedAPIKey: "sk-test",
			BaseURL:         serverURL,
			Timeout:         2 * time.Second,
		})

		_, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "test prompt"})
		require.Error(t, err)

		var perr *ProviderError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, ErrProviderUnavailable, perr.Code)
		assert.True(t, perr.Retryable)
		assert.Equal(t, 0, perr.UpstreamStatus)
	})

	t.Run("Context_DeadlineExceeded_Timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := NewOpenAIProvider(ProviderConfig{
			ProviderName:    "openai",
			DecryptedAPIKey: "sk-test",
			BaseURL:         server.URL,
			Timeout:         500 * time.Millisecond,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := provider.Generate(ctx, GenerateRequest{Prompt: "test prompt"})
		require.Error(t, err)

		var perr *ProviderError
		require.ErrorAs(t, err, &perr)
		assert.Equal(t, ErrTimeout, perr.Code)
		assert.True(t, perr.Retryable)
		assert.Equal(t, 0, perr.UpstreamStatus)
	})
}

func TestMapProviderError(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		expectedCode string
	}{
		{"RequestRejected", &ProviderError{Code: ErrRequestRejected, Message: "bad request"}, ErrCodeGenerationFailed},
		{"PayloadTooLarge", &ProviderError{Code: ErrPayloadTooLarge, Message: "payload too large"}, ErrCodeGenerationFailed},
		{"RateLimited", &ProviderError{Code: ErrRateLimited, Message: "rate limited"}, ErrCodeProviderRateLimited},
		{"Timeout", &ProviderError{Code: ErrTimeout, Message: "timed out"}, ErrCodeProviderTimeout},
		{"InvalidCredential", &ProviderError{Code: ErrInvalidCredential, Message: "invalid key"}, ErrCodeProviderInvalidCred},
		{"CredentialNotConfigured", &ProviderError{Code: ErrCredentialNotConfigured, Message: "no key"}, ErrCodeProviderInvalidCred},
		{"UnsupportedModel", &ProviderError{Code: ErrUnsupportedModel, Message: "unsupported model"}, ErrCodeSettingsIncomplete},
		{"ProviderUnavailable", &ProviderError{Code: ErrProviderUnavailable, Message: "server down"}, ErrCodeGenerationFailed},
		{"ContextDeadlineExceeded", context.DeadlineExceeded, ErrCodeProviderTimeout},
		{"ContextCanceled", context.Canceled, ErrCodeGenerationFailed},
		{"GenericError", errors.New("network socket closed"), ErrCodeGenerationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			genErr := mapProviderError(tt.inputErr)
			require.NotNil(t, genErr)
			assert.Equal(t, tt.expectedCode, genErr.Code)
		})
	}
}
