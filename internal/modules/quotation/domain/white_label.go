package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxWhiteLabelMoney          = int64(9_000_000_000_000_000)
	MaxWhiteLabelNameRunes      = 200
	MaxWhiteLabelTextRunes      = 1_000
	MaxWhiteLabelLicenseItems   = 50
	MaxWhiteLabelComponentItems = 50
	MaxWhiteLabelPaymentRows    = 60

	MaxWhiteLabelModuleGroups     = 30
	MaxWhiteLabelFeaturesPerGroup = 100
	MaxWhiteLabelTimelineRows     = 100
	MaxWhiteLabelRightsStatements = 20
	MaxWhiteLabelLimitationCodes  = 10
)

type WhiteLabelTaxTreatment string

const (
	WhiteLabelTaxNotStated WhiteLabelTaxTreatment = "not_stated"
	WhiteLabelTaxExclusive WhiteLabelTaxTreatment = "exclusive_ppn"
	WhiteLabelTaxInclusive WhiteLabelTaxTreatment = "inclusive_ppn"
)

func (t WhiteLabelTaxTreatment) valid() bool {
	return t == WhiteLabelTaxNotStated || t == WhiteLabelTaxExclusive || t == WhiteLabelTaxInclusive
}

type WhiteLabelPaymentComponent string

const (
	WhiteLabelPaymentLicense            WhiteLabelPaymentComponent = "license"
	WhiteLabelPaymentMaintenanceHosting WhiteLabelPaymentComponent = "maintenance_hosting"
)

func (c WhiteLabelPaymentComponent) valid() bool {
	return c == WhiteLabelPaymentLicense || c == WhiteLabelPaymentMaintenanceHosting
}

type WhiteLabelLicenseItemInput struct {
	Name     string `json:"name"`
	UnitRate int64  `json:"unit_rate"`
	Quantity int32  `json:"quantity"`
}

type WhiteLabelMaintenanceItemInput struct {
	Name         string `json:"name"`
	MonthlyRate  int64  `json:"monthly_rate"`
	FTEQuarters  int32  `json:"fte_quarters"`
	PeriodMonths int32  `json:"period_months"`
}

type WhiteLabelHostingItemInput struct {
	Name         string `json:"name"`
	MonthlyRate  int64  `json:"monthly_rate"`
	Quantity     int32  `json:"quantity"`
	PeriodMonths int32  `json:"period_months"`
}

type WhiteLabelModuleGroupInput struct {
	Name     string   `json:"name"`
	Features []string `json:"features"`
}

type WhiteLabelTimelineRowInput struct {
	Description string `json:"description"`
	Timeline    string `json:"timeline"`
}

type WhiteLabelPaymentRowInput struct {
	WorkPeriod       string                     `json:"work_period"`
	Terms            string                     `json:"terms"`
	InvoiceDateLabel string                     `json:"invoice_date_label"`
	DueDateLabel     string                     `json:"due_date_label"`
	Component        WhiteLabelPaymentComponent `json:"component"`
	Nominal          int64                      `json:"nominal"`
}

type WhiteLabelInput struct {
	ProductName      string                           `json:"product_name"`
	QuotationDate    time.Time                        `json:"quotation_date"`
	TaxTreatment     WhiteLabelTaxTreatment           `json:"tax_treatment"`
	LicenseItems     []WhiteLabelLicenseItemInput     `json:"license_items"`
	MaintenanceItems []WhiteLabelMaintenanceItemInput `json:"maintenance_items,omitempty"`
	HostingItems     []WhiteLabelHostingItemInput     `json:"hosting_items,omitempty"`
	ModuleGroups     []WhiteLabelModuleGroupInput     `json:"module_groups,omitempty"`
	TimelineRows     []WhiteLabelTimelineRowInput     `json:"timeline_rows,omitempty"`
	PaymentSchedule  []WhiteLabelPaymentRowInput      `json:"payment_schedule"`
	RightsStatements []string                         `json:"rights_statements,omitempty"`
	LimitationCodes  []string                         `json:"limitation_codes"`
}

var whiteLabelLimitationCodes = map[string]struct{}{
	"out_of_pocket": {},
	"overtime":      {},
	"third_party":   {},
	"scope_change":  {},
}

