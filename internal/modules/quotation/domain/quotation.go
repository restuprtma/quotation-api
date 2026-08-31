package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Quotation merepresentasikan data master quotation.
type Quotation struct {
	ID                        uuid.UUID  `json:"id" db:"id"`
	CompanyID                 uuid.UUID  `json:"company_id" db:"company_id"`
	ClientID                  uuid.UUID  `json:"client_id" db:"client_id"`
	QuotationNumber           string     `json:"quotation_number" db:"quotation_number"`
	Subject                   *string    `json:"subject" db:"subject"`
	SolutionType              string     `json:"solution_type" db:"solution_type"`
	CommercialScheme          string     `json:"commercial_scheme" db:"commercial_scheme"`
	Status                    string     `json:"status" db:"status"`
	CurrentRevision           int        `json:"current_revision" db:"current_revision"`
	PublicToken               uuid.UUID  `json:"public_token" db:"public_token"`
	ActivePublishedRevisionID *uuid.UUID `json:"active_published_revision_id" db:"active_published_revision_id"`
	// ClientName is populated by JOIN with core.clients for display (list/detail).
	ClientName         *string    `json:"client_name,omitempty" db:"client_name"`
	ClientNameSnapshot *string    `json:"client_name_snapshot,omitempty" db:"client_name_snapshot"`
	BudgetMaximum      *int64     `json:"budget_maximum,omitempty" db:"budget_maximum"`
	CreatedBy          *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at" db:"deleted_at"`
}

type QuotationRevision struct {
	ID                uuid.UUID `json:"id" db:"id"`
	CompanyID         uuid.UUID `json:"company_id" db:"company_id"`
	QuotationID       uuid.UUID `json:"quotation_id" db:"quotation_id"`
	RevisionNumber    int       `json:"revision_number" db:"revision_number"`
	IdempotencyKey    *string   `json:"idempotency_key" db:"idempotency_key"`
	PublicationStatus string    `json:"publication_status" db:"publication_status"`
	GenerationStatus  string    `json:"generation_status" db:"generation_status"`
	GenerationError   *string   `json:"generation_error,omitempty" db:"generation_error"`

	GenerationInput       json.RawMessage `json:"generation_input" db:"generation_input"`
	BudgetMaximum         *int64          `json:"budget_maximum" db:"budget_maximum"`
	AllocationSnapshot    json.RawMessage `json:"allocation_snapshot" db:"allocation_snapshot"`
	RateCardSnapshot      json.RawMessage `json:"rate_card_snapshot" db:"rate_card_snapshot"`
	LegalTemplateSnapshot json.RawMessage `json:"legal_template_snapshot" db:"legal_template_snapshot"`
	ContentHTML           *string         `json:"content_html" db:"content_html"`
	ContentVersion        *string         `json:"content_version" db:"content_version"`
	ContentHash           *string         `json:"content_hash" db:"content_hash"`

	AiProvider            *string         `json:"ai_provider" db:"ai_provider"`
	AiModel               *string         `json:"ai_model" db:"ai_model"`
	SkillSnapshot         json.RawMessage `json:"skill_snapshot" db:"skill_snapshot"`
	SafetyEnvelopeVersion *string         `json:"safety_envelope_version" db:"safety_envelope_version"`

	LegalContentModified      bool       `json:"legal_content_modified" db:"legal_content_modified"`
	LegalModifiedBy           *uuid.UUID `json:"legal_modified_by" db:"legal_modified_by"`
	LegalModifiedAt           *time.Time `json:"legal_modified_at" db:"legal_modified_at"`
	CommercialContentModified bool       `json:"commercial_content_modified" db:"commercial_content_modified"`
	CommercialModifiedBy      *uuid.UUID `json:"commercial_modified_by" db:"commercial_modified_by"`
	CommercialModifiedAt      *time.Time `json:"commercial_modified_at" db:"commercial_modified_at"`
	GeneratedAt               *time.Time `json:"generated_at" db:"generated_at"`
	PublishedAt               *time.Time `json:"published_at" db:"published_at"`

	SignerName        *string    `json:"signer_name" db:"signer_name"`
	SignerEmail       *string    `json:"signer_email" db:"signer_email"`
	SignatureData     *string    `json:"signature_data" db:"signature_data"`
	SignedContentHash *string    `json:"signed_content_hash" db:"signed_content_hash"`
	SignedAt          *time.Time `json:"signed_at" db:"signed_at"`
	SignedIP          *string    `json:"signed_ip" db:"signed_ip"`
	SignedUserAgent   *string    `json:"signed_user_agent" db:"signed_user_agent"`

	CreatedBy *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
}
