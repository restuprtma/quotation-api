package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const (
	PhaseKeyStaffing          PhaseKey = "staffing"
	staffingMaxTeam                    = 30
	staffingMaxDurationMonths          = 120
	staffingMaxHeadcount               = 100
)

var ErrStaffingPhaseValidation = errors.New("staffing phase validation failed")

// StaffingCatalogRole is the deliberately narrow, non-commercial projection
// of a rate-card item exposed to P3. It contains an opaque role ID and its
// required seniority, never a display price or any financial attribute.
type StaffingCatalogRole struct {
	ID        string
	Seniority string
}

type StaffingEvidence struct {
	Refs    []string `json:"refs"`
	Support string   `json:"support"`
}

type StaffingDuration struct {
	Value    int              `json:"value"`
	Evidence StaffingEvidence `json:"evidence"`
}

type StaffingTeamLine struct {
	RoleID    string           `json:"role_id"`
	Seniority string           `json:"seniority"`
	Headcount int              `json:"headcount"`
	Evidence  StaffingEvidence `json:"evidence"`
}

// StaffingResult owns duration_months and team. Mapping to the legacy prompt
// state and its rate-card-backed fields happens only during deterministic P5.
type StaffingResult struct {
	DurationMonths *StaffingDuration  `json:"duration_months"`
	Team           []StaffingTeamLine `json:"team"`
	EvidenceRefs   []string           `json:"-"`
}

func (StaffingResult) PhaseKey() PhaseKey { return PhaseKeyStaffing }
func (r StaffingResult) SafeResultJSON() ([]byte, error) {
	return json.Marshal(struct {
		DurationMonths *StaffingDuration  `json:"duration_months"`
		Team           []StaffingTeamLine `json:"team"`
	}{r.DurationMonths, r.Team})
}
func (r StaffingResult) EvidenceReferenceIDs() []string {
	return append([]string(nil), r.EvidenceRefs...)
}
func (StaffingResult) WarningCodes() []string { return nil }

type StaffingPhase struct {
	roles map[string]StaffingCatalogRole
}

func NewStaffingPhase(catalog []domain.RateCardItem) *StaffingPhase {
	roles := make(map[string]StaffingCatalogRole, len(catalog))
	for _, item := range catalog {
		id, seniority := strings.TrimSpace(item.ID.String()), strings.TrimSpace(item.Seniority)
		if id == "" || id == "00000000-0000-0000-0000-000000000000" || seniority == "" {
			continue
		}
		roles[id] = StaffingCatalogRole{ID: id, Seniority: seniority}
	}
	return &StaffingPhase{roles: roles}
}

func (p *StaffingPhase) Key() PhaseKey     { return PhaseKeyStaffing }
func (p *StaffingPhase) Owns() []FieldPath { return []FieldPath{"duration_months", "team"} }

func (p *StaffingPhase) BuildPrompt(_ context.Context, in PhaseInput) (ProviderRequest, error) {
	if in.Document == nil {
		return ProviderRequest{}, fmt.Errorf("%w: missing document", ErrStaffingPhaseValidation)
	}
	schema, err := p.Schema(in)
	if err != nil {
		return ProviderRequest{}, err
	}
	prompt := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p3_staffing.txt")
	if prompt == "" {
		return ProviderRequest{}, fmt.Errorf("%w: prompt unavailable", ErrStaffingPhaseValidation)
	}
	return ProviderRequest{Prompt: prompt + "\n<meeting_notes>\n" + in.Document.Canonical() + "\n</meeting_notes>", JSONSchema: schema, MaxOutputTokens: 1200}, nil
}

func (p *StaffingPhase) Schema(PhaseInput) (json.RawMessage, error) {
	if len(p.roles) == 0 {
		return nil, fmt.Errorf("%w: empty role catalog", ErrStaffingPhaseValidation)
	}
	raw := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p3_staffing_schema.json")
	if raw == "" {
		return nil, fmt.Errorf("%w: schema unavailable", ErrStaffingPhaseValidation)
	}
	ids := make([]string, 0, len(p.roles))
	for id := range p.roles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("%w: role enum", ErrStaffingPhaseValidation)
	}
	raw = strings.ReplaceAll(raw, "{{ROLE_IDS}}", string(encoded))
	if !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("%w: invalid schema", ErrStaffingPhaseValidation)
	}
	return json.RawMessage(raw), nil
}

func (p *StaffingPhase) Validate(_ context.Context, in PhaseInput, candidate json.RawMessage) (PhaseResult, error) {
	if in.Document == nil || !json.Valid(candidate) {
		return nil, fmt.Errorf("%w: malformed candidate", ErrStaffingPhaseValidation)
	}
	decoder := json.NewDecoder(bytes.NewReader(candidate))
	decoder.DisallowUnknownFields()
	var parsed StaffingResult
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%w: shape", ErrStaffingPhaseValidation)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: shape", ErrStaffingPhaseValidation)
	}
	if err := p.validate(parsed); err != nil {
		return nil, err
	}
	parsed.EvidenceRefs = staffingRefs(parsed)
	return parsed, nil
}

func (p *StaffingPhase) validate(result StaffingResult) error {
	if result.DurationMonths != nil && (result.DurationMonths.Value < 1 || result.DurationMonths.Value > staffingMaxDurationMonths || !validStaffingEvidence(result.DurationMonths.Evidence)) {
		return fmt.Errorf("%w: duration_months", ErrStaffingPhaseValidation)
	}
	if len(result.Team) > staffingMaxTeam {
		return fmt.Errorf("%w: team", ErrStaffingPhaseValidation)
	}
	seen := make(map[string]struct{}, len(result.Team))
	for _, line := range result.Team {
		role, ok := p.roles[strings.TrimSpace(line.RoleID)]
		if !ok || strings.TrimSpace(line.Seniority) != role.Seniority || line.Headcount < 1 || line.Headcount > staffingMaxHeadcount || !validStaffingEvidence(line.Evidence) {
			return fmt.Errorf("%w: team", ErrStaffingPhaseValidation)
		}
		if _, duplicate := seen[line.RoleID]; duplicate {
			return fmt.Errorf("%w: duplicate role", ErrStaffingPhaseValidation)
		}
		seen[line.RoleID] = struct{}{}
	}
	return nil
}

func validStaffingEvidence(e StaffingEvidence) bool {
	return e.Support == "explicit" && validPlanRefs(e.Refs)
}
func staffingRefs(result StaffingResult) []string {
	seen := map[string]struct{}{}
	add := func(refs []string) {
		for _, ref := range refs {
			seen[ref] = struct{}{}
		}
	}
	if result.DurationMonths != nil {
		add(result.DurationMonths.Evidence.Refs)
	}
	for _, line := range result.Team {
		add(line.Evidence.Refs)
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}
