package rnd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProviderOllamaRequestAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model, Prompt string
			Stream        bool
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "test/model" || body.Stream {
			t.Fatalf("unexpected body: %+v", body)
		}
		for _, value := range []string{"safe", "instruction", "allocation", `"value":"payload"`} {
			if !strings.Contains(body.Prompt, value) {
				t.Fatalf("prompt missing %q: %s", value, body.Prompt)
			}
		}
		io.WriteString(w, `{"response":"generated","done":true}`)
	}))
	defer server.Close()
	provider, err := NewProvider("ollama", ProviderConfig{OllamaBaseURL: server.URL, Timeout: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), ProviderRequest{Stage: StageAllocation, Model: "test/model", System: "safe", Instruction: "instruction", Payload: map[string]string{"value": "payload"}})
	if err != nil || response.Raw != "generated" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestProviderGeminiRequestAndResponse(t *testing.T) {
	const key = "top-secret-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.RequestURI, "/v1beta/models/model%2Fspecial:generateContent") {
			t.Fatalf("unexpected URI %q", r.RequestURI)
		}
		if r.Header.Get("x-goog-api-key") != key || r.URL.Query().Get("key") != "" {
			t.Fatalf("API key transport is unsafe")
		}
		var body struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Contents) != 1 || len(body.Contents[0].Parts) != 1 || !strings.Contains(body.Contents[0].Parts[0].Text, "PAYLOAD_JSON") {
			t.Fatalf("unexpected body: %+v", body)
		}
		io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"gemini output"}]}}]}`)
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = rewriteTransport{base: mustParseURL(t, server.URL), next: client.Transport}
	provider, err := NewProvider("gemini", ProviderConfig{GeminiAPIKey: key}, client)
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), ProviderRequest{Stage: StageHTML, Model: "model/special", Payload: map[string]bool{"ok": true}})
	if err != nil || response.Raw != "gemini output" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestProviderAgentRouterRequestAndResponse(t *testing.T) {
	const key = "top-secret-agent-router-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Fatalf("agent router authorization is unsafe")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "gpt-5.6-sol" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || !strings.Contains(body.Messages[0].Content, "PAYLOAD_JSON") {
			t.Fatalf("unexpected body: %+v", body)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"agent router output"}}]}`)
	}))
	defer server.Close()
	provider, err := NewProvider("agentrouter", ProviderConfig{AgentRouterBaseURL: server.URL + "/v1", AgentRouterAPIKey: key}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Generate(context.Background(), ProviderRequest{Stage: StageAllocation, Model: "gpt-5.6-sol", System: "safe", Instruction: "instruction", Payload: map[string]bool{"ok": true}})
	if err != nil || response.Raw != "agent router output" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestProviderErrorsAreBoundedAndRedacted(t *testing.T) {
	const secret = "secret-api-key"
	const responseBody = "private provider response body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, responseBody, http.StatusBadGateway) }))
	defer server.Close()
	provider, err := NewProvider("ollama", ProviderConfig{OllamaBaseURL: server.URL + "?query-secret=" + secret}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Generate(context.Background(), ProviderRequest{Stage: StageAllocation, Model: "model", System: "prompt-secret"})
	if err == nil || !strings.Contains(err.Error(), "ollama provider returned HTTP 502") {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, forbidden := range []string{secret, responseBody, "prompt-secret", "query-secret"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error leaked %q: %v", forbidden, err)
		}
	}
}

func TestProviderMalformedEmptyAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer server.Close()
	provider, _ := NewProvider("ollama", ProviderConfig{OllamaBaseURL: server.URL}, server.Client())
	if _, err := provider.Generate(context.Background(), ProviderRequest{Model: "m"}); err == nil {
		t.Fatal("expected empty response error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider.Generate(ctx, ProviderRequest{Model: "m"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestProviderTokenUsageExtraction(t *testing.T) {
	const key = "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{
			"candidates": [{"content": {"parts": [{"text": "response text"}]}}],
			"usageMetadata": {
				"promptTokenCount": 150,
				"candidatesTokenCount": 420,
				"totalTokenCount": 570
			}
		}`)
	}))
	defer server.Close()

	client := server.Client()
	client.Transport = rewriteTransport{base: mustParseURL(t, server.URL), next: client.Transport}
	provider, err := NewProvider("gemini", ProviderConfig{GeminiAPIKey: key}, client)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := provider.Generate(context.Background(), ProviderRequest{Stage: StageHTML, Model: "gemini-3.5-flash-lite", Payload: map[string]bool{"ok": true}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Raw != "response text" {
		t.Errorf("expected Raw='response text', got %q", resp.Raw)
	}
	if resp.PromptTokens != 150 {
		t.Errorf("expected PromptTokens=150, got %d", resp.PromptTokens)
	}
	if resp.CandidateTokens != 420 {
		t.Errorf("expected CandidateTokens=420, got %d", resp.CandidateTokens)
	}
	if resp.TotalTokens != 570 {
		t.Errorf("expected TotalTokens=570, got %d", resp.TotalTokens)
	}
}

func TestProviderFactoryValidation(t *testing.T) {
	for _, name := range []string{"unknown", "ollama", "gemini", "agentrouter"} {
		if _, err := NewProvider(name, ProviderConfig{}, nil); err == nil {
			t.Fatalf("expected %s validation error", name)
		}
	}
	provider, err := NewProvider("mock", ProviderConfig{}, nil)
	if err != nil || provider.Name() != "mock" {
		t.Fatalf("provider=%v err=%v", provider, err)
	}
}

type rewriteTransport struct {
	base *url.URL
	next http.RoundTripper
}

func (t rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = t.base.Scheme, t.base.Host
	return t.next.RoundTrip(clone)
}
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
