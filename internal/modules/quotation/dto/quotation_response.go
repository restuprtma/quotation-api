package dto

import (
	"time"

	"github.com/google/uuid"
)



// QuotationRevisionResponse is the API view of one revision of a quotation.
// blueprint_data and ai_raw_response are stored as JSONB and returned as-is.
type QuotationRevisionResponse struct {
	ID                 uuid.UUID                `json:"id"`
	RevisionNumber     int                      `json:"revision_number"`
	PublicToken        uuid.UUID                `json:"public_token"`
	Budget             int64                    `json:"budget"`
	ProjectDescription *string                  `json:"project_description"`
	ScopeOfWork        *string                  `json:"scope_of_work"`
	MaintenanceInfo    *string                  `json:"maintenance_info"`
	HostingInfo        *string                  `json:"hosting_info"`
	Notes              *string                  `json:"notes"`
	QuotationDate      *string                  `json:"quotation_date,omitempty"`
	ValidUntil         *string                  `json:"valid_until,omitempty"`
	ClientSnapshot     map[string]interface{}   `json:"client_snapshot,omitempty"`
	SignatureImageURL  *string                  `json:"signature_image_url"`
	SignedAt           *time.Time               `json:"signed_at"`
}

// QuotationResponse is the API view of a quotation, optionally including
// its active revision (detail endpoint) or the client display name.
type QuotationResponse struct {
	ID              uuid.UUID  `json:"id"`
	CompanyID       uuid.UUID  `json:"company_id"`
	ClientID        uuid.UUID  `json:"client_id"`
	ClientName      *string    `json:"client_name,omitempty"`
	Subject         *string    `json:"subject,omitempty"`
	QuotationNumber string     `json:"quotation_number"`
	ProjectType     string     `json:"project_type"`
	Status          string     `json:"status"`
	CurrentRevision int        `json:"current_revision"`
	CreatedBy       *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// Revision is the active revision (current_revision). Populated on the
	// detail endpoint and on create/update responses.
	Revision *QuotationRevisionResponse `json:"revision,omitempty"`
}

// QuotationListResponse wraps a paginated list of quotations (without the
// revision payload to keep list responses light).
type QuotationListResponse struct {
	Items []QuotationResponse `json:"items"`
	Total int64               `json:"total"`
}
