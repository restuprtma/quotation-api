package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ErrMeetingExtractionP5Mapper is intentionally a safe, static error. The
// mapper never includes note text, provider payloads, or catalog prices in an
// error because its input may originate at the extraction boundary.
var ErrMeetingExtractionP5Mapper = errors.New("meeting extraction p5 mapper failed")

// MeetingExtractionSchemeMapper is the executable readiness contract for a
// pipeline scheme. A scheme is ready only when its mapper is registered here;
// callers must not maintain a second readiness allowlist.
type MeetingExtractionSchemeMapper func(MeetingExtractionP5MapperInput) (MeetingExtractionP5MapperResult, error)

var pipelineSchemeMappers = map[string]MeetingExtractionSchemeMapper{
	domain.CommercialSchemeDedicatedSquad: MapDedicatedSquadPromptState,
}

func PipelineSchemeMapper(scheme string) (MeetingExtractionSchemeMapper, bool) {
	mapper, ok := pipelineSchemeMappers[strings.TrimSpace(scheme)]
	return mapper, ok && mapper != nil
}

func PipelineSchemeReady(scheme string) bool {
	_, ok := PipelineSchemeMapper(scheme)
	return ok
}

// ValidatePipelineSchemeRegistry protects startup and tests from a registry
// entry that advertises readiness without an executable mapper, or from a
// commercial scheme silently disappearing from the registry.
func ValidatePipelineSchemeRegistry() error {
	for _, scheme := range domain.CommercialSchemes() {
		mapper, registered := pipelineSchemeMappers[scheme]
		if registered && mapper == nil {
			return fmt.Errorf("pipeline scheme %s is ready without mapper", scheme)
		}
	}
	return nil
}

// MeetingExtractionValidatedPhaseData is the typed output of P1-P4. The
// mapper accepts these values only as validated phase context; it never
// decodes provider JSON or reads a live catalog.
type MeetingExtractionValidatedPhaseData struct {
	Plan     PlanMetadataResult
	Scope    ScopeResult
	Staffing StaffingResult
	Service  ServiceDetailsResult
}

// MeetingExtractionP5MapperInput is the complete, safe input to a scheme
// mapper. Merge.Values is already selected by MergeExtraction using
// user > extracted > default precedence. Catalog is the immutable snapshot
// loaded for this attempt and is used only to resolve opaque role IDs.
type MeetingExtractionP5MapperInput struct {
	Scheme  string
	Merge   MergeExtractionResult
	Phases  MeetingExtractionValidatedPhaseData
	Catalog []domain.RateCardItem
}

type MeetingExtractionP5MapperResult struct {
	State       *domain.PromptQuotationState
	Scheme      string
	NeedsInput  bool
	NeedsReview bool
}

