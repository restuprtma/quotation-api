package service

import (
	"context"
	"venturo-skeleton-go/internal/modules/quotation/domain"
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
	}

	return factory.New(ctx, cfg)
}
