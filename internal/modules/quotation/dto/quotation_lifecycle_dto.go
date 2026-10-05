package dto

import (
	"time"

	"github.com/google/uuid"
)

type PublishQuotationRequest struct {
	RevisionID                string `json:"revision_id" binding:"required"`
	ContentVersion            int    `json:"content_version" binding:"required"`
	ConfirmLegalModified      bool   `json:"confirm_legal_modified"`
	ConfirmCommercialModified bool   `json:"confirm_commercial_modified"`
}

type QuotationRevisionHistoryItem struct {
	ID                        uuid.UUID  `json:"id"`
	RevisionNumber            int        `json:"revision_number"`
	PublicationStatus         string     `json:"publication_status"`
	IsSigned                  bool       `json:"is_signed"`
	LegalContentModified      bool       `json:"legal_content_modified"`
	CommercialContentModified bool       `json:"commercial_content_modified"`
	CreatedAt                 time.Time  `json:"created_at"`
	GeneratedAt               *time.Time `json:"generated_at,omitempty"`
	PublishedAt               *time.Time `json:"published_at,omitempty"`
	SignedAt                  *time.Time `json:"signed_at,omitempty"`
	CreatedBy                 *uuid.UUID `json:"created_by,omitempty"`
	RateCardVersion           *string    `json:"rate_card_version,omitempty"`
	LegalTemplateVersion      *string    `json:"legal_template_version,omitempty"`
	PromptVersion             *string    `json:"prompt_version,omitempty"`
	SafetyEnvelopeVersion     *string    `json:"safety_envelope_version,omitempty"`
}
