package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/pkg/logger"

	"go.uber.org/zap"
)

type OllamaProvider struct {
	cfg config.AIConfig
}

func NewOllamaProvider(cfg config.AIConfig) *OllamaProvider {
	return &OllamaProvider{cfg: cfg}
}

func (p *OllamaProvider) GenerateDraft(ctx context.Context, req *dto.AIGenerateRequest) (*dto.AIGenerateResponse, error) {
	sysPrompt := buildSystemPrompt()
	userPrompt := buildUserPrompt(req)

	payload := map[string]interface{}{
		"model":  p.cfg.OllamaModel,
		"format": "json",
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	body, _ := json.Marshal(payload)

	var lastErr error
	// Retry loop for transient errors (up to 3 tries)
	for i := 0; i < 3; i++ {
		res, err := p.doRequest(ctx, body)
		if err != nil {
			logger.Warn("Ollama request failed", zap.Error(err), zap.Int("attempt", i+1))
			lastErr = err
			continue // network error, retry
		}

		// Unmarshal and validate
		var draft dto.AIGenerateResponse
		if err := json.Unmarshal([]byte(res), &draft); err != nil {
			logger.Warn("Failed to unmarshal Ollama JSON", zap.Error(err))
			lastErr = fmt.Errorf("invalid json format from provider: %w", err)
			continue // try again, maybe the model hallucinated bad json
		}

		if err := ValidateAIBundle(&draft); err != nil {
			logger.Warn("Ollama bundle validation failed", zap.Error(err))
			// Validation errors are NOT transient. Return immediately.
			return nil, err
		}

		return &draft, nil
	}

	return nil, fmt.Errorf("ollama provider failed after retries: %w", lastErr)
}

func (p *OllamaProvider) doRequest(ctx context.Context, body []byte) (string, error) {
	url := strings.TrimRight(p.cfg.OllamaBaseURL, "/") + "/api/chat"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: p.cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("ollama server error: %d", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		// e.g. 404 model not found, no point retrying
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return result.Message.Content, nil
}
