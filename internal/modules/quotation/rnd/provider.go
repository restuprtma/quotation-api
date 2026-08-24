package rnd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProviderConfig struct {
	OllamaBaseURL      string
	GeminiAPIKey       string
	AgentRouterBaseURL string
	AgentRouterAPIKey  string
	Timeout            time.Duration
}

func NewProvider(name string, config ProviderConfig, client *http.Client) (Provider, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name != "mock" && name != "ollama" && name != "gemini" && name != "agentrouter" {
		return nil, fmt.Errorf("unsupported provider %q", name)
	}
	if name == "ollama" && strings.TrimSpace(config.OllamaBaseURL) == "" {
		return nil, errors.New("ollama base URL is required")
	}
	if name == "gemini" && strings.TrimSpace(config.GeminiAPIKey) == "" {
		return nil, errors.New("gemini API key is required")
	}
	if name == "agentrouter" && strings.TrimSpace(config.AgentRouterBaseURL) == "" {
		return nil, errors.New("agent router base URL is required")
	}
	if name == "agentrouter" && strings.TrimSpace(config.AgentRouterAPIKey) == "" {
		return nil, errors.New("agent router API key is required")
	}
	if client == nil {
		client = &http.Client{Timeout: config.Timeout}
	}
	if name == "mock" {
		return &mockProvider{}, nil
	}
	return &httpProvider{name: name, config: config, client: client}, nil
}

type httpProvider struct {
	name   string
	config ProviderConfig
	client *http.Client
}

func (p *httpProvider) Name() string { return p.name }

func (p *httpProvider) Generate(ctx context.Context, request ProviderRequest) (ProviderResponse, error) {
	prompt, err := composePrompt(request)
	if err != nil {
		return ProviderResponse{}, err
	}
	started := time.Now()
	var endpoint string
	var body []byte
	if p.name == "ollama" {
		endpoint = strings.TrimRight(p.config.OllamaBaseURL, "/") + "/api/generate"
		body, err = json.Marshal(struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}{request.Model, prompt, false})
	} else if p.name == "gemini" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(request.Model) + ":generateContent"
		body, err = json.Marshal(struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}{[]struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{{Parts: []struct {
			Text string `json:"text"`
		}{{Text: prompt}}}}})
	} else {
		endpoint = strings.TrimRight(p.config.AgentRouterBaseURL, "/") + "/chat/completions"
		body, err = json.Marshal(struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}{Model: request.Model, Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{{Role: "user", Content: prompt}}})
	}
	if err != nil {
		return ProviderResponse{}, errors.New("provider request serialization failed")
	}
	var resp *http.Response
	var lastErr error
	var data []byte

	for attempt := 1; attempt <= 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return ProviderResponse{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return ProviderResponse{}, errors.New("provider request creation failed")
		}
		req.Header.Set("Content-Type", "application/json")
		if p.name == "gemini" {
			req.Header.Set("x-goog-api-key", p.config.GeminiAPIKey)
		} else if p.name == "agentrouter" {
			req.Header.Set("Authorization", "Bearer "+p.config.AgentRouterAPIKey)
		}

		resp, err = p.client.Do(req)
		if err != nil {
			lastErr = err
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return ProviderResponse{}, fmt.Errorf("%s provider request: %w", p.name, err)
			}
			if attempt < 3 {
				select {
				case <-ctx.Done():
					return ProviderResponse{}, ctx.Err()
				case <-time.After(time.Duration(attempt*2) * time.Second):
					continue
				}
			}
			return ProviderResponse{}, fmt.Errorf("%s provider request failed", p.name)
		}

		var readErr error
		data, readErr = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < 3 {
				time.Sleep(time.Duration(attempt*2) * time.Second)
				continue
			}
			return ProviderResponse{}, fmt.Errorf("%s provider response read failed", p.name)
		}

		if resp.StatusCode == 503 || resp.StatusCode == 500 || resp.StatusCode == 502 {
			lastErr = fmt.Errorf("%s provider returned HTTP %d", p.name, resp.StatusCode)
			if attempt < 3 {
				select {
				case <-ctx.Done():
					return ProviderResponse{}, ctx.Err()
				case <-time.After(time.Duration(attempt*3) * time.Second):
					continue
				}
			}
			return ProviderResponse{}, lastErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return ProviderResponse{}, fmt.Errorf("%s provider returned HTTP %d", p.name, resp.StatusCode)
		}

		break
	}

	if resp == nil {
		return ProviderResponse{}, fmt.Errorf("%s provider request failed: %v", p.name, lastErr)
	}

	raw, tokens, err := extractProviderResponse(p.name, data)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("%s provider response invalid", p.name)
	}
	return ProviderResponse{
		Raw:             raw,
		Latency:         time.Since(started),
		PromptTokens:    tokens.PromptTokens,
		CandidateTokens: tokens.CandidateTokens,
		TotalTokens:     tokens.TotalTokens,
	}, nil
}

