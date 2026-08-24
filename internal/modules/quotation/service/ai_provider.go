package service

import (
	"context"
)

const (
	ErrInvalidCredential       = "invalid_credential"
	ErrCredentialNotConfigured = "credential_not_configured"
	ErrUnsupportedModel        = "unsupported_model"
	ErrRateLimited             = "rate_limited"
	ErrTimeout                 = "timeout"
	ErrProviderUnavailable     = "provider_unavailable"
)

type ProviderError struct {
	Code    string
	Message string
}

func (e *ProviderError) Error() string {
	return e.Code + ": " + e.Message
}

type GenerateRequest struct {
	Prompt string
}

type GenerateResult struct {
	Text string
}

type AIProvider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
}
