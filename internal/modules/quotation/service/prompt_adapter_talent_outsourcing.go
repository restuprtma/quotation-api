package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const PromptTalentOutsourcingServiceInputKind = "talent_outsourcing_v1"

type PromptTalentCostItem struct {
	Name         domain.Fact[string]     `json:"name"`
	Quantity     domain.Fact[int]        `json:"quantity"`
	QuantityUnit domain.Fact[string]     `json:"quantity_unit"`
	PeriodCount  domain.Fact[int]        `json:"period_count"`
	PeriodUnit   domain.TalentPeriodUnit `json:"period_unit"`
	UnitRate     domain.MoneyInput       `json:"unit_rate"`
}

type PromptTalentCostGroup struct {
	Category domain.TalentCostCategory `json:"category"`
	Title    domain.Fact[string]       `json:"title"`
	Items    []PromptTalentCostItem    `json:"items"`
}

type PromptTalentPreparationItem struct {
	Name            domain.Fact[string] `json:"name"`
	Headcount       domain.Fact[int]    `json:"headcount"`
	QuotedLineTotal domain.MoneyInput   `json:"quoted_line_total"`
}

type PromptTalentSquadItem struct {
	Role         domain.Fact[string] `json:"role"`
	MonthlyRate  domain.MoneyInput   `json:"monthly_rate"`
	Headcount    domain.Fact[int]    `json:"headcount"`
	PeriodMonths domain.Fact[int]    `json:"period_months"`
}

type PromptTalentScopeBlock struct {
	Title            domain.Fact[string]   `json:"title"`
	Description      domain.Fact[string]   `json:"description"`
	Responsibilities []domain.Fact[string] `json:"responsibilities,omitempty"`
	Qualifications   []domain.Fact[string] `json:"qualifications,omitempty"`
	Deliverables     []domain.Fact[string] `json:"deliverables,omitempty"`
	Timeline         domain.Fact[string]   `json:"timeline,omitempty"`
}

