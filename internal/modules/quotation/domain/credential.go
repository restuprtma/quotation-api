package domain

import (
	"time"

	"github.com/google/uuid"
)

type Credential struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	CompanyID       uuid.UUID  `json:"company_id" db:"company_id"`
	Provider        string     `json:"provider" db:"provider"`
	Model           string     `json:"model" db:"model"`
	EncryptedAPIKey string     `json:"encrypted_api_key" db:"encrypted_api_key"`
	KeyHint         string     `json:"key_hint" db:"key_hint"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	CreatedBy       *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}
