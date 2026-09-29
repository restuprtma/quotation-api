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

// PromptCustomDevelopmentServiceInputKind is the versioned prompt-state payload
// for custom development quotations. Descriptive meeting facts remain separate
// from prices, which are accepted only after a catalog or BA supplies them.
const PromptCustomDevelopmentServiceInputKind = "custom_development_v1"

type CustomDevelopmentPricingStatus string

const (
	CustomDevelopmentPricingUnknown  CustomDevelopmentPricingStatus = "unknown"
	CustomDevelopmentPricingExplicit CustomDevelopmentPricingStatus = "explicit"
	CustomDevelopmentPricingIncluded CustomDevelopmentPricingStatus = "included"
)

type PromptCustomDevelopmentCostItem struct {
	Name            domain.Fact[string]            `json:"name"`
	DisplayRate     domain.MoneyInput              `json:"display_rate,omitempty"`
	Headcount       domain.Fact[int]               `json:"headcount"`
	PeriodLabel     domain.Fact[string]            `json:"period_label"`
	PricingStatus   CustomDevelopmentPricingStatus `json:"pricing_status,omitempty"`
	QuotedLineTotal domain.MoneyInput              `json:"quoted_line_total,omitempty"`
}

type PromptCustomDevelopmentCostGroup struct {
	Title domain.Fact[string]               `json:"title"`
	Items []PromptCustomDevelopmentCostItem `json:"items"`
}

type PromptCustomDevelopmentMonthlyItem struct {
	Name          domain.Fact[string]  `json:"name"`
	Specification *domain.Fact[string] `json:"specification,omitempty"`
	UnitRate      domain.MoneyInput    `json:"unit_rate"`
	Quantity      domain.Fact[int]     `json:"quantity"`
}

type PromptCustomDevelopmentModuleGroup struct {
	Name     domain.Fact[string]   `json:"name"`
	Features []domain.Fact[string] `json:"features"`
}

type PromptCustomDevelopmentTimelineRow struct {
	Description domain.Fact[string] `json:"description"`
	Timeline    domain.Fact[string] `json:"timeline"`
}

