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
)

const (
	PhaseKeyScope        PhaseKey = "scope"
	scopeMaxItems                 = 32
	scopeMaxFeatureRunes          = 256
)

var ErrScopePhaseValidation = errors.New("scope phase validation failed")

// ScopeEvidence intentionally contains source identifiers only. Source text
// and provider payloads must never be persisted with a phase result.
type ScopeEvidence struct {
	Refs    []string `json:"refs"`
	Support string   `json:"support"`
}

type ScopeFact struct {
	Feature  string        `json:"feature"`
	Evidence ScopeEvidence `json:"evidence"`
}

type ScopeResult struct {
	Scope        []ScopeFact `json:"scope"`
	EvidenceRefs []string    `json:"-"`
	Warnings     []string    `json:"-"`
}

func (ScopeResult) PhaseKey() PhaseKey { return PhaseKeyScope }
func (r ScopeResult) SafeResultJSON() ([]byte, error) {
	return json.Marshal(struct {
		Scope []ScopeFact `json:"scope"`
	}{Scope: r.Scope})
}
func (r ScopeResult) EvidenceReferenceIDs() []string { return append([]string(nil), r.EvidenceRefs...) }
func (r ScopeResult) WarningCodes() []string         { return append([]string(nil), r.Warnings...) }

// ScopePhase owns only the generic scope feature list. Scheme-specific detail
// belongs to P4 and is deliberately absent from this contract.
type ScopePhase struct{}

func NewScopePhase() *ScopePhase      { return &ScopePhase{} }
func (*ScopePhase) Key() PhaseKey     { return PhaseKeyScope }
func (*ScopePhase) Owns() []FieldPath { return []FieldPath{"scope"} }

// ShouldSkip is true only when P1 did not route a scope section. The worker
// can mark P2 skipped without spending a provider call.
func (*ScopePhase) ShouldSkip(in PhaseInput) (bool, error) {
	plan, err := p2PlanFromDependencies(in)
	if err != nil {
		return false, err
	}
	for _, section := range plan.AvailableSections {
		if section.Section == "scope" {
			return false, nil
		}
	}
	return true, nil
}

func (p *ScopePhase) BuildPrompt(_ context.Context, in PhaseInput) (ProviderRequest, error) {
	if in.Document == nil {
		return ProviderRequest{}, fmt.Errorf("%w: missing document", ErrScopePhaseValidation)
	}
	plan, err := p2PlanFromDependencies(in)
	if err != nil {
		return ProviderRequest{}, err
	}
	refs := p2ScopeRefs(plan)
	if len(refs) == 0 {
		return ProviderRequest{}, fmt.Errorf("%w: scope section unavailable", ErrScopePhaseValidation)
	}
	schema, err := p.Schema(in)
	if err != nil {
		return ProviderRequest{}, err
	}
	prompt := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p2_scope.txt")
	if prompt == "" {
		return ProviderRequest{}, fmt.Errorf("%w: prompt unavailable", ErrScopePhaseValidation)
	}
	route := in.Document.Route(refs)
	if route.Fallback {
		prompt += "\nRouting warning: use the supplied bounded context only."
	}
	return ProviderRequest{
		Prompt:          prompt + "\n<meeting_notes>\n" + renderScopeRoute(route) + "\n</meeting_notes>",
		JSONSchema:      schema,
		MaxOutputTokens: 1200,
	}, nil
}

func (*ScopePhase) Schema(PhaseInput) (json.RawMessage, error) {
	raw := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p2_scope_schema.json")
	if raw == "" || !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("%w: schema unavailable", ErrScopePhaseValidation)
	}
	return json.RawMessage(raw), nil
}

