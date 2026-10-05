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
	PlanSchemeUnknown      = "unknown"
	PlanSchemeMultiService = "multi_service"
	planMaxSections        = 7
	planMaxRefs            = 16
	planMaxMetadataLength  = 256
)

var ErrPlanPhaseValidation = errors.New("plan phase validation failed")

type PlanDisposition string

const (
	PlanDispositionContinue    PlanDisposition = "continue"
	PlanDispositionNeedsReview PlanDisposition = "needs_review"
)

type PlanSchemeSource string

const (
	PlanSchemeSourceExtracted PlanSchemeSource = "extracted"
	PlanSchemeSourceManual    PlanSchemeSource = "manual"
	PlanSchemeSourceUser      PlanSchemeSource = "user"
)

type PlanUserHints struct{ ClientCompany, ProjectName, PICName, ClientName *string }
type PlanEvidence struct {
	Refs    []string `json:"refs"`
	Support string   `json:"support"`
}
type PlanMetadataFact struct {
	Value    string           `json:"value"`
	Evidence PlanEvidence     `json:"evidence"`
	Source   PlanSchemeSource `json:"-"`
}
type PlanMetadata struct {
	ClientCompany *PlanMetadataFact `json:"client_company"`
	ProjectName   *PlanMetadataFact `json:"project_name"`
	PICName       *PlanMetadataFact `json:"pic_name"`
	ClientName    *PlanMetadataFact `json:"client_name"`
}
type PlanAvailableSection struct {
	Section string   `json:"section"`
	Refs    []string `json:"refs"`
}
type PlanMetadataResult struct {
	Scheme            string                 `json:"scheme,omitempty"`
	Confidence        *float64               `json:"confidence,omitempty"`
	SchemeSource      PlanSchemeSource       `json:"-"`
	AvailableSections []PlanAvailableSection `json:"available_sections"`
	Metadata          PlanMetadata           `json:"metadata"`
	Disposition       PlanDisposition        `json:"-"`
	EvidenceRefs      []string               `json:"-"`
	Warnings          []string               `json:"-"`
}

func (PlanMetadataResult) PhaseKey() PhaseKey { return PhaseKeyPlanMetadata }
func (r PlanMetadataResult) SafeResultJSON() ([]byte, error) {
	type persisted struct {
		Scheme            string                 `json:"scheme,omitempty"`
		Confidence        *float64               `json:"confidence,omitempty"`
		AvailableSections []PlanAvailableSection `json:"available_sections"`
		Metadata          PlanMetadata           `json:"metadata"`
	}
	return json.Marshal(persisted{r.Scheme, r.Confidence, r.AvailableSections, r.Metadata})
}
func (r PlanMetadataResult) EvidenceReferenceIDs() []string {
	return append([]string(nil), r.EvidenceRefs...)
}
func (r PlanMetadataResult) WarningCodes() []string { return append([]string(nil), r.Warnings...) }
func (r PlanMetadataResult) ContinuePipeline() bool { return r.Disposition == PlanDispositionContinue }

type PlanMetadataPhase struct{ manualScheme string }

func NewPlanMetadataPhase(manualScheme string) *PlanMetadataPhase {
	return &PlanMetadataPhase{manualScheme: normalizedManualScheme(manualScheme)}
}
func (p *PlanMetadataPhase) Key() PhaseKey { return PhaseKeyPlanMetadata }
func (p *PlanMetadataPhase) Owns() []FieldPath {
	return []FieldPath{"client_name", "client_company", "pic_name", "project_name", "commercial_scheme"}
}
func (p *PlanMetadataPhase) BuildPrompt(_ context.Context, in PhaseInput) (ProviderRequest, error) {
	if in.Document == nil {
		return ProviderRequest{}, fmt.Errorf("%w: missing document", ErrPlanPhaseValidation)
	}
	schema, err := p.Schema(in)
	if err != nil {
		return ProviderRequest{}, err
	}
	prompt := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p1_plan_metadata.txt")
	if prompt == "" {
		return ProviderRequest{}, fmt.Errorf("%w: prompt unavailable", ErrPlanPhaseValidation)
	}
	if p.effectiveManualScheme(in) != "" {
		prompt = strings.ReplaceAll(prompt, "{{CLASSIFICATION_INSTRUCTION}}", "Do not classify a scheme; extract metadata only.")
	} else {
		prompt = strings.ReplaceAll(prompt, "{{CLASSIFICATION_INSTRUCTION}}", "Classify exactly one scheme, unknown, or multi_service.")
	}
	return ProviderRequest{Prompt: prompt + "\n<meeting_notes>\n" + in.Document.Canonical() + "\n</meeting_notes>", JSONSchema: schema, MaxOutputTokens: 1200}, nil
}
func (p *PlanMetadataPhase) Schema(in PhaseInput) (json.RawMessage, error) {
	return p.schemaFor(p.effectiveManualScheme(in))
}

func (p *PlanMetadataPhase) schemaFor(manualScheme string) (json.RawMessage, error) {
	name := "meeting_extraction_pipeline_v1/p1_plan_metadata_schema.json"
	if p.manualScheme != "" || manualScheme != "" {
		name = "meeting_extraction_pipeline_v1/p1_plan_metadata_manual_schema.json"
	}
	raw := GetSafetyEnvelope(name)
	if raw == "" || !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("%w: schema unavailable", ErrPlanPhaseValidation)
	}
	return json.RawMessage(raw), nil
}