type PromptCustomDevelopmentPaymentRow struct {
	WorkPeriod       domain.Fact[string]                      `json:"work_period"`
	Terms            domain.Fact[string]                      `json:"terms"`
	InvoiceDateLabel domain.Fact[string]                      `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string]                      `json:"due_date_label"`
	Component        domain.CustomDevelopmentPaymentComponent `json:"component"`
	Nominal          domain.MoneyInput                        `json:"nominal"`
}

type PromptCustomDevelopmentServiceInput struct {
	Kind                  string                               `json:"kind"`
	SystemName            domain.Fact[string]                  `json:"system_name"`
	DevelopmentTableMode  domain.Fact[string]                  `json:"development_table_mode"`
	TaxTreatment          domain.Fact[string]                  `json:"tax_treatment"`
	DevelopmentGroups     []PromptCustomDevelopmentCostGroup   `json:"development_groups"`
	DiscountLabel         *domain.Fact[string]                 `json:"discount_label,omitempty"`
	MonthlyItems          []PromptCustomDevelopmentMonthlyItem `json:"monthly_items,omitempty"`
	MonthlyNotes          []domain.Fact[string]                `json:"monthly_notes,omitempty"`
	ModuleGroups          []PromptCustomDevelopmentModuleGroup `json:"module_groups"`
	TimelineRows          []PromptCustomDevelopmentTimelineRow `json:"timeline_rows"`
	PaymentSchedule       []PromptCustomDevelopmentPaymentRow  `json:"payment_schedule"`
	RightsStatements      []domain.Fact[string]                `json:"rights_statements,omitempty"`
	LimitationCodes       []domain.Fact[string]                `json:"limitation_codes"`
	AdditionalLimitations []domain.Fact[string]                `json:"additional_limitations,omitempty"`
}

// PromptCustomDevelopmentSnapshot is the immutable calculator and renderer input
// a prompt-first caller persists alongside its normal quotation snapshots.
type PromptCustomDevelopmentSnapshot struct {
	Input       domain.CustomDevelopmentInput           `json:"input"`
	Calculation domain.CustomDevelopmentCalculation     `json:"calculation"`
	Profile     domain.ResolvedCustomDevelopmentProfile `json:"profile"`
}

type PromptCustomDevelopmentAdapter struct {
	renderer CustomDevelopmentHTMLRenderer
}

func NewPromptCustomDevelopmentAdapter(renderer CustomDevelopmentHTMLRenderer) *PromptCustomDevelopmentAdapter {
	if renderer == nil {
		renderer = NewCustomDevelopmentHTMLRenderer()
	}
	return &PromptCustomDevelopmentAdapter{renderer: renderer}
}

func (a *PromptCustomDevelopmentAdapter) Scheme() string {
	return domain.CommercialSchemeCustomDevelopment
}

func DecodePromptCustomDevelopmentServiceInput(raw json.RawMessage) (PromptCustomDevelopmentServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptCustomDevelopmentServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptCustomDevelopmentServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptCustomDevelopmentServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PromptCustomDevelopmentServiceInput{}, fmt.Errorf("service_input: must contain one JSON object")
	}
	if input.Kind != PromptCustomDevelopmentServiceInputKind {
		return PromptCustomDevelopmentServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptCustomDevelopmentServiceInputKind)
	}
	normalizeUnknownCustomDevelopmentMoney(&input)
	return input, nil
}

func EncodePromptCustomDevelopmentServiceInput(input PromptCustomDevelopmentServiceInput) ([]byte, error) {
	if input.Kind != PromptCustomDevelopmentServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptCustomDevelopmentServiceInputKind)
	}
	return json.Marshal(input)
}

// BuildPromptCustomDevelopmentInput maps trusted price inputs and descriptive facts only.
// It maps included lines to intentional nil, and requires explicit quoted_line_total for explicit lines.
// It never derives development line total from period_label or display_rate.
func BuildPromptCustomDevelopmentInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.CustomDevelopmentInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 16)
	if state == nil {
		return domain.CustomDevelopmentInput{}, append(issues, customDevelopmentIssue("custom_development.state.missing", "state", "prompt state is required"))
	}
	if err := state.Validate(); err != nil {
		return domain.CustomDevelopmentInput{}, append(issues, customDevelopmentIssue("custom_development.state.invalid", "state", err.Error()))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeCustomDevelopment {
		return domain.CustomDevelopmentInput{}, append(issues, customDevelopmentIssue("custom_development.scheme.invalid", "commercial_scheme", "custom development adapter requires the custom_development scheme"))
	}
	serviceInput, err := DecodePromptCustomDevelopmentServiceInput(state.ServiceInput)
	if err != nil {
		return domain.CustomDevelopmentInput{}, append(issues, customDevelopmentIssue("custom_development.service_input.invalid", "service_input", err.Error()))
	}
	if quotationDate.IsZero() {
		issues = append(issues, customDevelopmentIssue("custom_development.quotation_date.missing", "quotation_date", "quotation date is required"))
	}

	modeStr := requiredCustomDevelopmentText(&issues, "development_table_mode", serviceInput.DevelopmentTableMode)
	mode := domain.CustomDevelopmentTableMode(modeStr)
	taxTreatmentStr := requiredCustomDevelopmentText(&issues, "tax_treatment", serviceInput.TaxTreatment)
	taxTreatment := domain.CustomDevelopmentTaxTreatment(taxTreatmentStr)

	input := domain.CustomDevelopmentInput{
		SystemName:           requiredCustomDevelopmentText(&issues, "system_name", serviceInput.SystemName),
		QuotationDate:        quotationDate,
		DevelopmentTableMode: mode,
		TaxTreatment:         taxTreatment,
		DiscountLabel:        optionalCustomDevelopmentText(&issues, "discount_label", serviceInput.DiscountLabel),
	}

	if len(serviceInput.DevelopmentGroups) == 0 {
		issues = append(issues, customDevelopmentIssue("custom_development.development_groups.missing", "development_groups", "development groups are required"))
	}

	for gIdx, group := range serviceInput.DevelopmentGroups {
		gField := fmt.Sprintf("development_groups[%d]", gIdx)
		groupTitle := requiredCustomDevelopmentText(&issues, gField+".title", group.Title)
		groupInput := domain.CustomDevelopmentCostGroupInput{
			Title: groupTitle,
		}

		if len(group.Items) == 0 {
			issues = append(issues, customDevelopmentIssue("custom_development.development_groups.items_missing", gField+".items", "group must contain items"))
		}

		for iIdx, item := range group.Items {
			iField := fmt.Sprintf("%s.items[%d]", gField, iIdx)
			itemName := requiredCustomDevelopmentText(&issues, iField+".name", item.Name)
			headcount := int32(requiredCustomDevelopmentInt(&issues, iField+".headcount", item.Headcount))
			periodLabel := requiredCustomDevelopmentText(&issues, iField+".period_label", item.PeriodLabel)

			itemInput := domain.CustomDevelopmentCostItemInput{
				Name:        itemName,
				Headcount:   headcount,
				PeriodLabel: periodLabel,
			}

			// Display rate is optional display data only
			if item.DisplayRate.Amount != nil {
				if item.DisplayRate.Source != domain.SourceCatalog && item.DisplayRate.Source != domain.SourceUser {
					issues = append(issues, customDevelopmentIssue("custom_development.display_rate.untrusted_source", iField+".display_rate", "display rate must come from a catalog or explicit user input"))
				} else if err := item.DisplayRate.Validate(); err != nil {
					issues = append(issues, customDevelopmentIssue("custom_development.display_rate.invalid", iField+".display_rate", err.Error()))
				} else if *item.DisplayRate.Amount <= 0 {
					issues = append(issues, customDevelopmentIssue("custom_development.display_rate.invalid", iField+".display_rate", "display rate must be greater than zero"))
				} else {
					displayRate := *item.DisplayRate.Amount
					itemInput.DisplayRate = &displayRate
				}
			}

			pricingStatus := item.PricingStatus
			if pricingStatus == "" {
				if item.QuotedLineTotal.Amount != nil {
					pricingStatus = CustomDevelopmentPricingExplicit
				} else {
					pricingStatus = CustomDevelopmentPricingUnknown
				}
			}

			switch pricingStatus {
			case CustomDevelopmentPricingIncluded:
				// Explicit included status: intentional nil quoted_line_total
				itemInput.QuotedLineTotal = nil
			case CustomDevelopmentPricingExplicit:
				if item.QuotedLineTotal.Amount == nil {
					issues = append(issues, customDevelopmentIssue("custom_development.quoted_line_total.unavailable", iField+".quoted_line_total", "price is not available"))
				} else if item.QuotedLineTotal.Source != domain.SourceCatalog && item.QuotedLineTotal.Source != domain.SourceUser {
					issues = append(issues, customDevelopmentIssue("custom_development.quoted_line_total.untrusted_source", iField+".quoted_line_total", "price must come from a catalog or explicit user input"))
				} else if err := item.QuotedLineTotal.Validate(); err != nil {
					issues = append(issues, customDevelopmentIssue("custom_development.quoted_line_total.invalid", iField+".quoted_line_total", err.Error()))
				} else if *item.QuotedLineTotal.Amount <= 0 {
					issues = append(issues, customDevelopmentIssue("custom_development.quoted_line_total.invalid", iField+".quoted_line_total", "price must be greater than zero"))
				} else {
					lineTotal := *item.QuotedLineTotal.Amount
					itemInput.QuotedLineTotal = &lineTotal
				}
			case CustomDevelopmentPricingUnknown:
				issues = append(issues, customDevelopmentIssue("custom_development.quoted_line_total.unavailable", iField+".quoted_line_total", "line quote is missing"))
			default:
				issues = append(issues, customDevelopmentIssue("custom_development.pricing_status.invalid", iField+".pricing_status", fmt.Sprintf("unsupported pricing status %q", pricingStatus)))
			}

			groupInput.Items = append(groupInput.Items, itemInput)
		}
		input.DevelopmentGroups = append(input.DevelopmentGroups, groupInput)
	}

	for mIdx, item := range serviceInput.MonthlyItems {
		mField := fmt.Sprintf("monthly_items[%d]", mIdx)
		mName := requiredCustomDevelopmentText(&issues, mField+".name", item.Name)
		mSpec := optionalCustomDevelopmentText(&issues, mField+".specification", item.Specification)
		mRate := requiredCustomDevelopmentMoney(&issues, "custom_development.monthly_unit_rate", mField+".unit_rate", item.UnitRate)
		mQty := int32(requiredCustomDevelopmentInt(&issues, mField+".quantity", item.Quantity))

		input.MonthlyItems = append(input.MonthlyItems, domain.CustomDevelopmentMonthlyItemInput{
			Name:          mName,
			Specification: mSpec,
			UnitRate:      mRate,
			Quantity:      mQty,
		})
	}

	for nIdx, note := range serviceInput.MonthlyNotes {
		input.MonthlyNotes = append(input.MonthlyNotes, requiredCustomDevelopmentText(&issues, fmt.Sprintf("monthly_notes[%d]", nIdx), note))
	}

	if len(serviceInput.ModuleGroups) == 0 {
		issues = append(issues, customDevelopmentIssue("custom_development.module_groups.missing", "module_groups", "module groups are required"))
	}
	for mgIdx, group := range serviceInput.ModuleGroups {
		mgField := fmt.Sprintf("module_groups[%d]", mgIdx)
		mgName := requiredCustomDevelopmentText(&issues, mgField+".name", group.Name)
		mgInput := domain.CustomDevelopmentModuleGroupInput{
			Name: mgName,
		}
		if len(group.Features) == 0 {
			issues = append(issues, customDevelopmentIssue("custom_development.module_groups.features_missing", mgField+".features", "features are required in module group"))
		}
		for fIdx, feat := range group.Features {
			featText := requiredCustomDevelopmentText(&issues, fmt.Sprintf("%s.features[%d]", mgField, fIdx), feat)
			mgInput.Features = append(mgInput.Features, featText)
		}
		input.ModuleGroups = append(input.ModuleGroups, mgInput)
	}

	if len(serviceInput.TimelineRows) == 0 {
		issues = append(issues, customDevelopmentIssue("custom_development.timeline_rows.missing", "timeline_rows", "timeline rows are required"))
	}
	for tIdx, row := range serviceInput.TimelineRows {
		tField := fmt.Sprintf("timeline_rows[%d]", tIdx)
		desc := requiredCustomDevelopmentText(&issues, tField+".description", row.Description)
		timeline := requiredCustomDevelopmentText(&issues, tField+".timeline", row.Timeline)
		input.TimelineRows = append(input.TimelineRows, domain.CustomDevelopmentTimelineRowInput{
			Description: desc,
			Timeline:    timeline,
		})
	}

	if len(serviceInput.PaymentSchedule) == 0 {
		issues = append(issues, customDevelopmentIssue("custom_development.payment_schedule.missing", "payment_schedule", "payment schedule is required"))
	}
	for pIdx, row := range serviceInput.PaymentSchedule {
		pField := fmt.Sprintf("payment_schedule[%d]", pIdx)
		workPeriod := requiredCustomDevelopmentText(&issues, pField+".work_period", row.WorkPeriod)
		terms := requiredCustomDevelopmentText(&issues, pField+".terms", row.Terms)
		invDate := requiredCustomDevelopmentText(&issues, pField+".invoice_date_label", row.InvoiceDateLabel)
		dueDate := requiredCustomDevelopmentText(&issues, pField+".due_date_label", row.DueDateLabel)
		nominal := requiredCustomDevelopmentMoney(&issues, "custom_development.payment_nominal", pField+".nominal", row.Nominal)

		input.PaymentSchedule = append(input.PaymentSchedule, domain.CustomDevelopmentPaymentRowInput{
			WorkPeriod:       workPeriod,
			Terms:            terms,
			InvoiceDateLabel: invDate,
			DueDateLabel:     dueDate,
			Component:        row.Component,
			Nominal:          nominal,
		})
	}

	for rIdx, stmt := range serviceInput.RightsStatements {
		input.RightsStatements = append(input.RightsStatements, requiredCustomDevelopmentText(&issues, fmt.Sprintf("rights_statements[%d]", rIdx), stmt))
	}

	if len(serviceInput.LimitationCodes) == 0 {
		issues = append(issues, customDevelopmentIssue("custom_development.limitation_codes.missing", "limitation_codes", "limitation codes are required"))
	}
	for lIdx, code := range serviceInput.LimitationCodes {
		input.LimitationCodes = append(input.LimitationCodes, requiredCustomDevelopmentText(&issues, fmt.Sprintf("limitation_codes[%d]", lIdx), code))
	}

	for aIdx, add := range serviceInput.AdditionalLimitations {
		input.AdditionalLimitations = append(input.AdditionalLimitations, requiredCustomDevelopmentText(&issues, fmt.Sprintf("additional_limitations[%d]", aIdx), add))
	}

	return input, issues
}

func CalculatePromptCustomDevelopment(state *domain.PromptQuotationState, quotationDate time.Time) (domain.CustomDevelopmentInput, domain.CustomDevelopmentCalculation, domain.ResolvedCustomDevelopmentProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptCustomDevelopmentInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.CustomDevelopmentCalculation{}, domain.ResolvedCustomDevelopmentProfile{}, issues, nil
	}

	// Apply discount from state.AdditionalDiscountBPS after initial price subtotal calculation
	if state.AdditionalDiscountBPS != nil && *state.AdditionalDiscountBPS > 0 {
		var subtotal int64
		for _, group := range input.DevelopmentGroups {
			for _, item := range group.Items {
				if item.QuotedLineTotal != nil {
					subtotal += *item.QuotedLineTotal
				}
			}
		}
		if subtotal > 0 {
			discount := (subtotal * int64(*state.AdditionalDiscountBPS)) / 10000
			input.DiscountAmount = discount
			input.ShowDiscountRow = true
			if input.DiscountLabel == "" {
				input.DiscountLabel = "DISCOUNT"
			}
		}
	}

	if err := input.Validate(); err != nil {
		issues = append(issues, customDevelopmentIssue("custom_development.input.invalid", "service_input", err.Error()))
		return input, domain.CustomDevelopmentCalculation{}, domain.ResolvedCustomDevelopmentProfile{}, issues, nil
	}

	calculation, err := CalculateCustomDevelopment(input)
	if err != nil {
		code, field := "custom_development.calculation.invalid", "calculation"
		if strings.Contains(err.Error(), "payment_schedule: development payment total") {
			code, field = "custom_development.payment.development_mismatch", "payment_schedule"
		} else if strings.Contains(err.Error(), "monthly payment nominal") {
			code, field = "custom_development.payment.monthly_nominal_mismatch", "payment_schedule"
		} else if strings.Contains(err.Error(), "missing monthly recurring payment row") {
			code, field = "custom_development.payment.missing_monthly", "payment_schedule"
		} else if strings.Contains(err.Error(), "unexpected monthly recurring payment row") {
			code, field = "custom_development.payment.unexpected_monthly", "payment_schedule"
		}
		issues = append(issues, customDevelopmentIssue(code, field, err.Error()))
		return input, domain.CustomDevelopmentCalculation{}, domain.ResolvedCustomDevelopmentProfile{}, issues, nil
	}

	profile, err := ResolveCustomDevelopmentProfile(input)
	if err != nil {
		issues = append(issues, customDevelopmentIssue("custom_development.profile.invalid", "profile", err.Error()))
		return input, domain.CustomDevelopmentCalculation{}, domain.ResolvedCustomDevelopmentProfile{}, issues, nil
	}

	return input, calculation, profile, issues, nil
}

func (a *PromptCustomDevelopmentAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.CustomDevelopmentInput, domain.CustomDevelopmentCalculation, domain.ResolvedCustomDevelopmentProfile, []domain.DraftIssue, error) {
	return CalculatePromptCustomDevelopment(state, quotationDate)
}

func (a *PromptCustomDevelopmentAdapter) Render(metadata QuotationMetadata, snapshot PromptCustomDevelopmentSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("custom development renderer is unavailable")
	}
	return a.renderer.Render(CustomDevelopmentDocumentInput{
		Metadata:    metadata,
		Input:       snapshot.Input,
		Calculation: snapshot.Calculation,
		Profile:     snapshot.Profile,
		LegalHTML:   legalHTML,
	})
}

func customDevelopmentIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func requiredCustomDevelopmentText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateCustomDevelopmentFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, customDevelopmentIssue("custom_development."+field+".missing", field, "value is required"))
	}
	return value
}

func optionalCustomDevelopmentText(issues *[]domain.DraftIssue, field string, fact *domain.Fact[string]) string {
	if fact == nil {
		return ""
	}
	validateCustomDevelopmentFact(issues, field, fact.Source, fact.Evidence)
	return strings.TrimSpace(fact.Value)
}

func requiredCustomDevelopmentInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateCustomDevelopmentFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, customDevelopmentIssue("custom_development."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func validateCustomDevelopmentFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, customDevelopmentIssue("custom_development."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, customDevelopmentIssue("custom_development."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredCustomDevelopmentMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, customDevelopmentIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, customDevelopmentIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, customDevelopmentIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, customDevelopmentIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

// Extraction omits amounts. Keep an omitted amount explicitly unknown so the adapter
// emits a blocking issue instead of accepting zero or treating the model as the source of a price.
func normalizeUnknownCustomDevelopmentMoney(input *PromptCustomDevelopmentServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for gIdx := range input.DevelopmentGroups {
		for iIdx := range input.DevelopmentGroups[gIdx].Items {
			item := &input.DevelopmentGroups[gIdx].Items[iIdx]
			normalize(&item.DisplayRate)
			normalize(&item.QuotedLineTotal)
			if item.PricingStatus == "" {
				if item.QuotedLineTotal.Amount != nil {
					item.PricingStatus = CustomDevelopmentPricingExplicit
				} else {
					item.PricingStatus = CustomDevelopmentPricingUnknown
				}
			}
		}
	}
	for index := range input.MonthlyItems {
		normalize(&input.MonthlyItems[index].UnitRate)
	}
	for index := range input.PaymentSchedule {
		normalize(&input.PaymentSchedule[index].Nominal)
	}
}
