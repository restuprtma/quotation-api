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

// PhaseKeyServiceDetails is deliberately descriptive-only.  It is not an
// adapter input: M7 owns evidence reconstruction and M5 finalization builds
// exact generator input from these facts.
const PhaseKeyServiceDetails PhaseKey = "service_details"

const maxServiceFacts = 32

type ServiceDetailFact struct {
	Key      string          `json:"key"`
	Value    json.RawMessage `json:"value"`
	Evidence PlanEvidence    `json:"evidence"`
}

type ServiceDetailsResult struct {
	Scheme      string              `json:"scheme"`
	Facts       []ServiceDetailFact `json:"facts"`
	NeedsReview bool                `json:"-"`
}

func (ServiceDetailsResult) PhaseKey() PhaseKey { return PhaseKeyServiceDetails }
func (r ServiceDetailsResult) SafeResultJSON() ([]byte, error) {
	return json.Marshal(struct {
		Scheme string              `json:"scheme"`
		Facts  []ServiceDetailFact `json:"facts"`
	}{r.Scheme, r.Facts})
}
func (r ServiceDetailsResult) EvidenceReferenceIDs() []string {
	var refs []string
	for _, fact := range r.Facts {
		refs = append(refs, fact.Evidence.Refs...)
	}
	sort.Strings(refs)
	return uniqueStrings(refs)
}
func (r ServiceDetailsResult) WarningCodes() []string {
	if r.NeedsReview {
		return []string{"SERVICE_DETAILS_REVIEW_REQUIRED"}
	}
	return nil
}

// ServiceDetailsPhase is selected only after P1 selected a valid scheme. A
// caller that deliberately asks for the generic phase receives needs_review;
// it cannot turn into an apparently successful quotation.
type ServiceDetailsPhase struct {
	scheme          string
	genericFallback bool
}

func NewServiceDetailsPhase(scheme string) *ServiceDetailsPhase {
	return &ServiceDetailsPhase{scheme: strings.TrimSpace(scheme)}
}
func NewGenericServiceDetailsFallback() *ServiceDetailsPhase {
	return &ServiceDetailsPhase{genericFallback: true}
}
func (p *ServiceDetailsPhase) Key() PhaseKey     { return PhaseKeyServiceDetails }
func (p *ServiceDetailsPhase) Owns() []FieldPath { return []FieldPath{"service_input"} }
func (p *ServiceDetailsPhase) BuildPrompt(_ context.Context, in PhaseInput) (ProviderRequest, error) {
	if in.Document == nil {
		return ProviderRequest{}, fmt.Errorf("%w: missing document", ErrPlanPhaseValidation)
	}
	schema, err := p.Schema(in)
	if err != nil {
		return ProviderRequest{}, err
	}
	if p.genericFallback {
		return ProviderRequest{}, fmt.Errorf("%w: generic service fallback requires review", ErrPlanPhaseValidation)
	}
	prompt := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p4_" + p.scheme + ".txt")
	if prompt == "" {
		return ProviderRequest{}, fmt.Errorf("%w: prompt unavailable", ErrPlanPhaseValidation)
	}
	refs := plannerReferences(in.Dependencies)
	route := in.Document.Route(refs)
	return ProviderRequest{Prompt: prompt + "\n<meeting_notes>\n" + route.Text() + "\n</meeting_notes>", JSONSchema: schema, MaxOutputTokens: 1800}, nil
}
func (p *ServiceDetailsPhase) Schema(_ PhaseInput) (json.RawMessage, error) {
	if p.genericFallback {
		return nil, fmt.Errorf("%w: generic service fallback requires review", ErrPlanPhaseValidation)
	}
	if !domain.IsCommercialScheme(p.scheme) {
		return nil, fmt.Errorf("%w: unsupported scheme", ErrPlanPhaseValidation)
	}
	raw := GetSafetyEnvelope("meeting_extraction_pipeline_v1/p4_" + p.scheme + "_schema.json")
	if raw == "" || !json.Valid([]byte(raw)) {
		return nil, fmt.Errorf("%w: schema unavailable", ErrPlanPhaseValidation)
	}
	return json.RawMessage(raw), nil
}
func (p *ServiceDetailsPhase) Validate(_ context.Context, _ PhaseInput, candidate json.RawMessage) (PhaseResult, error) {
	if p.genericFallback {
		return ServiceDetailsResult{Scheme: "generic", NeedsReview: true}, nil
	}
	if !domain.IsCommercialScheme(p.scheme) || !json.Valid(candidate) {
		return nil, fmt.Errorf("%w: service candidate", ErrPlanPhaseValidation)
	}
	dec := json.NewDecoder(bytes.NewReader(candidate))
	dec.DisallowUnknownFields()
	var result ServiceDetailsResult
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: service shape", ErrPlanPhaseValidation)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: service shape", ErrPlanPhaseValidation)
	}
	if result.Scheme != p.scheme || len(result.Facts) > maxServiceFacts {
		return nil, fmt.Errorf("%w: service facts", ErrPlanPhaseValidation)
	}
	allowed := serviceDetailKeys[p.scheme]
	seen := make(map[string]bool, len(result.Facts))
	for _, fact := range result.Facts {
		if !allowed[fact.Key] || seen[fact.Key] || !validPlanRefs(fact.Evidence.Refs) || (fact.Evidence.Support != "explicit" && fact.Evidence.Support != "inferred") || !validServiceDetailValue(fact.Value) {
			return nil, fmt.Errorf("%w: service fact", ErrPlanPhaseValidation)
		}
		seen[fact.Key] = true
	}
	return result, nil
}

