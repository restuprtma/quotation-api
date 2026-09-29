package dto

import (
	"encoding/json"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CreateFromMeetingRequest is the prompt-first entry point. It carries only
// user-entered identity hints and meeting notes; all commercial values are
// resolved by the backend catalog and calculator.
type CreateFromMeetingRequest struct {
	ProjectName       *string    `json:"project_name,omitempty"`
	ClientID          *uuid.UUID `json:"client_id,omitempty"`
	ClientName        *string    `json:"client_name,omitempty"` // deprecated compatibility
	ClientCompanyName *string    `json:"client_company_name,omitempty"`
	PICName           *string    `json:"pic_name,omitempty"`
	MeetingNotes      string     `json:"meeting_notes"`
	IdempotencyKey    string     `json:"idempotency_key"`
}

// MeetingDraftResponse is intentionally editor-oriented: the caller gets the
// saved aggregate identifiers, version tokens, prompt state, and sanitized
// HTML in one response. Calculation is omitted for incomplete drafts.
type MeetingDraftResponse struct {
	QuotationID      uuid.UUID                   `json:"quotation_id"`
	RevisionID       uuid.UUID                   `json:"revision_id"`
	QuotationNumber  string                      `json:"quotation_number"`
	ContentHTML      string                      `json:"content_html"`
	ContentVersion   int                         `json:"content_version"`
	StateVersion     int64                       `json:"state_version"`
	PromptState      domain.PromptQuotationState `json:"prompt_state"`
	Issues           []domain.DraftIssue         `json:"issues"`
	Readiness        string                      `json:"readiness"`
	Calculation      json.RawMessage             `json:"calculation,omitempty"`
	CatalogRoles     []PromptCatalogRole         `json:"catalog_roles,omitempty"`
	IdempotentReplay bool                        `json:"idempotent_replay"`
}

// PromptCatalogRole is the safe, editor-facing projection of a rate card
// item. Commercial rates and internal costs intentionally remain server-only.
type PromptCatalogRole struct {
	RateItemID  uuid.UUID `json:"rate_item_id"`
	Role        string    `json:"role"`
	DisplayName string    `json:"display_name"`
	Seniority   string    `json:"seniority"`
	TermType    string    `json:"term_type"`
}

const (
	PromptPatchSetDurationMonths = "set_duration_months"
	PromptPatchSetTeam           = "set_team"
	PromptPatchSetDiscountBPS    = "set_discount_bps"
	PromptPatchSetPaymentPattern = "set_payment_pattern"
	PromptPatchConfirmField      = "confirm_field"
)

// PromptPatchOperation is deliberately typed. Commercial changes may not use
// an arbitrary JSON map, so identity and narrative fields cannot be altered
// through the price recalculation endpoint.
type PromptPatchOperation struct {
	Type           string                   `json:"type"`
	DurationMonths *int                     `json:"duration_months,omitempty"`
	Team           *[]domain.PromptTeamLine `json:"team,omitempty"`
	DiscountBPS    *int32                   `json:"discount_bps,omitempty"`
	PaymentPattern *string                  `json:"payment_pattern,omitempty"`
	Field          *string                  `json:"field,omitempty"`
}

type PromptPatchRequest struct {
	RevisionID     uuid.UUID              `json:"revision_id"`
	ContentVersion int                    `json:"content_version"`
	StateVersion   int64                  `json:"state_version"`
	Operations     []PromptPatchOperation `json:"operations"`
}

// ReviseSectionRequest carries a natural-language revision plus the normal
// optimistic-lock versions. TargetSection is optional for commercial intent,
// and required when the caller wants the model to revise narrative content.
type ReviseSectionRequest struct {
	RevisionID     uuid.UUID `json:"revision_id"`
	ContentVersion int       `json:"content_version"`
	StateVersion   int64     `json:"state_version"`
	Instruction    string    `json:"instruction"`
	TargetSection  string    `json:"target_section,omitempty"`
}

type PromptPatchResponse struct {
	QuotationID    uuid.UUID                        `json:"quotation_id"`
	RevisionID     uuid.UUID                        `json:"revision_id"`
	ContentHTML    string                           `json:"content_html"`
	ContentVersion int                              `json:"content_version"`
	StateVersion   int64                            `json:"state_version"`
	PromptState    domain.PromptQuotationState      `json:"prompt_state"`
	Issues         []domain.DraftIssue              `json:"issues"`
	Readiness      string                           `json:"readiness"`
	Calculation    domain.DedicatedSquadCalculation `json:"calculation"`
	CatalogRoles   []PromptCatalogRole              `json:"catalog_roles,omitempty"`
}

// MeetingExtractionRequest contains only the BA-authored source text. It is
// deliberately not a pricing request; pricing is resolved by the backend.
type MeetingExtractionRequest struct {
	MeetingNotes string `json:"meeting_notes"`
}
