package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// PromptWhiteLabelServiceInputKind is the versioned prompt-state payload for
// white-label quotations. Descriptive meeting facts remain separate from
// prices, which are accepted only after a catalog or BA supplies them.
const PromptWhiteLabelServiceInputKind = "white_label_v1"

type PromptWhiteLabelLicenseItem struct {
	Name     domain.Fact[string] `json:"name"`
	UnitRate domain.MoneyInput   `json:"unit_rate"`
	Quantity domain.Fact[int]    `json:"quantity"`
}

type PromptWhiteLabelMaintenanceItem struct {
	Name         domain.Fact[string] `json:"name"`
	MonthlyRate  domain.MoneyInput   `json:"monthly_rate"`
	FTEQuarters  domain.Fact[int]    `json:"fte_quarters"`
	PeriodMonths domain.Fact[int]    `json:"period_months"`
}

type PromptWhiteLabelHostingItem struct {
	Name         domain.Fact[string] `json:"name"`
	MonthlyRate  domain.MoneyInput   `json:"monthly_rate"`
	Quantity     domain.Fact[int]    `json:"quantity"`
	PeriodMonths domain.Fact[int]    `json:"period_months"`
}

type PromptWhiteLabelModuleGroup struct {
	Name     domain.Fact[string]   `json:"name"`
	Features []domain.Fact[string] `json:"features"`
}

type PromptWhiteLabelTimelineRow struct {
	Description domain.Fact[string] `json:"description"`
	Timeline    domain.Fact[string] `json:"timeline"`
}

