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

// PromptCloudInfrastructureServiceInputKind is the versioned prompt-state payload
// for Cloud Infrastructure quotations. Descriptive meeting facts remain separate
// from prices, which are accepted only after a catalog or BA supplies them.
const PromptCloudInfrastructureServiceInputKind = "cloud_infrastructure_v1"

type PromptCloudPackageItem struct {
	Category  domain.Fact[string] `json:"category"`
	Name      domain.Fact[string] `json:"name"`
	VCPU      domain.Fact[int]    `json:"vcpu"`
	RAMGB     domain.Fact[int]    `json:"ram_gb"`
	StorageGB domain.Fact[int]    `json:"storage_gb"`
	UnitRate  domain.MoneyInput   `json:"unit_rate"`
}

type PromptCloudSetupItem struct {
	Name   domain.Fact[string] `json:"name"`
	Amount domain.MoneyInput   `json:"amount"`
}

type PromptCloudScopeSection struct {
	Title      domain.Fact[string]   `json:"title"`
	Activities []domain.Fact[string] `json:"activities"`
}

type PromptCloudScopeRow struct {
	Sections []PromptCloudScopeSection `json:"sections"`
	Timeline domain.Fact[string]       `json:"timeline"`
}

type PromptCloudPaymentRow struct {
	WorkPeriod       domain.Fact[string]          `json:"work_period"`
	Terms            domain.Fact[string]          `json:"terms"`
	InvoiceDateLabel domain.Fact[string]          `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string]          `json:"due_date_label"`
	Component        domain.CloudPaymentComponent `json:"component"`
	Nominal          domain.MoneyInput            `json:"nominal"`
}

type PromptCloudInfrastructureServiceInput struct {
	Kind                  string                   `json:"kind"`
	DocumentNumber        domain.Fact[string]      `json:"document_number"`
	ServiceName           domain.Fact[string]      `json:"service_name"`
	TaxTreatment          domain.Fact[string]      `json:"tax_treatment"`
	PackageItems          []PromptCloudPackageItem `json:"package_items"`
	SetupItems            []PromptCloudSetupItem   `json:"setup_items,omitempty"`
	ScopeRows             []PromptCloudScopeRow    `json:"scope_rows,omitempty"`
	PaymentSchedule       []PromptCloudPaymentRow  `json:"payment_schedule"`
	PaymentDueDays        domain.Fact[int]         `json:"payment_due_days"`
	LimitationCodes       []domain.Fact[string]    `json:"limitation_codes"`
	AdditionalLimitations []domain.Fact[string]    `json:"additional_limitations,omitempty"`
}

// PromptCloudInfrastructureSnapshot is the immutable calculator and renderer input
// a prompt-first caller persists alongside its normal quotation snapshots.
type PromptCloudInfrastructureSnapshot struct {
	Input       domain.CloudInfrastructureInput           `json:"input"`
	Calculation domain.CloudInfrastructureCalculation     `json:"calculation"`
	Profile     domain.ResolvedCloudInfrastructureProfile `json:"profile"`
}

type PromptCloudInfrastructureAdapter struct {
	renderer CloudInfrastructureRenderer
}

func NewPromptCloudInfrastructureAdapter(renderer CloudInfrastructureRenderer) *PromptCloudInfrastructureAdapter {
	if renderer == nil {
		renderer = NewCloudInfrastructureRenderer()
	}
	return &PromptCloudInfrastructureAdapter{renderer: renderer}
}

func (a *PromptCloudInfrastructureAdapter) Scheme() string {
	return domain.CommercialSchemeCloudInfrastructure
}

func DecodePromptCloudInfrastructureServiceInput(raw json.RawMessage) (PromptCloudInfrastructureServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptCloudInfrastructureServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptCloudInfrastructureServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptCloudInfrastructureServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PromptCloudInfrastructureServiceInput{}, fmt.Errorf("service_input: must contain one JSON object")
	}
	if input.Kind != PromptCloudInfrastructureServiceInputKind {
		return PromptCloudInfrastructureServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptCloudInfrastructureServiceInputKind)
	}
	normalizeUnknownCloudInfrastructureMoney(&input)
	return input, nil
}

func EncodePromptCloudInfrastructureServiceInput(input PromptCloudInfrastructureServiceInput) ([]byte, error) {
	if input.Kind != PromptCloudInfrastructureServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptCloudInfrastructureServiceInputKind)
	}
	return json.Marshal(input)
}

// BuildPromptCloudInfrastructureInput maps trusted price inputs and descriptive facts only.
// It enforces that one package item has unit rate per month and no quantity.
func BuildPromptCloudInfrastructureInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.CloudInfrastructureInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 16)
	if state == nil {
		return domain.CloudInfrastructureInput{}, append(issues, cloudIssue("cloud_infrastructure.state.missing", "state", "prompt state is required"))
	}
	if err := state.Validate(); err != nil {
		return domain.CloudInfrastructureInput{}, append(issues, cloudIssue("cloud_infrastructure.state.invalid", "state", err.Error()))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeCloudInfrastructure {
		return domain.CloudInfrastructureInput{}, append(issues, cloudIssue("cloud_infrastructure.scheme.invalid", "commercial_scheme", "cloud infrastructure adapter requires the cloud_infrastructure scheme"))
	}
	serviceInput, err := DecodePromptCloudInfrastructureServiceInput(state.ServiceInput)
	if err != nil {
		return domain.CloudInfrastructureInput{}, append(issues, cloudIssue("cloud_infrastructure.service_input.invalid", "service_input", err.Error()))
	}
	if quotationDate.IsZero() {
		issues = append(issues, cloudIssue("cloud_infrastructure.quotation_date.missing", "quotation_date", "quotation date is required"))
	}

	input := domain.CloudInfrastructureInput{
		DocumentNumber: requiredCloudText(&issues, "document_number", serviceInput.DocumentNumber),
		ServiceName:    requiredCloudText(&issues, "service_name", serviceInput.ServiceName),
		QuotationDate:  quotationDate,
		TaxTreatment:   domain.CloudTaxTreatment(requiredCloudText(&issues, "tax_treatment", serviceInput.TaxTreatment)),
	}

	if input.TaxTreatment != domain.CloudTaxExclusive && input.TaxTreatment != domain.CloudTaxInclusive && input.TaxTreatment != domain.CloudTaxNotStated {
		issues = append(issues, cloudIssue("cloud_infrastructure.tax_treatment.invalid", "tax_treatment", fmt.Sprintf("unsupported tax treatment %q", input.TaxTreatment)))
	}

	if len(serviceInput.PackageItems) == 0 {
		issues = append(issues, cloudIssue("cloud_infrastructure.package_items.missing", "package_items", "package items are required"))
	}
	for i, item := range serviceInput.PackageItems {
		pField := fmt.Sprintf("package_items[%d]", i)
		catStr := requiredCloudText(&issues, pField+".category", item.Category)
		cat := domain.CloudPackageCategory(catStr)
		if cat != domain.CloudPackageSharedHosting && cat != domain.CloudPackageVPS {
			issues = append(issues, cloudIssue("cloud_infrastructure.package_item.category.invalid", pField+".category", fmt.Sprintf("unsupported package category %q", catStr)))
		}
		input.PackageItems = append(input.PackageItems, domain.CloudPackageItemInput{
			Category:  cat,
			Name:      requiredCloudText(&issues, pField+".name", item.Name),
			VCPU:      int32(requiredCloudInt(&issues, pField+".vcpu", item.VCPU)),
			RAMGB:     int32(requiredCloudInt(&issues, pField+".ram_gb", item.RAMGB)),
			StorageGB: int32(requiredCloudInt(&issues, pField+".storage_gb", item.StorageGB)),
			UnitRate:  requiredCloudMoney(&issues, "cloud_infrastructure.package_item.unit_rate", pField+".unit_rate", item.UnitRate),
		})
	}

	for i, item := range serviceInput.SetupItems {
		sField := fmt.Sprintf("setup_items[%d]", i)
		input.SetupItems = append(input.SetupItems, domain.CloudSetupItemInput{
			Name:   requiredCloudText(&issues, sField+".name", item.Name),
			Amount: requiredCloudMoney(&issues, "cloud_infrastructure.setup_item.amount", sField+".amount", item.Amount),
		})
	}

	for i, row := range serviceInput.ScopeRows {
		rField := fmt.Sprintf("scope_rows[%d]", i)
		scopeRow := domain.CloudScopeRowInput{
			Timeline: requiredCloudText(&issues, rField+".timeline", row.Timeline),
		}
		for j, sec := range row.Sections {
			secField := fmt.Sprintf("%s.sections[%d]", rField, j)
			scopeSec := domain.CloudScopeSectionInput{
				Title: requiredCloudText(&issues, secField+".title", sec.Title),
			}
			for k, act := range sec.Activities {
				scopeSec.Activities = append(scopeSec.Activities, requiredCloudText(&issues, fmt.Sprintf("%s.activities[%d]", secField, k), act))
			}
			scopeRow.Sections = append(scopeRow.Sections, scopeSec)
		}
		input.ScopeRows = append(input.ScopeRows, scopeRow)
	}

	if len(serviceInput.PaymentSchedule) == 0 {
		issues = append(issues, cloudIssue("cloud_infrastructure.payment_schedule.missing", "payment_schedule", "payment schedule is required"))
	}
	for i, row := range serviceInput.PaymentSchedule {
		payField := fmt.Sprintf("payment_schedule[%d]", i)
		input.PaymentSchedule = append(input.PaymentSchedule, domain.CloudPaymentRowInput{
			WorkPeriod:       requiredCloudText(&issues, payField+".work_period", row.WorkPeriod),
			Terms:            requiredCloudText(&issues, payField+".terms", row.Terms),
			InvoiceDateLabel: requiredCloudText(&issues, payField+".invoice_date_label", row.InvoiceDateLabel),
			DueDateLabel:     requiredCloudText(&issues, payField+".due_date_label", row.DueDateLabel),
			Component:        row.Component,
			Nominal:          requiredCloudMoney(&issues, "cloud_infrastructure.payment_nominal", payField+".nominal", row.Nominal),
		})
	}

	if serviceInput.PaymentDueDays.Value > 0 {
		input.PaymentDueDays = int32(requiredCloudInt(&issues, "payment_due_days", serviceInput.PaymentDueDays))
	} else {
		input.PaymentDueDays = 14
	}

	if len(serviceInput.LimitationCodes) == 0 {
		issues = append(issues, cloudIssue("cloud_infrastructure.limitation_codes.missing", "limitation_codes", "limitation codes are required"))
	}
	for i, code := range serviceInput.LimitationCodes {
		input.LimitationCodes = append(input.LimitationCodes, domain.CloudLimitationCode(requiredCloudText(&issues, fmt.Sprintf("limitation_codes[%d]", i), code)))
	}

	for i, add := range serviceInput.AdditionalLimitations {
		input.AdditionalLimitations = append(input.AdditionalLimitations, requiredCloudText(&issues, fmt.Sprintf("additional_limitations[%d]", i), add))
	}

	return input, issues
}

// CalculatePromptCloudInfrastructure validates input, executes the domain calculator,
// resolves document profile, and maps domain errors to blocking draft issues.
func CalculatePromptCloudInfrastructure(state *domain.PromptQuotationState, quotationDate time.Time) (domain.CloudInfrastructureInput, domain.CloudInfrastructureCalculation, domain.ResolvedCloudInfrastructureProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptCloudInfrastructureInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.CloudInfrastructureCalculation{}, domain.ResolvedCloudInfrastructureProfile{}, issues, nil
	}

	if err := input.Validate(); err != nil {
		issues = append(issues, cloudIssue("cloud_infrastructure.input.invalid", "service_input", err.Error()))
		return input, domain.CloudInfrastructureCalculation{}, domain.ResolvedCloudInfrastructureProfile{}, issues, nil
	}

	calculation, err := CalculateCloudInfrastructure(input)
	if err != nil {
		code, field := "cloud_infrastructure.calculation.invalid", "calculation"
		if strings.Contains(err.Error(), "payment_schedule") || strings.Contains(err.Error(), "payment total") || strings.Contains(err.Error(), "monthly_service nominal") || strings.Contains(err.Error(), "missing monthly_service row") {
			code, field = "cloud_infrastructure.payment.mismatch", "payment_schedule"
		}
		issues = append(issues, cloudIssue(code, field, err.Error()))
		return input, domain.CloudInfrastructureCalculation{}, domain.ResolvedCloudInfrastructureProfile{}, issues, nil
	}

	profile, err := ResolveCloudInfrastructureProfile(input)
	if err != nil {
		issues = append(issues, cloudIssue("cloud_infrastructure.profile.invalid", "profile", err.Error()))
		return input, domain.CloudInfrastructureCalculation{}, domain.ResolvedCloudInfrastructureProfile{}, issues, nil
	}

	return input, calculation, profile, issues, nil
}

func (a *PromptCloudInfrastructureAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.CloudInfrastructureInput, domain.CloudInfrastructureCalculation, domain.ResolvedCloudInfrastructureProfile, []domain.DraftIssue, error) {
	return CalculatePromptCloudInfrastructure(state, quotationDate)
}

func (a *PromptCloudInfrastructureAdapter) Render(metadata QuotationMetadata, snapshot PromptCloudInfrastructureSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("cloud infrastructure renderer is unavailable")
	}
	return a.renderer.Render(CloudInfrastructureDocumentInput{
		Metadata:    metadata,
		Input:       snapshot.Input,
		Calculation: snapshot.Calculation,
		Profile:     snapshot.Profile,
		LegalHTML:   legalHTML,
	})
}

func cloudIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func requiredCloudText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateCloudFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, cloudIssue("cloud_infrastructure."+field+".missing", field, "value is required"))
	}
	return value
}

func requiredCloudInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateCloudFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, cloudIssue("cloud_infrastructure."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func validateCloudFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, cloudIssue("cloud_infrastructure."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, cloudIssue("cloud_infrastructure."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredCloudMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, cloudIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, cloudIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, cloudIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, cloudIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

// Extraction omits amounts. Keep an omitted amount explicitly unknown so the adapter
// emits a blocking issue instead of accepting zero or treating the model as the source of a price.
func normalizeUnknownCloudInfrastructureMoney(input *PromptCloudInfrastructureServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for i := range input.PackageItems {
		normalize(&input.PackageItems[i].UnitRate)
	}
	for i := range input.SetupItems {
		normalize(&input.SetupItems[i].Amount)
	}
	for i := range input.PaymentSchedule {
		normalize(&input.PaymentSchedule[i].Nominal)
	}
}
