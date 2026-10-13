package dto

type SignatureSettingsResponse struct {
	DirectorName          string `json:"director_name"`
	DirectorTitle         string `json:"director_title"`
	DirectorSignatureData string `json:"director_signature_data"`
	CompanyStampData      string `json:"company_stamp_data"`
	IsConfigured          bool   `json:"is_configured"`
}

type UpdateSignatureSettingsRequest struct {
	DirectorName          string `json:"director_name"`
	DirectorTitle         string `json:"director_title"`
	DirectorSignatureData string `json:"director_signature_data"`
	CompanyStampData      string `json:"company_stamp_data"`
}
