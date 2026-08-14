package dto

// CreateQuotationItemRequest is an optional price line submitted together
// with a new quotation. Prices are input manually — no auto-pricing engine.
type CreateQuotationItemRequest struct {
	ItemName    string `json:"item_name" binding:"required"`
	Description string `json:"description"`
	Amount      int64  `json:"amount" binding:"min=0"`
	SortOrder   int    `json:"sort_order"`
}

type CreateQuotationRequest struct {
	ClientID    string                       `json:"client_id" binding:"required,uuid"`
	Subject     *string                      `json:"subject"`
	ProjectType string                       `json:"project_type" binding:"required,oneof=product_based project_based"`
	Budget      int64                        `json:"budget" binding:"required,min=1"`
	Items       []CreateQuotationItemRequest `json:"items"`
	Sections    []PricingSectionDTO          `json:"sections" binding:"dive"`
}

type UpdateQuotationRequest struct {
	// Budget is optional (pointer) so callers can adjust the revision budget
	// without resending the content fields. nil means "keep current budget".
	Budget             *int64                 `json:"budget" binding:"omitempty,min=1"`
	Subject            *string                `json:"subject"`
	ProjectDescription string                 `json:"project_description" binding:"required"`
	ScopeOfWork        string                 `json:"scope_of_work" binding:"required"`
	MaintenanceInfo    string                 `json:"maintenance_info"`
	HostingInfo        string                 `json:"hosting_info"`
	Notes              string                 `json:"notes"`
	BlueprintData      map[string]interface{} `json:"blueprint_data"` // sent as raw json object
	QuotationDate      *string                `json:"quotation_date"`
	ValidUntil         *string                `json:"valid_until"`
	ClientSnapshot     map[string]interface{} `json:"client_snapshot"`
	NarrativeData      map[string]interface{} `json:"narrative_data"`
	ProductDetailData  map[string]interface{} `json:"product_detail_data"`
	Sections           []PricingSectionDTO    `json:"sections" binding:"dive"`
}
