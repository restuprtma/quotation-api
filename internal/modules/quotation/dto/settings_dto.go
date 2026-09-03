package dto

import (
	"github.com/google/uuid"
	"time"
)

type CredentialRequest struct {
	ProviderType string `json:"provider" binding:"required"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url,omitempty"`
}

// CredentialResponse must explicitly exclude any plaintext or encrypted API key.
// We use KeyHint to show the last 4 characters to the user.
type CredentialResponse struct {
	ID                   uuid.UUID `json:"id"`
	CompanyID            uuid.UUID `json:"company_id"`
	Provider             string    `json:"provider"`
	Model                string    `json:"model"`
	BaseURL              string    `json:"base_url,omitempty"`
	KeyHint              string    `json:"key_hint"`
	IsActive             bool      `json:"is_active"`
	CredentialConfigured bool      `json:"credential_configured"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
