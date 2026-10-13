package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProviderJSONOutputMode(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	}))
	defer server.Close()
	_, err := NewOpenAIProvider(ProviderConfig{BaseURL: server.URL}).Generate(context.Background(), GenerateRequest{Prompt: "json", OutputMode: GenerateOutputJSON})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"type": "json_object"}, body["response_format"])
}

func TestGeminiProviderJSONOutputMode(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{}"}]}}]}`))
	}))
	defer server.Close()
	_, err := NewGeminiProvider(ProviderConfig{BaseURL: server.URL}).Generate(context.Background(), GenerateRequest{Prompt: "json", OutputMode: GenerateOutputJSON})
	require.NoError(t, err)
	assert.Equal(t, "application/json", body["generationConfig"].(map[string]any)["responseMimeType"])
}

func TestOllamaProviderJSONOutputMode(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"response":"{}"}`))
	}))
	defer server.Close()
	_, err := NewOllamaProvider(ProviderConfig{BaseURL: server.URL}).Generate(context.Background(), GenerateRequest{Prompt: "json", OutputMode: GenerateOutputJSON})
	require.NoError(t, err)
	assert.Equal(t, "json", body["format"])
}

func TestProviderDefaultOutputModeOmitsJSONFields(t *testing.T) {
	tests := []struct {
		name     string
		provider AIProvider
		response string
		field    string
	}{
		{"openai", NewOpenAIProvider(ProviderConfig{}), `{"choices":[{"message":{"content":"{}"}}]}`, "response_format"},
		{"gemini", NewGeminiProvider(ProviderConfig{}), `{"candidates":[{"content":{"parts":[{"text":"{}"}]}}]}`, "generationConfig"},
		{"ollama", NewOllamaProvider(ProviderConfig{}), `{"response":"{}"}`, "format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()
			switch p := tt.provider.(type) {
			case *OpenAIProvider:
				p.cfg.BaseURL = server.URL
			case *GeminiProvider:
				p.cfg.BaseURL = server.URL
			case *OllamaProvider:
				p.cfg.BaseURL = server.URL
			}
			_, err := tt.provider.Generate(context.Background(), GenerateRequest{Prompt: "plain"})
			require.NoError(t, err)
			assert.NotContains(t, body, tt.field)
		})
	}
}

func TestGeminiProviderMultipartAndThought(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"thought":true,"text":"internal reasoning"},{"text":"{\"scope\":"},{"text":"[]}"}]}}]}`))
	}))
	defer server.Close()

	provider := NewGeminiProvider(ProviderConfig{BaseURL: server.URL})
	result, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "generate"})
	require.NoError(t, err)
	assert.Equal(t, `{"scope":[]}`, result.Text)
	assert.Equal(t, "STOP", result.FinishReason)
}

func TestOpenAIProviderFinishReasonLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"content":"{\"partial\":\"data"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(ProviderConfig{BaseURL: server.URL})
	result, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "generate"})
	require.NoError(t, err)
	assert.Equal(t, `{"partial":"data`, result.Text)
	assert.Equal(t, "length", result.FinishReason)
}

func TestOpenAIProviderFinishReasonStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"complete\":true}"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(ProviderConfig{BaseURL: server.URL})
	result, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "generate"})
	require.NoError(t, err)
	assert.Equal(t, `{"complete":true}`, result.Text)
	assert.Equal(t, "stop", result.FinishReason)
}

func TestGeminiProviderMissingFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello world"}]}}]}`))
	}))
	defer server.Close()

	provider := NewGeminiProvider(ProviderConfig{BaseURL: server.URL})
	result, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "generate"})
	require.NoError(t, err)
	assert.Equal(t, "hello world", result.Text)
	assert.Empty(t, result.FinishReason)
}

func TestOpenAIProviderMissingFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello custom gateway"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(ProviderConfig{BaseURL: server.URL})
	result, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "generate"})
	require.NoError(t, err)
	assert.Equal(t, "hello custom gateway", result.Text)
	assert.Empty(t, result.FinishReason)
}
