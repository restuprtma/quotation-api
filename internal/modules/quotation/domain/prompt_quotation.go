package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Prompt quotation state is the shared, wire-stable shape every prompt-first
// session reads and writes. It deliberately carries no money: rates, prices,
// subtotals, discounts, and totals stay server-authoritative and are produced
// by the calculator only once every blocking issue is resolved.

const (
	SchemaPromptQuotationV1   = "prompt_quotation_v1"
	DefaultPromptRateCardCode = "VENTURO-STD-2026"
)

const (
	ReadinessIncomplete    = "incomplete"
	ReadinessPriceable     = "priceable"
	ReadinessReadyForFinal = "ready_for_final"
)

const (
	MaxPromptHeadcountQuarters = 96
	MaxPromptPeriodQuarters    = 96
	MaxPromptScopeItems        = 64
	MaxPromptTeamLines         = 32
	MaxPromptMeetingNotes      = 16_384
)

type ValueSource string

const (
	SourceMeeting    ValueSource = "meeting"
	SourceCatalog    ValueSource = "catalog"
	SourceSuggestion ValueSource = "suggestion"
	SourceUser       ValueSource = "user"
)

func (source ValueSource) valid() bool {
	switch source {
	case SourceMeeting, SourceCatalog, SourceSuggestion, SourceUser:
		return true
	default:
		return false
	}
}

// Fact pairs one extracted value with where it came from. Facts from a meeting
// note always need evidence, so a BA can trace any draft sentence back to the
// note; suggestions and catalog values carry provenance instead.
type Fact[T any] struct {
	Value     T           `json:"value"`
	Source    ValueSource `json:"source"`
	Evidence  string      `json:"evidence,omitempty"`
	Confirmed bool        `json:"confirmed"`
}

type IssueSeverity string

const (
	IssueBlocking IssueSeverity = "blocking"
	IssueWarning  IssueSeverity = "warning"
)

type DraftIssue struct {
	Code     string        `json:"code"`
	Field    string        `json:"field"`
	Message  string        `json:"message"`
	Severity IssueSeverity `json:"severity"`
}

// ValidationError carries the DraftIssue that made a value unusable, so callers
// can surface the same blocking/warning shape the state emits.
type ValidationError struct {
	Issue DraftIssue
}

func (err *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", err.Issue.Field, err.Issue.Message)
}

// MoneyInput describes a price the backend still has to supply. A nil Amount
// means the price is not yet available and must stay absent, never guessed; a
// zero Amount is only legitimate where the existing domain explicitly records
// a zero or included value, and the Source field records that origin.
type MoneyInput struct {
	Amount     *int64      `json:"amount"`
	Source     ValueSource `json:"source"`
	CatalogRef *string     `json:"catalog_ref,omitempty"`
}

func (money MoneyInput) Validate() error {
	if !money.Source.valid() {
		return fmt.Errorf("source: unsupported value %q", money.Source)
	}
	if money.Amount == nil {
		return &ValidationError{Issue: DraftIssue{
			Code:     "money.amount_unavailable",
			Field:    "amount",
			Message:  "price is not yet available",
			Severity: IssueBlocking,
		}}
	}
	if *money.Amount < 0 {
		return &ValidationError{Issue: DraftIssue{
			Code:     "money.amount_negative",
			Field:    "amount",
			Message:  "must not be negative",
			Severity: IssueBlocking,
		}}
	}
	if money.Source == SourceCatalog && (money.CatalogRef == nil || strings.TrimSpace(*money.CatalogRef) == "") {
		return &ValidationError{Issue: DraftIssue{
			Code:     "money.catalog_ref_missing",
			Field:    "catalog_ref",
			Message:  "catalog prices must reference a rate card item",
			Severity: IssueBlocking,
		}}
	}
	return nil
}

