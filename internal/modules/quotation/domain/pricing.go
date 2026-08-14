package domain

import (
	"time"

	"github.com/google/uuid"
)

// Constants for Project Type
const (
	ProjectTypeProduct = "product_based"
	ProjectTypeProject = "project_based"
)

// Constants for Pricing Validation
const (
	PricingModeUnit     = "unit"
	PricingModeResource = "resource"
	PricingModeFixed    = "fixed"
)

const (
	BillingPeriodOneTime = "one_time"
	BillingPeriodMonthly = "monthly"
	BillingPeriodYearly  = "yearly"
	BillingPeriodCustom  = "custom"
)

type QuotationPricingSection struct {
	ID          uuid.UUID `json:"id" db:"id"`
	CompanyID   uuid.UUID `json:"company_id" db:"company_id"`
	RevisionID  uuid.UUID `json:"revision_id" db:"revision_id"`
	SectionName string    `json:"section_name" db:"section_name"`
	SectionType string    `json:"section_type" db:"section_type"`
	Description *string   `json:"description" db:"description"`
	SortOrder   int       `json:"sort_order" db:"sort_order"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type QuotationPricingItem struct {
	ID                   uuid.UUID              `json:"id" db:"id"`
	CompanyID            uuid.UUID              `json:"company_id" db:"company_id"`
	SectionID            uuid.UUID              `json:"section_id" db:"section_id"`
	ItemName             string                 `json:"item_name" db:"item_name"`
	Description          *string                `json:"description" db:"description"`
	Features             map[string]interface{} `json:"features" db:"features"`
	PricingMode          string                 `json:"pricing_mode" db:"pricing_mode"`
	UnitLabel            *string                `json:"unit_label" db:"unit_label"`
	UnitPrice            int64                  `json:"unit_price" db:"unit_price"`
	Quantity             int64                  `json:"quantity" db:"quantity"`
	EmployeeCount        int                    `json:"employee_count" db:"employee_count"`
	Duration             int                    `json:"duration" db:"duration"`
	DurationUnit         *string                `json:"duration_unit" db:"duration_unit"`
	BillingPeriod        string                 `json:"billing_period" db:"billing_period"`
	LineTotal            int64                  `json:"line_total" db:"line_total"`
	ManualOverrideReason *string                `json:"manual_override_reason" db:"manual_override_reason"`
	SortOrder            int                    `json:"sort_order" db:"sort_order"`
	CreatedAt            time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at" db:"updated_at"`
}
