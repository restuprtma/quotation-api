package dto

import "time"

// SignPublicQuotationRequest contains the PNG data URL drawn by the client.
// The service validates the prefix, decoded payload, and size before storage.
type SignPublicQuotationRequest struct {
	SignatureImage string `json:"signature_image" binding:"required"`
}

// PublicQuotationItemResponse exposes only price-line fields needed by a
// client-facing quotation document.
type PublicQuotationItemResponse struct {
	ItemName    string  `json:"item_name"`
	Description *string `json:"description,omitempty"`
	Amount      int64   `json:"amount"`
	SortOrder   int     `json:"sort_order"`
}

// PublicQuotationRevisionResponse intentionally excludes tenant, creator,
// audit IP, and raw AI response fields.
type PublicQuotationRevisionResponse struct {
	RevisionNumber     int                           `json:"revision_number"`
	PublicToken        string                        `json:"public_token"`
	Budget             int64                         `json:"budget"`
	ProjectDescription *string                       `json:"project_description"`
	ScopeOfWork        *string                       `json:"scope_of_work"`
	MaintenanceInfo    *string                       `json:"maintenance_info"`
	HostingInfo        *string                       `json:"hosting_info"`
	Notes              *string                       `json:"notes"`
	NarrativeData      map[string]interface{}        `json:"narrative_data,omitempty"`
	SignatureImageURL  *string                       `json:"signature_image_url"`
	SignedAt           *time.Time                    `json:"signed_at"`
	Items              []PublicQuotationItemResponse `json:"items"`
}

// PublicQuotationResponse is the safe client-facing quotation shape.
type PublicQuotationResponse struct {
	QuotationNumber string                           `json:"quotation_number"`
	ClientName      *string                          `json:"client_name,omitempty"`
	ProjectType     string                           `json:"project_type"`
	Status          string                           `json:"status"`
	CreatedAt       time.Time                        `json:"created_at"`
	Revision        *PublicQuotationRevisionResponse `json:"revision,omitempty"`
}

// PublicBlueprintResponse is the shape returned for the public blueprint endpoint.
type PublicBlueprintResponse struct {
	QuotationNumber string                 `json:"quotation_number"`
	ClientName      *string                `json:"client_name,omitempty"`
	Subject         *string                `json:"subject"`
	BlueprintData   map[string]interface{} `json:"blueprint_data"`
}

// PublicDetailResponse is the shape returned for the public product/project detail endpoint.
type PublicDetailResponse struct {
	QuotationNumber   string                 `json:"quotation_number"`
	ClientName        *string                `json:"client_name,omitempty"`
	Subject           *string                `json:"subject"`
	NarrativeData     map[string]interface{} `json:"narrative_data,omitempty"`
	ProductDetailData map[string]interface{} `json:"product_detail_data,omitempty"`
}