// PromptTeamLine is one team ask from a meeting note. An unresolved line keeps
// ResolvedRateItemID nil: unknown is never represented as a zero or an empty
// rate, and no price is attached until the line matches a rate card item.
type PromptTeamLine struct {
	RequestedRole      string      `json:"requested_role"`
	ResolvedRateItemID *string     `json:"resolved_rate_item_id,omitempty"`
	Seniority          string      `json:"seniority"`
	HeadcountQuarters  int32       `json:"headcount_quarters"`
	PeriodQuarters     int32       `json:"period_quarters"`
	Source             ValueSource `json:"source"`
	Evidence           string      `json:"evidence,omitempty"`
	Confirmed          bool        `json:"confirmed"`
}

type PromptQuotationState struct {
	SchemaVersion         string           `json:"schema_version"`
	MeetingNotes          string           `json:"meeting_notes"`
	ClientName            *Fact[string]    `json:"client_name,omitempty"` // legacy recipient alias for old snapshots/renderers
	ClientCompany         *Fact[string]    `json:"client_company,omitempty"`
	PICName               *Fact[string]    `json:"pic_name,omitempty"`
	ProjectName           *Fact[string]    `json:"project_name,omitempty"`
	CommercialScheme      *Fact[string]    `json:"commercial_scheme,omitempty"`
	Scope                 []Fact[string]   `json:"scope"`
	DurationMonths        *Fact[int]       `json:"duration_months,omitempty"`
	Team                  []PromptTeamLine `json:"team"`
	PaymentPattern        *Fact[string]    `json:"payment_pattern,omitempty"`
	AdditionalDiscountBPS *int32           `json:"additional_discount_bps,omitempty"`
	ServiceInput          json.RawMessage  `json:"service_input,omitempty"`
	Issues                []DraftIssue     `json:"issues"`
	Readiness             string           `json:"readiness"`
}

// Validate reports the first structural defect in the state. It touches no
// database and no AI, and it never fills a missing commercial value: a state
// that is merely incomplete validates successfully and stays incomplete.
func (state *PromptQuotationState) Validate() error {
	if state == nil {
		return fmt.Errorf("prompt_quotation: state is nil")
	}
	if state.SchemaVersion != SchemaPromptQuotationV1 {
		return fmt.Errorf("schema_version: must be %q", SchemaPromptQuotationV1)
	}
	if state.Readiness != "" && !validReadiness(state.Readiness) {
		return fmt.Errorf("readiness: must be %s, %s, or %s", ReadinessIncomplete, ReadinessPriceable, ReadinessReadyForFinal)
	}
	notes := strings.TrimSpace(state.MeetingNotes)
	if notes == "" {
		return fmt.Errorf("meeting_notes: is required")
	}
	if len([]rune(notes)) > MaxPromptMeetingNotes {
		return fmt.Errorf("meeting_notes: must be at most %d characters", MaxPromptMeetingNotes)
	}
	if len(state.Scope) > MaxPromptScopeItems {
		return fmt.Errorf("scope: must contain at most %d items", MaxPromptScopeItems)
	}
	if len(state.Team) > MaxPromptTeamLines {
		return fmt.Errorf("team: must contain at most %d lines", MaxPromptTeamLines)
	}

	textFacts := []struct {
		name string
		fact *Fact[string]
	}{
		{"client_name", state.ClientName},
		{"client_company", state.ClientCompany},
		{"pic_name", state.PICName},
		{"project_name", state.ProjectName},
		{"commercial_scheme", state.CommercialScheme},
		{"payment_pattern", state.PaymentPattern},
	}
	for _, item := range textFacts {
		if item.fact == nil {
			continue
		}
		if err := validateTextFact(item.name, item.fact); err != nil {
			return err
		}
	}
	for index, item := range state.Scope {
		if err := validateTextFact(fmt.Sprintf("scope[%d]", index), &item); err != nil {
			return err
		}
	}
	if state.DurationMonths != nil {
		if err := validateFactSource("duration_months", state.DurationMonths.Source, state.DurationMonths.Evidence); err != nil {
			return err
		}
		if state.DurationMonths.Value <= 0 {
			return fmt.Errorf("duration_months.value: must be greater than zero")
		}
	}
	if state.AdditionalDiscountBPS != nil && (*state.AdditionalDiscountBPS < 0 || *state.AdditionalDiscountBPS > 10_000) {
		return fmt.Errorf("additional_discount_bps: must be between 0 and 10000")
	}
	for index, line := range state.Team {
		if err := validateTeamLine(index, line); err != nil {
			return err
		}
	}

	if state.Readiness != "" {
		if computed := state.ComputeReadiness(); computed != state.Readiness {
			return fmt.Errorf("readiness: must be %q, state declares %q", computed, state.Readiness)
		}
	}
	return nil
}

