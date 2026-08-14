package domain

import (
	"github.com/google/uuid"
	"time"
)

// Quotation merepresentasikan data master quotation.
type Quotation struct {
	ID              uuid.UUID `json:"id" db:"id"`
	CompanyID       uuid.UUID `json:"company_id" db:"company_id"`
	ClientID        uuid.UUID `json:"client_id" db:"client_id"`
	QuotationNumber string    `json:"quotation_number" db:"quotation_number"`
	Subject         *string   `json:"subject" db:"subject"`
	ProjectType     string    `json:"project_type" db:"project_type"`
	Status          string    `json:"status" db:"status"`
	CurrentRevision int       `json:"current_revision" db:"current_revision"`
	// ClientName is populated by JOIN with core.clients for display (list/detail).
	ClientName *string    `json:"client_name,omitempty" db:"client_name"`
	CreatedBy  *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at" db:"deleted_at"`
}

type QuotationRevision struct {
	ID                 uuid.UUID              `json:"id" db:"id"`
	CompanyID          uuid.UUID              `json:"company_id" db:"company_id"`
	QuotationID        uuid.UUID              `json:"quotation_id" db:"quotation_id"`
	RevisionNumber     int                    `json:"revision_number" db:"revision_number"`
	PublicToken        uuid.UUID              `json:"public_token" db:"public_token"`
	QuotationDate      *time.Time             `json:"quotation_date" db:"quotation_date"`
	ValidUntil         *time.Time             `json:"valid_until" db:"valid_until"`
	ClientSnapshot     map[string]interface{} `json:"client_snapshot" db:"client_snapshot"`
	Budget             int64                  `json:"budget" db:"budget"`
	ProjectDescription *string                `json:"project_description" db:"project_description"`
	ScopeOfWork        *string                `json:"scope_of_work" db:"scope_of_work"`
	MaintenanceInfo    *string                `json:"maintenance_info" db:"maintenance_info"`
	HostingInfo        *string                `json:"hosting_info" db:"hosting_info"`
	Notes              *string                `json:"notes" db:"notes"`
	BlueprintData      map[string]interface{} `json:"blueprint_data" db:"blueprint_data"`
	AiRawResponse      map[string]interface{} `json:"ai_raw_response" db:"ai_raw_response"`
	NarrativeData      map[string]interface{} `json:"narrative_data" db:"narrative_data"`
	ProductDetailData  map[string]interface{} `json:"product_detail_data" db:"product_detail_data"`
	SignatureImageURL  *string                `json:"signature_image_url" db:"signature_image_url"`
	SignedAt           *time.Time             `json:"signed_at" db:"signed_at"`
	SignedIP           *string                `json:"signed_ip" db:"signed_ip"`
	CreatedBy          *uuid.UUID             `json:"created_by" db:"created_by"`
	CreatedAt          time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at" db:"updated_at"`
}

type QuotationItem struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CompanyID   uuid.UUID `json:"company_id" db:"company_id"`
	RevisionID  uuid.UUID `json:"revision_id" db:"revision_id"`
	ItemName    string    `json:"item_name" db:"item_name"`
	Description *string   `json:"description" db:"description"`
	Amount      int64     `json:"amount" db:"amount"`
	SortOrder   int       `json:"sort_order" db:"sort_order"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type Contract struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	CompanyID         uuid.UUID  `json:"company_id" db:"company_id"`
	QuotationID       uuid.UUID  `json:"quotation_id" db:"quotation_id"`
	RevisionID        uuid.UUID  `json:"revision_id" db:"revision_id"`
	ContractNumber    string     `json:"contract_number" db:"contract_number"`
	PublicToken       uuid.UUID  `json:"public_token" db:"public_token"`
	Status            string     `json:"status" db:"status"`
	AdditionalTerms   *string    `json:"additional_terms" db:"additional_terms"`
	SignatureImageURL *string    `json:"signature_image_url" db:"signature_image_url"`
	SignedAt          *time.Time `json:"signed_at" db:"signed_at"`
	SignedIP          *string    `json:"signed_ip" db:"signed_ip"`
	CreatedBy         *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at" db:"deleted_at"`
}