type providerTokens struct {
	PromptTokens    int
	CandidateTokens int
	TotalTokens     int
}

func composePrompt(request ProviderRequest) (string, error) {
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return "", errors.New("provider payload is not serializable")
	}
	return "SAFETY_ENVELOPE:\n" + request.System + "\n\nSKILL_INSTRUCTION:\n" + request.Instruction +
		"\n\nSTAGE:\n" + string(request.Stage) + "\n\nPAYLOAD_JSON:\n" + string(payload), nil
}

func extractProviderResponse(name string, data []byte) (string, providerTokens, error) {
	if name == "ollama" {
		var response struct {
			Response        string `json:"response"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
		}
		if err := json.Unmarshal(data, &response); err != nil || strings.TrimSpace(response.Response) == "" {
			return "", providerTokens{}, errors.New("empty ollama response")
		}
		total := response.PromptEvalCount + response.EvalCount
		return response.Response, providerTokens{
			PromptTokens:    response.PromptEvalCount,
			CandidateTokens: response.EvalCount,
			TotalTokens:     total,
		}, nil
	}
	if name == "agentrouter" {
		var response struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &response); err != nil || len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
			return "", providerTokens{}, errors.New("empty agent router response")
		}
		total := response.Usage.TotalTokens
		if total == 0 && (response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0) {
			total = response.Usage.PromptTokens + response.Usage.CompletionTokens
		}
		return response.Choices[0].Message.Content, providerTokens{
			PromptTokens:    response.Usage.PromptTokens,
			CandidateTokens: response.Usage.CompletionTokens,
			TotalTokens:     total,
		}, nil
	}
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &response); err != nil || len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 || strings.TrimSpace(response.Candidates[0].Content.Parts[0].Text) == "" {
		return "", providerTokens{}, errors.New("empty gemini response")
	}
	total := response.UsageMetadata.TotalTokenCount
	if total == 0 && (response.UsageMetadata.PromptTokenCount > 0 || response.UsageMetadata.CandidatesTokenCount > 0) {
		total = response.UsageMetadata.PromptTokenCount + response.UsageMetadata.CandidatesTokenCount
	}
	return response.Candidates[0].Content.Parts[0].Text, providerTokens{
		PromptTokens:    response.UsageMetadata.PromptTokenCount,
		CandidateTokens: response.UsageMetadata.CandidatesTokenCount,
		TotalTokens:     total,
	}, nil
}

func extractProviderText(name string, data []byte) (string, error) {
	text, _, err := extractProviderResponse(name, data)
	return text, err
}

type mockProvider struct{}

func (p *mockProvider) Name() string { return "mock" }

func (p *mockProvider) Generate(ctx context.Context, request ProviderRequest) (ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return ProviderResponse{}, err
	}
	started := time.Now()
	if request.Stage == StageAllocation {
		data, err := json.Marshal(request.Payload)
		if err != nil {
			return ProviderResponse{}, errors.New("mock allocation payload is invalid")
		}
		var payload allocationPayload
		if err := json.Unmarshal(data, &payload); err != nil || len(payload.GenerationInput.MandatoryScope) == 0 {
			return ProviderResponse{}, errors.New("mock allocation payload is invalid")
		}
		role, seniority := "programmer", "senior"
		if payload.GenerationInput.ProjectDurationMonths == 1 {
			role, seniority = "quality_assurance", "associate"
		}
		proposal := AllocationProposal{Resources: []AllocationResource{{
			Role: role, Seniority: seniority, EmployeeCount: 1,
			PeriodMonths:    payload.GenerationInput.ProjectDurationMonths,
			ScopeReferences: append([]string(nil), payload.GenerationInput.MandatoryScope...), Reason: "Implement approved scope",
		}}}
		raw, err := json.Marshal(proposal)
		if err != nil {
			return ProviderResponse{}, errors.New("mock allocation response failed")
		}
		return ProviderResponse{Raw: string(raw), Latency: time.Since(started)}, nil
	}
	if request.Stage == StageHTML {
		var payload struct {
			Metadata  string              `json:"metadata"`
			Scope     string              `json:"scope"`
			Closing   string              `json:"closing"`
			Signature string              `json:"signature"`
			Blocks    DeterministicBlocks `json:"blocks"`
		}
		data, err := json.Marshal(request.Payload)
		if err != nil || json.Unmarshal(data, &payload) != nil || payload.Blocks.CommercialHTML == "" {
			return ProviderResponse{}, errors.New("mock HTML payload is invalid")
		}
		return ProviderResponse{Raw: payload.Metadata + payload.Blocks.CommercialHTML + payload.Scope + payload.Blocks.PaymentHTML + payload.Blocks.LegalHTML + payload.Closing + payload.Signature, Latency: time.Since(started)}, nil
	}
	return ProviderResponse{Raw: "", Latency: time.Since(started)}, nil
}
