package dto

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrBudgetExceeded    = errors.New("allocation exceeds budget")
	ErrBudgetUnfeasible  = errors.New("budget is unfeasible")
	ErrMissingRateItem   = errors.New("rate_item_missing")
	ErrAllocationInvalid = errors.New("allocation_invalid")
)

type BudgetExceededError struct {
	Total    int64  `json:"total"`
	Budget   int64  `json:"budget"`
	Excess   int64  `json:"excess"`
	Currency string `json:"currency,omitempty"`
}

func (e *BudgetExceededError) Error() string {
	if e.Currency != "" {
		return fmt.Sprintf("%s by %d %s", ErrBudgetExceeded, e.Excess, e.Currency)
	}
	return fmt.Sprintf("%s by %d IDR", ErrBudgetExceeded, e.Excess)
}

func (e *BudgetExceededError) Unwrap() error {
	return ErrBudgetExceeded
}

type PaymentTermItem struct {
	MilestoneName string `json:"milestone_name"`
	Percentage    int64  `json:"percentage"`
	Description   string `json:"description,omitempty"`
}

type PaymentTermsInput struct {
	Mode  string            `json:"mode"`
	Items []PaymentTermItem `json:"items"`
}

type GenerateQuotationRequest struct {
	IdempotencyKey            *string           `json:"idempotency_key,omitempty"`
	SkillID                   uuid.UUID         `json:"skill_id"`
	ClientID                  *uuid.UUID        `json:"client_id,omitempty"`
	ClientName                string            `json:"client_name,omitempty"`
	Subject                   string            `json:"subject"`
	SolutionType              string            `json:"solution_type"`
	CommercialScheme          string            `json:"commercial_scheme"`
	RateCardID                uuid.UUID         `json:"rate_card_id"`
	RateCardCode              string            `json:"rate_card_code,omitempty"`
	ProjectDurationMonths     int64             `json:"project_duration_months"`
	BudgetMaximum             int64             `json:"budget_maximum"`
	Summary                   string            `json:"summary"`
	MandatoryScope            []string          `json:"mandatory_scope"`
	OptionalScope             []string          `json:"optional_scope"`
	AllowedStandardActivities []string          `json:"allowed_standard_activities,omitempty"`
	PaymentTerms              PaymentTermsInput `json:"payment_terms"`
	AdditionalNotes           string            `json:"additional_notes,omitempty"`
}

type AllocationResource struct {
	Role            string   `json:"role"`
	Seniority       string   `json:"seniority"`
	EmployeeCount   int64    `json:"employee_count"`
	PeriodMonths    int64    `json:"period_months"`
	ScopeReferences []string `json:"scope_references"`
	Reason          string   `json:"reason"`
}

type AllocationProposal struct {
	Resources             []AllocationResource `json:"resources"`
	SelectedOptionalScope []string             `json:"selected_optional_scope"`
	StandardActivities    []string             `json:"standard_activities"`
}

type EnrichedResource struct {
	Role            string   `json:"role"`
	Seniority       string   `json:"seniority"`
	TermType        string   `json:"term_type"`
	EmployeeCount   int64    `json:"employee_count"`
	PeriodMonths    int64    `json:"period_months"`
	AmountPerMonth  int64    `json:"amount_per_month"`
	LineTotal       int64    `json:"line_total"`
	ScopeReferences []string `json:"scope_references"`
	Reason          string   `json:"reason"`
}

type EnrichedAllocation struct {
	Resources             []EnrichedResource `json:"resources"`
	SelectedOptionalScope []string           `json:"selected_optional_scope"`
	StandardActivities    []string           `json:"standard_activities"`
	Currency              string             `json:"currency"`
	TermType              string             `json:"term_type"`
	TotalInvestment       int64              `json:"total_investment"`
	UtilizationPercent    int64              `json:"utilization_percent"`
}

type SaveDraftRequest struct {
	RevisionID     uuid.UUID `json:"revision_id"`
	ContentHTML    string    `json:"content_html"`
	ContentVersion int       `json:"content_version"`
}

type SaveDraftResponse struct {
	QuotationID               uuid.UUID `json:"quotation_id"`
	RevisionID                uuid.UUID `json:"revision_id"`
	ContentVersion            int       `json:"content_version"`
	ContentHTML               string    `json:"content_html"`
	LegalContentModified      bool      `json:"legal_content_modified"`
	CommercialContentModified bool      `json:"commercial_content_modified"`
	UpdatedAt                 time.Time `json:"updated_at"`
}