// ComputeIssues lists the business rules that still stop or limit the draft, in
// struct-field order so the paths stay deterministic. Blocking issues stop the
// calculator; warnings mark assumptions a BA has not confirmed yet.
func (state *PromptQuotationState) ComputeIssues() []DraftIssue {
	issues := make([]DraftIssue, 0, 12)

	if fact := state.ClientCompany; fact == nil {
		issues = append(issues, missingFact("client_company", "client company", true))
	} else if !fact.Confirmed {
		issues = append(issues, unconfirmedFact("client_company"))
	}
	if fact := state.PICName; fact == nil {
		issues = append(issues, missingFact("pic_name", "pic name", false))
	} else if !fact.Confirmed {
		issues = append(issues, unconfirmedFact("pic_name"))
	}
	if fact := state.ProjectName; fact == nil {
		issues = append(issues, missingFact("project_name", "project name", false))
	} else if !fact.Confirmed {
		issues = append(issues, unconfirmedFact("project_name"))
	}
	if fact := state.CommercialScheme; fact == nil {
		issues = append(issues, missingFact("commercial_scheme", "commercial scheme", true))
	} else if !fact.Confirmed {
		issues = append(issues, unconfirmedFact("commercial_scheme"))
	}
	if len(state.Scope) == 0 {
		issues = append(issues, DraftIssue{
			Code:     "scope.empty",
			Field:    "scope",
			Message:  "no scope item was extracted from the meeting note",
			Severity: IssueWarning,
		})
	}
	if !state.usesSpecializedServiceInput() {
		if fact := state.DurationMonths; fact == nil {
			issues = append(issues, missingFact("duration_months", "duration", true))
		} else if !fact.Confirmed {
			issues = append(issues, unconfirmedFact("duration_months"))
		}
		if len(state.Team) == 0 {
			issues = append(issues, DraftIssue{
				Code:     "team.empty",
				Field:    "team",
				Message:  "no team line was extracted from the meeting note",
				Severity: IssueBlocking,
			})
		}
		for index, line := range state.Team {
			field := fmt.Sprintf("team[%d].resolved_rate_item_id", index)
			switch {
			case line.ResolvedRateItemID == nil:
				issues = append(issues, DraftIssue{
					Code:     "team.rate_unresolved",
					Field:    field,
					Message:  fmt.Sprintf("requested role %q has no matching rate card item yet", line.RequestedRole),
					Severity: IssueBlocking,
				})
			case !line.Confirmed:
				issues = append(issues, DraftIssue{
					Code:     "team.rate_unconfirmed",
					Field:    field,
					Message:  "the resolved rate card item has not been confirmed",
					Severity: IssueWarning,
				})
			}
		}
		if fact := state.PaymentPattern; fact == nil {
			issues = append(issues, missingFact("payment_pattern", "payment pattern", false))
		} else if !fact.Confirmed {
			issues = append(issues, unconfirmedFact("payment_pattern"))
		}
	}

	return issues
}

