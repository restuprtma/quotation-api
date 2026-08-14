package dto

import (
	"time"

	"github.com/google/uuid"
)

// QuotationItemResponse is the API view of a single price line inside a
// revision (e.g. "Backend Development — Rp 300.000.000").
type QuotationItemResponse struct {
	ID          uuid.UUID `json:"id"`
	ItemName    string    `json:"item_name"`
	Description *string   `json:"description,omitempty"`
	Amount      int64     `json:"amount"`
	SortOrder   int       `json:"sort_order"`
}

type PricingSectionResponse struct {
	ID          uuid.UUID             `json:"id"`
	SectionName string                `json:"section_name"`
	SectionType string                `json:"section_type"`
	Description *string               `json:"description,omitempty"`
	SortOrder   int                   `json:"sort_order"`
	Items       []PricingItemResponse `json:"items"`
}

type PricingItemResponse struct {
	ID                   uuid.UUID              `json:"id"`
	ItemName             string                 `json:"item_name"`
	Description          *string                `json:"description,omitempty"`
	Features             map[string]interface{} `json:"features,omitempty"`
	PricingMode          string                 `json:"pricing_mode"`
	UnitLabel            *string                `json:"unit_label,omitempty"`
	UnitPrice            int64                  `json:"unit_price"`
	Quantity             int64                  `json:"quantity"`
	EmployeeCount        int                    `json:"employee_count"`
	Duration             int                    `json:"duration"`
	DurationUnit         *string                `json:"duration_unit,omitempty"`
	BillingPeriod        string                 `json:"billing_period"`
	LineTotal            int64                  `json:"line_total"`
	ManualOverrideReason *string                `json:"manual_override_reason,omitempty"`
	SortOrder            int                    `json:"sort_order"`
}

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
	BlueprintData      map[string]interface{}   `json:"blueprint_data"`
	QuotationDate      *string                  `json:"quotation_date,omitempty"`
	ValidUntil         *string                  `json:"valid_until,omitempty"`
	ClientSnapshot     map[string]interface{}   `json:"client_snapshot,omitempty"`
	NarrativeData      map[string]interface{}   `json:"narrative_data,omitempty"`
	ProductDetailData  map[string]interface{}   `json:"product_detail_data,omitempty"`
	SignatureImageURL  *string                  `json:"signature_image_url"`
	SignedAt           *time.Time               `json:"signed_at"`
	Items              []QuotationItemResponse  `json:"items,omitempty"`
	Sections           []PricingSectionResponse `json:"sections,omitempty"`
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
