package service

import (
	"testing"
	"time"

	"venturo-skeleton-go/internal/config"
)

func TestNewAIProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected interface{}
	}{
		{
			name:     "ollama provider",
			provider: "ollama",
			expected: &OllamaProvider{},
		},
		{
			name:     "gemini provider",
			provider: "gemini",
			expected: &GeminiProvider{},
		},
		{
			name:     "mock provider",
			provider: "mock",
			expected: &MockAIProvider{},
		},
		{
			name:     "unknown provider defaults to mock",
			provider: "unknown_xyz",
			expected: &MockAIProvider{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.AIConfig{
				Provider: tt.provider,
				Timeout:  10 * time.Second,
			}

			provider := NewAIProvider(cfg)

			// Simple type assertion check
			switch tt.expected.(type) {
			case *OllamaProvider:
				if _, ok := provider.(*OllamaProvider); !ok {
					t.Errorf("expected *OllamaProvider, got %T", provider)
				}
			case *GeminiProvider:
				if _, ok := provider.(*GeminiProvider); !ok {
					t.Errorf("expected *GeminiProvider, got %T", provider)
				}
			case *MockAIProvider:
				if _, ok := provider.(*MockAIProvider); !ok {
					t.Errorf("expected *MockAIProvider, got %T", provider)
				}
			}
		})
	}
}
