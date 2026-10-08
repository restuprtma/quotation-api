package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// QuotationRevisionResponse is the API view of one revision of a quotation.
// ai_raw_response and generated_content are stored as JSONB and returned as-is.
type QuotationRevisionResponse struct {
	ID                  uuid.UUID              `json:"id"`
	VersionNumber       int                    `json:"version_number"`
	Status              string                 `json:"status"`
	AIRawResponse       map[string]interface{} `json:"ai_raw_response,omitempty"`
	GeneratedContent    map[string]interface{} `json:"generated_content,omitempty"`
	ClientSnapshot      map[string]interface{} `json:"client_snapshot,omitempty"`
	ContractTerms       *string                `json:"contract_terms,omitempty"`
	SignatureImageURL   *string                `json:"signature_image_url,omitempty"`
	PromptState         json.RawMessage        `json:"prompt_state,omitempty"`
	PromptStateVersion  int64                  `json:"prompt_state_version,omitempty"`
	PromptSectionHashes json.RawMessage        `json:"prompt_section_hashes,omitempty"`
	SignedAt            *time.Time             `json:"signed_at,omitempty"`
	SignedBy            *string                `json:"signed_by,omitempty"`
	SignedIP            *string                `json:"signed_ip,omitempty"`

	*BlueprintSourceMetadata `json:",inline"`
}

// QuotationResponse is the API view of a quotation, optionally including
// its active revision (detail endpoint) or the client display name.
type QuotationResponse struct {
	ID                        uuid.UUID  `json:"id"`
	CompanyID                 uuid.UUID  `json:"company_id"`
	ClientID                  uuid.UUID  `json:"client_id"`
	ClientName                *string    `json:"client_name,omitempty"`
	PICName                   *string    `json:"pic_name,omitempty"`
	Subject                   *string    `json:"subject,omitempty"`
	QuotationNumber           string     `json:"quotation_number"`
	SolutionType              string     `json:"solution_type"`
	CommercialScheme          string     `json:"commercial_scheme"`
	BudgetMaximum             *int64     `json:"budget_maximum,omitempty"`
	TotalInvestment           *int64     `json:"total_investment,omitempty"`
	Status                    string     `json:"status"`
	ActivePublishedRevisionID *uuid.UUID `json:"active_published_revision_id,omitempty"`
	CreatedBy                 *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`

	// Revision is the active revision. Populated on the
	// detail endpoint and on create/update responses.
	Revision *QuotationRevisionResponse `json:"revision,omitempty"`
}

// QuotationListResponse wraps a paginated list of quotations (without the
// revision payload to keep list responses light).
type QuotationListResponse struct {
	Items []QuotationResponse `json:"items"`
	Total int64               `json:"total"`
}
