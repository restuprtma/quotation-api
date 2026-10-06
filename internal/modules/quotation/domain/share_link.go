package domain

import (
	"time"

	"github.com/google/uuid"
)

type ShareLink struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	CompanyID           uuid.UUID  `json:"company_id" db:"company_id"`
	BlueprintID         uuid.UUID  `json:"blueprint_id" db:"blueprint_id"`
	BlueprintRevisionID uuid.UUID  `json:"blueprint_revision_id" db:"blueprint_revision_id"`
	TokenHash           string     `json:"token_hash" db:"token_hash"`
	Label               string     `json:"label" db:"label"`
	CreatedBy           *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	RevokedAt           *time.Time `json:"revoked_at" db:"revoked_at"`
	LastAccessedAt      *time.Time `json:"last_accessed_at" db:"last_accessed_at"`
}

func (l *ShareLink) IsRevoked() bool {
	return l.RevokedAt != nil
}

func (l *ShareLink) IsActive() bool {
	return l.RevokedAt == nil
}
