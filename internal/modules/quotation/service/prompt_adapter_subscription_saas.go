package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// PromptSubscriptionSaaSServiceInputKind identifies the versioned service
// payload carried in PromptQuotationState.ServiceInput. It holds extracted
// descriptive facts separately from amounts whose source must be trusted.
const PromptSubscriptionSaaSServiceInputKind = "subscription_saas_v1"

type PromptSubscriptionPricePeriod struct {
	Label            domain.Fact[string] `json:"label"`
	StartMonthOffset int32               `json:"start_month_offset"`
	DurationMonths   int32               `json:"duration_months"`
	MonthlyRate      domain.MoneyInput   `json:"monthly_rate"`
}

type PromptSubscriptionOneTimeItem struct {
	Name   domain.Fact[string] `json:"name"`
	Amount domain.MoneyInput   `json:"amount"`
}

type PromptSubscriptionPaymentRow struct {
	WorkPeriod       domain.Fact[string] `json:"work_period"`
	StartMonthOffset int32               `json:"start_month_offset"`
	CoveredMonths    int32               `json:"covered_months"`
	InvoiceDateLabel domain.Fact[string] `json:"invoice_date_label"`
	DueDateLabel     domain.Fact[string] `json:"due_date_label"`
	Nominal          domain.MoneyInput   `json:"nominal"`
}

type PromptSubscriptionFeatureGroup struct {
	Name     domain.Fact[string]   `json:"name"`
	Features []domain.Fact[string] `json:"features"`
}

type PromptSubscriptionIncomeSharing struct {
	VenturoShareBPS            domain.Fact[int]    `json:"venturo_share_bps"`
	PartnerShareBPS            domain.Fact[int]    `json:"partner_share_bps"`
	EligibleRevenueDescription domain.Fact[string] `json:"eligible_revenue_description"`
	SettlementBusinessDays     domain.Fact[int]    `json:"settlement_business_days"`
	ProhibitUnilateralDiscount domain.Fact[bool]   `json:"prohibit_unilateral_discount"`
}

// PromptSubscriptionSaaSServiceInput contains only facts and BA/catalog
// commercial inputs. The extraction model may populate descriptive facts, but
// the adapter refuses MoneyInput values that originate from a meeting.
type PromptSubscriptionSaaSServiceInput struct {
	Kind              string                           `json:"kind"`
	ProductName       domain.Fact[string]              `json:"product_name"`
	SectionLayout     domain.Fact[string]              `json:"section_layout"`
	BillingCycle      domain.Fact[string]              `json:"billing_cycle"`
	TaxTreatment      domain.Fact[string]              `json:"tax_treatment"`
	InitialTermMonths domain.Fact[int]                 `json:"initial_term_months"`
	AutoRenewal       domain.Fact[bool]                `json:"auto_renewal"`
	PricePeriods      []PromptSubscriptionPricePeriod  `json:"price_periods"`
	OneTimeItems      []PromptSubscriptionOneTimeItem  `json:"one_time_items,omitempty"`
	PaymentSchedule   []PromptSubscriptionPaymentRow   `json:"payment_schedule"`
	FeatureGroups     []PromptSubscriptionFeatureGroup `json:"feature_groups,omitempty"`
	RightsClauseCode  domain.Fact[string]              `json:"rights_clause_code"`
	IsPartnership     domain.Fact[bool]                `json:"is_partnership"`
	IncomeSharing     *PromptSubscriptionIncomeSharing `json:"income_sharing,omitempty"`
}

// PromptSubscriptionSaaSSnapshot is the immutable calculator/renderer input
// that a caller can persist alongside the normal quotation snapshot.
type PromptSubscriptionSaaSSnapshot struct {
	Input       domain.SubscriptionSaaSInput           `json:"input"`
	Calculation domain.SubscriptionSaaSCalculation     `json:"calculation"`
	Profile     domain.ResolvedSubscriptionSaaSProfile `json:"profile"`
}

type PromptSubscriptionSaaSAdapter struct {
	renderer SubscriptionSaaSHTMLRenderer
}

func NewPromptSubscriptionSaaSAdapter(renderer SubscriptionSaaSHTMLRenderer) *PromptSubscriptionSaaSAdapter {
	if renderer == nil {
		renderer = NewSubscriptionSaaSHTMLRenderer()
	}
	return &PromptSubscriptionSaaSAdapter{renderer: renderer}
}

