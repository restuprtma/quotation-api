package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Blueprint struct {
	ID                        uuid.UUID  `json:"id" db:"id"`
	CompanyID                 uuid.UUID  `json:"company_id" db:"company_id"`
	QuotationID               uuid.UUID  `json:"quotation_id" db:"quotation_id"`
	Title                     string     `json:"title" db:"title"`
	Status                    string     `json:"status" db:"status"`
	CurrentRevision           int        `json:"current_revision" db:"current_revision"`
	ActivePublishedRevisionID *uuid.UUID `json:"active_published_revision_id" db:"active_published_revision_id"`
	CreatedBy                 *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt                 time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt                 *time.Time `json:"deleted_at" db:"deleted_at"`
}

type BlueprintRevision struct {
	ID                        uuid.UUID       `json:"id" db:"id"`
	CompanyID                 uuid.UUID       `json:"company_id" db:"company_id"`
	BlueprintID               uuid.UUID       `json:"blueprint_id" db:"blueprint_id"`
	RevisionNumber            int             `json:"revision_number" db:"revision_number"`
	IdempotencyKey            *string         `json:"idempotency_key" db:"idempotency_key"`
	SourceQuotationRevisionID *uuid.UUID      `json:"source_quotation_revision_id" db:"source_quotation_revision_id"`
	SourceContextSnapshot     json.RawMessage `json:"source_context_snapshot" db:"source_context_snapshot"`
	PublicationStatus         string          `json:"publication_status" db:"publication_status"`
	CompletenessStatus        string          `json:"completeness_status" db:"completeness_status"`
	GenerationStatus          string          `json:"generation_status" db:"generation_status"`
	ContentJSON               json.RawMessage `json:"content_json" db:"content_json"`
	ContentVersion            *string         `json:"content_version" db:"content_version"`
	ValidationResult          json.RawMessage `json:"validation_result" db:"validation_result"`
	CrossValidationResult     json.RawMessage `json:"cross_validation_result" db:"cross_validation_result"`
	SkillSnapshot             json.RawMessage `json:"skill_snapshot" db:"skill_snapshot"`
	SafetyEnvelopeVersion     *string         `json:"safety_envelope_version" db:"safety_envelope_version"`
	AiProvider                *string         `json:"ai_provider" db:"ai_provider"`
	AiModel                   *string         `json:"ai_model" db:"ai_model"`
	GeneratedAt               *time.Time      `json:"generated_at" db:"generated_at"`
	PublishedAt               *time.Time      `json:"published_at" db:"published_at"`
	CreatedBy                 *uuid.UUID      `json:"created_by" db:"created_by"`
	CreatedAt                 time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at" db:"updated_at"`
}

type BlueprintSectionRun struct {
	ID                  uuid.UUID       `json:"id" db:"id"`
	CompanyID           uuid.UUID       `json:"company_id" db:"company_id"`
	BlueprintRevisionID uuid.UUID       `json:"blueprint_revision_id" db:"blueprint_revision_id"`
	SectionType         string          `json:"section_type" db:"section_type"`
	Status              string          `json:"status" db:"status"`
	AttemptCount        int             `json:"attempt_count" db:"attempt_count"`
	ContentJSON         json.RawMessage `json:"content_json" db:"content_json"`
	ValidationResult    json.RawMessage `json:"validation_result" db:"validation_result"`
	SafeErrorCode       *string         `json:"safe_error_code" db:"safe_error_code"`
	StartedAt           *time.Time      `json:"started_at" db:"started_at"`
	CompletedAt         *time.Time      `json:"completed_at" db:"completed_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}
