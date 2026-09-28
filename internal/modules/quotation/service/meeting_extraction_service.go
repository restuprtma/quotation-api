package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

const (
	maxMeetingModelOutput        = 128 * 1024
	maxMeetingExtractionAttempts = 2
)

var (
	ErrMeetingExtractionInvalidInput  = errors.New("meeting extraction invalid input")
	ErrMeetingExtractionInvalidOutput = errors.New("meeting extraction invalid output")
	ErrMeetingExtractionProvider      = errors.New("meeting extraction provider failed")
	meetingExtractionSchema           = GetMeetingExtractionSchema
)

type MeetingExtractionError struct{ Reason string }

func (e *MeetingExtractionError) Error() string { return ErrMeetingExtractionInvalidOutput.Error() }

func (e *MeetingExtractionError) Unwrap() error { return ErrMeetingExtractionInvalidOutput }

// CatalogContext is the safe, non-commercial catalog projection sent to the
// model. Amounts, discount limits, and other pricing fields never enter it.
type CatalogContext struct {
	RateCardCode string
	RateCardName string
	Currency     string
	Items        []domain.RateCardItem
}

type meetingCatalogItem struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Seniority   string `json:"seniority"`
	Discipline  string `json:"discipline"`
	TermType    string `json:"term_type"`
}

type meetingCatalogPrompt struct {
	RateCardCode string               `json:"rate_card_code"`
	RateCardName string               `json:"rate_card_name,omitempty"`
	Currency     string               `json:"currency,omitempty"`
	Items        []meetingCatalogItem `json:"items"`
}

type MeetingExtractionService struct {
	providers ProviderSource
}

func NewMeetingExtractionService(providers ProviderSource) *MeetingExtractionService {
	return &MeetingExtractionService{providers: providers}
}

func (s *MeetingExtractionService) Extract(ctx context.Context, companyID uuid.UUID, req dto.MeetingExtractionRequest, catalog CatalogContext) (domain.PromptQuotationState, error) {
	if s == nil || s.providers == nil || companyID == uuid.Nil {
		return domain.PromptQuotationState{}, ErrMeetingExtractionInvalidInput
	}
	if strings.TrimSpace(catalog.RateCardCode) != domain.DefaultPromptRateCardCode {
		return domain.PromptQuotationState{}, ErrMeetingExtractionInvalidInput
	}
	notes := strings.TrimSpace(req.MeetingNotes)
	if notes == "" || len([]rune(notes)) > domain.MaxPromptMeetingNotes {
		return domain.PromptQuotationState{}, ErrMeetingExtractionInvalidInput
	}
	prompt, err := buildMeetingExtractionPrompt(notes, catalog)
	if err != nil {
		return domain.PromptQuotationState{}, ErrMeetingExtractionInvalidInput
	}
	provider, _, _, err := s.providers.ProviderForCompany(ctx, companyID)
	if err != nil {
		var generationErr *GenerationError
		if errors.As(err, &generationErr) {
			return domain.PromptQuotationState{}, generationErr
		}
		return domain.PromptQuotationState{}, newGenerationError(ErrCodeGenerationFailed, "AI provider is unavailable")
	}
	if provider == nil {
		return domain.PromptQuotationState{}, newGenerationError(ErrCodeGenerationFailed, "AI provider is unavailable")
	}

	var state *domain.PromptQuotationState
	attemptPrompt := prompt
	for attempt := 0; attempt < maxMeetingExtractionAttempts; attempt++ {
		result, generateErr := provider.Generate(ctx, GenerateRequest{
			Prompt: attemptPrompt, OutputMode: GenerateOutputJSON,
		})
		if generateErr != nil {
			var generationErr *GenerationError
			if errors.As(generateErr, &generationErr) {
				return domain.PromptQuotationState{}, generationErr
			}
			return domain.PromptQuotationState{}, mapProviderError(generateErr)
		}

		state, err = decodeMeetingExtraction(result.Text, notes)
		if err == nil {
			break
		}
		var extractionErr *MeetingExtractionError
		if !errors.As(err, &extractionErr) || attempt == maxMeetingExtractionAttempts-1 {
			return domain.PromptQuotationState{}, err
		}
		attemptPrompt = meetingExtractionRetryPrompt(prompt, extractionErr.Reason)
	}
	resolveCatalog(state, catalog)
	state.Issues = state.ComputeIssues()
	for i := range state.Issues {
		if state.Issues[i].Code == "team.rate_unresolved" {
			state.Issues[i].Code = "role_unresolved"
		}
	}
	state.Readiness = state.ComputeReadiness()
	if err := state.Validate(); err != nil {
		return domain.PromptQuotationState{}, &MeetingExtractionError{Reason: "state_validation"}
	}
	return *state, nil
}

