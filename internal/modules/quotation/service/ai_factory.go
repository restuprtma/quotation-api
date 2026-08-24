package service

import (
	"context"
	"time"
)

type ProviderConfig struct {
	ProviderName    string
	DecryptedAPIKey string
	Timeout         time.Duration
	Model           string
	BaseURL         string
}

type ProviderFactory struct{}

func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{}
}

func (f *ProviderFactory) New(ctx context.Context, cfg ProviderConfig) (AIProvider, error) {
	if cfg.DecryptedAPIKey == "" && cfg.ProviderName != "ollama" && cfg.ProviderName != "mock" {
		return nil, &ProviderError{Code: ErrCredentialNotConfigured, Message: "credential not configured"}
	}

	switch cfg.ProviderName {
	case "ollama":
		return NewOllamaProvider(cfg), nil
	case "gemini", "":
		return NewGeminiProvider(cfg), nil
	default:
		return nil, &ProviderError{Code: ErrUnsupportedModel, Message: "unsupported provider"}
	}
}