func (p *PlanMetadataPhase) effectiveManualScheme(in PhaseInput) string {
	if p.manualScheme != "" {
		return p.manualScheme
	}
	return normalizedManualScheme(in.ManualScheme)
}
func (p *PlanMetadataPhase) Validate(_ context.Context, in PhaseInput, candidate json.RawMessage) (PhaseResult, error) {
	if in.Document == nil || !json.Valid(candidate) {
		return nil, fmt.Errorf("%w: malformed candidate", ErrPlanPhaseValidation)
	}
	decoder := json.NewDecoder(bytes.NewReader(candidate))
	decoder.DisallowUnknownFields()
	var parsed planCandidate
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%w: shape", ErrPlanPhaseValidation)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: shape", ErrPlanPhaseValidation)
	}
	manual := p.effectiveManualScheme(in)
	if manual != "" && !domain.IsCommercialScheme(manual) {
		return nil, fmt.Errorf("%w: manual scheme", ErrPlanPhaseValidation)
	}
	if err := parsed.validate(manual != ""); err != nil {
		return nil, err
	}
	result := PlanMetadataResult{AvailableSections: parsed.AvailableSections, Metadata: parsed.Metadata, Disposition: PlanDispositionContinue}
	if manual != "" {
		result.Scheme, result.SchemeSource = manual, PlanSchemeSourceManual
	} else {
		result.Scheme, result.Confidence, result.SchemeSource = parsed.Scheme, parsed.Confidence, PlanSchemeSourceExtracted
	}
	applyPlanUserHints(&result.Metadata, in.UserHints)
	result.EvidenceRefs = collectPlanRefs(result.AvailableSections, result.Metadata)
	if result.Scheme == PlanSchemeUnknown || result.Scheme == PlanSchemeMultiService || (result.Confidence != nil && *result.Confidence < 0.70) {
		result.Disposition = PlanDispositionNeedsReview
		result.Warnings = []string{"SCHEME_REVIEW_REQUIRED"}
	}
	return result, nil
}

type planCandidate struct {
	Scheme            string                 `json:"scheme"`
	Confidence        *float64               `json:"confidence"`
	AvailableSections []PlanAvailableSection `json:"available_sections"`
	Metadata          PlanMetadata           `json:"metadata"`
}

func (c planCandidate) validate(manual bool) error {
	if manual {
		if c.Scheme != "" || c.Confidence != nil {
			return fmt.Errorf("%w: manual classification", ErrPlanPhaseValidation)
		}
	} else if !validPlanScheme(c.Scheme) || c.Confidence == nil || *c.Confidence < 0 || *c.Confidence > 1 {
		return fmt.Errorf("%w: classification", ErrPlanPhaseValidation)
	}
	if len(c.AvailableSections) > planMaxSections {
		return fmt.Errorf("%w: sections", ErrPlanPhaseValidation)
	}
	seen := map[string]bool{}
	for _, s := range c.AvailableSections {
		if !validPlanSection(s.Section) || seen[s.Section] || !validPlanRefs(s.Refs) {
			return fmt.Errorf("%w: section", ErrPlanPhaseValidation)
		}
		seen[s.Section] = true
	}
	for _, fact := range []*PlanMetadataFact{c.Metadata.ClientCompany, c.Metadata.ProjectName, c.Metadata.PICName, c.Metadata.ClientName} {
		if fact != nil && (!validPlanText(fact.Value) || fact.Evidence.Support != "explicit" || !validPlanRefs(fact.Evidence.Refs)) {
			return fmt.Errorf("%w: metadata", ErrPlanPhaseValidation)
		}
	}
	return nil
}
func validPlanScheme(v string) bool {
	return domain.IsCommercialScheme(v) || v == PlanSchemeUnknown || v == PlanSchemeMultiService
}
func validPlanSection(v string) bool {
	switch v {
	case "metadata", "scope", "staffing", "timeline", "payment", "commercial", "service_details":
		return true
	}
	return false
}
func validPlanText(v string) bool {
	return strings.TrimSpace(v) != "" && len([]rune(v)) <= planMaxMetadataLength
}
func validPlanRefs(refs []string) bool {
	if len(refs) == 0 || len(refs) > planMaxRefs {
		return false
	}
	for _, ref := range refs {
		if !validPlanRef(ref) {
			return false
		}
	}
	return true
}
func validPlanRef(ref string) bool {
	if _, _, ok := parseMeetingLineReference(ref); ok {
		return true
	}
	return len(ref) == 4 && ref[0] == 'C' && ref[1] >= '0' && ref[1] <= '9' && ref[2] >= '0' && ref[2] <= '9' && ref[3] >= '0' && ref[3] <= '9'
}
func applyPlanUserHints(m *PlanMetadata, h PlanUserHints) {
	apply := func(value *string, dst **PlanMetadataFact) {
		if value != nil && strings.TrimSpace(*value) != "" {
			*dst = &PlanMetadataFact{Value: strings.TrimSpace(*value), Source: PlanSchemeSourceUser}
		}
	}
	apply(h.ClientCompany, &m.ClientCompany)
	apply(h.ProjectName, &m.ProjectName)
	apply(h.PICName, &m.PICName)
	apply(h.ClientName, &m.ClientName)
}
func collectPlanRefs(sections []PlanAvailableSection, metadata PlanMetadata) []string {
	seen := map[string]struct{}{}
	add := func(refs []string) {
		for _, ref := range refs {
			seen[ref] = struct{}{}
		}
	}
	for _, section := range sections {
		add(section.Refs)
	}
	for _, fact := range []*PlanMetadataFact{metadata.ClientCompany, metadata.ProjectName, metadata.PICName, metadata.ClientName} {
		if fact != nil {
			add(fact.Evidence.Refs)
		}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}
