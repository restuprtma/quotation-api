package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/pkg/logger"
)

const (
	maxMeetingModelOutput        = 128 * 1024
	maxMeetingExtractionAttempts = 3
)

var (
	ErrMeetingExtractionInvalidInput  = errors.New("meeting extraction invalid input")
	ErrMeetingExtractionInvalidOutput = errors.New("meeting extraction invalid output")
	ErrMeetingExtractionProvider      = errors.New("meeting extraction provider failed")
	meetingExtractionSchema           = GetMeetingExtractionSchema
)

type MeetingExtractionError struct {
	Reason string
	Field  string
	Stage  string
	Offset int64
	Cause  error
}

// EvidenceViolation contains only an allow-listed, coarse field category.
// It must never carry raw model output or meeting-note content.
type EvidenceViolation struct{ Field string }

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

type meetingExtractionLogContextKey struct{}

type meetingExtractionLogContext struct {
	QuotationID string
	RevisionID  string
}

func withMeetingExtractionLogContext(ctx context.Context, quotationID, revisionID uuid.UUID) context.Context {
	return context.WithValue(ctx, meetingExtractionLogContextKey{}, meetingExtractionLogContext{
		QuotationID: quotationID.String(), RevisionID: revisionID.String(),
	})
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
	provider, providerName, model, err := s.providers.ProviderForCompany(ctx, companyID)
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
		if err := ctx.Err(); err != nil {
			return domain.PromptQuotationState{}, mapProviderError(err)
		}

		started := time.Now()
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

		if isSafetyFinishReason(result.FinishReason) {
			logger.Warn("meeting extraction provider safety refusal",
				logger.String("provider", providerName),
				logger.String("model", model),
				logger.String("finish_reason", result.FinishReason),
			)
			return domain.PromptQuotationState{}, newGenerationError(ErrCodeGenerationFailed, "AI provider rejected the request")
		}

		if isTruncatedFinishReason(result.FinishReason) {
			err = newMeetingExtractionError("malformed_json", "envelope_decode", errors.New("output truncated by token limit"))
		} else if strings.TrimSpace(result.Text) == "" {
			err = newMeetingExtractionError("malformed_json", "envelope_decode", io.EOF)
		} else {
			state, err = decodeMeetingExtraction(result.Text, notes)
		}

		if err == nil {
			break
		}
		var extractionErr *MeetingExtractionError
		if errors.As(err, &extractionErr) {
			logMeetingExtractionFailure(ctx, providerName, model, attempt+1, extractionErr, len(result.Text), time.Since(started))
		}
		if !errors.As(err, &extractionErr) || attempt == maxMeetingExtractionAttempts-1 {
			return domain.PromptQuotationState{}, err
		}
		attemptPrompt = meetingExtractionRetryPromptWithAttempt(prompt, extractionErr.Reason, extractionErr.Field, attempt+1, maxMeetingExtractionAttempts)
	}
	if state == nil {
		return domain.PromptQuotationState{}, ErrMeetingExtractionInvalidOutput
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
		extractionErr := newMeetingExtractionError("state_validation", "schema_validation", err)
		logMeetingExtractionFailure(ctx, providerName, model, maxMeetingExtractionAttempts, extractionErr, 0, 0)
		return domain.PromptQuotationState{}, extractionErr
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

func isSafetyFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "safety", "safety_violation", "recitation", "blocklist", "prohibited_content", "spii", "content_filter", "refusal":
		return true
	default:
		return false
	}
}

func isTruncatedFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "max_tokens", "length":
		return true
	default:
		return false
	}
}

func meetingExtractionRetryPrompt(prompt, reason, field string) string {
	return meetingExtractionRetryPromptWithAttempt(prompt, reason, field, 0, maxMeetingExtractionAttempts)
}

