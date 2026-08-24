package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateBlueprintRequest struct {
	QuotationID uuid.UUID `json:"quotation_id" binding:"required"`
}

type UpdateBlueprintSectionRunRequest struct {
	UserInput  string `json:"user_input"`
	IsApproved bool   `json:"is_approved"`
}

type BlueprintSectionRunResponse struct {
	ID                 uuid.UUID              `json:"id"`
	BlueprintRevID     uuid.UUID              `json:"blueprint_rev_id"`
	SectionName        string                 `json:"section_name"`
	SkillCommand       string                 `json:"skill_command"`
	PromptPayload      map[string]interface{} `json:"prompt_payload,omitempty"`
	SystemInstructions string                 `json:"system_instructions,omitempty"`
	AIRawResponse      map[string]interface{} `json:"ai_raw_response,omitempty"`
	GeneratedContent   map[string]interface{} `json:"generated_content,omitempty"`
	UserInput          *string                `json:"user_input,omitempty"`
	IsApproved         bool                   `json:"is_approved"`
	ExecutedAt         *time.Time             `json:"executed_at,omitempty"`
	ExecutedBy         *uuid.UUID             `json:"executed_by,omitempty"`
}

type BlueprintRevisionResponse struct {
	ID            uuid.UUID                     `json:"id"`
	BlueprintID   uuid.UUID                     `json:"blueprint_id"`
	VersionNumber int                           `json:"version_number"`
	Status        string                        `json:"status"`
	Sections      []BlueprintSectionRunResponse `json:"sections,omitempty"`
	CreatedAt     time.Time                     `json:"created_at"`
	UpdatedAt     time.Time                     `json:"updated_at"`
}

type BlueprintResponse struct {
	ID                        uuid.UUID                  `json:"id"`
	QuotationID               uuid.UUID                  `json:"quotation_id"`
	ActivePublishedRevisionID *uuid.UUID                 `json:"active_published_revision_id,omitempty"`
	Status                    string                     `json:"status"`
	CreatedAt                 time.Time                  `json:"created_at"`
	UpdatedAt                 time.Time                  `json:"updated_at"`
	Revision                  *BlueprintRevisionResponse `json:"revision,omitempty"`
}