func (in *WhiteLabelInput) UnmarshalJSON(data []byte) error {
	type inputAlias WhiteLabelInput
	var wire struct {
		*inputAlias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.inputAlias = (*inputAlias)(&WhiteLabelInput{})
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
	*in = WhiteLabelInput(*wire.inputAlias)
	return nil
}

func (in WhiteLabelInput) Validate() error {
	if strings.TrimSpace(in.ProductName) == "" {
		return fmt.Errorf("product_name: is required")
	}
	if len([]rune(strings.TrimSpace(in.ProductName))) > MaxWhiteLabelNameRunes {
		return fmt.Errorf("product_name: must be at most %d characters", MaxWhiteLabelNameRunes)
	}
	if in.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	if !in.TaxTreatment.valid() {
		return fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}

	if err := validateWhiteLabelLicenseItems(in.LicenseItems); err != nil {
		return err
	}
	if err := validateWhiteLabelMaintenanceItems(in.MaintenanceItems); err != nil {
		return err
	}
	if err := validateWhiteLabelHostingItems(in.HostingItems); err != nil {
		return err
	}
	if err := validateWhiteLabelModuleGroups(in.ModuleGroups); err != nil {
		return err
	}
	if err := validateWhiteLabelTimelineRows(in.TimelineRows); err != nil {
		return err
	}
	if err := validateWhiteLabelPaymentSchedule(in.PaymentSchedule); err != nil {
		return err
	}
	if err := validateWhiteLabelRightsStatements(in.RightsStatements); err != nil {
		return err
	}
	return validateWhiteLabelLimitationCodes(in.LimitationCodes)
}

func validateWhiteLabelLicenseItems(items []WhiteLabelLicenseItemInput) error {
	if len(items) < 1 || len(items) > MaxWhiteLabelLicenseItems {
		return fmt.Errorf("license_items: must contain between 1 and %d items", MaxWhiteLabelLicenseItems)
	}
	for i, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("license_items[%d].name: is required", i)
		}
		if len([]rune(strings.TrimSpace(item.Name))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("license_items[%d].name: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if item.UnitRate <= 0 {
			return fmt.Errorf("license_items[%d].unit_rate: must be greater than zero", i)
		}
		if item.UnitRate > MaxWhiteLabelMoney {
			return fmt.Errorf("license_items[%d].unit_rate: exceeds maximum safe amount", i)
		}
		if item.Quantity <= 0 {
			return fmt.Errorf("license_items[%d].quantity: must be greater than zero", i)
		}
	}
	return nil
}

func validateWhiteLabelMaintenanceItems(items []WhiteLabelMaintenanceItemInput) error {
	if len(items) > MaxWhiteLabelComponentItems {
		return fmt.Errorf("maintenance_items: must contain at most %d items", MaxWhiteLabelComponentItems)
	}
	for i, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("maintenance_items[%d].name: is required", i)
		}
		if len([]rune(strings.TrimSpace(item.Name))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("maintenance_items[%d].name: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if item.MonthlyRate <= 0 {
			return fmt.Errorf("maintenance_items[%d].monthly_rate: must be greater than zero", i)
		}
		if item.MonthlyRate > MaxWhiteLabelMoney {
			return fmt.Errorf("maintenance_items[%d].monthly_rate: exceeds maximum safe amount", i)
		}
		if item.FTEQuarters < 1 || item.FTEQuarters > 4 {
			return fmt.Errorf("maintenance_items[%d].fte_quarters: must be between 1 and 4", i)
		}
		if item.PeriodMonths <= 0 {
			return fmt.Errorf("maintenance_items[%d].period_months: must be greater than zero", i)
		}

		if item.MonthlyRate > MaxWhiteLabelMoney/int64(item.FTEQuarters) {
			return fmt.Errorf("maintenance_items[%d]: line calculation exceeds maximum safe amount", i)
		}
		inter := item.MonthlyRate * int64(item.FTEQuarters)
		if inter > MaxWhiteLabelMoney/int64(item.PeriodMonths) {
			return fmt.Errorf("maintenance_items[%d]: line calculation exceeds maximum safe amount", i)
		}
		numerator := inter * int64(item.PeriodMonths)
		if numerator%4 != 0 {
			return fmt.Errorf("maintenance_items[%d]: line calculation must be a whole rupiah amount (divisible by 4)", i)
		}
	}
	return nil
}