type PromptTalentPaymentRow struct {
	WorkPeriod       domain.Fact[string] `json:"work_period"`
	Terms            domain.Fact[string] `json:"terms"`
	InvoiceDateLabel domain.Fact[string] `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string] `json:"due_date_label"`
	Nominal          domain.MoneyInput   `json:"nominal"`
}

type PromptTalentOutsourcingServiceInput struct {
	Kind                       string                        `json:"kind"`
	EngagementMode             domain.Fact[string]           `json:"engagement_mode"`
	DocumentNumber             domain.Fact[string]           `json:"document_number"`
	ServiceName                domain.Fact[string]           `json:"service_name"`
	TaxTreatment               domain.Fact[string]           `json:"tax_treatment"`
	CostGroups                 []PromptTalentCostGroup       `json:"cost_groups,omitempty"`
	PreparationItems           []PromptTalentPreparationItem `json:"preparation_items,omitempty"`
	PreparationDiscountAmount  domain.MoneyInput             `json:"preparation_discount_amount,omitempty"`
	ShowPreparationDiscountRow domain.Fact[bool]             `json:"show_preparation_discount_row,omitempty"`
	SquadItems                 []PromptTalentSquadItem       `json:"squad_items,omitempty"`
	ScopeBlocks                []PromptTalentScopeBlock      `json:"scope_blocks"`
	PaymentSchedule            []PromptTalentPaymentRow      `json:"payment_schedule"`
	PaymentDueDays             domain.Fact[int]              `json:"payment_due_days"`
	LimitationCodes            []domain.TalentLimitationCode `json:"limitation_codes"`
	AdditionalLimitations      []domain.Fact[string]         `json:"additional_limitations,omitempty"`
	WarrantyStatements         []domain.Fact[string]         `json:"warranty_statements,omitempty"`
	RightsStatements           []domain.Fact[string]         `json:"rights_statements,omitempty"`
}

type PromptTalentOutsourcingSnapshot struct {
	Input       domain.TalentOutsourcingInput           `json:"input"`
	Calculation domain.TalentOutsourcingCalculation     `json:"calculation"`
	Profile     domain.ResolvedTalentOutsourcingProfile `json:"profile"`
}

type PromptTalentOutsourcingAdapter struct {
	renderer TalentOutsourcingRenderer
}

func NewPromptTalentOutsourcingAdapter(renderer TalentOutsourcingRenderer) *PromptTalentOutsourcingAdapter {
	if renderer == nil {
		renderer = NewTalentOutsourcingRenderer()
	}
	return &PromptTalentOutsourcingAdapter{renderer: renderer}
}

func (a *PromptTalentOutsourcingAdapter) Scheme() string {
	return domain.CommercialSchemeTalentOutsourcing
}

func DecodePromptTalentOutsourcingServiceInput(raw json.RawMessage) (PromptTalentOutsourcingServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptTalentOutsourcingServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptTalentOutsourcingServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptTalentOutsourcingServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	if input.Kind != PromptTalentOutsourcingServiceInputKind {
		return PromptTalentOutsourcingServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptTalentOutsourcingServiceInputKind)
	}
	normalizeUnknownTalentMoney(&input)
	return input, nil
}

func EncodePromptTalentOutsourcingServiceInput(input PromptTalentOutsourcingServiceInput) ([]byte, error) {
	if input.Kind != PromptTalentOutsourcingServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptTalentOutsourcingServiceInputKind)
	}
	return json.Marshal(input)
}

func BuildPromptTalentOutsourcingInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.TalentOutsourcingInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 8)
	if state == nil {
		return domain.TalentOutsourcingInput{}, append(issues, talentIssue("talent_outsourcing.state.missing", "state", "prompt state is required"))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeTalentOutsourcing {
		return domain.TalentOutsourcingInput{}, append(issues, talentIssue("talent_outsourcing.scheme.invalid", "commercial_scheme", "adapter requires talent_outsourcing scheme"))
	}
	serviceInput, err := DecodePromptTalentOutsourcingServiceInput(state.ServiceInput)
	if err != nil {
		return domain.TalentOutsourcingInput{}, append(issues, talentIssue("talent_outsourcing.service_input.invalid", "service_input", err.Error()))
	}

	input := domain.TalentOutsourcingInput{
		QuotationDate:   quotationDate,
		EngagementMode:  domain.TalentEngagementMode(requiredTalentText(&issues, "engagement_mode", serviceInput.EngagementMode)),
		DocumentNumber:  requiredTalentText(&issues, "document_number", serviceInput.DocumentNumber),
		ServiceName:     requiredTalentText(&issues, "service_name", serviceInput.ServiceName),
		TaxTreatment:    domain.TalentTaxTreatment(requiredTalentText(&issues, "tax_treatment", serviceInput.TaxTreatment)),
		PaymentDueDays:  int32(requiredTalentInt(&issues, "payment_due_days", serviceInput.PaymentDueDays)),
		LimitationCodes: serviceInput.LimitationCodes,
	}

	if input.TaxTreatment != domain.TalentTaxBPJSDPP12 && input.TaxTreatment != domain.TalentTaxExclusive && input.TaxTreatment != domain.TalentTaxNotStated {
		issues = append(issues, talentIssue("talent_outsourcing.tax_policy.unknown", "tax_treatment", "unknown tax policy"))
	}

	for gi, g := range serviceInput.CostGroups {
		group := domain.TalentCostGroupInput{
			Category: g.Category,
			Title:    requiredTalentText(&issues, fmt.Sprintf("cost_groups[%d].title", gi), g.Title),
		}
		for ii, item := range g.Items {
			prefix := fmt.Sprintf("cost_groups[%d].items[%d]", gi, ii)

			// Non-personnel blocking issue check
			var code string = "talent_outsourcing.cost_item.unit_rate"
			group.Items = append(group.Items, domain.TalentCostItemInput{
				Name:         requiredTalentText(&issues, prefix+".name", item.Name),
				Quantity:     int32(requiredTalentInt(&issues, prefix+".quantity", item.Quantity)),
				QuantityUnit: requiredTalentText(&issues, prefix+".quantity_unit", item.QuantityUnit),
				PeriodCount:  int32(requiredTalentInt(&issues, prefix+".period_count", item.PeriodCount)),
				PeriodUnit:   item.PeriodUnit,
				UnitRate:     requiredTalentMoney(&issues, code, prefix+".unit_rate", item.UnitRate),
			})
		}
		input.CostGroups = append(input.CostGroups, group)
	}

	for i, item := range serviceInput.PreparationItems {
		prefix := fmt.Sprintf("preparation_items[%d]", i)
		input.PreparationItems = append(input.PreparationItems, domain.TalentPreparationItemInput{
			Name:            requiredTalentText(&issues, prefix+".name", item.Name),
			Headcount:       int32(requiredTalentInt(&issues, prefix+".headcount", item.Headcount)),
			QuotedLineTotal: requiredTalentMoney(&issues, "talent_outsourcing.preparation_item.quoted_line_total", prefix+".quoted_line_total", item.QuotedLineTotal),
		})
	}

	if serviceInput.PreparationDiscountAmount.Amount != nil || (serviceInput.PreparationDiscountAmount.Source != "" && serviceInput.PreparationDiscountAmount.Source != domain.SourceSuggestion) {
		input.PreparationDiscountAmount = requiredTalentMoney(&issues, "talent_outsourcing.preparation_discount", "preparation_discount_amount", serviceInput.PreparationDiscountAmount)
	}
	input.ShowPreparationDiscountRow = serviceInput.ShowPreparationDiscountRow.Value

	for i, item := range serviceInput.SquadItems {
		prefix := fmt.Sprintf("squad_items[%d]", i)
		input.SquadItems = append(input.SquadItems, domain.TalentSquadItemInput{
			Role:         requiredTalentText(&issues, prefix+".role", item.Role),
			MonthlyRate:  requiredTalentMoney(&issues, "talent_outsourcing.squad_item.monthly_rate", prefix+".monthly_rate", item.MonthlyRate),
			Headcount:    int32(requiredTalentInt(&issues, prefix+".headcount", item.Headcount)),
			PeriodMonths: int32(requiredTalentInt(&issues, prefix+".period_months", item.PeriodMonths)),
		})
	}

	for i, block := range serviceInput.ScopeBlocks {
		prefix := fmt.Sprintf("scope_blocks[%d]", i)
		mapped := domain.TalentScopeBlockInput{
			Title:       requiredTalentText(&issues, prefix+".title", block.Title),
			Description: optionalTalentText(&issues, prefix+".description", block.Description),
			Timeline:    optionalTalentText(&issues, prefix+".timeline", block.Timeline),
		}
		for j, fact := range block.Responsibilities {
			mapped.Responsibilities = append(mapped.Responsibilities, requiredTalentText(&issues, fmt.Sprintf("%s.responsibilities[%d]", prefix, j), fact))
		}
		for j, fact := range block.Qualifications {
			mapped.Qualifications = append(mapped.Qualifications, requiredTalentText(&issues, fmt.Sprintf("%s.qualifications[%d]", prefix, j), fact))
		}
		for j, fact := range block.Deliverables {
			mapped.Deliverables = append(mapped.Deliverables, requiredTalentText(&issues, fmt.Sprintf("%s.deliverables[%d]", prefix, j), fact))
		}
		input.ScopeBlocks = append(input.ScopeBlocks, mapped)
	}

	for i, row := range serviceInput.PaymentSchedule {
		prefix := fmt.Sprintf("payment_schedule[%d]", i)
		input.PaymentSchedule = append(input.PaymentSchedule, domain.TalentPaymentRowInput{
			WorkPeriod:       requiredTalentText(&issues, prefix+".work_period", row.WorkPeriod),
			Terms:            requiredTalentText(&issues, prefix+".terms", row.Terms),
			InvoiceDateLabel: requiredTalentText(&issues, prefix+".invoice_date_label", row.InvoiceDateLabel),
			DueDateLabel:     requiredTalentText(&issues, prefix+".due_date_label", row.DueDateLabel),
			Nominal:          requiredTalentMoney(&issues, "talent_outsourcing.payment_nominal", prefix+".nominal", row.Nominal),
		})
	}

	for i, fact := range serviceInput.AdditionalLimitations {
		input.AdditionalLimitations = append(input.AdditionalLimitations, requiredTalentText(&issues, fmt.Sprintf("additional_limitations[%d]", i), fact))
	}
	for i, fact := range serviceInput.WarrantyStatements {
		input.WarrantyStatements = append(input.WarrantyStatements, requiredTalentText(&issues, fmt.Sprintf("warranty_statements[%d]", i), fact))
	}
	for i, fact := range serviceInput.RightsStatements {
		input.RightsStatements = append(input.RightsStatements, requiredTalentText(&issues, fmt.Sprintf("rights_statements[%d]", i), fact))
	}

	return input, issues
}

func CalculatePromptTalentOutsourcing(state *domain.PromptQuotationState, quotationDate time.Time) (domain.TalentOutsourcingInput, domain.TalentOutsourcingCalculation, domain.ResolvedTalentOutsourcingProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptTalentOutsourcingInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.TalentOutsourcingCalculation{}, domain.ResolvedTalentOutsourcingProfile{}, issues, nil
	}
	if err := input.Validate(); err != nil {
		issues = append(issues, talentIssue("talent_outsourcing.input.invalid", "service_input", err.Error()))
		return input, domain.TalentOutsourcingCalculation{}, domain.ResolvedTalentOutsourcingProfile{}, issues, nil
	}
	calculation, err := CalculateTalentOutsourcing(input)
	if err != nil {
		code, field := "talent_outsourcing.calculation.invalid", "calculation"
		issues = append(issues, talentIssue(code, field, err.Error()))
		return input, domain.TalentOutsourcingCalculation{}, domain.ResolvedTalentOutsourcingProfile{}, issues, nil
	}
	profile, err := ResolveTalentOutsourcingProfile(input)
	if err != nil {
		return input, domain.TalentOutsourcingCalculation{}, domain.ResolvedTalentOutsourcingProfile{}, issues, err
	}
	return input, calculation, profile, issues, nil
}

func (a *PromptTalentOutsourcingAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.TalentOutsourcingInput, domain.TalentOutsourcingCalculation, domain.ResolvedTalentOutsourcingProfile, []domain.DraftIssue, error) {
	return CalculatePromptTalentOutsourcing(state, quotationDate)
}

func (a *PromptTalentOutsourcingAdapter) Render(metadata QuotationMetadata, snapshot PromptTalentOutsourcingSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("talent outsourcing renderer is unavailable")
	}
	return a.renderer.Render(TalentOutsourcingDocumentInput{
		Metadata:    metadata,
		Input:       snapshot.Input,
		Calculation: snapshot.Calculation,
		Profile:     snapshot.Profile,
		LegalHTML:   legalHTML,
	})
}

func talentIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func validateTalentFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, talentIssue("talent_outsourcing."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, talentIssue("talent_outsourcing."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredTalentText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateTalentFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, talentIssue("talent_outsourcing."+field+".missing", field, "value is required"))
	}
	return value
}

func optionalTalentText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	if fact.Source == "" {
		return ""
	}
	validateTalentFact(issues, field, fact.Source, fact.Evidence)
	return strings.TrimSpace(fact.Value)
}

func requiredTalentInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateTalentFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, talentIssue("talent_outsourcing."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func requiredTalentMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, talentIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, talentIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, talentIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, talentIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

func normalizeUnknownTalentMoney(input *PromptTalentOutsourcingServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for i := range input.CostGroups {
		for j := range input.CostGroups[i].Items {
			normalize(&input.CostGroups[i].Items[j].UnitRate)
		}
	}
	for i := range input.PreparationItems {
		normalize(&input.PreparationItems[i].QuotedLineTotal)
	}
	normalize(&input.PreparationDiscountAmount)
	for i := range input.SquadItems {
		normalize(&input.SquadItems[i].MonthlyRate)
	}
	for i := range input.PaymentSchedule {
		normalize(&input.PaymentSchedule[i].Nominal)
	}
}
