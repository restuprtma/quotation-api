package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxCustomDevelopmentMoney                 = int64(9_000_000_000_000_000)
	MaxCustomDevelopmentShortTextRunes         = 200
	MaxCustomDevelopmentLongTextRunes          = 4_000
	MaxCustomDevelopmentPeriodRunes            = 50
	MaxCustomDevelopmentGroups                = 30
	MaxCustomDevelopmentItemsPerGroup         = 100
	MaxCustomDevelopmentHeadcount             = 100
	MaxCustomDevelopmentMonthlyItems          = 50
	MaxCustomDevelopmentMonthlyQuantity       = 10_000
	MaxCustomDevelopmentMonthlyNotes          = 20
	MaxCustomDevelopmentModuleGroups          = 30
	MaxCustomDevelopmentFeaturesPerGroup      = 100
	MaxCustomDevelopmentTimelineRows          = 100
	MaxCustomDevelopmentPaymentRows           = 60
	MaxCustomDevelopmentRightsStatements      = 20
	MaxCustomDevelopmentLimitationCodes       = 4
	MaxCustomDevelopmentAdditionalLimitations = 20
)

type CustomDevelopmentTableMode string

const (
	CustomDevelopmentMonthlyPeriod CustomDevelopmentTableMode = "monthly_period"
	CustomDevelopmentDailyPeriod   CustomDevelopmentTableMode = "daily_period"
	CustomDevelopmentFixedDays     CustomDevelopmentTableMode = "fixed_days"
)

func (m CustomDevelopmentTableMode) valid() bool {
	return m == CustomDevelopmentMonthlyPeriod || m == CustomDevelopmentDailyPeriod || m == CustomDevelopmentFixedDays
}

type CustomDevelopmentTaxTreatment string

const (
	CustomDevelopmentTaxNotStated CustomDevelopmentTaxTreatment = "not_stated"
	CustomDevelopmentTaxExclusive CustomDevelopmentTaxTreatment = "exclusive_ppn"
	CustomDevelopmentTaxInclusive CustomDevelopmentTaxTreatment = "inclusive_ppn"
)

func (t CustomDevelopmentTaxTreatment) valid() bool {
	return t == CustomDevelopmentTaxNotStated || t == CustomDevelopmentTaxExclusive || t == CustomDevelopmentTaxInclusive
}

type CustomDevelopmentPaymentComponent string

const (
	CustomDevelopmentPaymentProject CustomDevelopmentPaymentComponent = "development"
	CustomDevelopmentPaymentMonthly CustomDevelopmentPaymentComponent = "monthly_recurring"
)

func (c CustomDevelopmentPaymentComponent) valid() bool {
	return c == CustomDevelopmentPaymentProject || c == CustomDevelopmentPaymentMonthly
}

type CustomDevelopmentCostItemInput struct {
	Name            string `json:"name"`
	DisplayRate     *int64 `json:"display_rate,omitempty"`
	Headcount       int32  `json:"headcount"`
	PeriodLabel     string `json:"period_label"`
	QuotedLineTotal *int64 `json:"quoted_line_total,omitempty"`
}

type CustomDevelopmentCostGroupInput struct {
	Title string                           `json:"title"`
	Items []CustomDevelopmentCostItemInput `json:"items"`
}

type CustomDevelopmentMonthlyItemInput struct {
	Name          string `json:"name"`
	Specification string `json:"specification,omitempty"`
	UnitRate      int64  `json:"unit_rate"`
	Quantity      int32  `json:"quantity"`
}

type CustomDevelopmentModuleGroupInput struct {
	Name     string   `json:"name"`
	Features []string `json:"features"`
}

type CustomDevelopmentTimelineRowInput struct {
	Description string `json:"description"`
	Timeline    string `json:"timeline"`
}

type CustomDevelopmentPaymentRowInput struct {
	WorkPeriod       string                            `json:"work_period"`
	Terms            string                            `json:"terms"`
	InvoiceDateLabel string                            `json:"invoice_date_label"`
	DueDateLabel     string                            `json:"due_date_label"`
	Component        CustomDevelopmentPaymentComponent `json:"component"`
	Nominal          int64                             `json:"nominal"`
}

