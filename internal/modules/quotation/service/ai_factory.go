package service

import (
	"go.uber.org/zap"
	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/pkg/logger"
)

// NewAIProvider creates an AIProvider based on the configuration.
func NewAIProvider(cfg config.AIConfig) AIProvider {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaProvider(cfg)
	case "gemini":
		return NewGeminiProvider(cfg)
	case "mock":
		return &MockAIProvider{}
	default:
		logger.Warn("Unknown AI provider in config, falling back to mock", zap.String("provider", cfg.Provider))
		return &MockAIProvider{}
	}
}
