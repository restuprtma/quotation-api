package dto

type CreateQuotationRequest struct {
	ClientID         string  `json:"client_id" binding:"required,uuid"`
	Subject          *string `json:"subject"`
	SolutionType     string  `json:"solution_type" binding:"required"`
	CommercialScheme string  `json:"commercial_scheme" binding:"required"`
	BudgetMaximum    *int64  `json:"budget_maximum"`
}

type UpdateQuotationRequest struct {
	Subject          *string                `json:"subject"`
	SolutionType     *string                `json:"solution_type"`
	CommercialScheme *string                `json:"commercial_scheme"`
	BudgetMaximum    *int64                 `json:"budget_maximum"`
	QuotationDate    *string                `json:"quotation_date"`
	ValidUntil       *string                `json:"valid_until"`
	ClientSnapshot   map[string]interface{} `json:"client_snapshot"`
}