type CustomDevelopmentInput struct {
	SystemName            string                              `json:"system_name"`
	QuotationDate         time.Time                           `json:"quotation_date"`
	DevelopmentTableMode  CustomDevelopmentTableMode          `json:"development_table_mode"`
	TaxTreatment          CustomDevelopmentTaxTreatment       `json:"tax_treatment"`
	DevelopmentGroups     []CustomDevelopmentCostGroupInput    `json:"development_groups"`
	DiscountLabel         string                              `json:"discount_label"`
	DiscountAmount        int64                               `json:"discount_amount"`
	ShowDiscountRow       bool                                `json:"show_discount_row"`
	MonthlyItems          []CustomDevelopmentMonthlyItemInput `json:"monthly_items,omitempty"`
	MonthlyNotes          []string                            `json:"monthly_notes,omitempty"`
	ModuleGroups          []CustomDevelopmentModuleGroupInput `json:"module_groups"`
	TimelineRows          []CustomDevelopmentTimelineRowInput `json:"timeline_rows"`
	PaymentSchedule       []CustomDevelopmentPaymentRowInput  `json:"payment_schedule"`
	RightsStatements      []string                            `json:"rights_statements,omitempty"`
	LimitationCodes       []string                            `json:"limitation_codes"`
	AdditionalLimitations []string                            `json:"additional_limitations,omitempty"`
}

var customDevelopmentLimitationCodes = map[string]struct{}{
	"out_of_pocket": {},
	"overtime":      {},
	"third_party":   {},
	"scope_change":  {},
}

