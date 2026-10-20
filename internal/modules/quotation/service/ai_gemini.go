package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GeminiProvider struct {
	cfg ProviderConfig
	now func() time.Time
}

func (p *GeminiProvider) ExtractionCapabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, JSONSchema: true, UsageMetadata: true, FinishReason: true, ThinkingModes: []ThinkingMode{ThinkingDefault, ThinkingMinimal}, SupportsTemperatureZero: true}
}

func (p *GeminiProvider) GenerateExtraction(ctx context.Context, req ExtractionGenerateRequest) (ExtractionGenerateResult, error) {
	callCtx, cancel := ExtractionCallContext(ctx, p.cfg.Timeout)
	defer cancel()
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if p.cfg.BaseURL != "" {
		baseURL = strings.TrimRight(p.cfg.BaseURL, "/")
	}
	bodyMap := map[string]any{"contents": []map[string]any{{"parts": []map[string]any{{"text": req.Prompt}}}}}
	gc := map[string]any{}
	if len(req.JSONSchema) > 0 {
		var schema any
		if err := json.Unmarshal(req.JSONSchema, &schema); err != nil {
			return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionSchemaInvalid}
		}
		gc["responseMimeType"] = "application/json"
		gc["responseJsonSchema"] = schema
	}
	if req.MaxOutputTokens > 0 {
		gc["maxOutputTokens"] = req.MaxOutputTokens
	}
	if req.Temperature != nil && p.ExtractionCapabilities().SupportsTemperatureZero {
		gc["temperature"] = *req.Temperature
	}
	if len(gc) > 0 {
		bodyMap["generationConfig"] = gc
	}
	if req.Thinking.Mode == ThinkingMinimal {
		bodyMap["thinkingConfig"] = map[string]any{"thinkingBudget": req.Thinking.Budget}
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionInternal}
	}
	endpoint := baseURL + "/models/" + url.PathEscape(p.cfg.Model) + ":generateContent"
	if req.Stream {
		endpoint = baseURL + "/models/" + url.PathEscape(p.cfg.Model) + ":streamGenerateContent?alt=sse"
	}
	startedAt := p.now()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionInternal}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.cfg.DecryptedAPIKey)
	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return ExtractionGenerateResult{}, ExtractionErrorFromProvider(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		pe := mapGeminiError(resp.StatusCode)
		safe := ExtractionErrorFromProvider(pe)
		if resp.StatusCode == http.StatusTooManyRequests {
			if retry, ok := ParseRetryAfter(resp.Header.Get("Retry-After"), p.now()); ok {
				safe.RetryAfter = retry
			}
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return ExtractionGenerateResult{}, safe
	}
	if req.Stream {
		return parseGeminiExtractionStream(resp.Body, startedAt, p.now)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionProviderUnavailable, Retryable: true}
	}
	var result struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int64 `json:"promptTokenCount"`
			CandidatesTokenCount int64 `json:"candidatesTokenCount"`
			ThoughtsTokenCount   int64 `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionInvalidJSON}
	}
	finish := result.Candidates[0].FinishReason
	if strings.EqualFold(finish, "SAFETY") {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionProviderUnavailable}
	}
	if strings.Contains(strings.ToUpper(finish), "MAX_TOKEN") {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionSchemaInvalid, Retryable: true}
	}
	return ExtractionGenerateResult{Text: result.Candidates[0].Content.Parts[0].Text, FinishReason: finish, Usage: TokenUsage{InputTokens: result.UsageMetadata.PromptTokenCount, OutputTokens: result.UsageMetadata.CandidatesTokenCount, ThinkingTokens: result.UsageMetadata.ThoughtsTokenCount}}, nil
}

func parseGeminiExtractionStream(body io.Reader, startedAt time.Time, now func() time.Time) (ExtractionGenerateResult, error) {
	scanner := bufio.NewScanner(io.LimitReader(body, 4<<20))
	var result ExtractionGenerateResult
	var first, last time.Time
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		at := now()
		if first.IsZero() {
			first = at
		} else if gap := at.Sub(last); gap > result.IdleGap {
			result.IdleGap = gap
		}
		last = at
		var chunk struct {
			Candidates []struct {
				FinishReason string `json:"finishReason"`
				Content      struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int64 `json:"promptTokenCount"`
				CandidatesTokenCount int64 `json:"candidatesTokenCount"`
				ThoughtsTokenCount   int64 `json:"thoughtsTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionInvalidJSON}
		}
		if len(chunk.Candidates) > 0 {
			for _, part := range chunk.Candidates[0].Content.Parts {
				result.Text += part.Text
			}
			if chunk.Candidates[0].FinishReason != "" {
				result.FinishReason = chunk.Candidates[0].FinishReason
			}
		}
		result.Usage = TokenUsage{InputTokens: chunk.UsageMetadata.PromptTokenCount, OutputTokens: chunk.UsageMetadata.CandidatesTokenCount, ThinkingTokens: chunk.UsageMetadata.ThoughtsTokenCount}
	}
	if err := scanner.Err(); err != nil || first.IsZero() {
		return ExtractionGenerateResult{}, &ExtractionProviderError{Code: ErrExtractionProviderUnavailable, Retryable: true}
	}
	result.TTFB = first.Sub(startedAt)
	return result, nil
}

