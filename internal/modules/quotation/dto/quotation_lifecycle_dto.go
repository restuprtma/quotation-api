package dto

type PublishQuotationRequest struct {
	RevisionID                string `json:"revision_id" binding:"required"`
	ContentVersion            int    `json:"content_version" binding:"required"`
	ConfirmLegalModified      bool   `json:"confirm_legal_modified"`
	ConfirmCommercialModified bool   `json:"confirm_commercial_modified"`
}
