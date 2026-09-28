package service

import (
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
}

func NewGeminiProvider(cfg ProviderConfig) *GeminiProvider {
	if cfg.Model == "" {
		cfg.Model = "gemini-1.5-flash"
	}
	return &GeminiProvider{cfg: cfg}
}

func (p *GeminiProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if p.cfg.BaseURL != "" {
		baseURL = strings.TrimRight(p.cfg.BaseURL, "/")
	}
	endpoint := baseURL + "/models/" + url.PathEscape(p.cfg.Model) + ":generateContent"

	bodyMap := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": req.Prompt},
				},
			},
		},
	}
	if req.OutputMode == GenerateOutputJSON {
		bodyMap["generationConfig"] = map[string]any{"responseMimeType": "application/json"}
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "failed to marshal request"}
	}

	timeout := p.cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "failed to create request"}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.cfg.DecryptedAPIKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return GenerateResult{}, &ProviderError{Code: ErrTimeout, Message: "request timed out"}
		}
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "provider request failed"}
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
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(data, &response); err != nil || len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "invalid response format"}
	}

	return GenerateResult{Text: response.Candidates[0].Content.Parts[0].Text}, nil
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