func (a *PromptSubscriptionSaaSAdapter) Scheme() string {
	return domain.CommercialSchemeSubscriptionSaaS
}

// PromptServiceAdapterRegistry is deliberately additive. Existing dedicated
// quotation paths keep their own calculator contract while prompt-first SaaS
// callers can discover the service-specific adapter by scheme.
type PromptServiceAdapterRegistry struct {
	subscriptionSaaS      *PromptSubscriptionSaaSAdapter
	talentOutsourcing     *PromptTalentOutsourcingAdapter
	whiteLabel            *PromptWhiteLabelAdapter
	standaloneMaintenance *PromptStandaloneMaintenanceAdapter
	customDevelopment     *PromptCustomDevelopmentAdapter
	kioskQueue            *PromptKioskQueueAdapter
	cloudInfrastructure   *PromptCloudInfrastructureAdapter
}

func NewPromptServiceAdapterRegistry(
	subscriptionSaaS *PromptSubscriptionSaaSAdapter,
	talentOutsourcing *PromptTalentOutsourcingAdapter,
	whiteLabel *PromptWhiteLabelAdapter,
	standaloneMaintenance *PromptStandaloneMaintenanceAdapter,
	customDevelopment ...*PromptCustomDevelopmentAdapter,
) *PromptServiceAdapterRegistry {
	var cd *PromptCustomDevelopmentAdapter
	if len(customDevelopment) > 0 {
		cd = customDevelopment[0]
	}
	if subscriptionSaaS == nil {
		subscriptionSaaS = NewPromptSubscriptionSaaSAdapter(nil)
	}
	if talentOutsourcing == nil {
		talentOutsourcing = NewPromptTalentOutsourcingAdapter(nil)
	}
	if whiteLabel == nil {
		whiteLabel = NewPromptWhiteLabelAdapter(nil)
	}
	if standaloneMaintenance == nil {
		standaloneMaintenance = NewPromptStandaloneMaintenanceAdapter(nil)
	}
	if cd == nil {
		cd = NewPromptCustomDevelopmentAdapter(nil)
	}
	return &PromptServiceAdapterRegistry{
		subscriptionSaaS:      subscriptionSaaS,
		talentOutsourcing:     talentOutsourcing,
		whiteLabel:            whiteLabel,
		standaloneMaintenance: standaloneMaintenance,
		customDevelopment:     cd,
		kioskQueue:            NewPromptKioskQueueAdapter(nil),
		cloudInfrastructure:   NewPromptCloudInfrastructureAdapter(nil),
	}
}

func (r *PromptServiceAdapterRegistry) SubscriptionSaaS() *PromptSubscriptionSaaSAdapter {
	if r == nil {
		return nil
	}
	return r.subscriptionSaaS
}

func (r *PromptServiceAdapterRegistry) TalentOutsourcing() *PromptTalentOutsourcingAdapter {
	if r == nil {
		return nil
	}
	return r.talentOutsourcing
}

func (r *PromptServiceAdapterRegistry) WhiteLabel() *PromptWhiteLabelAdapter {
	if r == nil {
		return nil
	}
	return r.whiteLabel
}

func (r *PromptServiceAdapterRegistry) StandaloneMaintenance() *PromptStandaloneMaintenanceAdapter {
	if r == nil {
		return nil
	}
	return r.standaloneMaintenance
}

func (r *PromptServiceAdapterRegistry) CustomDevelopment() *PromptCustomDevelopmentAdapter {
	if r == nil {
		return nil
	}
	return r.customDevelopment
}

func (r *PromptServiceAdapterRegistry) KioskQueue() *PromptKioskQueueAdapter {
	if r == nil {
		return nil
	}
	if r.kioskQueue == nil {
		r.kioskQueue = NewPromptKioskQueueAdapter(nil)
	}
	return r.kioskQueue
}

func (r *PromptServiceAdapterRegistry) CloudInfrastructure() *PromptCloudInfrastructureAdapter {
	if r == nil {
		return nil
	}
	if r.cloudInfrastructure == nil {
		r.cloudInfrastructure = NewPromptCloudInfrastructureAdapter(nil)
	}
	return r.cloudInfrastructure
}

