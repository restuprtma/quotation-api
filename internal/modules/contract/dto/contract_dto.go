package dto

import (
	"time"

	"venturo-skeleton-go/internal/modules/contract/domain"
)

type CreateContractRequest struct {
	QuotationID string `json:"quotation_id" binding:"required,uuid"`
}

type UpdateContractRequest struct {
	ScopeData        *string `json:"scope_data"`
	DeliverablesData *string `json:"deliverables_data"`
	CommercialTerms  *string `json:"commercial_terms"`
	PaymentTerms     *string `json:"payment_terms"`
	TimelineData     *string `json:"timeline_data"`
	AdditionalTerms  *string `json:"additional_terms"`
}

type SignContractRequest struct {
	SignatureDataURL string `json:"signature_data_url" binding:"required"`
}

type ContractResponse struct {
	ID               string                 `json:"id"`
	QuotationID      string                 `json:"quotation_id"`
	RevisionID       string                 `json:"revision_id"`
	ContractNumber   string                 `json:"contract_number"`
	PublicToken      string                 `json:"public_token"`
	Status           domain.ContractStatus  `json:"status"`
	ClientSnapshot   map[string]interface{} `json:"client_snapshot"`
	ScopeData        *string                `json:"scope_data"`
	DeliverablesData *string                `json:"deliverables_data"`
	CommercialTerms  *string                `json:"commercial_terms"`
	PaymentTerms     *string                `json:"payment_terms"`
	TimelineData     *string                `json:"timeline_data"`
	AdditionalTerms  *string                `json:"additional_terms"`
	SignedAt         *time.Time             `json:"signed_at"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type PublicContractResponse struct {
	QuotationID      string                 `json:"quotation_id"`
	RevisionID       string                 `json:"revision_id"`
	ContractNumber   string                 `json:"contract_number"`
	Status           domain.ContractStatus  `json:"status"`
	ClientSnapshot   map[string]interface{} `json:"client_snapshot"`
	ScopeData        *string                `json:"scope_data"`
	DeliverablesData *string                `json:"deliverables_data"`
	CommercialTerms  *string                `json:"commercial_terms"`
	PaymentTerms     *string                `json:"payment_terms"`
	TimelineData     *string                `json:"timeline_data"`
	AdditionalTerms  *string                `json:"additional_terms"`
	SignedAt         *time.Time             `json:"signed_at"`
	CreatedAt        time.Time              `json:"created_at"`
}
