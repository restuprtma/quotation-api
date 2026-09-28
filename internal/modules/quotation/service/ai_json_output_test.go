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