func NewGeminiProvider(cfg ProviderConfig) *GeminiProvider {
	if cfg.Model == "" {
		cfg.Model = "gemini-1.5-flash"
	}
	return &GeminiProvider{cfg: cfg, now: time.Now}
}

func (p *GeminiProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if p.cfg.BaseURL != "" {
		baseURL = strings.TrimRight(p.cfg.BaseURL, "/")
	}
	endpoint := baseURL + "/models/" + url.PathEscape(p.cfg.Model) + ":generateContent"

	timeout := p.cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	useJSONSchema := (req.OutputMode == GenerateOutputJSONSchema || len(req.JSONSchema) > 0) && len(req.JSONSchema) > 0

	makeRequest := func(withSchema bool) (*http.Response, error) {
		bodyMap := map[string]any{
			"contents": []map[string]any{
				{
					"parts": []map[string]any{
						{"text": req.Prompt},
					},
				},
			},
		}
		if withSchema {
			var schemaObj any
			if err := json.Unmarshal(req.JSONSchema, &schemaObj); err == nil {
				bodyMap["generationConfig"] = map[string]any{
					"responseMimeType":   "application/json",
					"responseJsonSchema": schemaObj,
				}
			} else {
				bodyMap["generationConfig"] = map[string]any{"responseMimeType": "application/json"}
			}
		} else if req.OutputMode == GenerateOutputJSON || req.OutputMode == GenerateOutputJSONSchema || len(req.JSONSchema) > 0 {
			bodyMap["generationConfig"] = map[string]any{"responseMimeType": "application/json"}
		}

		body, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, &ProviderError{Code: ErrProviderUnavailable, Message: "failed to marshal request"}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, &ProviderError{Code: ErrProviderUnavailable, Message: "failed to create request"}
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", p.cfg.DecryptedAPIKey)

		resp, err := client.Do(httpReq)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, &ProviderError{Code: ErrTimeout, Message: "request timed out"}
			}
			return nil, &ProviderError{Code: ErrProviderUnavailable, Message: "provider request failed"}
		}
		return resp, nil
	}

	var resp *http.Response
	var err error
	if useJSONSchema {
		resp, err = makeRequest(true)
		if err == nil && (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity) {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			resp, err = makeRequest(false)
		}
	} else {
		resp, err = makeRequest(false)
	}
	if err != nil {
		return GenerateResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GenerateResult{}, mapGeminiError(resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "failed to read response"}
	}

	var response struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(data, &response); err != nil || len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "invalid response format"}
	}

	var output strings.Builder
	for _, part := range response.Candidates[0].Content.Parts {
		if !part.Thought {
			output.WriteString(part.Text)
		}
	}

	return GenerateResult{
		Text:         output.String(),
		FinishReason: response.Candidates[0].FinishReason,
	}, nil
}

func mapGeminiError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{Code: ErrInvalidCredential, Message: "invalid provider credential"}
	case http.StatusNotFound:
		return &ProviderError{Code: ErrUnsupportedModel, Message: "unsupported model"}
	case http.StatusTooManyRequests:
		return &ProviderError{Code: ErrRateLimited, Message: "provider rate limited"}
	case http.StatusGatewayTimeout, http.StatusServiceUnavailable:
		return &ProviderError{Code: ErrTimeout, Message: "provider timed out"}
	default:
		return &ProviderError{Code: ErrProviderUnavailable, Message: "provider unavailable"}
	}
}