type PromptWhiteLabelPaymentRow struct {
	WorkPeriod       domain.Fact[string]               `json:"work_period"`
	Terms            domain.Fact[string]               `json:"terms"`
	InvoiceDateLabel domain.Fact[string]               `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string]               `json:"due_date_label"`
	Component        domain.WhiteLabelPaymentComponent `json:"component"`
	Nominal          domain.MoneyInput                 `json:"nominal"`
}

type PromptWhiteLabelServiceInput struct {
	Kind             string                            `json:"kind"`
	ProductName      domain.Fact[string]               `json:"product_name"`
	TaxTreatment     domain.Fact[string]               `json:"tax_treatment"`
	LicenseItems     []PromptWhiteLabelLicenseItem     `json:"license_items"`
	MaintenanceItems []PromptWhiteLabelMaintenanceItem `json:"maintenance_items,omitempty"`
	HostingItems     []PromptWhiteLabelHostingItem     `json:"hosting_items,omitempty"`
	ModuleGroups     []PromptWhiteLabelModuleGroup     `json:"module_groups,omitempty"`
	TimelineRows     []PromptWhiteLabelTimelineRow     `json:"timeline_rows,omitempty"`
	PaymentSchedule  []PromptWhiteLabelPaymentRow      `json:"payment_schedule"`
	RightsStatements []domain.Fact[string]             `json:"rights_statements,omitempty"`
	LimitationCodes  []domain.Fact[string]             `json:"limitation_codes"`
}

// PromptWhiteLabelSnapshot is the immutable calculator and renderer input a
// prompt-first caller persists alongside its normal quotation snapshots.
type PromptWhiteLabelSnapshot struct {
	Input       domain.WhiteLabelInput           `json:"input"`
	Calculation domain.WhiteLabelCalculation     `json:"calculation"`
	Profile     domain.ResolvedWhiteLabelProfile `json:"profile"`
}

type PromptWhiteLabelAdapter struct {
	renderer WhiteLabelHTMLRenderer
}

func NewPromptWhiteLabelAdapter(renderer WhiteLabelHTMLRenderer) *PromptWhiteLabelAdapter {
	if renderer == nil {
		renderer = NewWhiteLabelHTMLRenderer()
	}
	return &PromptWhiteLabelAdapter{renderer: renderer}
}

func (a *PromptWhiteLabelAdapter) Scheme() string { return domain.CommercialSchemeWhiteLabel }

func DecodePromptWhiteLabelServiceInput(raw json.RawMessage) (PromptWhiteLabelServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptWhiteLabelServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptWhiteLabelServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptWhiteLabelServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PromptWhiteLabelServiceInput{}, fmt.Errorf("service_input: must contain one JSON object")
	}
	if input.Kind != PromptWhiteLabelServiceInputKind {
		return PromptWhiteLabelServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptWhiteLabelServiceInputKind)
	}
	normalizeUnknownWhiteLabelMoney(&input)
	return input, nil
}

func EncodePromptWhiteLabelServiceInput(input PromptWhiteLabelServiceInput) ([]byte, error) {
	if input.Kind != PromptWhiteLabelServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptWhiteLabelServiceInputKind)
	}
	return json.Marshal(input)
}

// BuildPromptWhiteLabelInput maps trusted price inputs and descriptive facts
// only. It does not calculate any amount or produce payment amounts: the
// domain calculator owns line totals, component subtotals, and reconciliation.
func BuildPromptWhiteLabelInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.WhiteLabelInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 12)
	if state == nil {
		return domain.WhiteLabelInput{}, append(issues, whiteLabelIssue("white_label.state.missing", "state", "prompt state is required"))
	}
	if err := state.Validate(); err != nil {
		return domain.WhiteLabelInput{}, append(issues, whiteLabelIssue("white_label.state.invalid", "state", err.Error()))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeWhiteLabel {
		return domain.WhiteLabelInput{}, append(issues, whiteLabelIssue("white_label.scheme.invalid", "commercial_scheme", "white-label adapter requires the white_label scheme"))
	}
	serviceInput, err := DecodePromptWhiteLabelServiceInput(state.ServiceInput)
	if err != nil {
		return domain.WhiteLabelInput{}, append(issues, whiteLabelIssue("white_label.service_input.invalid", "service_input", err.Error()))
	}
	if quotationDate.IsZero() {
		issues = append(issues, whiteLabelIssue("white_label.quotation_date.missing", "quotation_date", "quotation date is required"))
	}

	input := domain.WhiteLabelInput{
		ProductName:   requiredWhiteLabelText(&issues, "product_name", serviceInput.ProductName),
		QuotationDate: quotationDate,
		TaxTreatment:  domain.WhiteLabelTaxTreatment(requiredWhiteLabelText(&issues, "tax_treatment", serviceInput.TaxTreatment)),
	}
	for index, item := range serviceInput.LicenseItems {
		field := fmt.Sprintf("license_items[%d]", index)
		input.LicenseItems = append(input.LicenseItems, domain.WhiteLabelLicenseItemInput{
			Name:     requiredWhiteLabelText(&issues, field+".name", item.Name),
			UnitRate: requiredWhiteLabelMoney(&issues, "white_label.license_unit_rate", field+".unit_rate", item.UnitRate),
			Quantity: int32(requiredWhiteLabelInt(&issues, field+".quantity", item.Quantity)),
		})
	}
	for index, item := range serviceInput.MaintenanceItems {
		field := fmt.Sprintf("maintenance_items[%d]", index)
		input.MaintenanceItems = append(input.MaintenanceItems, domain.WhiteLabelMaintenanceItemInput{
			Name:         requiredWhiteLabelText(&issues, field+".name", item.Name),
			MonthlyRate:  requiredWhiteLabelMoney(&issues, "white_label.maintenance_monthly_rate", field+".monthly_rate", item.MonthlyRate),
			FTEQuarters:  int32(requiredWhiteLabelInt(&issues, field+".fte_quarters", item.FTEQuarters)),
			PeriodMonths: int32(requiredWhiteLabelInt(&issues, field+".period_months", item.PeriodMonths)),
		})
	}
	for index, item := range serviceInput.HostingItems {
		field := fmt.Sprintf("hosting_items[%d]", index)
		input.HostingItems = append(input.HostingItems, domain.WhiteLabelHostingItemInput{
			Name:         requiredWhiteLabelText(&issues, field+".name", item.Name),
			MonthlyRate:  requiredWhiteLabelMoney(&issues, "white_label.hosting_monthly_rate", field+".monthly_rate", item.MonthlyRate),
			Quantity:     int32(requiredWhiteLabelInt(&issues, field+".quantity", item.Quantity)),
			PeriodMonths: int32(requiredWhiteLabelInt(&issues, field+".period_months", item.PeriodMonths)),
		})
	}
	for index, group := range serviceInput.ModuleGroups {
		mapped := domain.WhiteLabelModuleGroupInput{Name: requiredWhiteLabelText(&issues, fmt.Sprintf("module_groups[%d].name", index), group.Name)}
		for featureIndex, feature := range group.Features {
			mapped.Features = append(mapped.Features, requiredWhiteLabelText(&issues, fmt.Sprintf("module_groups[%d].features[%d]", index, featureIndex), feature))
		}
		input.ModuleGroups = append(input.ModuleGroups, mapped)
	}
	for index, row := range serviceInput.TimelineRows {
		input.TimelineRows = append(input.TimelineRows, domain.WhiteLabelTimelineRowInput{
			Description: requiredWhiteLabelText(&issues, fmt.Sprintf("timeline_rows[%d].description", index), row.Description),
			Timeline:    requiredWhiteLabelText(&issues, fmt.Sprintf("timeline_rows[%d].timeline", index), row.Timeline),
		})
	}
	for index, row := range serviceInput.PaymentSchedule {
		field := fmt.Sprintf("payment_schedule[%d]", index)
		input.PaymentSchedule = append(input.PaymentSchedule, domain.WhiteLabelPaymentRowInput{
			WorkPeriod:       requiredWhiteLabelText(&issues, field+".work_period", row.WorkPeriod),
			Terms:            requiredWhiteLabelText(&issues, field+".terms", row.Terms),
			InvoiceDateLabel: requiredWhiteLabelText(&issues, field+".invoice_date_label", row.InvoiceDateLabel),
			DueDateLabel:     requiredWhiteLabelText(&issues, field+".due_date_label", row.DueDateLabel),
			Component:        row.Component,
			Nominal:          requiredWhiteLabelMoney(&issues, "white_label.payment_nominal", field+".nominal", row.Nominal),
		})
	}
	for index, statement := range serviceInput.RightsStatements {
		input.RightsStatements = append(input.RightsStatements, requiredWhiteLabelText(&issues, fmt.Sprintf("rights_statements[%d]", index), statement))
	}
	for index, code := range serviceInput.LimitationCodes {
		input.LimitationCodes = append(input.LimitationCodes, requiredWhiteLabelText(&issues, fmt.Sprintf("limitation_codes[%d]", index), code))
	}
	return input, issues
}

func CalculatePromptWhiteLabel(state *domain.PromptQuotationState, quotationDate time.Time) (domain.WhiteLabelInput, domain.WhiteLabelCalculation, domain.ResolvedWhiteLabelProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptWhiteLabelInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.WhiteLabelCalculation{}, domain.ResolvedWhiteLabelProfile{}, issues, nil
	}
	if err := input.Validate(); err != nil {
		issues = append(issues, whiteLabelIssue("white_label.input.invalid", "service_input", err.Error()))
		return input, domain.WhiteLabelCalculation{}, domain.ResolvedWhiteLabelProfile{}, issues, nil
	}
	calculation, err := CalculateWhiteLabel(input)
	if err != nil {
		code, field := "white_label.calculation.invalid", "calculation"
		switch {
		case strings.Contains(err.Error(), "payment_schedule.license"):
			code, field = "white_label.payment.license_mismatch", "payment_schedule"
		case strings.Contains(err.Error(), "payment_schedule.maintenance_hosting"):
			code, field = "white_label.payment.maintenance_hosting_mismatch", "payment_schedule"
		}
		issues = append(issues, whiteLabelIssue(code, field, err.Error()))
		return input, domain.WhiteLabelCalculation{}, domain.ResolvedWhiteLabelProfile{}, issues, nil
	}
	profile, err := ResolveWhiteLabelProfile(input)
	if err != nil {
		return input, domain.WhiteLabelCalculation{}, domain.ResolvedWhiteLabelProfile{}, issues, err
	}
	return input, calculation, profile, issues, nil
}

func (a *PromptWhiteLabelAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.WhiteLabelInput, domain.WhiteLabelCalculation, domain.ResolvedWhiteLabelProfile, []domain.DraftIssue, error) {
	return CalculatePromptWhiteLabel(state, quotationDate)
}

func (a *PromptWhiteLabelAdapter) Render(metadata QuotationMetadata, snapshot PromptWhiteLabelSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("white-label renderer is unavailable")
	}
	return a.renderer.Render(WhiteLabelDocumentInput{Metadata: metadata, Input: snapshot.Input, Calculation: snapshot.Calculation, Profile: snapshot.Profile, LegalHTML: legalHTML})
}

func whiteLabelIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func requiredWhiteLabelText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateWhiteLabelFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, whiteLabelIssue("white_label."+field+".missing", field, "value is required"))
	}
	return value
}

func requiredWhiteLabelInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateWhiteLabelFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, whiteLabelIssue("white_label."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func validateWhiteLabelFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, whiteLabelIssue("white_label."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, whiteLabelIssue("white_label."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredWhiteLabelMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, whiteLabelIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, whiteLabelIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, whiteLabelIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, whiteLabelIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

// Extraction omits amounts. A missing amount stays explicit unknown so an
// adapter emits a blocking issue instead of accepting zero or a meeting price.
func normalizeUnknownWhiteLabelMoney(input *PromptWhiteLabelServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for index := range input.LicenseItems {
		normalize(&input.LicenseItems[index].UnitRate)
	}
	for index := range input.MaintenanceItems {
		normalize(&input.MaintenanceItems[index].MonthlyRate)
	}
	for index := range input.HostingItems {
		normalize(&input.HostingItems[index].MonthlyRate)
	}
	for index := range input.PaymentSchedule {
		normalize(&input.PaymentSchedule[index].Nominal)
	}
}
