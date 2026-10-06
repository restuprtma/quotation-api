package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateBlueprintShareLinkRequest struct {
	BlueprintRevisionID uuid.UUID `json:"blueprint_revision_id" binding:"required"`
	Label               string    `json:"label" binding:"required,min=1,max=255"`
}

type CreateBlueprintShareLinkResponse struct {
	ID             uuid.UUID `json:"id"`
	Label          string    `json:"label"`
	RevisionNumber int       `json:"revision_number"`
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"created_at"`
}

type BlueprintShareLinkItemResponse struct {
	ID                  uuid.UUID  `json:"id"`
	Label               string     `json:"label"`
	RevisionNumber      int        `json:"revision_number"`
	BlueprintRevisionID uuid.UUID  `json:"blueprint_revision_id"`
	CreatedAt           time.Time  `json:"created_at"`
	RevokedAt           *time.Time `json:"revoked_at"`
	LastAccessedAt      *time.Time `json:"last_accessed_at"`
	IsRevoked           bool       `json:"is_revoked"`
}