func meetingExtractionRetryPromptWithAttempt(prompt, reason, field string, attempt, maxAttempts int) string {
	var correction string
	switch reason {
	case "malformed_json":
		correction = "Return one complete JSON object matching OUTPUT_SCHEMA. " +
			"Do not use Markdown, prose, or comments. Match the declared JSON types; " +
			"numbers must not be quoted. Keep evidence short and copied from MEETING_NOTES. " +
			"Use empty arrays for unavailable collections and omit unavailable optional facts."
	case "trailing_content":
		correction = "Return exactly one JSON object matching OUTPUT_SCHEMA without trailing text, commentary, or multiple documents. Stop output immediately after closing the root JSON object."
	case "unknown_field":
		correction = "Return only fields declared in OUTPUT_SCHEMA. Do not include unknown or undeclared fields. Strictly adhere to the schema definitions."
	case "schema_version":
		correction = "The schema_version field must be set exactly to \"prompt_quotation_v1\". Return one complete JSON object matching OUTPUT_SCHEMA."
	case "state_validation":
		correction = "The previous JSON response failed state validation. Regenerate a complete JSON object matching OUTPUT_SCHEMA and domain constraints without inventing unsupported values."
	case "missing_required_field":
		correction = "The previous response was rejected because it has missing required fields. " +
			"Return all of these fields and never use null: schema_version, meeting_notes, scope, team, issues, readiness. " +
			"Use empty arrays [] for scope, team, or issues when no value is available."
	case "unsupported_evidence":
		correction = "The previous JSON response was rejected because the meeting evidence for " + field +
			" was not a literal substring of MEETING_NOTES. Regenerate the complete JSON object. " +
			"For every source meeting fact, copy evidence as a non-empty literal contiguous substring (a contiguous literal substring) from MEETING_NOTES; " +
			"do not paraphrase, summarize, translate, or combine separate bullets. Omit a fact that cannot be supported."
		if attempt >= maxAttempts-1 {
			correction += " Preserve only facts supported by a contiguous quotation from MEETING_NOTES. " +
				"Omit unsupported optional facts. Return empty arrays for unsupported collections. " +
				"Do not guess missing duration, headcount, role IDs, or monetary values. " +
				"Return a complete schema-valid object; the application determines readiness."
		}
	default:
		return prompt
	}
	return prompt + "\n\n<validation_correction>\n" +
		correction + " Return JSON only. Do not invent facts, prices, catalog IDs, or extra JSON fields.\n" +
		"</validation_correction>"
}

func decodeMeetingExtraction(raw, notes string) (*domain.PromptQuotationState, error) {
	invalid := func(reason, stage string, cause error) (*domain.PromptQuotationState, error) {
		return nil, newMeetingExtractionError(reason, stage, cause)
	}
	if len(raw) > maxMeetingModelOutput {
		return invalid("malformed_json", "output_size", nil)
	}
	var envelope map[string]json.RawMessage
	envelopeDecoder := json.NewDecoder(strings.NewReader(raw))
	if err := envelopeDecoder.Decode(&envelope); err != nil {
		return invalid("malformed_json", "envelope_decode", err)
	}
	if envelope == nil {
		return invalid("malformed_json", "envelope_decode", errors.New("JSON root must be an object"))
	}
	var envelopeExtra any
	if err := envelopeDecoder.Decode(&envelopeExtra); !errors.Is(err, io.EOF) {
		return invalid("trailing_content", "envelope_decode", err)
	}
	for _, key := range []string{"schema_version", "meeting_notes", "scope", "team", "issues", "readiness"} {
		value, ok := envelope[key]
		if !ok || string(value) == "null" {
			return invalid("missing_required_field", "schema_validation", nil)
		}
	}
	dec := json.NewDecoder(io.LimitReader(strings.NewReader(raw), maxMeetingModelOutput+1))
	dec.DisallowUnknownFields()
	var state domain.PromptQuotationState
	if err := dec.Decode(&state); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return invalid("unknown_field", "struct_decode", err)
		}
		return invalid("malformed_json", "struct_decode", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalid("trailing_content", "struct_decode", err)
	}
	state.MeetingNotes = notes
	normalizeState(&state)
	if state.SchemaVersion != domain.SchemaPromptQuotationV1 {
		return invalid("schema_version", "schema_validation", nil)
	}
	if err := validateServiceInputStructure(&state); err != nil {
		return invalid("state_validation", "schema_validation", err)
	}
	if violation := firstEvidenceViolation(&state, notes); violation != nil {
		return nil, &MeetingExtractionError{Reason: "unsupported_evidence", Field: violation.Field, Stage: "evidence_validation"}
	}
	if err := state.Validate(); err != nil {
		return invalid("state_validation", "schema_validation", err)
	}
	return &state, nil
}