// MapDedicatedSquadPromptState is the first production mapper in the
// dedicated-squad migration slice. It maps only structural prompt state. In
// particular, it has no field for money: every amount/rate/price-shaped merge
// value is deliberately outside the allowlist and cannot reach the state.
func MapDedicatedSquadPromptState(in MeetingExtractionP5MapperInput) (MeetingExtractionP5MapperResult, error) {
	scheme := strings.TrimSpace(in.Scheme)
	if scheme != domain.CommercialSchemeDedicatedSquad && scheme != PlanSchemeUnknown && scheme != PlanSchemeMultiService {
		return MeetingExtractionP5MapperResult{}, fmt.Errorf("%w: unsupported scheme", ErrMeetingExtractionP5Mapper)
	}
	if err := validateMapperPhaseRoute(scheme, in.Phases); err != nil {
		return MeetingExtractionP5MapperResult{}, err
	}

	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		// MeetingNotes is an execution-only input. The mapper has no notes
		// parameter and leaves it empty by construction.
		MeetingNotes: "",
		Scope:        []domain.Fact[string]{},
		Team:         []domain.PromptTeamLine{},
		Issues:       []domain.DraftIssue{},
	}
	values := in.Merge.Values
	if value, ok := values["client_name"]; ok {
		state.ClientName = mapperTextFact(value)
	}
	if value, ok := values["client_company"]; ok {
		state.ClientCompany = mapperTextFact(value)
	}
	if value, ok := values["pic_name"]; ok {
		state.PICName = mapperTextFact(value)
	}
	if value, ok := values["project_name"]; ok {
		state.ProjectName = mapperTextFact(value)
	}
	if value, ok := values["commercial_scheme"]; ok {
		text, ok := mapperString(value.Value)
		if !ok || strings.TrimSpace(text) != scheme {
			return MeetingExtractionP5MapperResult{}, fmt.Errorf("%w: scheme value mismatch", ErrMeetingExtractionP5Mapper)
		}
		state.CommercialScheme = mapperFact[string](value, text)
	}
	if value, ok := values["scope"]; ok {
		mapped, err := mapperScope(value)
		if err != nil {
			return MeetingExtractionP5MapperResult{}, err
		}
		state.Scope = mapped
	}
	if value, ok := values["duration_months"]; ok {
		months, ok := mapperInteger(value.Value)
		if !ok || months <= 0 {
			return MeetingExtractionP5MapperResult{}, fmt.Errorf("%w: duration value", ErrMeetingExtractionP5Mapper)
		}
		state.DurationMonths = &domain.Fact[int]{Value: months, Source: mapperSource(value.Provenance.Source), Evidence: mapperEvidence(value.Provenance), Confirmed: value.Provenance.Source == MergeSourceUser}
	}
	if value, ok := values["team"]; ok {
		periodQuarters := int32(0)
		if state.DurationMonths != nil {
			periodQuarters = int32(state.DurationMonths.Value) * 4
		}
		team, err := mapperTeam(value, in.Catalog, periodQuarters)
		if err != nil {
			return MeetingExtractionP5MapperResult{}, err
		}
		state.Team = team
	}
	if value, ok := values["payment_pattern"]; ok {
		state.PaymentPattern = mapperTextFact(value)
	}

	state.Issues = state.ComputeIssues()
	state.Readiness = state.ComputeReadiness()
	result := MeetingExtractionP5MapperResult{State: state, Scheme: scheme}
	result.NeedsReview = scheme == PlanSchemeUnknown || scheme == PlanSchemeMultiService || in.Phases.Service.NeedsReview
	result.NeedsInput = !result.NeedsReview && (in.Merge.NeedsInput || len(in.Merge.RepairFields) > 0 || hasBlockingMapperIssue(state.Issues))
	return result, nil
}

func validateMapperPhaseRoute(scheme string, phases MeetingExtractionValidatedPhaseData) error {
	if planScheme := strings.TrimSpace(phases.Plan.Scheme); planScheme != "" && planScheme != scheme {
		return fmt.Errorf("%w: plan scheme mismatch", ErrMeetingExtractionP5Mapper)
	}
	if serviceScheme := strings.TrimSpace(phases.Service.Scheme); serviceScheme != "" && serviceScheme != scheme && scheme == domain.CommercialSchemeDedicatedSquad {
		return fmt.Errorf("%w: service scheme mismatch", ErrMeetingExtractionP5Mapper)
	}
	return nil
}

func mapperTextFact(value MergedValue) *domain.Fact[string] {
	text, ok := mapperString(value.Value)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	return mapperFact[string](value, text)
}

func mapperFact[T any](value MergedValue, result T) *domain.Fact[T] {
	return &domain.Fact[T]{
		Value:     result,
		Source:    mapperSource(value.Provenance.Source),
		Evidence:  mapperEvidence(value.Provenance),
		Confirmed: value.Provenance.Source == MergeSourceUser,
	}
}

func mapperSource(source MergeSource) domain.ValueSource {
	switch source {
	case MergeSourceUser:
		return domain.SourceUser
	case MergeSourceDefault:
		return domain.SourceSuggestion
	default:
		return domain.SourceMeeting
	}
}

func mapperEvidence(provenance MergeProvenance) string {
	if strings.TrimSpace(provenance.EvidenceToken) != "" {
		return provenance.EvidenceToken
	}
	if len(provenance.EvidenceRefs) > 0 {
		return strings.Join(provenance.EvidenceRefs, ",")
	}
	return ""
}

