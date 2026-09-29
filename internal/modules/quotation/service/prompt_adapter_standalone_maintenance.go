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

// PromptStandaloneMaintenanceServiceInputKind is the versioned prompt-state payload
// for standalone maintenance quotations. Descriptive meeting facts remain separate
// from prices, which are accepted only after a catalog or BA supplies them.
const PromptStandaloneMaintenanceServiceInputKind = "standalone_maintenance_v1"

type PromptMaintenanceRetainerItem struct {
	Name         domain.Fact[string] `json:"name"`
	MonthlyRate  domain.MoneyInput   `json:"monthly_rate"`
	FTEQuarters  domain.Fact[int]    `json:"fte_quarters"`
	PeriodMonths domain.Fact[int]    `json:"period_months"`
}

type PromptMaintenanceHourlyCostItem struct {
	Name               domain.Fact[string] `json:"name"`
	DisplayMonthlyRate domain.MoneyInput   `json:"display_monthly_rate"`
	Headcount          domain.Fact[int]    `json:"headcount"`
	HoursHalfUnits     domain.Fact[int]    `json:"hours_half_units"`
	QuotedLineTotal    domain.MoneyInput   `json:"quoted_line_total"`
}

type PromptMaintenanceDefectItem struct {
	ItemNumber  domain.Fact[string]  `json:"item_number"`
	Priority    domain.Fact[string]  `json:"priority"`
	Function    domain.Fact[string]  `json:"function"`
	ScreenFlow  domain.Fact[string]  `json:"screen_flow"`
	Existing    domain.Fact[string]  `json:"existing"`
	Improvement domain.Fact[string]  `json:"improvement"`
	Reference   *domain.Fact[string] `json:"reference,omitempty"`
}

type PromptMaintenanceDefectGroup struct {
	Title domain.Fact[string]           `json:"title"`
	Items []PromptMaintenanceDefectItem `json:"items"`
}

type PromptMaintenanceHourlyTask struct {
	Description    domain.Fact[string] `json:"description"`
	Platform       domain.Fact[string] `json:"platform"`
	HoursHalfUnits domain.Fact[int]    `json:"hours_half_units"`
}

