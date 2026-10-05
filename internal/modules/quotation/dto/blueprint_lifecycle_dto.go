package dto

import (
	"time"

	"github.com/google/uuid"
)

type PublishBlueprintRequest struct {
	RevisionID     string   `json:"revision_id" binding:"required"`
	ConfirmPartial bool     `json:"confirm_partial"`
	WarningCodes   []string `json:"warning_codes"`
}

type CreateBlueprintRevisionRequest struct {
	Mode                 string `json:"mode"` // "clone" (default) or "regenerate_from_source"
	SourceQuotationRevID string `json:"source_quotation_revision_id,omitempty"`
}

type BlueprintRevisionHistoryItem struct {
	ID                        uuid.UUID         `json:"id"`
	RevisionNumber            int               `json:"revision_number"`
	PublicationStatus         string            `json:"publication_status"`
	CompletenessStatus        string            `json:"completeness_status"`
	SourceQuotationRevisionID *uuid.UUID        `json:"source_quotation_revision_id,omitempty"`
	SourceQuotationRevNumber  *int              `json:"source_quotation_rev_number,omitempty"`
	CompletenessMatrix        map[string]string `json:"completeness_matrix,omitempty"` // section -> status
	CreatedAt                 time.Time         `json:"created_at"`
	GeneratedAt               *time.Time        `json:"generated_at,omitempty"`
	PublishedAt               *time.Time        `json:"published_at,omitempty"`
	CreatedBy                 *uuid.UUID        `json:"created_by,omitempty"`
}