func validateWhiteLabelHostingItems(items []WhiteLabelHostingItemInput) error {
	if len(items) > MaxWhiteLabelComponentItems {
		return fmt.Errorf("hosting_items: must contain at most %d items", MaxWhiteLabelComponentItems)
	}
	for i, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("hosting_items[%d].name: is required", i)
		}
		if len([]rune(strings.TrimSpace(item.Name))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("hosting_items[%d].name: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if item.MonthlyRate <= 0 {
			return fmt.Errorf("hosting_items[%d].monthly_rate: must be greater than zero", i)
		}
		if item.MonthlyRate > MaxWhiteLabelMoney {
			return fmt.Errorf("hosting_items[%d].monthly_rate: exceeds maximum safe amount", i)
		}
		if item.Quantity <= 0 {
			return fmt.Errorf("hosting_items[%d].quantity: must be greater than zero", i)
		}
		if item.PeriodMonths <= 0 {
			return fmt.Errorf("hosting_items[%d].period_months: must be greater than zero", i)
		}
	}
	return nil
}

func validateWhiteLabelModuleGroups(groups []WhiteLabelModuleGroupInput) error {
	if len(groups) > MaxWhiteLabelModuleGroups {
		return fmt.Errorf("module_groups: must contain at most %d groups", MaxWhiteLabelModuleGroups)
	}
	moduleNames := make(map[string]struct{}, len(groups))
	for i, g := range groups {
		nameTrimmed := strings.TrimSpace(g.Name)
		if nameTrimmed == "" {
			return fmt.Errorf("module_groups[%d].name: is required", i)
		}
		if len([]rune(nameTrimmed)) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("module_groups[%d].name: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		nameKey := strings.ToLower(nameTrimmed)
		if _, exists := moduleNames[nameKey]; exists {
			return fmt.Errorf("module_groups[%d].name: duplicate module name %q", i, g.Name)
		}
		moduleNames[nameKey] = struct{}{}

		if len(g.Features) > MaxWhiteLabelFeaturesPerGroup {
			return fmt.Errorf("module_groups[%d].features: must contain at most %d features", i, MaxWhiteLabelFeaturesPerGroup)
		}
		for j, feat := range g.Features {
			featTrimmed := strings.TrimSpace(feat)
			if featTrimmed == "" {
				return fmt.Errorf("module_groups[%d].features[%d]: feature string cannot be empty", i, j)
			}
			if len([]rune(featTrimmed)) > MaxWhiteLabelNameRunes {
				return fmt.Errorf("module_groups[%d].features[%d]: feature string must be at most %d characters", i, j, MaxWhiteLabelNameRunes)
			}
		}
	}
	return nil
}

func validateWhiteLabelTimelineRows(rows []WhiteLabelTimelineRowInput) error {
	if len(rows) > MaxWhiteLabelTimelineRows {
		return fmt.Errorf("timeline_rows: must contain at most %d rows", MaxWhiteLabelTimelineRows)
	}
	for i, row := range rows {
		descTrimmed := strings.TrimSpace(row.Description)
		if descTrimmed == "" {
			return fmt.Errorf("timeline_rows[%d].description: is required", i)
		}
		if len([]rune(descTrimmed)) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("timeline_rows[%d].description: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		timeTrimmed := strings.TrimSpace(row.Timeline)
		if timeTrimmed == "" {
			return fmt.Errorf("timeline_rows[%d].timeline: is required", i)
		}
		if len([]rune(timeTrimmed)) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("timeline_rows[%d].timeline: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
	}
	return nil
}

func validateWhiteLabelPaymentSchedule(schedule []WhiteLabelPaymentRowInput) error {
	if len(schedule) < 1 || len(schedule) > MaxWhiteLabelPaymentRows {
		return fmt.Errorf("payment_schedule: must contain between 1 and %d payment rows", MaxWhiteLabelPaymentRows)
	}
	for i, row := range schedule {
		if strings.TrimSpace(row.WorkPeriod) == "" {
			return fmt.Errorf("payment_schedule[%d].work_period: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.WorkPeriod))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("payment_schedule[%d].work_period: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if strings.TrimSpace(row.Terms) == "" {
			return fmt.Errorf("payment_schedule[%d].terms: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.Terms))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("payment_schedule[%d].terms: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if strings.TrimSpace(row.InvoiceDateLabel) == "" {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.InvoiceDateLabel))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if strings.TrimSpace(row.DueDateLabel) == "" {
			return fmt.Errorf("payment_schedule[%d].due_date_label: is required", i)
		}
		if len([]rune(strings.TrimSpace(row.DueDateLabel))) > MaxWhiteLabelNameRunes {
			return fmt.Errorf("payment_schedule[%d].due_date_label: must be at most %d characters", i, MaxWhiteLabelNameRunes)
		}
		if !row.Component.valid() {
			return fmt.Errorf("payment_schedule[%d].component: unsupported value %q", i, row.Component)
		}
		if row.Nominal <= 0 {
			return fmt.Errorf("payment_schedule[%d].nominal: must be greater than zero", i)
		}
		if row.Nominal > MaxWhiteLabelMoney {
			return fmt.Errorf("payment_schedule[%d].nominal: exceeds maximum safe amount", i)
		}
	}
	return nil
}

func validateWhiteLabelRightsStatements(statements []string) error {
	if len(statements) > MaxWhiteLabelRightsStatements {
		return fmt.Errorf("rights_statements: must contain at most %d statements", MaxWhiteLabelRightsStatements)
	}
	for i, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			return fmt.Errorf("rights_statements[%d]: statement cannot be empty", i)
		}
		if len([]rune(trimmed)) > MaxWhiteLabelTextRunes {
			return fmt.Errorf("rights_statements[%d]: must be at most %d characters", i, MaxWhiteLabelTextRunes)
		}
	}
	return nil
}

func validateWhiteLabelLimitationCodes(codes []string) error {
	if len(codes) < 1 || len(codes) > MaxWhiteLabelLimitationCodes {
		return fmt.Errorf("limitation_codes: must contain between 1 and %d codes", MaxWhiteLabelLimitationCodes)
	}
	seenCodes := make(map[string]struct{}, len(codes))
	for i, code := range codes {
		trimmed := strings.TrimSpace(code)
		if _, ok := whiteLabelLimitationCodes[trimmed]; !ok {
			return fmt.Errorf("limitation_codes[%d]: unsupported limitation %q", i, code)
		}
		if _, exists := seenCodes[trimmed]; exists {
			return fmt.Errorf("limitation_codes[%d]: duplicate limitation code %q", i, code)
		}
		seenCodes[trimmed] = struct{}{}
	}
	return nil
}

type WhiteLabelLicenseLineCalculation struct {
	WhiteLabelLicenseItemInput
	LineTotal int64 `json:"line_total"`
}

type WhiteLabelMaintenanceLineCalculation struct {
	WhiteLabelMaintenanceItemInput
	LineTotal int64 `json:"line_total"`
}

type WhiteLabelHostingLineCalculation struct {
	WhiteLabelHostingItemInput
	LineTotal int64 `json:"line_total"`
}

type WhiteLabelCalculation struct {
	License                        []WhiteLabelLicenseLineCalculation     `json:"license"`
	Maintenance                    []WhiteLabelMaintenanceLineCalculation `json:"maintenance,omitempty"`
	Hosting                        []WhiteLabelHostingLineCalculation     `json:"hosting,omitempty"`
	LicenseSubtotal                int64                                  `json:"license_subtotal"`
	MaintenanceSubtotal            int64                                  `json:"maintenance_subtotal"`
	HostingSubtotal                int64                                  `json:"hosting_subtotal"`
	MaintenanceHostingSubtotal     int64                                  `json:"maintenance_hosting_subtotal"`
	TotalInvestment                int64                                  `json:"total_investment"`
	LicensePaymentTotal            int64                                  `json:"license_payment_total"`
	MaintenanceHostingPaymentTotal int64                                  `json:"maintenance_hosting_payment_total"`
	PaymentTotal                   int64                                  `json:"payment_total"`
}
