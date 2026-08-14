package domain

import (
	"time"

	"github.com/google/uuid"
)

// Constants for Artifact Types
const (
	ArtifactTypeQuotation = "quotation"
	ArtifactTypeBlueprint = "blueprint"
	ArtifactTypeDetail    = "detail"
)

type QuotationPublicArtifact struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	CompanyID    uuid.UUID  `json:"company_id" db:"company_id"`
	RevisionID   uuid.UUID  `json:"revision_id" db:"revision_id"`
	ArtifactType string     `json:"artifact_type" db:"artifact_type"`
	PublicToken  uuid.UUID  `json:"public_token" db:"public_token"`
	PublishedAt  *time.Time `json:"published_at" db:"published_at"`
	RevokedAt    *time.Time `json:"revoked_at" db:"revoked_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}