func (*ScopePhase) Validate(_ context.Context, in PhaseInput, candidate json.RawMessage) (PhaseResult, error) {
	if in.Document == nil || !json.Valid(candidate) {
		return nil, fmt.Errorf("%w: malformed candidate", ErrScopePhaseValidation)
	}
	if skip, err := (&ScopePhase{}).ShouldSkip(in); err != nil {
		return nil, err
	} else if skip {
		return nil, fmt.Errorf("%w: scope section unavailable", ErrScopePhaseValidation)
	}
	decoder := json.NewDecoder(bytes.NewReader(candidate))
	decoder.DisallowUnknownFields()
	var parsed struct {
		Scope []ScopeFact `json:"scope"`
	}
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%w: shape", ErrScopePhaseValidation)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: shape", ErrScopePhaseValidation)
	}
	if len(parsed.Scope) > scopeMaxItems {
		return nil, fmt.Errorf("%w: too many scope items", ErrScopePhaseValidation)
	}
	for _, fact := range parsed.Scope {
		if !validScopeFact(fact) {
			return nil, fmt.Errorf("%w: scope fact", ErrScopePhaseValidation)
		}
	}
	result := ScopeResult{Scope: dedupeScopeFacts(parsed.Scope)}
	result.EvidenceRefs = collectScopeRefs(result.Scope)
	return result, nil
}

type p2PlanDependency struct {
	Scheme            string                 `json:"scheme"`
	Confidence        *float64               `json:"confidence"`
	AvailableSections []PlanAvailableSection `json:"available_sections"`
	Metadata          PlanMetadata           `json:"metadata"`
}

func p2PlanFromDependencies(in PhaseInput) (p2PlanDependency, error) {
	raw, ok := in.Dependencies[string(PhaseKeyPlanMetadata)]
	if !ok || !json.Valid(raw) {
		return p2PlanDependency{}, fmt.Errorf("%w: plan dependency unavailable", ErrScopePhaseValidation)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var plan p2PlanDependency
	if err := decoder.Decode(&plan); err != nil {
		return p2PlanDependency{}, fmt.Errorf("%w: plan dependency", ErrScopePhaseValidation)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return p2PlanDependency{}, fmt.Errorf("%w: plan dependency", ErrScopePhaseValidation)
	}
	return plan, nil
}

func p2ScopeRefs(plan p2PlanDependency) []string {
	var refs []string
	for _, section := range plan.AvailableSections {
		if section.Section == "scope" {
			refs = append(refs, section.Refs...)
		}
	}
	return refs
}

func validScopeFact(f ScopeFact) bool {
	if strings.TrimSpace(f.Feature) == "" || len([]rune(f.Feature)) > scopeMaxFeatureRunes {
		return false
	}
	if f.Evidence.Support != "explicit" && f.Evidence.Support != "inferred" {
		return false
	}
	return validPlanRefs(f.Evidence.Refs)
}

func dedupeScopeFacts(facts []ScopeFact) []ScopeFact {
	byFeature := make(map[string]ScopeFact, len(facts))
	for _, fact := range facts {
		key := strings.ToLower(strings.Join(strings.Fields(fact.Feature), " "))
		if current, ok := byFeature[key]; !ok || scopeFactSortKey(fact) < scopeFactSortKey(current) {
			byFeature[key] = fact
		}
	}
	keys := make([]string, 0, len(byFeature))
	for key := range byFeature {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]ScopeFact, 0, len(keys))
	for _, key := range keys {
		result = append(result, byFeature[key])
	}
	return result
}

func scopeFactSortKey(f ScopeFact) string {
	refs := append([]string(nil), f.Evidence.Refs...)
	sort.Strings(refs)
	return f.Evidence.Support + "\x00" + strings.Join(refs, "\x00") + "\x00" + f.Feature
}

func collectScopeRefs(scope []ScopeFact) []string {
	seen := make(map[string]struct{})
	for _, fact := range scope {
		for _, ref := range fact.Evidence.Refs {
			seen[ref] = struct{}{}
		}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func renderScopeRoute(route MeetingNoteRoute) string {
	var b strings.Builder
	for chunkIndex, chunk := range route.Chunks {
		if chunkIndex > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("[")
		b.WriteString(chunk.ID)
		b.WriteString("]\n")
		for _, line := range chunk.Lines {
			b.WriteString("[")
			b.WriteString(line.ID)
			b.WriteString("] ")
			b.WriteString(line.Text)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}
