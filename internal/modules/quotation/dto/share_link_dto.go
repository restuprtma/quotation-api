package dto

import (
	"github.com/google/uuid"
	"time"
)

type CreateShareLinkRequest struct {
	BlueprintID uuid.UUID `json:"blueprint_id" binding:"required"`
	ExpiresAt   time.Time `json:"expires_at" binding:"required"`
}

type ShareLinkResponse struct {
	ID          uuid.UUID  `json:"id"`
	CompanyID   uuid.UUID  `json:"company_id"`
	BlueprintID uuid.UUID  `json:"blueprint_id"`
	TokenHash   string     `json:"-"`               // Explicitly omit token hash from API
	Token       string     `json:"token,omitempty"` // Only populated on creation
	IsRevoked   bool       `json:"is_revoked"`
	ExpiresAt   time.Time  `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty"`
}