func newMeetingExtractionError(reason, stage string, cause error) *MeetingExtractionError {
	extractionErr := &MeetingExtractionError{Reason: reason, Stage: stage, Cause: cause}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(cause, &syntaxErr):
		extractionErr.Offset = syntaxErr.Offset
	case errors.As(cause, &typeErr):
		extractionErr.Offset = typeErr.Offset
	}
	return extractionErr
}

func logMeetingExtractionFailure(ctx context.Context, providerName, model string, attempt int, extractionErr *MeetingExtractionError, outputBytes int, elapsed time.Duration) {
	metadata, _ := ctx.Value(meetingExtractionLogContextKey{}).(meetingExtractionLogContext)
	logger.Warn("meeting extraction validation failed",
		logger.String("quotation_id", metadata.QuotationID),
		logger.String("revision_id", metadata.RevisionID),
		logger.String("provider", providerName),
		logger.String("model", model),
		logger.Int("attempt", attempt),
		logger.String("stage", extractionErr.Stage),
		logger.String("reason", extractionErr.Reason),
		logger.Int64("offset", extractionErr.Offset),
		logger.Int("output_bytes", outputBytes),
		logger.Any("elapsed", elapsed),
	)
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

func firstEvidenceViolation(state *domain.PromptQuotationState, notes string) *EvidenceViolation {
	var violation *EvidenceViolation
	check := func(field string, source domain.ValueSource, evidence string) bool {
		if violation != nil || source != domain.SourceMeeting || normalizedEvidenceIsInNotes(notes, evidence) {
			return true
		}
		violation = &EvidenceViolation{Field: field}
		return false
	}
	for _, fact := range []struct {
		field string
		fact  *domain.Fact[string]
	}{
		{"client_company", state.ClientName},
		{"client_company", state.ClientCompany},
		{"client_company", state.PICName},
		{"project_name", state.ProjectName},
		{"commercial_scheme", state.CommercialScheme},
		{"payment_pattern", state.PaymentPattern},
	} {
		if fact.fact != nil && !check(fact.field, fact.fact.Source, fact.fact.Evidence) {
			return violation
		}
	}
	for i := range state.Scope {
		if !check("scope", state.Scope[i].Source, state.Scope[i].Evidence) {
			return violation
		}
	}
	if state.DurationMonths != nil && !check("duration_months", state.DurationMonths.Source, state.DurationMonths.Evidence) {
		return violation
	}
	for _, line := range state.Team {
		if !check("team", line.Source, line.Evidence) {
			return violation
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeSubscriptionSaaS {
		if input, err := DecodePromptSubscriptionSaaSServiceInput(state.ServiceInput); err == nil {
			if !subscriptionFactsAreInNotes(input, check) {
				return &EvidenceViolation{Field: "service_input"}
			}
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeWhiteLabel {
		if input, err := DecodePromptWhiteLabelServiceInput(state.ServiceInput); err == nil {
			if !whiteLabelFactsAreInNotes(input, check) {
				return &EvidenceViolation{Field: "service_input"}
			}
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeStandaloneMaintenance {
		if input, err := DecodePromptStandaloneMaintenanceServiceInput(state.ServiceInput); err == nil {
			if !standaloneMaintenanceFactsAreInNotes(input, check) {
				return &EvidenceViolation{Field: "service_input"}
			}
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeCustomDevelopment {
		if input, err := DecodePromptCustomDevelopmentServiceInput(state.ServiceInput); err == nil {
			if !customDevelopmentFactsAreInNotes(input, check) {
				return &EvidenceViolation{Field: "service_input"}
			}
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeTalentOutsourcing {
		if input, err := DecodePromptTalentOutsourcingServiceInput(state.ServiceInput); err == nil {
			if !talentOutsourcingFactsAreInNotes(input, check) {
				return &EvidenceViolation{Field: "service_input"}
			}
		}
	}
	if state.CommercialScheme != nil && state.CommercialScheme.Value == domain.CommercialSchemeCloudInfrastructure {
		if input, err := DecodePromptCloudInfrastructureServiceInput(state.ServiceInput); err == nil {
			if !cloudInfrastructureFactsAreInNotes(input, check) {
				return &EvidenceViolation{Field: "service_input"}
			}
		}
	}
	return violation
}

func validateServiceInputStructure(state *domain.PromptQuotationState) error {
	if state == nil || state.CommercialScheme == nil || len(bytes.TrimSpace(state.ServiceInput)) == 0 || string(bytes.TrimSpace(state.ServiceInput)) == "null" {
		return nil
	}
	switch state.CommercialScheme.Value {
	case domain.CommercialSchemeSubscriptionSaaS:
		_, err := DecodePromptSubscriptionSaaSServiceInput(state.ServiceInput)
		return err
	case domain.CommercialSchemeWhiteLabel:
		_, err := DecodePromptWhiteLabelServiceInput(state.ServiceInput)
		return err
	case domain.CommercialSchemeStandaloneMaintenance:
		_, err := DecodePromptStandaloneMaintenanceServiceInput(state.ServiceInput)
		return err
	case domain.CommercialSchemeCustomDevelopment:
		_, err := DecodePromptCustomDevelopmentServiceInput(state.ServiceInput)
		return err
	case domain.CommercialSchemeTalentOutsourcing:
		_, err := DecodePromptTalentOutsourcingServiceInput(state.ServiceInput)
		return err
	case domain.CommercialSchemeCloudInfrastructure:
		_, err := DecodePromptCloudInfrastructureServiceInput(state.ServiceInput)
		return err
	default:
		return nil
	}
}

func talentOutsourcingFactsAreInNotes(in PromptTalentOutsourcingServiceInput, evidenceCheck func(string, domain.ValueSource, string) bool) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return evidenceCheck("service_input", source, evidence)
	}
	if !check(in.ServiceName.Source, in.ServiceName.Evidence) {
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
	return true
}

func cloudInfrastructureFactsAreInNotes(in PromptCloudInfrastructureServiceInput, evidenceCheck func(string, domain.ValueSource, string) bool) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return evidenceCheck("service_input", source, evidence)
	}
	if !check(in.ServiceName.Source, in.ServiceName.Evidence) {
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
		} else if resolution.Status == RoleAmbiguous {
			if matched := DisambiguateRateCandidates(resolution.Candidates, state.Team[i].Seniority, state.Team[i].PeriodQuarters, state.DurationMonths); matched != nil {
				id := matched.ID.String()
				state.Team[i].ResolvedRateItemID = &id
			}
		}
	}
}

func subscriptionFactsAreInNotes(input PromptSubscriptionSaaSServiceInput, evidenceCheck func(string, domain.ValueSource, string) bool) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return evidenceCheck("service_input", source, evidence)
	}
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	if !text(input.ProductName) {
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
		return text(sharing.EligibleRevenueDescription)
	}
	return true
}

func whiteLabelFactsAreInNotes(input PromptWhiteLabelServiceInput, evidenceCheck func(string, domain.ValueSource, string) bool) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return evidenceCheck("service_input", source, evidence)
	}
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	if !text(input.ProductName) {
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
	return true
}

func standaloneMaintenanceFactsAreInNotes(input PromptStandaloneMaintenanceServiceInput, evidenceCheck func(string, domain.ValueSource, string) bool) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return evidenceCheck("service_input", source, evidence)
	}
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	if !text(input.SystemName) {
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
	return true
}

func customDevelopmentFactsAreInNotes(input PromptCustomDevelopmentServiceInput, evidenceCheck func(string, domain.ValueSource, string) bool) bool {
	check := func(source domain.ValueSource, evidence string) bool {
		return evidenceCheck("service_input", source, evidence)
	}
	text := func(f domain.Fact[string]) bool { return check(f.Source, f.Evidence) }
	integer := func(f domain.Fact[int]) bool { return check(f.Source, f.Evidence) }
	if !text(input.SystemName) {
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
	return true
}
