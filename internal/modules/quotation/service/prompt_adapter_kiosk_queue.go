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

// PromptKioskQueueServiceInputKind is the versioned prompt-state payload for
// Kiosk & Queue System quotations. Descriptive meeting facts remain separate
// from prices, which are accepted only after a catalog or BA supplies them.
const PromptKioskQueueServiceInputKind = "kiosk_queue_system_v1"

type PromptKioskQueueScopePhase struct {
	Title        domain.Fact[string]   `json:"title"`
	Deliverables []domain.Fact[string] `json:"deliverables"`
}

type PromptKioskQueueExpertItem struct {
	Role                 domain.Fact[string] `json:"role"`
	Headcount            domain.Fact[int]    `json:"headcount"`
	DurationLabel        domain.Fact[string] `json:"duration_label"`
	MinimumQualification domain.Fact[string] `json:"minimum_qualification"`
}

type PromptKioskQueueServiceItem struct {
	Name           domain.Fact[string] `json:"name"`
	MonthlyRate    domain.MoneyInput   `json:"monthly_rate"`
	Headcount      domain.Fact[int]    `json:"headcount"`
	PeriodQuarters domain.Fact[int]    `json:"period_quarters"`
}

type PromptKioskQueueServiceGroup struct {
	Title domain.Fact[string]           `json:"title"`
	Items []PromptKioskQueueServiceItem `json:"items"`
}

type PromptKioskQueueHardwareItem struct {
	Name      domain.Fact[string] `json:"name"`
	UnitPrice domain.MoneyInput   `json:"unit_price"`
	Quantity  domain.Fact[int]    `json:"quantity"`
}

type PromptKioskQueueTimelineRow struct {
	Description domain.Fact[string] `json:"description"`
	Timeline    domain.Fact[string] `json:"timeline"`
}

type PromptKioskQueueGovernmentPaymentRow struct {
	Stage        domain.Fact[string] `json:"stage"`
	BillingBasis domain.Fact[string] `json:"billing_basis"`
	Nominal      domain.MoneyInput   `json:"nominal"`
}