type PromptMaintenancePaymentRow struct {
	WorkPeriod       domain.Fact[string] `json:"work_period"`
	Terms            domain.Fact[string] `json:"terms"`
	InvoiceDateLabel domain.Fact[string] `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string] `json:"due_date_label"`
	Nominal          domain.MoneyInput   `json:"nominal"`
}

type PromptStandaloneMaintenanceServiceInput struct {
	Kind             string                            `json:"kind"`
	SystemName       domain.Fact[string]               `json:"system_name"`
	Mode             domain.Fact[string]               `json:"mode"`
	TaxTreatment     domain.Fact[string]               `json:"tax_treatment"`
	RetainerItems    []PromptMaintenanceRetainerItem   `json:"retainer_items,omitempty"`
	HourlyCostItems  []PromptMaintenanceHourlyCostItem `json:"hourly_cost_items,omitempty"`
	DefectGroups     []PromptMaintenanceDefectGroup    `json:"defect_groups,omitempty"`
	HourlyTasks      []PromptMaintenanceHourlyTask     `json:"hourly_tasks,omitempty"`
	PaymentSchedule  []PromptMaintenancePaymentRow     `json:"payment_schedule"`
	RightsStatements []domain.Fact[string]             `json:"rights_statements,omitempty"`
}

// PromptStandaloneMaintenanceSnapshot is the immutable calculator and renderer input
// a prompt-first caller persists alongside its normal quotation snapshots.
type PromptStandaloneMaintenanceSnapshot struct {
	Input       domain.StandaloneMaintenanceInput           `json:"input"`
	Calculation domain.StandaloneMaintenanceCalculation     `json:"calculation"`
	Profile     domain.ResolvedStandaloneMaintenanceProfile `json:"profile"`
}

type PromptStandaloneMaintenanceAdapter struct {
	renderer StandaloneMaintenanceHTMLRenderer
}

func NewPromptStandaloneMaintenanceAdapter(renderer StandaloneMaintenanceHTMLRenderer) *PromptStandaloneMaintenanceAdapter {
	if renderer == nil {
		renderer = NewStandaloneMaintenanceHTMLRenderer()
	}
	return &PromptStandaloneMaintenanceAdapter{renderer: renderer}
}

func (a *PromptStandaloneMaintenanceAdapter) Scheme() string {
	return domain.CommercialSchemeStandaloneMaintenance
}

func DecodePromptStandaloneMaintenanceServiceInput(raw json.RawMessage) (PromptStandaloneMaintenanceServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptStandaloneMaintenanceServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptStandaloneMaintenanceServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptStandaloneMaintenanceServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PromptStandaloneMaintenanceServiceInput{}, fmt.Errorf("service_input: must contain one JSON object")
	}
	if input.Kind != PromptStandaloneMaintenanceServiceInputKind {
		return PromptStandaloneMaintenanceServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptStandaloneMaintenanceServiceInputKind)
	}
	normalizeUnknownStandaloneMaintenanceMoney(&input)
	return input, nil
}

func EncodePromptStandaloneMaintenanceServiceInput(input PromptStandaloneMaintenanceServiceInput) ([]byte, error) {
	if input.Kind != PromptStandaloneMaintenanceServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptStandaloneMaintenanceServiceInputKind)
	}
	return json.Marshal(input)
}

// BuildPromptStandaloneMaintenanceInput maps trusted price inputs and descriptive facts only.
// It does not calculate amounts or derive hourly prices from display rates.
func BuildPromptStandaloneMaintenanceInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.StandaloneMaintenanceInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 12)
	if state == nil {
		return domain.StandaloneMaintenanceInput{}, append(issues, standaloneMaintenanceIssue("standalone_maintenance.state.missing", "state", "prompt state is required"))
	}
	if err := state.Validate(); err != nil {
		return domain.StandaloneMaintenanceInput{}, append(issues, standaloneMaintenanceIssue("standalone_maintenance.state.invalid", "state", err.Error()))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeStandaloneMaintenance {
		return domain.StandaloneMaintenanceInput{}, append(issues, standaloneMaintenanceIssue("standalone_maintenance.scheme.invalid", "commercial_scheme", "standalone maintenance adapter requires the standalone_maintenance scheme"))
	}
	serviceInput, err := DecodePromptStandaloneMaintenanceServiceInput(state.ServiceInput)
	if err != nil {
		return domain.StandaloneMaintenanceInput{}, append(issues, standaloneMaintenanceIssue("standalone_maintenance.service_input.invalid", "service_input", err.Error()))
	}
	if quotationDate.IsZero() {
		issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.quotation_date.missing", "quotation_date", "quotation date is required"))
	}

	modeStr := requiredStandaloneMaintenanceText(&issues, "mode", serviceInput.Mode)
	mode := domain.StandaloneMaintenanceMode(modeStr)
	taxTreatmentStr := requiredStandaloneMaintenanceText(&issues, "tax_treatment", serviceInput.TaxTreatment)
	taxTreatment := domain.StandaloneMaintenanceTaxTreatment(taxTreatmentStr)

	input := domain.StandaloneMaintenanceInput{
		SystemName:    requiredStandaloneMaintenanceText(&issues, "system_name", serviceInput.SystemName),
		QuotationDate: quotationDate,
		Mode:          mode,
		TaxTreatment:  taxTreatment,
	}

	switch mode {
	case domain.StandaloneMaintenanceRetainer:
		if len(serviceInput.RetainerItems) == 0 {
			issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.retainer_items.missing", "retainer_items", "retainer mode requires at least one retainer item"))
		}
		for index, item := range serviceInput.RetainerItems {
			field := fmt.Sprintf("retainer_items[%d]", index)
			input.RetainerItems = append(input.RetainerItems, domain.MaintenanceRetainerItemInput{
				Name:         requiredStandaloneMaintenanceText(&issues, field+".name", item.Name),
				MonthlyRate:  requiredStandaloneMaintenanceMoney(&issues, "standalone_maintenance.retainer_monthly_rate", field+".monthly_rate", item.MonthlyRate),
				FTEQuarters:  int32(requiredStandaloneMaintenanceInt(&issues, field+".fte_quarters", item.FTEQuarters)),
				PeriodMonths: int32(requiredStandaloneMaintenanceInt(&issues, field+".period_months", item.PeriodMonths)),
			})
		}
		if len(serviceInput.DefectGroups) == 0 {
			issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.defect_groups.missing", "defect_groups", "retainer mode requires at least one defect group"))
		}
		for gIdx, group := range serviceInput.DefectGroups {
			gField := fmt.Sprintf("defect_groups[%d]", gIdx)
			mappedGroup := domain.MaintenanceDefectGroupInput{
				Title: requiredStandaloneMaintenanceText(&issues, gField+".title", group.Title),
			}
			for iIdx, item := range group.Items {
				iField := fmt.Sprintf("%s.items[%d]", gField, iIdx)
				priorityStr := requiredStandaloneMaintenanceText(&issues, iField+".priority", item.Priority)
				priority := domain.StandaloneMaintenancePriority(priorityStr)
				mappedGroup.Items = append(mappedGroup.Items, domain.MaintenanceDefectItemInput{
					ItemNumber:  requiredStandaloneMaintenanceText(&issues, iField+".item_number", item.ItemNumber),
					Priority:    priority,
					Function:    requiredStandaloneMaintenanceText(&issues, iField+".function", item.Function),
					ScreenFlow:  requiredStandaloneMaintenanceText(&issues, iField+".screen_flow", item.ScreenFlow),
					Existing:    requiredStandaloneMaintenanceText(&issues, iField+".existing", item.Existing),
					Improvement: requiredStandaloneMaintenanceText(&issues, iField+".improvement", item.Improvement),
					Reference:   optionalStandaloneMaintenanceText(&issues, iField+".reference", item.Reference),
				})
			}
			input.DefectGroups = append(input.DefectGroups, mappedGroup)
		}

	case domain.StandaloneMaintenanceHourlyTask:
		if len(serviceInput.HourlyCostItems) == 0 {
			issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.hourly_cost_items.missing", "hourly_cost_items", "hourly task mode requires at least one cost item"))
		}
		for index, item := range serviceInput.HourlyCostItems {
			field := fmt.Sprintf("hourly_cost_items[%d]", index)
			input.HourlyCostItems = append(input.HourlyCostItems, domain.MaintenanceHourlyCostItemInput{
				Name:               requiredStandaloneMaintenanceText(&issues, field+".name", item.Name),
				DisplayMonthlyRate: requiredStandaloneMaintenanceMoney(&issues, "standalone_maintenance.hourly_display_rate", field+".display_monthly_rate", item.DisplayMonthlyRate),
				Headcount:          int32(requiredStandaloneMaintenanceInt(&issues, field+".headcount", item.Headcount)),
				HoursHalfUnits:     int32(requiredStandaloneMaintenanceInt(&issues, field+".hours_half_units", item.HoursHalfUnits)),
				QuotedLineTotal:    requiredStandaloneMaintenanceMoney(&issues, "standalone_maintenance.hourly_quoted_total", field+".quoted_line_total", item.QuotedLineTotal),
			})
		}
		if len(serviceInput.HourlyTasks) == 0 {
			issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.hourly_tasks.missing", "hourly_tasks", "hourly task mode requires at least one task"))
		}
		for index, task := range serviceInput.HourlyTasks {
			field := fmt.Sprintf("hourly_tasks[%d]", index)
			input.HourlyTasks = append(input.HourlyTasks, domain.MaintenanceHourlyTaskInput{
				Description:    requiredStandaloneMaintenanceText(&issues, field+".description", task.Description),
				Platform:       requiredStandaloneMaintenanceText(&issues, field+".platform", task.Platform),
				HoursHalfUnits: int32(requiredStandaloneMaintenanceInt(&issues, field+".hours_half_units", task.HoursHalfUnits)),
			})
		}

	default:
		issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.mode.invalid", "mode", fmt.Sprintf("unsupported mode %q", modeStr)))
	}

	if len(serviceInput.PaymentSchedule) == 0 {
		issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.payment_schedule.missing", "payment_schedule", "payment schedule is required"))
	}
	for index, row := range serviceInput.PaymentSchedule {
		field := fmt.Sprintf("payment_schedule[%d]", index)
		input.PaymentSchedule = append(input.PaymentSchedule, domain.MaintenancePaymentRowInput{
			WorkPeriod:       requiredStandaloneMaintenanceText(&issues, field+".work_period", row.WorkPeriod),
			Terms:            requiredStandaloneMaintenanceText(&issues, field+".terms", row.Terms),
			InvoiceDateLabel: requiredStandaloneMaintenanceText(&issues, field+".invoice_date_label", row.InvoiceDateLabel),
			DueDateLabel:     requiredStandaloneMaintenanceText(&issues, field+".due_date_label", row.DueDateLabel),
			Nominal:          requiredStandaloneMaintenanceMoney(&issues, "standalone_maintenance.payment_nominal", field+".nominal", row.Nominal),
		})
	}

	for index, stmt := range serviceInput.RightsStatements {
		input.RightsStatements = append(input.RightsStatements, requiredStandaloneMaintenanceText(&issues, fmt.Sprintf("rights_statements[%d]", index), stmt))
	}

	return input, issues
}

func CalculatePromptStandaloneMaintenance(state *domain.PromptQuotationState, quotationDate time.Time) (domain.StandaloneMaintenanceInput, domain.StandaloneMaintenanceCalculation, domain.ResolvedStandaloneMaintenanceProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptStandaloneMaintenanceInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.StandaloneMaintenanceCalculation{}, domain.ResolvedStandaloneMaintenanceProfile{}, issues, nil
	}

	// Apply discount from state.AdditionalDiscountBPS after initial price calculation
	if state.AdditionalDiscountBPS != nil && *state.AdditionalDiscountBPS > 0 {
		var subtotal int64
		switch input.Mode {
		case domain.StandaloneMaintenanceRetainer:
			for _, item := range input.RetainerItems {
				numerator := item.MonthlyRate * int64(item.FTEQuarters) * int64(item.PeriodMonths)
				subtotal += numerator / 4
			}
		case domain.StandaloneMaintenanceHourlyTask:
			for _, item := range input.HourlyCostItems {
				subtotal += item.QuotedLineTotal
			}
		}
		if subtotal > 0 {
			discount := (subtotal * int64(*state.AdditionalDiscountBPS)) / 10000
			input.DiscountAmount = discount
			input.ShowDiscountRow = true
		}
	}

	if err := input.Validate(); err != nil {
		issues = append(issues, standaloneMaintenanceIssue("standalone_maintenance.input.invalid", "service_input", err.Error()))
		return input, domain.StandaloneMaintenanceCalculation{}, domain.ResolvedStandaloneMaintenanceProfile{}, issues, nil
	}
	calculation, err := CalculateStandaloneMaintenance(input)
	if err != nil {
		code, field := "standalone_maintenance.calculation.invalid", "calculation"
		if strings.Contains(err.Error(), "payment_schedule: total") && strings.Contains(err.Error(), "must equal total investment") {
			code, field = "standalone_maintenance.payment.total_mismatch", "payment_schedule"
		}
		issues = append(issues, standaloneMaintenanceIssue(code, field, err.Error()))
		return input, domain.StandaloneMaintenanceCalculation{}, domain.ResolvedStandaloneMaintenanceProfile{}, issues, nil
	}
	profile := ResolveStandaloneMaintenanceProfile(input)
	return input, calculation, profile, issues, nil
}

func (a *PromptStandaloneMaintenanceAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.StandaloneMaintenanceInput, domain.StandaloneMaintenanceCalculation, domain.ResolvedStandaloneMaintenanceProfile, []domain.DraftIssue, error) {
	return CalculatePromptStandaloneMaintenance(state, quotationDate)
}

func (a *PromptStandaloneMaintenanceAdapter) Render(metadata QuotationMetadata, snapshot PromptStandaloneMaintenanceSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("standalone maintenance renderer is unavailable")
	}
	return a.renderer.Render(StandaloneMaintenanceDocumentInput{
		Metadata:    metadata,
		Input:       snapshot.Input,
		Calculation: snapshot.Calculation,
		Profile:     snapshot.Profile,
		LegalHTML:   legalHTML,
	})
}

func standaloneMaintenanceIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func requiredStandaloneMaintenanceText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateStandaloneMaintenanceFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, standaloneMaintenanceIssue("standalone_maintenance."+field+".missing", field, "value is required"))
	}
	return value
}

func optionalStandaloneMaintenanceText(issues *[]domain.DraftIssue, field string, fact *domain.Fact[string]) string {
	if fact == nil {
		return ""
	}
	validateStandaloneMaintenanceFact(issues, field, fact.Source, fact.Evidence)
	return strings.TrimSpace(fact.Value)
}

func requiredStandaloneMaintenanceInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateStandaloneMaintenanceFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, standaloneMaintenanceIssue("standalone_maintenance."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func validateStandaloneMaintenanceFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, standaloneMaintenanceIssue("standalone_maintenance."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, standaloneMaintenanceIssue("standalone_maintenance."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredStandaloneMaintenanceMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, standaloneMaintenanceIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, standaloneMaintenanceIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, standaloneMaintenanceIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, standaloneMaintenanceIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

// Extraction omits amounts. Keep an omitted amount explicitly unknown so the adapter
// emits a blocking issue instead of accepting zero or treating the model as the source of a price.
func normalizeUnknownStandaloneMaintenanceMoney(input *PromptStandaloneMaintenanceServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for index := range input.RetainerItems {
		normalize(&input.RetainerItems[index].MonthlyRate)
	}
	for index := range input.HourlyCostItems {
		normalize(&input.HourlyCostItems[index].DisplayMonthlyRate)
		normalize(&input.HourlyCostItems[index].QuotedLineTotal)
	}
	for index := range input.PaymentSchedule {
		normalize(&input.PaymentSchedule[index].Nominal)
	}
}
