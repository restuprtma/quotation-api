package dto

type PricingSectionDTO struct {
	SectionName string           `json:"section_name" binding:"required"`
	SectionType string           `json:"section_type" binding:"required,oneof=package service phase resource_group other"`
	Description *string          `json:"description"`
	SortOrder   int              `json:"sort_order"`
	Items       []PricingItemDTO `json:"items" binding:"dive"`
}

type PricingItemDTO struct {
	ItemName             string                 `json:"item_name" binding:"required"`
	Description          *string                `json:"description"`
	Features             map[string]interface{} `json:"features"`
	PricingMode          string                 `json:"pricing_mode" binding:"required,oneof=unit resource fixed"`
	UnitLabel            *string                `json:"unit_label"`
	UnitPrice            int64                  `json:"unit_price" binding:"min=0"`
	Quantity             int64                  `json:"quantity" binding:"min=0"`
	EmployeeCount        int                    `json:"employee_count" binding:"min=0"`
	Duration             int                    `json:"duration" binding:"min=0"`
	DurationUnit         *string                `json:"duration_unit"`
	BillingPeriod        string                 `json:"billing_period" binding:"required,oneof=one_time monthly yearly custom"`
	LineTotal            int64                  `json:"line_total"` // client boleh mengirim tapi diabaikan/dihitung ulang oleh backend
	ManualOverrideReason *string                `json:"manual_override_reason"`
	SortOrder            int                    `json:"sort_order"`
}
