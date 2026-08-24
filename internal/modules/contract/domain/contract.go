package domain

import (
	"time"
)

type ContractStatus string

const (
	ContractStatusDraft  ContractStatus = "draft"
	ContractStatusShared ContractStatus = "shared"
	ContractStatusSigned ContractStatus = "signed"
)

type Contract struct {
	ID                string                 `json:"id"`
	CompanyID         string                 `json:"company_id"`
	QuotationID       string                 `json:"quotation_id"`
	RevisionID        string                 `json:"revision_id"`
	ContractNumber    string                 `json:"contract_number"`
	PublicToken       string                 `json:"public_token"`
	Status            ContractStatus         `json:"status"`
	ClientSnapshot    map[string]interface{} `json:"client_snapshot"`
	ScopeData         *string                `json:"scope_data"`
	DeliverablesData  *string                `json:"deliverables_data"`
	CommercialTerms   *string                `json:"commercial_terms"`
	PaymentTerms      *string                `json:"payment_terms"`
	TimelineData      *string                `json:"timeline_data"`
	AdditionalTerms   *string                `json:"additional_terms"`
	SignatureImageURL *string                `json:"signature_image_url"`
	SignedAt          *time.Time             `json:"signed_at"`
	SignedIP          *string                `json:"signed_ip"`
	CreatedBy         *string                `json:"created_by"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type CreateContractParam struct {
	CompanyID      string
	QuotationID    string
	RevisionID     string
	ContractNumber string
	ClientSnapshot map[string]interface{}
	CreatedBy      *string
}

type UpdateContractParam struct {
	ScopeData        *string
	DeliverablesData *string
	CommercialTerms  *string
	PaymentTerms     *string
	TimelineData     *string
	AdditionalTerms  *string
}