func DecodePromptSubscriptionSaaSServiceInput(raw json.RawMessage) (PromptSubscriptionSaaSServiceInput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PromptSubscriptionSaaSServiceInput{}, fmt.Errorf("service_input: is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input PromptSubscriptionSaaSServiceInput
	if err := decoder.Decode(&input); err != nil {
		return PromptSubscriptionSaaSServiceInput{}, fmt.Errorf("service_input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return PromptSubscriptionSaaSServiceInput{}, fmt.Errorf("service_input: must contain one JSON object")
	}
	if input.Kind != PromptSubscriptionSaaSServiceInputKind {
		return PromptSubscriptionSaaSServiceInput{}, fmt.Errorf("service_input.kind: must be %q", PromptSubscriptionSaaSServiceInputKind)
	}
	normalizeUnknownSubscriptionMoney(&input)
	return input, nil
}

func EncodePromptSubscriptionSaaSServiceInput(input PromptSubscriptionSaaSServiceInput) ([]byte, error) {
	if input.Kind != PromptSubscriptionSaaSServiceInputKind {
		return nil, fmt.Errorf("service_input.kind: must be %q", PromptSubscriptionSaaSServiceInputKind)
	}
	return json.Marshal(input)
}

func BuildPromptSubscriptionSaaSInput(state *domain.PromptQuotationState, quotationDate time.Time) (domain.SubscriptionSaaSInput, []domain.DraftIssue) {
	issues := make([]domain.DraftIssue, 0, 8)
	if state == nil {
		return domain.SubscriptionSaaSInput{}, append(issues, subscriptionIssue("subscription.state.missing", "state", "prompt state is required"))
	}
	if state.SchemaVersion != domain.SchemaPromptQuotationV1 {
		return domain.SubscriptionSaaSInput{}, append(issues, subscriptionIssue("subscription.state.invalid", "schema_version", "prompt state schema is invalid"))
	}
	if state.CommercialScheme == nil || state.CommercialScheme.Value != domain.CommercialSchemeSubscriptionSaaS {
		return domain.SubscriptionSaaSInput{}, append(issues, subscriptionIssue("subscription.scheme.invalid", "commercial_scheme", "subscription SaaS adapter requires the subscription_saas scheme"))
	}
	serviceInput, err := DecodePromptSubscriptionSaaSServiceInput(state.ServiceInput)
	if err != nil {
		return domain.SubscriptionSaaSInput{}, append(issues, subscriptionIssue("subscription.service_input.invalid", "service_input", err.Error()))
	}
	if quotationDate.IsZero() {
		issues = append(issues, subscriptionIssue("subscription.quotation_date.missing", "quotation_date", "quotation date is required"))
	}

	input := domain.SubscriptionSaaSInput{
		QuotationDate:     quotationDate,
		ProductName:       requiredSubscriptionText(&issues, "product_name", serviceInput.ProductName),
		SectionLayout:     domain.SubscriptionSectionLayout(requiredSubscriptionText(&issues, "section_layout", serviceInput.SectionLayout)),
		BillingCycle:      domain.SubscriptionBillingCycle(requiredSubscriptionText(&issues, "billing_cycle", serviceInput.BillingCycle)),
		TaxTreatment:      domain.SubscriptionTaxTreatment(requiredSubscriptionText(&issues, "tax_treatment", serviceInput.TaxTreatment)),
		InitialTermMonths: int32(requiredSubscriptionInt(&issues, "initial_term_months", serviceInput.InitialTermMonths)),
		AutoRenewal:       serviceInput.AutoRenewal.Value,
		RightsClauseCode:  optionalSubscriptionText(&issues, "rights_clause_code", serviceInput.RightsClauseCode),
		IsPartnership:     serviceInput.IsPartnership.Value,
	}
	validateSubscriptionFact(&issues, "auto_renewal", serviceInput.AutoRenewal.Source, serviceInput.AutoRenewal.Evidence)
	validateSubscriptionFact(&issues, "is_partnership", serviceInput.IsPartnership.Source, serviceInput.IsPartnership.Evidence)

	for index, period := range serviceInput.PricePeriods {
		field := fmt.Sprintf("price_periods[%d]", index)
		input.PricePeriods = append(input.PricePeriods, domain.SubscriptionPricePeriodInput{
			Label: requiredSubscriptionText(&issues, field+".label", period.Label), StartMonthOffset: period.StartMonthOffset, DurationMonths: period.DurationMonths,
			MonthlyRate: requiredSubscriptionMoney(&issues, "subscription.monthly_rate", field+".monthly_rate", period.MonthlyRate),
		})
	}
	for index, item := range serviceInput.OneTimeItems {
		field := fmt.Sprintf("one_time_items[%d]", index)
		input.OneTimeItems = append(input.OneTimeItems, domain.SubscriptionOneTimeItemInput{Name: requiredSubscriptionText(&issues, field+".name", item.Name), Amount: requiredSubscriptionMoney(&issues, "subscription.one_time_amount", field+".amount", item.Amount)})
	}
	for index, row := range serviceInput.PaymentSchedule {
		field := fmt.Sprintf("payment_schedule[%d]", index)
		input.PaymentSchedule = append(input.PaymentSchedule, domain.SubscriptionPaymentRowInput{
			WorkPeriod: requiredSubscriptionText(&issues, field+".work_period", row.WorkPeriod), StartMonthOffset: row.StartMonthOffset, CoveredMonths: row.CoveredMonths,
			InvoiceDateLabel: requiredSubscriptionText(&issues, field+".invoice_date_label", row.InvoiceDateLabel), DueDateLabel: requiredSubscriptionText(&issues, field+".due_date_label", row.DueDateLabel),
			Nominal: requiredSubscriptionMoney(&issues, "subscription.payment_nominal", field+".nominal", row.Nominal),
		})
	}
	for index, group := range serviceInput.FeatureGroups {
		mapped := domain.SubscriptionFeatureGroupInput{Name: requiredSubscriptionText(&issues, fmt.Sprintf("feature_groups[%d].name", index), group.Name)}
		for featureIndex, feature := range group.Features {
			mapped.Features = append(mapped.Features, requiredSubscriptionText(&issues, fmt.Sprintf("feature_groups[%d].features[%d]", index, featureIndex), feature))
		}
		input.FeatureGroups = append(input.FeatureGroups, mapped)
	}
	if serviceInput.IncomeSharing != nil {
		sharing := serviceInput.IncomeSharing
		input.IncomeSharing = &domain.IncomeSharingInput{
			VenturoShareBPS:            int32(requiredSubscriptionInt(&issues, "income_sharing.venturo_share_bps", sharing.VenturoShareBPS)),
			PartnerShareBPS:            int32(requiredSubscriptionInt(&issues, "income_sharing.partner_share_bps", sharing.PartnerShareBPS)),
			EligibleRevenueDescription: requiredSubscriptionText(&issues, "income_sharing.eligible_revenue_description", sharing.EligibleRevenueDescription),
			SettlementBusinessDays:     int32(requiredSubscriptionInt(&issues, "income_sharing.settlement_business_days", sharing.SettlementBusinessDays)),
			ProhibitUnilateralDiscount: sharing.ProhibitUnilateralDiscount.Value,
		}
		validateSubscriptionFact(&issues, "income_sharing.prohibit_unilateral_discount", sharing.ProhibitUnilateralDiscount.Source, sharing.ProhibitUnilateralDiscount.Evidence)
	}
	return input, issues
}

// CalculatePromptSubscriptionSaaS is the prompt-first orchestration gate.
// Blocking source/completeness defects always return a zero calculation; the
// domain calculator remains the only source of a total and payment check.
func CalculatePromptSubscriptionSaaS(state *domain.PromptQuotationState, quotationDate time.Time) (domain.SubscriptionSaaSInput, domain.SubscriptionSaaSCalculation, domain.ResolvedSubscriptionSaaSProfile, []domain.DraftIssue, error) {
	input, issues := BuildPromptSubscriptionSaaSInput(state, quotationDate)
	if hasBlockingIssue(issues) {
		return input, domain.SubscriptionSaaSCalculation{}, domain.ResolvedSubscriptionSaaSProfile{}, issues, nil
	}
	if err := input.Validate(); err != nil {
		issues = append(issues, subscriptionIssue("subscription.input.invalid", "service_input", err.Error()))
		return input, domain.SubscriptionSaaSCalculation{}, domain.ResolvedSubscriptionSaaSProfile{}, issues, nil
	}
	calculation, err := CalculateSubscriptionSaaS(input)
	if err != nil {
		code, field := "subscription.calculation.invalid", "calculation"
		if strings.Contains(err.Error(), "payment_schedule: nominal total must equal total contract") {
			code, field = "subscription.payment.total_mismatch", "payment_schedule"
		}
		issues = append(issues, subscriptionIssue(code, field, err.Error()))
		return input, domain.SubscriptionSaaSCalculation{}, domain.ResolvedSubscriptionSaaSProfile{}, issues, nil
	}
	profile, err := ResolveSubscriptionSaaSProfile(input)
	if err != nil {
		return input, domain.SubscriptionSaaSCalculation{}, domain.ResolvedSubscriptionSaaSProfile{}, issues, err
	}
	return input, calculation, profile, issues, nil
}

func (a *PromptSubscriptionSaaSAdapter) Calculate(state *domain.PromptQuotationState, quotationDate time.Time) (domain.SubscriptionSaaSInput, domain.SubscriptionSaaSCalculation, domain.ResolvedSubscriptionSaaSProfile, []domain.DraftIssue, error) {
	return CalculatePromptSubscriptionSaaS(state, quotationDate)
}

func (a *PromptSubscriptionSaaSAdapter) Render(metadata QuotationMetadata, snapshot PromptSubscriptionSaaSSnapshot, legalHTML string) (string, error) {
	if a == nil || a.renderer == nil {
		return "", fmt.Errorf("subscription SaaS renderer is unavailable")
	}
	return a.renderer.Render(SubscriptionSaaSDocumentInput{Metadata: metadata, Input: snapshot.Input, Calculation: snapshot.Calculation, Profile: snapshot.Profile, FeatureGroups: snapshot.Input.FeatureGroups, LegalHTML: legalHTML})
}

func subscriptionIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func requiredSubscriptionText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateSubscriptionFact(issues, field, fact.Source, fact.Evidence)
	value := strings.TrimSpace(fact.Value)
	if value == "" {
		*issues = append(*issues, subscriptionIssue("subscription."+field+".missing", field, "value is required"))
	}
	return value
}

func optionalSubscriptionText(issues *[]domain.DraftIssue, field string, fact domain.Fact[string]) string {
	validateSubscriptionFact(issues, field, fact.Source, fact.Evidence)
	return strings.TrimSpace(fact.Value)
}

func requiredSubscriptionInt(issues *[]domain.DraftIssue, field string, fact domain.Fact[int]) int {
	validateSubscriptionFact(issues, field, fact.Source, fact.Evidence)
	if fact.Value <= 0 {
		*issues = append(*issues, subscriptionIssue("subscription."+field+".missing", field, "value must be greater than zero"))
	}
	return fact.Value
}

func validateSubscriptionFact(issues *[]domain.DraftIssue, field string, source domain.ValueSource, evidence string) {
	switch source {
	case domain.SourceMeeting, domain.SourceCatalog, domain.SourceSuggestion, domain.SourceUser:
	default:
		*issues = append(*issues, subscriptionIssue("subscription."+field+".source_invalid", field, "value source is invalid"))
		return
	}
	if source == domain.SourceMeeting && strings.TrimSpace(evidence) == "" {
		*issues = append(*issues, subscriptionIssue("subscription."+field+".evidence_missing", field, "meeting-sourced value requires evidence"))
	}
}

func requiredSubscriptionMoney(issues *[]domain.DraftIssue, code, field string, money domain.MoneyInput) int64 {
	if money.Amount == nil {
		*issues = append(*issues, subscriptionIssue(code+".unavailable", field, "price is not available"))
		return 0
	}
	if money.Source != domain.SourceCatalog && money.Source != domain.SourceUser {
		*issues = append(*issues, subscriptionIssue(code+".untrusted_source", field, "price must come from a catalog or explicit user input"))
		return 0
	}
	if err := money.Validate(); err != nil {
		*issues = append(*issues, subscriptionIssue(code+".invalid", field, err.Error()))
		return 0
	}
	if *money.Amount <= 0 {
		*issues = append(*issues, subscriptionIssue(code+".invalid", field, "price must be greater than zero"))
		return 0
	}
	return *money.Amount
}

// Extraction intentionally omits prices. Keep an omitted amount explicitly
// unknown so the adapter emits the unavailable issue rather than accepting a
// zero or treating the model as the source of a price.
func normalizeUnknownSubscriptionMoney(input *PromptSubscriptionSaaSServiceInput) {
	normalize := func(money *domain.MoneyInput) {
		if money.Amount == nil && money.Source == "" {
			money.Source = domain.SourceSuggestion
		}
	}
	for index := range input.PricePeriods {
		normalize(&input.PricePeriods[index].MonthlyRate)
	}
	for index := range input.OneTimeItems {
		normalize(&input.OneTimeItems[index].Amount)
	}
	for index := range input.PaymentSchedule {
		normalize(&input.PaymentSchedule[index].Nominal)
	}
}