func buildMeetingExtractionPrompt(notes string, catalog CatalogContext) (string, error) {
	items := make([]meetingCatalogItem, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		items = append(items, meetingCatalogItem{ID: item.ID.String(), Role: item.Role, DisplayName: item.DisplayName, Seniority: item.Seniority, Discipline: item.Discipline, TermType: item.TermType})
	}
	catalogJSON, err := json.Marshal(meetingCatalogPrompt{RateCardCode: catalog.RateCardCode, RateCardName: catalog.RateCardName, Currency: catalog.Currency, Items: items})
	if err != nil {
		return "", err
	}
	schema := strings.TrimSpace(meetingExtractionSchema())
	var schemaDocument map[string]json.RawMessage
	if schema == "" || json.Unmarshal([]byte(schema), &schemaDocument) != nil || schemaDocument == nil {
		return "", errors.New("meeting extraction schema unavailable")
	}
	prompt := GetMeetingExtractionPrompt()
	if prompt == "" {
		return "", errors.New("meeting extraction prompt unavailable")
	}
	prompt = strings.ReplaceAll(prompt, "{{OUTPUT_SCHEMA}}", schema)
	prompt = strings.ReplaceAll(prompt, "{{CATALOG_JSON}}", string(catalogJSON))
	prompt = strings.ReplaceAll(prompt, "{{MEETING_NOTES}}", notes)
	if strings.Contains(prompt, "{{") {
		return "", errors.New("meeting extraction prompt has unresolved placeholder")
	}
	return prompt, nil
}

func meetingExtractionRetryPrompt(prompt, reason string) string {
	if reason != "unsupported_evidence" {
		return prompt
	}
	return prompt + "\n\n<validation_correction>\n" +
		"The previous response was rejected because meeting-sourced evidence was not grounded in MEETING_NOTES. Return JSON only. " +
		"For each fact with source meeting, evidence must be a non-empty contiguous literal substring copied from MEETING_NOTES. " +
		"If unsupported, use source suggestion and omit evidence. Do not invent facts, prices, catalog IDs, or extra JSON fields.\n" +
		"</validation_correction>"
}

func decodeMeetingExtraction(raw, notes string) (*domain.PromptQuotationState, error) {
	invalid := func(reason string) (*domain.PromptQuotationState, error) {
		return nil, &MeetingExtractionError{Reason: reason}
	}
	if len(raw) > maxMeetingModelOutput {
		return invalid("malformed_json")
	}
	var envelope map[string]json.RawMessage
	envelopeDecoder := json.NewDecoder(strings.NewReader(raw))
	if err := envelopeDecoder.Decode(&envelope); err != nil {
		return invalid("malformed_json")
	}
	var envelopeExtra any
	if err := envelopeDecoder.Decode(&envelopeExtra); !errors.Is(err, io.EOF) {
		return invalid("trailing_content")
	}
	for _, key := range []string{"schema_version", "meeting_notes", "scope", "team", "issues", "readiness"} {
		value, ok := envelope[key]
		if !ok || string(value) == "null" {
			return invalid("missing_required_field")
		}
	}
	dec := json.NewDecoder(io.LimitReader(strings.NewReader(raw), maxMeetingModelOutput+1))
	dec.DisallowUnknownFields()
	var state domain.PromptQuotationState
	if err := dec.Decode(&state); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return invalid("unknown_field")
		}
		return invalid("malformed_json")
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalid("trailing_content")
	}
	state.MeetingNotes = notes
	normalizeState(&state)
	if state.SchemaVersion != domain.SchemaPromptQuotationV1 {
		return invalid("schema_version")
	}
	if !evidenceIsInNotes(&state, notes) {
		return invalid("unsupported_evidence")
	}
	if err := state.Validate(); err != nil {
		return invalid("state_validation")
	}
	return &state, nil
}