func mapperScope(value MergedValue) ([]domain.Fact[string], error) {
	result := make([]domain.Fact[string], 0)
	switch facts := value.Value.(type) {
	case []ScopeFact:
		for _, fact := range facts {
			if strings.TrimSpace(fact.Feature) == "" {
				return nil, fmt.Errorf("%w: scope value", ErrMeetingExtractionP5Mapper)
			}
			result = append(result, domain.Fact[string]{Value: fact.Feature, Source: mapperSource(value.Provenance.Source), Evidence: mapperEvidence(value.Provenance), Confirmed: value.Provenance.Source == MergeSourceUser})
		}
	case []domain.Fact[string]:
		result = append(result, facts...)
	case []string:
		for _, feature := range facts {
			if strings.TrimSpace(feature) == "" {
				return nil, fmt.Errorf("%w: scope value", ErrMeetingExtractionP5Mapper)
			}
			result = append(result, domain.Fact[string]{Value: feature, Source: mapperSource(value.Provenance.Source), Evidence: mapperEvidence(value.Provenance), Confirmed: value.Provenance.Source == MergeSourceUser})
		}
	default:
		return nil, fmt.Errorf("%w: scope type", ErrMeetingExtractionP5Mapper)
	}
	return result, nil
}

func mapperTeam(value MergedValue, catalog []domain.RateCardItem, periodQuarters int32) ([]domain.PromptTeamLine, error) {
	byID := make(map[string]domain.RateCardItem, len(catalog))
	for _, item := range catalog {
		if item.ID != (domain.RateCardItem{}).ID {
			byID[item.ID.String()] = item
		}
	}
	result := make([]domain.PromptTeamLine, 0)
	switch lines := value.Value.(type) {
	case []StaffingTeamLine:
		for _, line := range lines {
			item, ok := byID[strings.TrimSpace(line.RoleID)]
			if !ok || item.Seniority != line.Seniority {
				return nil, fmt.Errorf("%w: team catalog resolution", ErrMeetingExtractionP5Mapper)
			}
			if line.Headcount <= 0 {
				return nil, fmt.Errorf("%w: team headcount", ErrMeetingExtractionP5Mapper)
			}
			id := item.ID.String()
			result = append(result, domain.PromptTeamLine{
				RequestedRole:      item.Role,
				ResolvedRateItemID: &id,
				Seniority:          line.Seniority,
				HeadcountQuarters:  int32(line.Headcount) * 4,
				PeriodQuarters:     periodQuarters,
				Source:             mapperSource(value.Provenance.Source),
				Evidence:           mapperEvidence(value.Provenance),
				Confirmed:          value.Provenance.Source == MergeSourceUser,
			})
		}
	case []domain.PromptTeamLine:
		for _, line := range lines {
			if line.ResolvedRateItemID == nil {
				return nil, fmt.Errorf("%w: unresolved team catalog item", ErrMeetingExtractionP5Mapper)
			}
			if _, ok := byID[strings.TrimSpace(*line.ResolvedRateItemID)]; !ok {
				return nil, fmt.Errorf("%w: unknown team catalog item", ErrMeetingExtractionP5Mapper)
			}
			line.Source = mapperSource(value.Provenance.Source)
			line.Evidence = mapperEvidence(value.Provenance)
			line.Confirmed = value.Provenance.Source == MergeSourceUser
			result = append(result, line)
		}
	default:
		return nil, fmt.Errorf("%w: team type", ErrMeetingExtractionP5Mapper)
	}
	return result, nil
}

func mapperString(value any) (string, bool) {
	text, ok := value.(string)
	return strings.TrimSpace(text), ok
}

func mapperInteger(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int32:
		return int(number), true
	case int64:
		if int64(int(number)) != number {
			return 0, false
		}
		return int(number), true
	case float64:
		if number != math.Trunc(number) || number > math.MaxInt || number < math.MinInt {
			return 0, false
		}
		return int(number), true
	case json.Number:
		parsed, err := number.Int64()
		if err != nil || int64(int(parsed)) != parsed {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func hasBlockingMapperIssue(issues []domain.DraftIssue) bool {
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			return true
		}
	}
	return false
}
