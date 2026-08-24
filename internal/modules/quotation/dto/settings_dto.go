package dto

import (
	"github.com/google/uuid"
	"time"
)

type CredentialRequest struct {
	ProviderType string `json:"provider_type" binding:"required"`
	APIKey       string `json:"api_key"`
}

// CredentialResponse must explicitly exclude any plaintext or encrypted API key.
// We use KeyHint to show the last 4 characters to the user.
type CredentialResponse struct {
	ID           uuid.UUID `json:"id"`
	CompanyID    uuid.UUID `json:"company_id"`
	ProviderType string    `json:"provider_type"`
	KeyHint      string    `json:"key_hint"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
