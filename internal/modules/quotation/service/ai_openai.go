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

type OpenAIProvider struct {
	cfg ProviderConfig
}

func NewOpenAIProvider(cfg ProviderConfig) *OpenAIProvider {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	return &OpenAIProvider{cfg: cfg}
}

func (p *OpenAIProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	baseURL := "https://api.openai.com/v1"
	if p.cfg.BaseURL != "" {
		baseURL = strings.TrimRight(p.cfg.BaseURL, "/")
	}
	endpoint := baseURL + "/chat/completions"

	body, err := json.Marshal(map[string]any{
		"model": p.cfg.Model,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": req.Prompt,
			},
		},
	})
	if err != nil {
		return GenerateResult{}, &ProviderError{
			Code:      ErrProviderUnavailable,
			Message:   "failed to marshal request",
			Retryable: false,
		}
	}

	timeout := p.cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return GenerateResult{}, &ProviderError{
			Code:      ErrProviderUnavailable,
			Message:   "failed to create request",
			Retryable: false,
		}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.DecryptedAPIKey)
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36")
	httpReq.Header.Set("Origin", "https://opencode.ai")
	httpReq.Header.Set("Referer", "https://opencode.ai/")

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return GenerateResult{}, &ProviderError{
				Code:      ErrTimeout,
				Message:   "request timed out",
				Retryable: true,
			}
		}
		return GenerateResult{}, &ProviderError{
			Code:      ErrProviderUnavailable,
			Message:   "provider request failed",
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		reqID := resp.Header.Get("x-request-id")
		if reqID == "" {
			reqID = resp.Header.Get("request-id")
		}
		perr := mapOpenAIError(resp.StatusCode)
		perr.RequestID = reqID
		return GenerateResult{}, perr
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return GenerateResult{}, &ProviderError{
			Code:      ErrProviderUnavailable,
			Message:   "failed to read response",
			Retryable: true,
		}
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(data, &response); err != nil || len(response.Choices) == 0 {
		return GenerateResult{}, &ProviderError{
			Code:      ErrProviderUnavailable,
			Message:   "invalid response format",
			Retryable: false,
		}
	}

	return GenerateResult{Text: response.Choices[0].Message.Content}, nil
}

func mapOpenAIError(statusCode int) *ProviderError {
	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return &ProviderError{
			Code:           ErrRequestRejected,
			Message:        "provider rejected request",
			UpstreamStatus: statusCode,
			Retryable:      false,
		}
	case http.StatusUnauthorized, http.StatusForbidden:
		return &ProviderError{
			Code:           ErrInvalidCredential,
			Message:        "invalid provider credential",
			UpstreamStatus: statusCode,
			Retryable:      false,
		}
	case http.StatusNotFound:
		return &ProviderError{
			Code:           ErrUnsupportedModel,
			Message:        "unsupported model",
			UpstreamStatus: statusCode,
			Retryable:      false,
		}
	case http.StatusRequestEntityTooLarge:
		return &ProviderError{
			Code:           ErrPayloadTooLarge,
			Message:        "provider payload too large",
			UpstreamStatus: statusCode,
			Retryable:      false,
		}
	case http.StatusTooManyRequests:
		return &ProviderError{
			Code:           ErrRateLimited,
			Message:        "provider rate limited",
			UpstreamStatus: statusCode,
			Retryable:      true,
		}
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return &ProviderError{
			Code:           ErrTimeout,
			Message:        "provider timed out",
			UpstreamStatus: statusCode,
			Retryable:      true,
		}
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
		return &ProviderError{
			Code:           ErrProviderUnavailable,
			Message:        "provider unavailable",
			UpstreamStatus: statusCode,
			Retryable:      true,
		}
	default:
		if statusCode >= 500 {
			return &ProviderError{
				Code:           ErrProviderUnavailable,
				Message:        "provider unavailable",
				UpstreamStatus: statusCode,
				Retryable:      true,
			}
		}
		return &ProviderError{
			Code:           ErrRequestRejected,
			Message:        "provider rejected request",
			UpstreamStatus: statusCode,
			Retryable:      false,
		}
	}
}
