package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateBlueprintGenerationRequest struct {
	IdempotencyKey            uuid.UUID `json:"idempotency_key" binding:"required"`
	QuotationID               uuid.UUID `json:"quotation_id" binding:"required"`
	SourceQuotationRevisionID uuid.UUID `json:"source_quotation_revision_id" binding:"required"`
	Title                     string    `json:"title" binding:"required,min=1,max=160"`
	BusinessGoals             []string  `json:"business_goals" binding:"required,min=1,dive,required,max=200"`
	Actors                    []string  `json:"actors" binding:"required,min=1,dive,required,max=120"`
	Channels                  []string  `json:"channels" binding:"max=20,dive,required,max=120"`
	BusinessPhases            []string  `json:"business_phases" binding:"required,min=1,max=30,dive,required,max=160"`
	BusinessRules             []string  `json:"business_rules" binding:"max=50,dive,required,max=300"`
	KnownStates               []string  `json:"known_states" binding:"max=100,dive,required,max=100"`
	Integrations              []string  `json:"integrations" binding:"max=50,dive,required,max=160"`
	TechnologyPreferences     []string  `json:"technology_preferences" binding:"max=30,dive,required,max=160"`
	OperationalConstraints    []string  `json:"operational_constraints" binding:"max=50,dive,required,max=300"`
	AdditionalNotes           string    `json:"additional_notes" binding:"max=2000"`
}

type BlueprintSectionStatusResponse struct {
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	ErrorCode   *string    `json:"error_code,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type BlueprintStatusResponse struct {
	BlueprintID        uuid.UUID                        `json:"blueprint_id"`
	RevisionID         uuid.UUID                        `json:"revision_id"`
	Title              string                           `json:"title"`
	GenerationStatus   string                           `json:"generation_status"`
	PublicationStatus  string                           `json:"publication_status"`
	CompletenessStatus string                           `json:"completeness_status"`
	ContentVersion     int                              `json:"content_version"`
	Sections           []BlueprintSectionStatusResponse `json:"sections"`
	CreatedAt          time.Time                        `json:"created_at"`
	UpdatedAt          time.Time                        `json:"updated_at"`
}
