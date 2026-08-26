package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type OllamaProvider struct {
	cfg ProviderConfig
}

func NewOllamaProvider(cfg ProviderConfig) *OllamaProvider {
	if cfg.Model == "" {
		cfg.Model = "llama3"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	return &OllamaProvider{cfg: cfg}
}

func (p *OllamaProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/api/generate"

	body, err := json.Marshal(map[string]any{
		"model":  p.cfg.Model,
		"prompt": req.Prompt,
		"stream": false,
	})
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
	if p.cfg.DecryptedAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.cfg.DecryptedAPIKey)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return GenerateResult{}, &ProviderError{Code: ErrTimeout, Message: "request timed out"}
		}
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GenerateResult{}, mapOllamaError(resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "failed to read response"}
	}

	var response struct {
		Response string `json:"response"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return GenerateResult{}, &ProviderError{Code: ErrProviderUnavailable, Message: "invalid response format"}
	}

	return GenerateResult{Text: response.Response}, nil
}

func mapOllamaError(statusCode int) error {
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