func normalizeState(state *domain.PromptQuotationState) {
	trimFact := func(f *domain.Fact[string]) {
		if f != nil {
			f.Value = strings.TrimSpace(f.Value)
			f.Evidence = strings.TrimSpace(f.Evidence)
		}
	}
	trimFact(state.ClientName)
	trimFact(state.ClientCompany)
	trimFact(state.PICName)
	trimFact(state.ProjectName)
	trimFact(state.CommercialScheme)
	trimFact(state.PaymentPattern)
	for i := range state.Scope {
		trimFact(&state.Scope[i])
	}
	if state.DurationMonths != nil {
		state.DurationMonths.Evidence = strings.TrimSpace(state.DurationMonths.Evidence)
	}
	for i := range state.Team {
		// Catalog references are backend-owned provenance and must not affect
		// validation or readiness before the resolver runs.
		state.Team[i].ResolvedRateItemID = nil
		state.Team[i].RequestedRole = strings.TrimSpace(state.Team[i].RequestedRole)
		state.Team[i].Seniority = strings.TrimSpace(state.Team[i].Seniority)
		state.Team[i].Evidence = strings.TrimSpace(state.Team[i].Evidence)
	}
}

func normalizeEvidenceText(value string) string {
	var out strings.Builder
	spacePending := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '%' {
			if spacePending && out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteRune(r)
			spacePending = false
		} else {
			spacePending = out.Len() > 0
		}
	}
	return strings.TrimSpace(out.String())
}

func normalizedEvidenceIsInNotes(notes, evidence string) bool {
	normalizedEvidence := normalizeEvidenceText(evidence)
	if normalizedEvidence == "" {
		return false
	}
	return strings.Contains(" "+normalizeEvidenceText(notes)+" ", " "+normalizedEvidence+" ")
}