func (in *CustomDevelopmentInput) UnmarshalJSON(data []byte) error {
	type inputAlias CustomDevelopmentInput
	var wire struct {
		*inputAlias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.inputAlias = (*inputAlias)(&CustomDevelopmentInput{})
	if err := decodeStrictJSON(data, &wire); err != nil {
		return err
	}
	qDate, err := decodeOptionalDate(wire.QuotationDate)
	if err != nil {
		return fmt.Errorf("quotation_date: %w", err)
	}
	if qDate == nil || qDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	wire.inputAlias.QuotationDate = *qDate
	*in = CustomDevelopmentInput(*wire.inputAlias)
	return nil
}

func (in CustomDevelopmentInput) Validate() error {
	systemName := strings.TrimSpace(in.SystemName)
	if systemName == "" {
		return fmt.Errorf("system_name: is required")
	}
	if len([]rune(systemName)) > MaxCustomDevelopmentShortTextRunes {
		return fmt.Errorf("system_name: must be at most %d characters", MaxCustomDevelopmentShortTextRunes)
	}
	if in.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	if !in.DevelopmentTableMode.valid() {
		return fmt.Errorf("development_table_mode: unsupported value %q", in.DevelopmentTableMode)
	}
	if !in.TaxTreatment.valid() {
		return fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}

	subtotal, err := validateCustomDevelopmentGroups(in.DevelopmentGroups, in.DevelopmentTableMode)
	if err != nil {
		return err
	}

	if len([]rune(strings.TrimSpace(in.DiscountLabel))) > MaxCustomDevelopmentShortTextRunes {
		return fmt.Errorf("discount_label: must be at most %d characters", MaxCustomDevelopmentShortTextRunes)
	}
	if in.DiscountAmount < 0 {
		return fmt.Errorf("discount_amount: must be greater than or equal to zero")
	}
	if in.DiscountAmount > MaxCustomDevelopmentMoney {
		return fmt.Errorf("discount_amount: exceeds maximum safe amount")
	}
	if in.DiscountAmount > 0 && !in.ShowDiscountRow {
		return fmt.Errorf("show_discount_row: discount is positive but show_discount_row is false")
	}
	if in.DiscountAmount > subtotal {
		return fmt.Errorf("discount_amount: cannot exceed development subtotal")
	}

	if err := validateCustomDevelopmentMonthly(in.MonthlyItems, in.MonthlyNotes); err != nil {
		return err
	}
	if err := validateCustomDevelopmentModuleGroups(in.ModuleGroups); err != nil {
		return err
	}
	if err := validateCustomDevelopmentTimelineRows(in.TimelineRows); err != nil {
		return err
	}
	if err := validateCustomDevelopmentPaymentSchedule(in.PaymentSchedule); err != nil {
		return err
	}
	if err := validateCustomDevelopmentRightsStatements(in.RightsStatements); err != nil {
		return err
	}
	if err := validateCustomDevelopmentLimitationCodes(in.LimitationCodes); err != nil {
		return err
	}
	return validateCustomDevelopmentAdditionalLimitations(in.AdditionalLimitations)
}

func validateCustomDevelopmentGroups(groups []CustomDevelopmentCostGroupInput, mode CustomDevelopmentTableMode) (int64, error) {
	if len(groups) < 1 || len(groups) > MaxCustomDevelopmentGroups {
		return 0, fmt.Errorf("development_groups: must contain between 1 and %d groups", MaxCustomDevelopmentGroups)
	}
	var subtotal int64
	pricedItemsCount := 0

	for gIdx, group := range groups {
		groupTitle := strings.TrimSpace(group.Title)
		if groupTitle == "" {
			return 0, fmt.Errorf("development_groups[%d].title: is required", gIdx)
		}
		if len([]rune(groupTitle)) > MaxCustomDevelopmentShortTextRunes {
			return 0, fmt.Errorf("development_groups[%d].title: must be at most %d characters", gIdx, MaxCustomDevelopmentShortTextRunes)
		}
		if len(group.Items) < 1 || len(group.Items) > MaxCustomDevelopmentItemsPerGroup {
			return 0, fmt.Errorf("development_groups[%d].items: must contain between 1 and %d items", gIdx, MaxCustomDevelopmentItemsPerGroup)
		}

		for iIdx, item := range group.Items {
			itemName := strings.TrimSpace(item.Name)
			if itemName == "" {
				return 0, fmt.Errorf("development_groups[%d].items[%d].name: is required", gIdx, iIdx)
			}
			if len([]rune(itemName)) > MaxCustomDevelopmentShortTextRunes {
				return 0, fmt.Errorf("development_groups[%d].items[%d].name: must be at most %d characters", gIdx, iIdx, MaxCustomDevelopmentShortTextRunes)
			}
			if item.Headcount < 1 || item.Headcount > MaxCustomDevelopmentHeadcount {
				return 0, fmt.Errorf("development_groups[%d].items[%d].headcount: must be between 1 and %d", gIdx, iIdx, MaxCustomDevelopmentHeadcount)
			}
			periodLabel := strings.TrimSpace(item.PeriodLabel)
			if periodLabel == "" {
				return 0, fmt.Errorf("development_groups[%d].items[%d].period_label: is required", gIdx, iIdx)
			}
			if len([]rune(periodLabel)) > MaxCustomDevelopmentPeriodRunes {
				return 0, fmt.Errorf("development_groups[%d].items[%d].period_label: must be at most %d characters", gIdx, iIdx, MaxCustomDevelopmentPeriodRunes)
			}

			if mode == CustomDevelopmentFixedDays && item.DisplayRate != nil {
				return 0, fmt.Errorf("development_groups[%d].items[%d].display_rate: must be nil for fixed_days table mode", gIdx, iIdx)
			}
			if item.DisplayRate != nil {
				if *item.DisplayRate <= 0 {
					return 0, fmt.Errorf("development_groups[%d].items[%d].display_rate: must be greater than zero", gIdx, iIdx)
				}
				if *item.DisplayRate > MaxCustomDevelopmentMoney {
					return 0, fmt.Errorf("development_groups[%d].items[%d].display_rate: exceeds maximum safe amount", gIdx, iIdx)
				}
			}

			if item.QuotedLineTotal != nil {
				if *item.QuotedLineTotal <= 0 {
					return 0, fmt.Errorf("development_groups[%d].items[%d].quoted_line_total: must be greater than zero", gIdx, iIdx)
				}
				if *item.QuotedLineTotal > MaxCustomDevelopmentMoney {
					return 0, fmt.Errorf("development_groups[%d].items[%d].quoted_line_total: exceeds maximum safe amount", gIdx, iIdx)
				}
				if *item.QuotedLineTotal > MaxCustomDevelopmentMoney-subtotal {
					return 0, fmt.Errorf("development_groups: subtotal exceeds maximum safe amount")
				}
				subtotal += *item.QuotedLineTotal
				pricedItemsCount++
			}
		}
	}

	if pricedItemsCount == 0 {
		return 0, fmt.Errorf("development_groups: must contain at least one priced item")
	}

	return subtotal, nil
}

func validateCustomDevelopmentMonthly(items []CustomDevelopmentMonthlyItemInput, notes []string) error {
	if len(items) > MaxCustomDevelopmentMonthlyItems {
		return fmt.Errorf("monthly_items: must contain at most %d items", MaxCustomDevelopmentMonthlyItems)
	}
	for i, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("monthly_items[%d].name: is required", i)
		}
		if len([]rune(name)) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("monthly_items[%d].name: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
		spec := strings.TrimSpace(item.Specification)
		if len([]rune(spec)) > MaxCustomDevelopmentLongTextRunes {
			return fmt.Errorf("monthly_items[%d].specification: must be at most %d characters", i, MaxCustomDevelopmentLongTextRunes)
		}
		if item.UnitRate <= 0 {
			return fmt.Errorf("monthly_items[%d].unit_rate: must be greater than zero", i)
		}
		if item.UnitRate > MaxCustomDevelopmentMoney {
			return fmt.Errorf("monthly_items[%d].unit_rate: exceeds maximum safe amount", i)
		}
		if item.Quantity < 1 || item.Quantity > MaxCustomDevelopmentMonthlyQuantity {
			return fmt.Errorf("monthly_items[%d].quantity: must be between 1 and %d", i, MaxCustomDevelopmentMonthlyQuantity)
		}
		if item.UnitRate > MaxCustomDevelopmentMoney/int64(item.Quantity) {
			return fmt.Errorf("monthly_items[%d]: line calculation exceeds maximum safe amount", i)
		}
	}

	if len(notes) > 0 && len(items) == 0 {
		return fmt.Errorf("monthly_notes: cannot be present when monthly_items is empty")
	}
	if len(notes) > MaxCustomDevelopmentMonthlyNotes {
		return fmt.Errorf("monthly_notes: must contain at most %d notes", MaxCustomDevelopmentMonthlyNotes)
	}
	for i, note := range notes {
		trimmed := strings.TrimSpace(note)
		if trimmed == "" {
			return fmt.Errorf("monthly_notes[%d]: note cannot be empty", i)
		}
		if len([]rune(trimmed)) > MaxCustomDevelopmentLongTextRunes {
			return fmt.Errorf("monthly_notes[%d]: must be at most %d characters", i, MaxCustomDevelopmentLongTextRunes)
		}
	}
	return nil
}

