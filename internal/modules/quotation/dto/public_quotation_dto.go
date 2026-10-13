package dto

import "time"

type PublicQuotationClient struct {
	CompanyName string `json:"company_name"`
}

type PublicQuotationResponse struct {
	QuotationNumber string                `json:"quotation_number"`
	Subject         *string               `json:"subject"`
	Client          PublicQuotationClient `json:"client"`
	RevisionNumber  int                   `json:"revision_number"`
	ContentHTML     string                `json:"content_html"`
	PublishedAt     time.Time             `json:"published_at"`
	Signed          bool                  `json:"signed"`
	SignerName      *string               `json:"signer_name"`
	SignedAt        *time.Time            `json:"signed_at"`
}

type SignPublicQuotationRequest struct {
	SignerName    string `json:"signer_name,omitempty"`
	SignerEmail   string `json:"signer_email,omitempty"`
	SignatureData string `json:"signature_data"`
	Consent       bool   `json:"consent"`
}
