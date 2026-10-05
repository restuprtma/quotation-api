package dto

type PublishBlueprintRequest struct {
	RevisionID     string   `json:"revision_id" binding:"required"`
	ConfirmPartial bool     `json:"confirm_partial"`
	WarningCodes   []string `json:"warning_codes"`
}

type CreateBlueprintRevisionRequest struct {
	Mode                 string `json:"mode"` // "clone" (default) or "regenerate_from_source"
	SourceQuotationRevID string `json:"source_quotation_revision_id,omitempty"`
}