// ComputeReadiness derives the gate from the issues: blocking issues mean the
// calculator cannot run, remaining warnings mean a priceable draft that is not
// final yet, and no issue at all means the quotation is ready for finalization.
func (state *PromptQuotationState) ComputeReadiness() string {
	issues := state.ComputeIssues()
	for _, issue := range issues {
		if issue.Severity == IssueBlocking {
			return ReadinessIncomplete
		}
	}
	if len(issues) > 0 {
		return ReadinessPriceable
	}
	return ReadinessReadyForFinal
}

func validReadiness(value string) bool {
	return value == ReadinessIncomplete || value == ReadinessPriceable || value == ReadinessReadyForFinal
}

func (state *PromptQuotationState) usesSpecializedServiceInput() bool {
	if state == nil || state.CommercialScheme == nil {
		return false
	}
	return state.CommercialScheme.Value == CommercialSchemeSubscriptionSaaS ||
		state.CommercialScheme.Value == CommercialSchemeWhiteLabel ||
		state.CommercialScheme.Value == CommercialSchemeStandaloneMaintenance ||
		state.CommercialScheme.Value == CommercialSchemeCustomDevelopment ||
		state.CommercialScheme.Value == CommercialSchemeKioskQueueSystem ||
		state.CommercialScheme.Value == CommercialSchemeTalentOutsourcing ||
		state.CommercialScheme.Value == CommercialSchemeCloudInfrastructure
}

func validateTextFact(field string, fact *Fact[string]) error {
	if err := validateFactSource(field, fact.Source, fact.Evidence); err != nil {
		return err
	}
	if strings.TrimSpace(fact.Value) == "" {
		return fmt.Errorf("%s.value: must not be empty", field)
	}
	return nil
}

func validateFactSource(field string, source ValueSource, evidence string) error {
	if !source.valid() {
		return fmt.Errorf("%s.source: unsupported value %q", field, source)
	}
	if source == SourceMeeting && strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("%s.evidence: is required when source is meeting", field)
	}
	return nil
}

func validateTeamLine(index int, line PromptTeamLine) error {
	prefix := fmt.Sprintf("team[%d]", index)
	if strings.TrimSpace(line.RequestedRole) == "" {
		return fmt.Errorf("%s.requested_role: is required", prefix)
	}
	if strings.TrimSpace(line.Seniority) == "" {
		return fmt.Errorf("%s.seniority: is required", prefix)
	}
	if line.HeadcountQuarters <= 0 {
		return fmt.Errorf("%s.headcount_quarters: must be greater than zero", prefix)
	}
	if line.HeadcountQuarters > MaxPromptHeadcountQuarters {
		return fmt.Errorf("%s.headcount_quarters: must be at most %d", prefix, MaxPromptHeadcountQuarters)
	}
	if line.PeriodQuarters <= 0 {
		return fmt.Errorf("%s.period_quarters: must be greater than zero", prefix)
	}
	if line.PeriodQuarters > MaxPromptPeriodQuarters {
		return fmt.Errorf("%s.period_quarters: must be at most %d", prefix, MaxPromptPeriodQuarters)
	}
	if line.ResolvedRateItemID != nil && strings.TrimSpace(*line.ResolvedRateItemID) == "" {
		return fmt.Errorf("%s.resolved_rate_item_id: must not be blank", prefix)
	}
	return validateFactSource(prefix, line.Source, line.Evidence)
}

func missingFact(field, label string, blocking bool) DraftIssue {
	severity := IssueWarning
	if blocking {
		severity = IssueBlocking
	}
	return DraftIssue{
		Code:     field + ".missing",
		Field:    field,
		Message:  fmt.Sprintf("%s is required and was not found in the meeting note", label),
		Severity: severity,
	}
}

func unconfirmedFact(field string) DraftIssue {
	return DraftIssue{
		Code:     field + ".unconfirmed",
		Field:    field,
		Message:  "value comes from the meeting note and has not been confirmed yet",
		Severity: IssueWarning,
	}
}