func validServiceDetailValue(value json.RawMessage) bool {
	if len(value) == 0 || len(value) > 2048 {
		return false
	}
	var v any
	if json.Unmarshal(value, &v) != nil {
		return false
	}
	switch typed := v.(type) {
	case string:
		return strings.TrimSpace(typed) != "" && len([]rune(typed)) <= 512
	case float64:
		return typed >= 0
	case bool:
		return true
	default:
		return false
	}
}

func plannerReferences(dependencies map[string]json.RawMessage) []string {
	raw := dependencies[string(PhaseKeyPlanMetadata)]
	if len(raw) == 0 {
		return nil
	}
	var plan struct {
		AvailableSections []PlanAvailableSection `json:"available_sections"`
	}
	if json.Unmarshal(raw, &plan) != nil {
		return nil
	}
	var refs []string
	for _, section := range plan.AvailableSections {
		if section.Section == "service_details" || section.Section == "timeline" || section.Section == "payment" || section.Section == "commercial" {
			refs = append(refs, section.Refs...)
		}
	}
	return refs
}

func uniqueStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

var serviceDetailKeys = map[string]map[string]bool{
	domain.CommercialSchemeDedicatedSquad:        {"model": true, "requirement": true, "maintenance_intent": true, "payment_count": true, "anchor_date": true, "timeline_label": true},
	domain.CommercialSchemeSubscriptionSaaS:      {"product_name": true, "section_layout": true, "billing_cycle": true, "initial_term_months": true, "auto_renewal": true, "feature_groups": true, "partnership": true, "sharing_terms": true, "payment_label": true},
	domain.CommercialSchemeWhiteLabel:            {"product_name": true, "module_groups": true, "timeline": true, "maintenance_intent": true, "hosting_intent": true, "payment_label": true, "rights_code": true, "limitation_code": true},
	domain.CommercialSchemeStandaloneMaintenance: {"system_name": true, "mode": true, "defect_groups": true, "hourly_tasks": true, "period_quantity": true, "headcount_quantity": true, "payment_label": true, "rights_code": true},
	domain.CommercialSchemeCustomDevelopment:     {"system_name": true, "table_mode": true, "module_groups": true, "timeline": true, "period_quantity": true, "headcount_quantity": true, "monthly_item_description": true, "payment_label": true, "rights_code": true, "limitation_code": true},
	domain.CommercialSchemeKioskQueueSystem:      {"variant": true, "system_descriptor": true, "document_descriptor": true, "hardware_provision_mode": true, "government_scope_phase": true, "expert_requirement": true, "service_description": true, "hardware_description": true, "quantity": true, "timeline": true, "payment_label": true, "rights_code": true, "limitation_code": true},
	domain.CommercialSchemeTalentOutsourcing:     {"engagement_mode": true, "service_descriptor": true, "document_descriptor": true, "cost_item_quantity": true, "period_quantity": true, "squad_role": true, "headcount_quantity": true, "months": true, "scope": true, "responsibility": true, "qualification": true, "deliverable": true, "timeline": true, "payment_label": true, "warranty_code": true, "rights_code": true},
	domain.CommercialSchemeCloudInfrastructure:   {"service_descriptor": true, "document_descriptor": true, "package_category": true, "package_name": true, "vcpu": true, "ram_gb": true, "storage_gb": true, "setup_descriptor": true, "scope": true, "timeline": true, "payment_label": true, "due_days": true, "limitation_code": true},
}

// ServiceDetailsMapper is intentionally narrower than a generator adapter. It
// translates a validated P4 payload to the scheme-owned descriptive facts
// only; M7 later combines those facts with verified evidence and backend
// defaults before any generator receives input.
type ServiceDetailsMapper interface {
	Scheme() string
	Map(ServiceDetailsResult) (map[string]json.RawMessage, error)
}

type fixedServiceDetailsMapper struct{ scheme string }

func (m fixedServiceDetailsMapper) Scheme() string { return m.scheme }
func (m fixedServiceDetailsMapper) Map(result ServiceDetailsResult) (map[string]json.RawMessage, error) {
	if result.Scheme != m.scheme || result.NeedsReview {
		return nil, fmt.Errorf("%w: service mapper scheme", ErrPlanPhaseValidation)
	}
	allowed := serviceDetailKeys[m.scheme]
	mapped := make(map[string]json.RawMessage, len(result.Facts))
	for _, fact := range result.Facts {
		if !allowed[fact.Key] || !validServiceDetailValue(fact.Value) {
			return nil, fmt.Errorf("%w: service mapper fact", ErrPlanPhaseValidation)
		}
		if _, exists := mapped[fact.Key]; exists {
			return nil, fmt.Errorf("%w: service mapper duplicate", ErrPlanPhaseValidation)
		}
		mapped[fact.Key] = append(json.RawMessage(nil), fact.Value...)
	}
	return mapped, nil
}

var serviceDetailsMappers = func() map[string]ServiceDetailsMapper {
	mappers := make(map[string]ServiceDetailsMapper, len(serviceDetailKeys))
	for scheme := range serviceDetailKeys {
		mappers[scheme] = fixedServiceDetailsMapper{scheme: scheme}
	}
	return mappers
}()

func ServiceDetailsMapperFor(scheme string) (ServiceDetailsMapper, bool) {
	mapper, ok := serviceDetailsMappers[scheme]
	return mapper, ok
}

func registeredServiceDetailMapperKeys() map[string]struct{} {
	keys := make(map[string]struct{}, len(serviceDetailsMappers))
	for scheme := range serviceDetailsMappers {
		keys[scheme] = struct{}{}
	}
	return keys
}