type PromptKioskQueuePrivatePaymentRow struct {
	WorkPeriod       domain.Fact[string] `json:"work_period"`
	Terms            domain.Fact[string] `json:"terms"`
	InvoiceDateLabel domain.Fact[string] `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string] `json:"due_date_label"`
	Nominal          domain.MoneyInput   `json:"nominal"`
}

type PromptKioskQueueServiceInput struct {
	Kind                      string                                 `json:"kind"`
	Variant                   domain.Fact[string]                    `json:"variant"`
	DocumentNumber            *domain.Fact[string]                   `json:"document_number,omitempty"`
	SystemName                domain.Fact[string]                    `json:"system_name"`
	HardwareProvision         domain.Fact[string]                    `json:"hardware_provision"`
	TaxTreatment              domain.Fact[string]                    `json:"tax_treatment"`
	GovernmentScopePhases     []PromptKioskQueueScopePhase           `json:"government_scope_phases,omitempty"`
	ExpertRequirements        []PromptKioskQueueExpertItem           `json:"expert_requirements,omitempty"`
	ServiceGroups             []PromptKioskQueueServiceGroup         `json:"service_groups"`
	HardwareItems             []PromptKioskQueueHardwareItem         `json:"hardware_items,omitempty"`
	MaintenanceItems          []PromptKioskQueueServiceItem          `json:"maintenance_items,omitempty"`
	PrivateTimelineRows       []PromptKioskQueueTimelineRow          `json:"private_timeline_rows,omitempty"`
	GovernmentPaymentSchedule []PromptKioskQueueGovernmentPaymentRow `json:"government_payment_schedule,omitempty"`
	PrivatePaymentSchedule    []PromptKioskQueuePrivatePaymentRow    `json:"private_payment_schedule,omitempty"`
	PaymentDueDays            *domain.Fact[int]                      `json:"payment_due_days,omitempty"`
	LimitationCodes           []domain.Fact[string]                  `json:"limitation_codes"`
	AdditionalLimitations     []domain.Fact[string]                  `json:"additional_limitations,omitempty"`
	RightsStatements          []domain.Fact[string]                  `json:"rights_statements,omitempty"`
}

// PromptKioskQueueSnapshot is the immutable calculator and renderer input a
// prompt-first caller persists alongside its normal quotation snapshots.
type PromptKioskQueueSnapshot struct {
	Input       domain.KioskQueueSystemInput     `json:"input"`
	Calculation domain.KioskQueueCalculation     `json:"calculation"`
	Profile     domain.ResolvedKioskQueueProfile `json:"profile"`
}

type PromptKioskQueueAdapter struct {
	renderer KioskQueueHTMLRenderer
}

func NewPromptKioskQueueAdapter(renderer KioskQueueHTMLRenderer) *PromptKioskQueueAdapter {
	if renderer == nil {
		renderer = NewKioskQueueHTMLRenderer()
	}
	return &PromptKioskQueueAdapter{renderer: renderer}
}

func (a *PromptKioskQueueAdapter) Scheme() string {
	return domain.CommercialSchemeKioskQueueSystem
}

func DecodePromptKioskQueueServiceInput(raw json.RawMessage) (PromptKioskQueueServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptKioskQueueServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptKioskQueueServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptKioskQueueServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PromptKioskQueueServiceInput{}, fmt.Errorf("service_input: must contain one JSON object")
	}
	if input.Kind != PromptKioskQueueServiceInputKind {
		return PromptKioskQueueServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptKioskQueueServiceInputKind)
	}
	normalizeUnknownKioskQueueMoney(&input)
	return input, nil
}

func EncodePromptKioskQueueServiceInput(input PromptKioskQueueServiceInput) ([]byte, error) {
	if input.Kind != PromptKioskQueueServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptKioskQueueServiceInputKind)
	}
	return json.Marshal(input)
}

// BuildPromptKioskQueueInput maps trusted price inputs and descriptive facts only.
// It maps period_quarters as quarter-month units and verifies tax policies per variant.
// Client-supplied hardware never produces zero-priced hardware lines.
func BuildPromptKioskQueueInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.KioskQueueSystemInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 16)
	if state == nil {
		return domain.KioskQueueSystemInput{}, append(issues, kioskQueueIssue("kiosk_queue.state.missing", "state", "prompt state is required"))
	}
	if err := state.Validate(); err != nil {
		return domain.KioskQueueSystemInput{}, append(issues, kioskQueueIssue("kiosk_queue.state.invalid", "state", err.Error()))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeKioskQueueSystem {
		return domain.KioskQueueSystemInput{}, append(issues, kioskQueueIssue("kiosk_queue.scheme.invalid", "commercial_scheme", "kiosk queue adapter requires the kiosk_queue_system scheme"))
	}
	serviceInput, err := DecodePromptKioskQueueServiceInput(state.ServiceInput)
	if err != nil {
		return domain.KioskQueueSystemInput{}, append(issues, kioskQueueIssue("kiosk_queue.service_input.invalid", "service_input", err.Error()))
	}
	if quotationDate.IsZero() {
		issues = append(issues, kioskQueueIssue("kiosk_queue.quotation_date.missing", "quotation_date", "quotation date is required"))
	}

	variantStr := requiredKioskQueueText(&issues, "variant", serviceInput.Variant)
	variant := domain.KioskQueueVariant(variantStr)
	if variant != domain.KioskQueueGovernmentMPP && variant != domain.KioskQueuePrivateFoodCourt {
		issues = append(issues, kioskQueueIssue("kiosk_queue.variant.invalid", "variant", fmt.Sprintf("unsupported variant %q", variantStr)))
	}

	systemName := requiredKioskQueueText(&issues, "system_name", serviceInput.SystemName)
	documentNumber := optionalKioskQueueText(&issues, "document_number", serviceInput.DocumentNumber)

	hardwareProvisionStr := requiredKioskQueueText(&issues, "hardware_provision", serviceInput.HardwareProvision)
	hardwareProvision := domain.HardwareProvision(hardwareProvisionStr)
	if hardwareProvision != domain.HardwareVenturoSupplied && hardwareProvision != domain.HardwareClientSupplied {
		issues = append(issues, kioskQueueIssue("kiosk_queue.hardware_provision.invalid", "hardware_provision", fmt.Sprintf("unsupported hardware provision %q", hardwareProvisionStr)))
	}

	taxTreatmentStr := requiredKioskQueueText(&issues, "tax_treatment", serviceInput.TaxTreatment)
	taxTreatment := domain.KioskQueueTaxTreatment(taxTreatmentStr)

	// Tax policy gate per variant
	switch variant {
	case domain.KioskQueueGovernmentMPP:
		if taxTreatment != domain.KioskTaxInclusivePPN11 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.tax_treatment.invalid", "tax_treatment", fmt.Sprintf("government_mpp requires %q tax treatment, got %q", domain.KioskTaxInclusivePPN11, taxTreatmentStr)))
		}
	case domain.KioskQueuePrivateFoodCourt:
		if taxTreatment != domain.KioskTaxExclusivePPN {
			issues = append(issues, kioskQueueIssue("kiosk_queue.tax_treatment.invalid", "tax_treatment", fmt.Sprintf("private_food_court requires %q tax treatment, got %q", domain.KioskTaxExclusivePPN, taxTreatmentStr)))
		}
		if hardwareProvision != domain.HardwareClientSupplied {
			issues = append(issues, kioskQueueIssue("kiosk_queue.hardware_provision.invalid", "hardware_provision", fmt.Sprintf("private_food_court requires %q hardware provision", domain.HardwareClientSupplied)))
		}
	}

	input := domain.KioskQueueSystemInput{
		Variant:           variant,
		DocumentNumber:    documentNumber,
		SystemName:        systemName,
		QuotationDate:     quotationDate,
		HardwareProvision: hardwareProvision,
		TaxTreatment:      taxTreatment,
	}

	// Scope phases (Government MPP)
	if variant == domain.KioskQueueGovernmentMPP {
		if len(serviceInput.GovernmentScopePhases) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.government_scope_phases.missing", "government_scope_phases", "government scope phases are required"))
		}
		for pIdx, phase := range serviceInput.GovernmentScopePhases {
			pField := fmt.Sprintf("government_scope_phases[%d]", pIdx)
			phaseInput := domain.KioskQueueScopePhaseInput{
				Title: requiredKioskQueueText(&issues, pField+".title", phase.Title),
			}
			if len(phase.Deliverables) == 0 {
				issues = append(issues, kioskQueueIssue("kiosk_queue.government_scope_phases.deliverables_missing", pField+".deliverables", "phase deliverables are required"))
			}
			for dIdx, deliv := range phase.Deliverables {
				phaseInput.Deliverables = append(phaseInput.Deliverables, requiredKioskQueueText(&issues, fmt.Sprintf("%s.deliverables[%d]", pField, dIdx), deliv))
			}
			input.GovernmentScopePhases = append(input.GovernmentScopePhases, phaseInput)
		}

		if len(serviceInput.ExpertRequirements) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.expert_requirements.missing", "expert_requirements", "expert requirements are required"))
		}
		for eIdx, exp := range serviceInput.ExpertRequirements {
			eField := fmt.Sprintf("expert_requirements[%d]", eIdx)
			input.ExpertRequirements = append(input.ExpertRequirements, domain.KioskQueueExpertInput{
				Role:                 requiredKioskQueueText(&issues, eField+".role", exp.Role),
				Headcount:            int32(requiredKioskQueueInt(&issues, eField+".headcount", exp.Headcount)),
				DurationLabel:        requiredKioskQueueText(&issues, eField+".duration_label", exp.DurationLabel),
				MinimumQualification: requiredKioskQueueText(&issues, eField+".minimum_qualification", exp.MinimumQualification),
			})
		}
	}

	// Service groups (Both variants)
	if len(serviceInput.ServiceGroups) == 0 {
		issues = append(issues, kioskQueueIssue("kiosk_queue.service_groups.missing", "service_groups", "service groups are required"))
	}
	for gIdx, group := range serviceInput.ServiceGroups {
		gField := fmt.Sprintf("service_groups[%d]", gIdx)
		groupInput := domain.KioskQueueServiceGroupInput{
			Title: requiredKioskQueueText(&issues, gField+".title", group.Title),
		}
		if len(group.Items) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.service_groups.items_missing", gField+".items", "service group items are required"))
		}
		for iIdx, item := range group.Items {
			iField := fmt.Sprintf("%s.items[%d]", gField, iIdx)
			groupInput.Items = append(groupInput.Items, domain.KioskQueueServiceItemInput{
				Name:           requiredKioskQueueText(&issues, iField+".name", item.Name),
				MonthlyRate:    requiredKioskQueueMoney(&issues, "kiosk_queue.service_monthly_rate", iField+".monthly_rate", item.MonthlyRate),
				Headcount:      int32(requiredKioskQueueInt(&issues, iField+".headcount", item.Headcount)),
				PeriodQuarters: int32(requiredKioskQueueInt(&issues, iField+".period_quarters", item.PeriodQuarters)),
			})
		}
		input.ServiceGroups = append(input.ServiceGroups, groupInput)
	}

	// Hardware items (Only for Venturo-supplied hardware in Government MPP)
	if variant == domain.KioskQueueGovernmentMPP && hardwareProvision == domain.HardwareVenturoSupplied {
		if len(serviceInput.HardwareItems) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.hardware_items.missing", "hardware_items", "hardware items are required when hardware provision is venturo_supplied"))
		}
		for hIdx, item := range serviceInput.HardwareItems {
			hField := fmt.Sprintf("hardware_items[%d]", hIdx)
			input.HardwareItems = append(input.HardwareItems, domain.KioskQueueHardwareItemInput{
				Name:      requiredKioskQueueText(&issues, hField+".name", item.Name),
				UnitPrice: requiredKioskQueueMoney(&issues, "kiosk_queue.hardware_unit_price", hField+".unit_price", item.UnitPrice),
				Quantity:  int32(requiredKioskQueueInt(&issues, hField+".quantity", item.Quantity)),
			})
		}
	} else {
		// Client supplied hardware or private food court must have no hardware items
		input.HardwareItems = nil
	}

	// Maintenance items (Private Food Court)
	if variant == domain.KioskQueuePrivateFoodCourt {
		for mIdx, item := range serviceInput.MaintenanceItems {
			mField := fmt.Sprintf("maintenance_items[%d]", mIdx)
			input.MaintenanceItems = append(input.MaintenanceItems, domain.KioskQueueServiceItemInput{
				Name:           requiredKioskQueueText(&issues, mField+".name", item.Name),
				MonthlyRate:    requiredKioskQueueMoney(&issues, "kiosk_queue.maintenance_monthly_rate", mField+".monthly_rate", item.MonthlyRate),
				Headcount:      int32(requiredKioskQueueInt(&issues, mField+".headcount", item.Headcount)),
				PeriodQuarters: int32(requiredKioskQueueInt(&issues, mField+".period_quarters", item.PeriodQuarters)),
			})
		}

		if len(serviceInput.PrivateTimelineRows) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.private_timeline_rows.missing", "private_timeline_rows", "private timeline rows are required"))
		}
		for tIdx, row := range serviceInput.PrivateTimelineRows {
			tField := fmt.Sprintf("private_timeline_rows[%d]", tIdx)
			input.PrivateTimelineRows = append(input.PrivateTimelineRows, domain.KioskQueueTimelineRowInput{
				Description: requiredKioskQueueText(&issues, tField+".description", row.Description),
				Timeline:    requiredKioskQueueText(&issues, tField+".timeline", row.Timeline),
			})
		}

		if len(serviceInput.PrivatePaymentSchedule) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.private_payment_schedule.missing", "private_payment_schedule", "private payment schedule is required"))
		}
		for pIdx, row := range serviceInput.PrivatePaymentSchedule {
			pField := fmt.Sprintf("private_payment_schedule[%d]", pIdx)
			input.PrivatePaymentSchedule = append(input.PrivatePaymentSchedule, domain.KioskQueuePrivatePaymentInput{
				WorkPeriod:       requiredKioskQueueText(&issues, pField+".work_period", row.WorkPeriod),
				Terms:            requiredKioskQueueText(&issues, pField+".terms", row.Terms),
				InvoiceDateLabel: requiredKioskQueueText(&issues, pField+".invoice_date_label", row.InvoiceDateLabel),
				DueDateLabel:     requiredKioskQueueText(&issues, pField+".due_date_label", row.DueDateLabel),
				Nominal:          requiredKioskQueueMoney(&issues, "kiosk_queue.private_payment_nominal", pField+".nominal", row.Nominal),
			})
		}
		input.PaymentDueDays = 0
	} else {
		// Government MPP Payment Schedule
		if len(serviceInput.GovernmentPaymentSchedule) == 0 {
			issues = append(issues, kioskQueueIssue("kiosk_queue.government_payment_schedule.missing", "government_payment_schedule", "government payment schedule is required"))
		}
		for pIdx, p := range serviceInput.GovernmentPaymentSchedule {
			pField := fmt.Sprintf("government_payment_schedule[%d]", pIdx)
			input.GovernmentPaymentSchedule = append(input.GovernmentPaymentSchedule, domain.KioskQueueGovernmentPaymentInput{
				Stage:        requiredKioskQueueText(&issues, pField+".stage", p.Stage),
				BillingBasis: requiredKioskQueueText(&issues, pField+".billing_basis", p.BillingBasis),
				Nominal:      requiredKioskQueueMoney(&issues, "kiosk_queue.government_payment_nominal", pField+".nominal", p.Nominal),
			})
		}
		if serviceInput.PaymentDueDays != nil {
			input.PaymentDueDays = int32(serviceInput.PaymentDueDays.Value)
		} else {
			input.PaymentDueDays = 14
		}
	}

	// Limitations and rights
	if len(serviceInput.LimitationCodes) == 0 {
		issues = append(issues, kioskQueueIssue("kiosk_queue.limitation_codes.missing", "limitation_codes", "limitation codes are required"))
	}
	for lIdx, code := range serviceInput.LimitationCodes {
		input.LimitationCodes = append(input.LimitationCodes, domain.KioskQueueLimitationCode(requiredKioskQueueText(&issues, fmt.Sprintf("limitation_codes[%d]", lIdx), code)))
	}
	for aIdx, add := range serviceInput.AdditionalLimitations {
		input.AdditionalLimitations = append(input.AdditionalLimitations, requiredKioskQueueText(&issues, fmt.Sprintf("additional_limitations[%d]", aIdx), add))
	}
	for rIdx, stmt := range serviceInput.RightsStatements {
		input.RightsStatements = append(input.RightsStatements, requiredKioskQueueText(&issues, fmt.Sprintf("rights_statements[%d]", rIdx), stmt))
	}

	return input, issues
}

// CalculatePromptKioskQueue validates input, applies any requested additional discount BPS,
// executes the domain calculator, resolves profile, and maps domain errors to blocking draft issues.
func CalculatePromptKioskQueue(state *domain.PromptQuotationState, quotationDate time.Time) (domain.KioskQueueSystemInput, domain.KioskQueueCalculation, domain.ResolvedKioskQueueProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptKioskQueueInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.KioskQueueCalculation{}, domain.ResolvedKioskQueueProfile{}, issues, nil
	}

	// Apply discount from state.AdditionalDiscountBPS after initial price subtotal calculation
	if state.AdditionalDiscountBPS != nil && *state.AdditionalDiscountBPS > 0 {
		var serviceSubtotal int64
		for _, group := range input.ServiceGroups {
			for _, item := range group.Items {
				lineTotal, _ := calculateQuarterLine(item.MonthlyRate, item.Headcount, item.PeriodQuarters)
				serviceSubtotal += lineTotal
			}
		}
		if serviceSubtotal > 0 {
			discount := (serviceSubtotal * int64(*state.AdditionalDiscountBPS)) / 10000
			input.ServiceDiscountAmount = discount
			input.ShowServiceDiscountRow = true
		}
	}

	if err := input.Validate(); err != nil {
		issues = append(issues, kioskQueueIssue("kiosk_queue.input.invalid", "service_input", err.Error()))
		return input, domain.KioskQueueCalculation{}, domain.ResolvedKioskQueueProfile{}, issues, nil
	}

	calculation, err := CalculateKioskQueueSystem(input)
	if err != nil {
		code, field := "kiosk_queue.calculation.invalid", "calculation"
		if strings.Contains(err.Error(), "payment") || strings.Contains(err.Error(), "payment schedule") {
			code, field = "kiosk_queue.payment.mismatch", "payment_schedule"
		} else if strings.Contains(err.Error(), "integer rupiah") || strings.Contains(err.Error(), "fractional rupiah") {
			code, field = "kiosk_queue.calculation.fractional_rupiah", "calculation"
		}
		issues = append(issues, kioskQueueIssue(code, field, err.Error()))
		return input, domain.KioskQueueCalculation{}, domain.ResolvedKioskQueueProfile{}, issues, nil
	}

	profile, err := ResolveKioskQueueProfile(input)
	if err != nil {
		issues = append(issues, kioskQueueIssue("kiosk_queue.profile.invalid", "profile", err.Error()))
		return input, domain.KioskQueueCalculation{}, domain.ResolvedKioskQueueProfile{}, issues, nil
	}

	return input, calculation, profile, issues, nil
}

func (a *PromptKioskQueueAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.KioskQueueSystemInput, domain.KioskQueueCalculation, domain.ResolvedKioskQueueProfile, []domain.DraftIssue, error) {
	return CalculatePromptKioskQueue(state, quotationDate)
}

func (a *PromptKioskQueueAdapter) Render(metadata QuotationMetadata, snapshot PromptKioskQueueSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("kiosk queue renderer is unavailable")
	}
	return a.renderer.Render(KioskQueueDocumentInput{
		Metadata:    metadata,
		Input:       snapshot.Input,
		Calculation: snapshot.Calculation,
		Profile:     snapshot.Profile,
		LegalHTML:   legalHTML,
	})
}

func kioskQueueIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func requiredKioskQueueText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateKioskQueueFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, kioskQueueIssue("kiosk_queue."+field+".missing", field, "value is required"))
	}
	return value
}

func optionalKioskQueueText(issues *[]domain.DraftIssue, field string, fact *domain.Fact[string]) string {
	if fact == nil {
		return ""
	}
	validateKioskQueueFact(issues, field, fact.Source, fact.Evidence)
	return strings.TrimSpace(fact.Value)
}

func requiredKioskQueueInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateKioskQueueFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, kioskQueueIssue("kiosk_queue."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func validateKioskQueueFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, kioskQueueIssue("kiosk_queue."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, kioskQueueIssue("kiosk_queue."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredKioskQueueMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, kioskQueueIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, kioskQueueIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, kioskQueueIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, kioskQueueIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

// Extraction omits amounts. Keep an omitted amount explicitly unknown so the adapter
// emits a blocking issue instead of accepting zero or treating the model as the source of a price.
func normalizeUnknownKioskQueueMoney(input *PromptKioskQueueServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for gIdx := range input.ServiceGroups {
		for iIdx := range input.ServiceGroups[gIdx].Items {
			normalize(&input.ServiceGroups[gIdx].Items[iIdx].MonthlyRate)
		}
	}
	for hIdx := range input.HardwareItems {
		normalize(&input.HardwareItems[hIdx].UnitPrice)
	}
	for mIdx := range input.MaintenanceItems {
		normalize(&input.MaintenanceItems[mIdx].MonthlyRate)
	}
	for pIdx := range input.GovernmentPaymentSchedule {
		normalize(&input.GovernmentPaymentSchedule[pIdx].Nominal)
	}
	for pIdx := range input.PrivatePaymentSchedule {
		normalize(&input.PrivatePaymentSchedule[pIdx].Nominal)
	}
}