func validateCustomDevelopmentModuleGroups(groups []CustomDevelopmentModuleGroupInput) error {
	if len(groups) < 1 || len(groups) > MaxCustomDevelopmentModuleGroups {
		return fmt.Errorf("module_groups: must contain between 1 and %d groups", MaxCustomDevelopmentModuleGroups)
	}
	moduleNames := make(map[string]struct{}, len(groups))
	for i, g := range groups {
		nameTrimmed := strings.TrimSpace(g.Name)
		if nameTrimmed == "" {
			return fmt.Errorf("module_groups[%d].name: is required", i)
		}
		if len([]rune(nameTrimmed)) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("module_groups[%d].name: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
		nameKey := strings.ToLower(nameTrimmed)
		if _, exists := moduleNames[nameKey]; exists {
			return fmt.Errorf("module_groups[%d].name: duplicate module name %q", i, g.Name)
		}
		moduleNames[nameKey] = struct{}{}

		if len(g.Features) < 1 || len(g.Features) > MaxCustomDevelopmentFeaturesPerGroup {
			return fmt.Errorf("module_groups[%d].features: must contain between 1 and %d features", i, MaxCustomDevelopmentFeaturesPerGroup)
		}
		for j, feat := range g.Features {
			featTrimmed := strings.TrimSpace(feat)
			if featTrimmed == "" {
				return fmt.Errorf("module_groups[%d].features[%d]: feature string cannot be empty", i, j)
			}
			if len([]rune(featTrimmed)) > MaxCustomDevelopmentLongTextRunes {
				return fmt.Errorf("module_groups[%d].features[%d]: feature string must be at most %d characters", i, j, MaxCustomDevelopmentLongTextRunes)
			}
		}
	}
	return nil
}

func validateCustomDevelopmentTimelineRows(rows []CustomDevelopmentTimelineRowInput) error {
	if len(rows) < 1 || len(rows) > MaxCustomDevelopmentTimelineRows {
		return fmt.Errorf("timeline_rows: must contain between 1 and %d rows", MaxCustomDevelopmentTimelineRows)
	}
	for i, row := range rows {
		descTrimmed := strings.TrimSpace(row.Description)
		if descTrimmed == "" {
			return fmt.Errorf("timeline_rows[%d].description: is required", i)
		}
		if len([]rune(descTrimmed)) > MaxCustomDevelopmentLongTextRunes {
			return fmt.Errorf("timeline_rows[%d].description: must be at most %d characters", i, MaxCustomDevelopmentLongTextRunes)
		}
		timeTrimmed := strings.TrimSpace(row.Timeline)
		if timeTrimmed == "" {
			return fmt.Errorf("timeline_rows[%d].timeline: is required", i)
		}
		if len([]rune(timeTrimmed)) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("timeline_rows[%d].timeline: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
	}
	return nil
}

func validateCustomDevelopmentPaymentSchedule(schedule []CustomDevelopmentPaymentRowInput) error {
	if len(schedule) < 1 || len(schedule) > MaxCustomDevelopmentPaymentRows {
		return fmt.Errorf("payment_schedule: must contain between 1 and %d rows", MaxCustomDevelopmentPaymentRows)
	}
	for i, row := range schedule {
		if strings.TrimSpace(row.WorkPeriod) == "" {
			return fmt.Errorf("payment_schedule[%d].work_period: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.WorkPeriod))) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].work_period: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
		if strings.TrimSpace(row.Terms) == "" {
			return fmt.Errorf("payment_schedule[%d].terms: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.Terms))) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].terms: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
		if strings.TrimSpace(row.InvoiceDateLabel) == "" {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.InvoiceDateLabel))) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
		if strings.TrimSpace(row.DueDateLabel) == "" {
			return fmt.Errorf("payment_schedule[%d].due_date_label: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.DueDateLabel))) > MaxCustomDevelopmentShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].due_date_label: must be at most %d characters", i, MaxCustomDevelopmentShortTextRunes)
		}
		if !row.Component.valid() {
			return fmt.Errorf("payment_schedule[%d].component: unsupported value %q", i, row.Component)
		}
		if row.Nominal <= 0 {
			return fmt.Errorf("payment_schedule[%d].nominal: must be greater than zero", i)
		}
		if row.Nominal > MaxCustomDevelopmentMoney {
			return fmt.Errorf("payment_schedule[%d].nominal: exceeds maximum safe amount", i)
		}
	}
	return nil
}

func validateCustomDevelopmentRightsStatements(statements []string) error {
	if len(statements) > MaxCustomDevelopmentRightsStatements {
		return fmt.Errorf("rights_statements: must contain at most %d statements", MaxCustomDevelopmentRightsStatements)
	}
	for i, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			return fmt.Errorf("rights_statements[%d]: statement cannot be empty", i)
		}
		if len([]rune(trimmed)) > MaxCustomDevelopmentLongTextRunes {
			return fmt.Errorf("rights_statements[%d]: must be at most %d characters", i, MaxCustomDevelopmentLongTextRunes)
		}
	}
	return nil
}

func validateCustomDevelopmentLimitationCodes(codes []string) error {
	if len(codes) < 1 || len(codes) > MaxCustomDevelopmentLimitationCodes {
		return fmt.Errorf("limitation_codes: must contain between 1 and %d codes", MaxCustomDevelopmentLimitationCodes)
	}
	seenCodes := make(map[string]struct{}, len(codes))
	for i, code := range codes {
		trimmed := strings.TrimSpace(code)
		if _, ok := customDevelopmentLimitationCodes[trimmed]; !ok {
			return fmt.Errorf("limitation_codes[%d]: unsupported limitation %q", i, code)
		}
		if _, exists := seenCodes[trimmed]; exists {
			return fmt.Errorf("limitation_codes[%d]: duplicate limitation code %q", i, code)
		}
		seenCodes[trimmed] = struct{}{}
	}
	return nil
}

func validateCustomDevelopmentAdditionalLimitations(limitations []string) error {
	if len(limitations) > MaxCustomDevelopmentAdditionalLimitations {
		return fmt.Errorf("additional_limitations: must contain at most %d limitations", MaxCustomDevelopmentAdditionalLimitations)
	}
	seenAdditional := make(map[string]struct{}, len(limitations))
	for i, stmt := range limitations {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			return fmt.Errorf("additional_limitations[%d]: limitation cannot be empty", i)
		}
		if len([]rune(trimmed)) > MaxCustomDevelopmentLongTextRunes {
			return fmt.Errorf("additional_limitations[%d]: must be at most %d characters", i, MaxCustomDevelopmentLongTextRunes)
		}
		key := strings.ToLower(trimmed)
		if _, exists := seenAdditional[key]; exists {
			return fmt.Errorf("additional_limitations[%d]: duplicate additional limitation %q", i, stmt)
		}
		seenAdditional[key] = struct{}{}
	}
	return nil
}

type CustomDevelopmentCostLineCalculation struct {
	CustomDevelopmentCostItemInput
	LineTotal *int64 `json:"line_total,omitempty"`
}

type CustomDevelopmentMonthlyLineCalculation struct {
	CustomDevelopmentMonthlyItemInput
	LineTotal int64 `json:"line_total"`
}

type CustomDevelopmentCostGroupCalculation struct {
	Title string                                 `json:"title"`
	Items []CustomDevelopmentCostLineCalculation `json:"items"`
}

type CustomDevelopmentCalculation struct {
	DevelopmentGroups       []CustomDevelopmentCostGroupCalculation   `json:"development_groups"`
	DevelopmentSubtotal     int64                                    `json:"development_subtotal"`
	DiscountAmount          int64                                    `json:"discount_amount"`
	TotalInvestment         int64                                    `json:"total_investment"`
	MonthlyLines            []CustomDevelopmentMonthlyLineCalculation `json:"monthly_lines"`
	MonthlyTotal            int64                                    `json:"monthly_total"`
	DevelopmentPaymentTotal int64                                    `json:"development_payment_total"`
	MonthlyPaymentRows      int32                                    `json:"monthly_payment_rows"`
}

type ResolvedCustomDevelopmentProfile struct {
	TableMode       CustomDevelopmentTableMode `json:"table_mode"`
	RateHeader      string                     `json:"rate_header"`
	PeriodHeader    string                     `json:"period_header"`
	ShowRateColumn  bool                       `json:"show_rate_column"`
	ShowMonthly     bool                       `json:"show_monthly"`
	ShowDiscountRow bool                       `json:"show_discount_row"`
	ShowRights      bool                       `json:"show_rights"`
	TaxClauseText   string                     `json:"tax_clause_text"`
	Sections        []DocumentSection          `json:"sections"`
}
