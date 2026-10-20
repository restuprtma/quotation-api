package service

import (
	"context"
	"encoding/json"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const (
	ErrInvalidCredential       = "invalid_credential"
	ErrCredentialNotConfigured = "credential_not_configured"
	ErrUnsupportedModel        = "unsupported_model"
	ErrRateLimited             = "rate_limited"
	ErrTimeout                 = "timeout"
	ErrProviderUnavailable     = "provider_unavailable"
	ErrRequestRejected         = "request_rejected"
	ErrPayloadTooLarge         = "payload_too_large"
)

type ProviderError struct {
	Code           string
	Message        string
	UpstreamStatus int
	RequestID      string
	Retryable      bool
}

func (e *ProviderError) Error() string {
	if e.Code != "" && e.Message != "" {
		return e.Code + ": " + e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return e.Message
}

type GenerateOutputMode string

const (
	GenerateOutputJSON       GenerateOutputMode = "json_object"
	GenerateOutputJSONSchema GenerateOutputMode = "json_schema"
)

type GenerateRequest struct {
	Prompt     string
	OutputMode GenerateOutputMode
	JSONSchema json.RawMessage
}

type GenerateResult struct {
	Text         string
	FinishReason string
}

type AIProvider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
}

// InstantiateProvider decrypts the credential's API key before passing it to the ProviderFactory
func InstantiateProvider(
	ctx context.Context,
	factory *ProviderFactory,
	cred *domain.Credential,
	encryptionKey string,
) (AIProvider, error) {
	var decryptedKey string
	if cred.EncryptedAPIKey != "" {
		cipher, err := NewCredentialCipher(encryptionKey)
		if err != nil {
			return nil, err
		}
		decryptedKey, err = cipher.Decrypt(cred.EncryptedAPIKey)
		if err != nil {
			return nil, &ProviderError{Code: ErrInvalidCredential, Message: "failed to decrypt credential"}
		}
	}

	cfg := ProviderConfig{
		ProviderName:    cred.Provider,
		DecryptedAPIKey: decryptedKey,
		Model:           cred.Model,
		BaseURL:         cred.BaseURL,
		Timeout:         DefaultAttemptTimeout,
	}

	return factory.New(ctx, cfg)
}