func evidenceIsInNotes(state *domain.PromptQuotationState, notes string) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return source != domain.SourceMeeting || normalizedEvidenceIsInNotes(notes, evidence)
	}
	for _, f := range []*domain.Fact[string]{state.ClientName, state.ClientCompany, state.PICName, state.ProjectName, state.CommercialScheme, state.PaymentPattern} {
		if f != nil && !check(f.Source, f.Evidence) {
			return false
		}
	}
	for i := range state.Scope {
		if !check(state.Scope[i].Source, state.Scope[i].Evidence) {
			return false
		}
	}
	if state.DurationMonths != nil && !check(state.DurationMonths.Source, state.DurationMonths.Evidence) {
		return false
	}
	for _, line := range state.Team {
		if !check(line.Source, line.Evidence) {
			return false
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeSubscriptionSaaS {
		input, err := DecodePromptSubscriptionSaaSServiceInput(state.ServiceInput)
		if err != nil || !subscriptionFactsAreInNotes(input, check) {
			return false
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeWhiteLabel {
		input, err := DecodePromptWhiteLabelServiceInput(state.ServiceInput)
		if err != nil || !whiteLabelFactsAreInNotes(input, check) {
			return false
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeStandaloneMaintenance {
		input, err := DecodePromptStandaloneMaintenanceServiceInput(state.ServiceInput)
		if err != nil || !standaloneMaintenanceFactsAreInNotes(input, check) {
			return false
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeCustomDevelopment {
		input, err := DecodePromptCustomDevelopmentServiceInput(state.ServiceInput)
		if err != nil || !customDevelopmentFactsAreInNotes(input, check) {
			return false
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeTalentOutsourcing {
		input, err := DecodePromptTalentOutsourcingServiceInput(state.ServiceInput)
		if err != nil || !talentOutsourcingFactsAreInNotes(input, check) {
			return false
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeCloudInfrastructure {
		input, err := DecodePromptCloudInfrastructureServiceInput(state.ServiceInput)
		if err != nil || !cloudInfrastructureFactsAreInNotes(input, check) {
			return false
		}
	}
	return true
}

func talentOutsourcingFactsAreInNotes(in PromptTalentOutsourcingServiceInput, check func(domain.ValueSource, string) bool) bool {
	if !check(in.EngagementMode.Source, in.EngagementMode.Evidence) ||
		!check(in.DocumentNumber.Source, in.DocumentNumber.Evidence) ||
		!check(in.ServiceName.Source, in.ServiceName.Evidence) ||
		!check(in.TaxTreatment.Source, in.TaxTreatment.Evidence) ||
		!check(in.ShowPreparationDiscountRow.Source, in.ShowPreparationDiscountRow.Evidence) ||
		!check(in.PaymentDueDays.Source, in.PaymentDueDays.Evidence) {
		return false
	}
	for _, g := range in.CostGroups {
		if !check(g.Title.Source, g.Title.Evidence) {
			return false
		}
		for _, item := range g.Items {
			if !check(item.Name.Source, item.Name.Evidence) ||
				!check(item.Quantity.Source, item.Quantity.Evidence) ||
				!check(item.QuantityUnit.Source, item.QuantityUnit.Evidence) ||
				!check(item.PeriodCount.Source, item.PeriodCount.Evidence) {
				return false
			}
		}
	}
	for _, item := range in.PreparationItems {
		if !check(item.Name.Source, item.Name.Evidence) ||
			!check(item.Headcount.Source, item.Headcount.Evidence) {
			return false
		}
	}
	for _, item := range in.SquadItems {
		if !check(item.Role.Source, item.Role.Evidence) ||
			!check(item.Headcount.Source, item.Headcount.Evidence) ||
			!check(item.PeriodMonths.Source, item.PeriodMonths.Evidence) {
			return false
		}
	}
	for _, block := range in.ScopeBlocks {
		if !check(block.Title.Source, block.Title.Evidence) ||
			!check(block.Description.Source, block.Description.Evidence) ||
			!check(block.Timeline.Source, block.Timeline.Evidence) {
			return false
		}
		for _, f := range block.Responsibilities {
			if !check(f.Source, f.Evidence) {
				return false
			}
		}
		for _, f := range block.Qualifications {
			if !check(f.Source, f.Evidence) {
				return false
			}
		}
		for _, f := range block.Deliverables {
			if !check(f.Source, f.Evidence) {
				return false
			}
		}
	}
	for _, row := range in.PaymentSchedule {
		if !check(row.WorkPeriod.Source, row.WorkPeriod.Evidence) ||
			!check(row.Terms.Source, row.Terms.Evidence) ||
			!check(row.InvoiceDateLabel.Source, row.InvoiceDateLabel.Evidence) ||
			!check(row.DueDateLabel.Source, row.DueDateLabel.Evidence) {
			return false
		}
	}
	for _, f := range in.AdditionalLimitations {
		if !check(f.Source, f.Evidence) {
			return false
		}
	}
	for _, f := range in.WarrantyStatements {
		if !check(f.Source, f.Evidence) {
			return false
		}
	}
	for _, f := range in.RightsStatements {
		if !check(f.Source, f.Evidence) {
			return false
		}
	}
	return true
}

func cloudInfrastructureFactsAreInNotes(in PromptCloudInfrastructureServiceInput, check func(domain.ValueSource, string) bool) bool {
	if !check(in.DocumentNumber.Source, in.DocumentNumber.Evidence) ||
		!check(in.ServiceName.Source, in.ServiceName.Evidence) ||
		!check(in.TaxTreatment.Source, in.TaxTreatment.Evidence) ||
		!check(in.PaymentDueDays.Source, in.PaymentDueDays.Evidence) {
		return false
	}
	for _, item := range in.PackageItems {
		if !check(item.Category.Source, item.Category.Evidence) ||
			!check(item.Name.Source, item.Name.Evidence) ||
			!check(item.VCPU.Source, item.VCPU.Evidence) ||
			!check(item.RAMGB.Source, item.RAMGB.Evidence) ||
			!check(item.StorageGB.Source, item.StorageGB.Evidence) {
			return false
		}
	}
	for _, item := range in.SetupItems {
		if !check(item.Name.Source, item.Name.Evidence) {
			return false
		}
	}
	for _, row := range in.ScopeRows {
		if !check(row.Timeline.Source, row.Timeline.Evidence) {
			return false
		}
		for _, s := range row.Sections {
			if !check(s.Title.Source, s.Title.Evidence) {
				return false
			}
			for _, a := range s.Activities {
				if !check(a.Source, a.Evidence) {
					return false
				}
			}
		}
	}
	for _, row := range in.PaymentSchedule {
		if !check(row.WorkPeriod.Source, row.WorkPeriod.Evidence) ||
			!check(row.Terms.Source, row.Terms.Evidence) ||
			!check(row.InvoiceDateLabel.Source, row.InvoiceDateLabel.Evidence) ||
			!check(row.DueDateLabel.Source, row.DueDateLabel.Evidence) {
			return false
		}
	}
	for _, c := range in.LimitationCodes {
		if !check(c.Source, c.Evidence) {
			return false
		}
	}
	for _, a := range in.AdditionalLimitations {
		if !check(a.Source, a.Evidence) {
			return false
		}
	}
	return true
}

func resolveCatalog(state *domain.PromptQuotationState, catalog CatalogContext) {
	code := strings.TrimSpace(catalog.RateCardCode)
	if code == "" {
		code = domain.DefaultPromptRateCardCode
	}
	if state.CommercialScheme != nil && (state.CommercialScheme.Value == domain.CommercialSchemeSubscriptionSaaS || state.CommercialScheme.Value == domain.CommercialSchemeWhiteLabel || state.CommercialScheme.Value == domain.CommercialSchemeStandaloneMaintenance || state.CommercialScheme.Value == domain.CommercialSchemeCustomDevelopment || state.CommercialScheme.Value == domain.CommercialSchemeTalentOutsourcing || state.CommercialScheme.Value == domain.CommercialSchemeKioskQueueSystem || state.CommercialScheme.Value == domain.CommercialSchemeCloudInfrastructure) {
		// Specialized services retain descriptive service input extracted from
		// the notes. Prices are intentionally absent until a catalog or BA
		// supplies them.
		return
	}
	provenance, _ := json.Marshal(struct {
		RateCardCode string `json:"rate_card_code"`
		Source       string `json:"source"`
	}{RateCardCode: code, Source: string(domain.SourceCatalog)})
	state.ServiceInput = provenance
	for i := range state.Team {
		// The model may describe a requested role, but it never gets to choose
		// or preserve a catalog item reference. Only this resolver can attach it.
		state.Team[i].ResolvedRateItemID = nil
		resolution := ResolveRole(state.Team[i].RequestedRole, state.Team[i].Seniority, catalog.Items)
		if resolution.Status == RoleResolved {
			id := resolution.Item.ID.String()
			state.Team[i].ResolvedRateItemID = &id
		}
	}
}

func subscriptionFactsAreInNotes(input PromptSubscriptionSaaSServiceInput, check func(domain.ValueSource, string) bool) bool {
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	boolean := func(f domain.Fact[bool]) bool { return check(f.Source, f.Evidence) }
	if !text(input.ProductName) || !text(input.SectionLayout) || !text(input.BillingCycle) || !text(input.TaxTreatment) || !integer(input.InitialTermMonths) || !boolean(input.AutoRenewal) || !text(input.RightsClauseCode) || !boolean(input.IsPartnership) {
		return false
	}
	for _, period := range input.PricePeriods {
		if !text(period.Label) {
			return false
		}
	}
	for _, item := range input.OneTimeItems {
		if !text(item.Name) {
			return false
		}
	}
	for _, payment := range input.PaymentSchedule {
		if !text(payment.WorkPeriod) || !text(payment.InvoiceDateLabel) || !text(payment.DueDateLabel) {
			return false
		}
	}
	for _, group := range input.FeatureGroups {
		if !text(group.Name) {
			return false
		}
		for _, feature := range group.Features {
			if !text(feature) {
				return false
			}
		}
	}
	if input.IncomeSharing != nil {
		sharing := input.IncomeSharing
		return integer(sharing.VenturoShareBPS) && integer(sharing.PartnerShareBPS) && text(sharing.EligibleRevenueDescription) && integer(sharing.SettlementBusinessDays) && boolean(sharing.ProhibitUnilateralDiscount)
	}
	return true
}

func whiteLabelFactsAreInNotes(input PromptWhiteLabelServiceInput, check func(domain.ValueSource, string) bool) bool {
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	if !text(input.ProductName) || !text(input.TaxTreatment) {
		return false
	}
	for _, item := range input.LicenseItems {
		if !text(item.Name) || !integer(item.Quantity) {
			return false
		}
	}
	for _, item := range input.MaintenanceItems {
		if !text(item.Name) || !integer(item.FTEQuarters) || !integer(item.PeriodMonths) {
			return false
		}
	}
	for _, item := range input.HostingItems {
		if !text(item.Name) || !integer(item.Quantity) || !integer(item.PeriodMonths) {
			return false
		}
	}
	for _, group := range input.ModuleGroups {
		if !text(group.Name) {
			return false
		}
		for _, feature := range group.Features {
			if !text(feature) {
				return false
			}
		}
	}
	for _, row := range input.TimelineRows {
		if !text(row.Description) || !text(row.Timeline) {
			return false
		}
	}
	for _, row := range input.PaymentSchedule {
		if !text(row.WorkPeriod) || !text(row.Terms) || !text(row.InvoiceDateLabel) || !text(row.DueDateLabel) {
			return false
		}
	}
	for _, statement := range input.RightsStatements {
		if !text(statement) {
			return false
		}
	}
	for _, code := range input.LimitationCodes {
		if !text(code) {
			return false
		}
	}
	return true
}

func standaloneMaintenanceFactsAreInNotes(input PromptStandaloneMaintenanceServiceInput, check func(domain.ValueSource, string) bool) bool {
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	if !text(input.SystemName) || !text(input.Mode) || !text(input.TaxTreatment) {
		return false
	}
	for _, item := range input.RetainerItems {
		if !text(item.Name) || !integer(item.FTEQuarters) || !integer(item.PeriodMonths) {
			return false
		}
	}
	for _, item := range input.HourlyCostItems {
		if !text(item.Name) || !integer(item.Headcount) || !integer(item.HoursHalfUnits) {
			return false
		}
	}
	for _, group := range input.DefectGroups {
		if !text(group.Title) {
			return false
		}
		for _, item := range group.Items {
			if !text(item.ItemNumber) || !text(item.Priority) || !text(item.Function) || !text(item.ScreenFlow) || !text(item.Existing) || !text(item.Improvement) {
				return false
			}
			if item.Reference != nil && !text(*item.Reference) {
				return false
			}
		}
	}
	for _, task := range input.HourlyTasks {
		if !text(task.Description) || !text(task.Platform) || !integer(task.HoursHalfUnits) {
			return false
		}
	}
	for _, row := range input.PaymentSchedule {
		if !text(row.WorkPeriod) || !text(row.Terms) || !text(row.InvoiceDateLabel) || !text(row.DueDateLabel) {
			return false
		}
	}
	for _, statement := range input.RightsStatements {
		if !text(statement) {
			return false
		}
	}
	return true
}

func customDevelopmentFactsAreInNotes(input PromptCustomDevelopmentServiceInput, check func(domain.ValueSource, string) bool) bool {
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	if !text(input.SystemName) || !text(input.DevelopmentTableMode) || !text(input.TaxTreatment) {
		return false
	}
	if input.DiscountLabel != nil && !text(*input.DiscountLabel) {
		return false
	}
	for _, group := range input.DevelopmentGroups {
		if !text(group.Title) {
			return false
		}
		for _, item := range group.Items {
			if !text(item.Name) || !text(item.PeriodLabel) || !integer(item.Headcount) {
				return false
			}
		}
	}
	for _, item := range input.MonthlyItems {
		if !text(item.Name) || !integer(item.Quantity) {
			return false
		}
		if item.Specification != nil && !text(*item.Specification) {
			return false
		}
	}
	for _, note := range input.MonthlyNotes {
		if !text(note) {
			return false
		}
	}
	for _, group := range input.ModuleGroups {
		if !text(group.Name) {
			return false
		}
		for _, feature := range group.Features {
			if !text(feature) {
				return false
			}
		}
	}
	for _, row := range input.TimelineRows {
		if !text(row.Description) || !text(row.Timeline) {
			return false
		}
	}
	for _, row := range input.PaymentSchedule {
		if !text(row.WorkPeriod) || !text(row.Terms) || !text(row.InvoiceDateLabel) || !text(row.DueDateLabel) {
			return false
		}
	}
	for _, statement := range input.RightsStatements {
		if !text(statement) {
			return false
		}
	}
	for _, code := range input.LimitationCodes {
		if !text(code) {
			return false
		}
	}
	for _, add := range input.AdditionalLimitations {
		if !text(add) {
			return false
		}
	}
	return true
}
