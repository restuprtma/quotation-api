package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const meetingExtractionRegistryAsset = "meeting_extraction_pipeline_v1/registry.json"

var ErrServiceExtractorRegistry = errors.New("invalid service extractor registry")

// RegistryPhaseConfig describes an optional fixed-graph phase. The
// orchestrator, rather than this manifest, remains responsible for phase
// ordering and deciding whether a phase is safe to run for a particular note.
type RegistryPhaseConfig struct {
	Enabled       bool   `json:"enabled"`
	PromptVersion string `json:"prompt_version"`
	SchemaVersion string `json:"schema_version"`
	PromptAsset   string `json:"prompt_asset"`
	SchemaAsset   string `json:"schema_asset"`
}

// ServiceExtractorRegistryEntry is data-driven configuration for one existing
// quotation scheme. Validator and mapper keys deliberately resolve through Go
// registrations: a manifest must never be able to select executable code.
type ServiceExtractorRegistryEntry struct {
	Scheme              string              `json:"scheme"`
	Version             string              `json:"version"`
	RequiredSections    []string            `json:"required_sections"`
	RateCardSelectorKey string              `json:"rate_card_selector_key"`
	ValidatorKey        string              `json:"validator_key"`
	MapperKey           string              `json:"mapper_key"`
	P2                  RegistryPhaseConfig `json:"p2"`
	P3                  RegistryPhaseConfig `json:"p3"`
	P4                  RegistryPhaseConfig `json:"p4"`
}

type serviceExtractorRegistryManifest struct {
	Version string                          `json:"version"`
	Entries []ServiceExtractorRegistryEntry `json:"entries"`
}

// ServiceExtractorRegistry is immutable after construction. Entry returns a
// defensive copy so callers cannot alter registry state shared by a worker.
type ServiceExtractorRegistry struct {
	version string
	entries map[string]ServiceExtractorRegistryEntry
}

func (r *ServiceExtractorRegistry) Version() string { return r.version }

func (r *ServiceExtractorRegistry) Entry(scheme string) (ServiceExtractorRegistryEntry, bool) {
	entry, ok := r.entries[scheme]
	if !ok {
		return ServiceExtractorRegistryEntry{}, false
	}
	entry.RequiredSections = append([]string(nil), entry.RequiredSections...)
	return entry, true
}

func (r *ServiceExtractorRegistry) Schemes() []string {
	schemes := make([]string, 0, len(r.entries))
	for scheme := range r.entries {
		schemes = append(schemes, scheme)
	}
	sort.Strings(schemes)
	return schemes
}

var knownRegistryRateCardSelectors = registryKeySet(domain.CommercialSchemes())
var knownRegistryValidators = registryKeySet(domain.CommercialSchemes())
var knownRegistryMappers = registeredServiceDetailMapperKeys()

func registryKeySet(schemes []string) map[string]struct{} {
	keys := make(map[string]struct{}, len(schemes))
	for _, scheme := range schemes {
		keys[scheme] = struct{}{}
	}
	return keys
}

// LoadServiceExtractorRegistry parses and validates a versioned manifest. The
// optional phases are the Go-owned phase registrations for this pipeline; any
// overlapping ownership is a programmer/configuration error and fails startup.
func LoadServiceExtractorRegistry(raw []byte, phases ...Phase) (*ServiceExtractorRegistry, error) {
	var manifest serviceExtractorRegistryManifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("%w: malformed manifest", ErrServiceExtractorRegistry)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		// Reject a second JSON value without returning its contents.
		return nil, fmt.Errorf("%w: trailing manifest value", ErrServiceExtractorRegistry)
	}
	if err := validateServiceExtractorRegistry(manifest, phases); err != nil {
		return nil, err
	}
	entries := make(map[string]ServiceExtractorRegistryEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		entry.RequiredSections = append([]string(nil), entry.RequiredSections...)
		entries[entry.Scheme] = entry
	}
	return &ServiceExtractorRegistry{version: manifest.Version, entries: entries}, nil
}

func validateServiceExtractorRegistry(manifest serviceExtractorRegistryManifest, phases []Phase) error {
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("%w: missing version", ErrServiceExtractorRegistry)
	}
	seenSchemes := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if !domain.IsCommercialScheme(entry.Scheme) {
			return fmt.Errorf("%w: unknown scheme", ErrServiceExtractorRegistry)
		}
		if _, exists := seenSchemes[entry.Scheme]; exists {
			return fmt.Errorf("%w: duplicate scheme", ErrServiceExtractorRegistry)
		}
		seenSchemes[entry.Scheme] = struct{}{}
		if strings.TrimSpace(entry.Version) == "" {
			return fmt.Errorf("%w: missing entry version", ErrServiceExtractorRegistry)
		}
		if _, ok := knownRegistryRateCardSelectors[entry.RateCardSelectorKey]; !ok {
			return fmt.Errorf("%w: unknown rate-card selector", ErrServiceExtractorRegistry)
		}
		if _, ok := knownRegistryValidators[entry.ValidatorKey]; !ok {
			return fmt.Errorf("%w: unknown validator", ErrServiceExtractorRegistry)
		}
		if _, ok := knownRegistryMappers[entry.MapperKey]; !ok {
			return fmt.Errorf("%w: unknown mapper", ErrServiceExtractorRegistry)
		}
		if err := validateRegistrySections(entry.RequiredSections); err != nil {
			return err
		}
		for _, phase := range []RegistryPhaseConfig{entry.P2, entry.P3, entry.P4} {
			if err := validateRegistryPhase(phase); err != nil {
				return err
			}
		}
	}
	for _, scheme := range domain.CommercialSchemes() {
		if _, ok := seenSchemes[scheme]; !ok {
			return fmt.Errorf("%w: missing scheme", ErrServiceExtractorRegistry)
		}
	}
	return validatePhaseOwnership(phases)
}

func validateRegistrySections(sections []string) error {
	if len(sections) == 0 {
		return fmt.Errorf("%w: missing required sections", ErrServiceExtractorRegistry)
	}
	allowed := map[string]struct{}{"scope": {}, "staffing": {}, "timeline": {}, "payment": {}, "commercial": {}, "service_details": {}}
	seen := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		if _, ok := allowed[section]; !ok {
			return fmt.Errorf("%w: unknown required section", ErrServiceExtractorRegistry)
		}
		if _, duplicate := seen[section]; duplicate {
			return fmt.Errorf("%w: duplicate required section", ErrServiceExtractorRegistry)
		}
		seen[section] = struct{}{}
	}
	return nil
}

func validateRegistryPhase(phase RegistryPhaseConfig) error {
	if !phase.Enabled {
		return nil
	}
	if strings.TrimSpace(phase.PromptVersion) == "" || strings.TrimSpace(phase.SchemaVersion) == "" || !validRegistryAssetPath(phase.PromptAsset) || !validRegistryAssetPath(phase.SchemaAsset) {
		return fmt.Errorf("%w: invalid phase configuration", ErrServiceExtractorRegistry)
	}
	return nil
}

func validRegistryAssetPath(path string) bool {
	return strings.HasPrefix(path, "meeting_extraction_pipeline_v1/") && !strings.Contains(path, "..") && (strings.HasSuffix(path, ".txt") || strings.HasSuffix(path, ".json"))
}

func validatePhaseOwnership(phases []Phase) error {
	owners := make(map[FieldPath]PhaseKey)
	for _, phase := range phases {
		if phase == nil {
			return fmt.Errorf("%w: nil phase", ErrServiceExtractorRegistry)
		}
		for _, field := range phase.Owns() {
			if _, exists := owners[field]; exists {
				return fmt.Errorf("%w: ownership overlap", ErrServiceExtractorRegistry)
			}
			owners[field] = phase.Key()
		}
	}
	return nil
}

var (
	defaultServiceExtractorRegistryOnce sync.Once
	defaultServiceExtractorRegistry     *ServiceExtractorRegistry
	defaultServiceExtractorRegistryErr  error
)

func DefaultServiceExtractorRegistry() (*ServiceExtractorRegistry, error) {
	defaultServiceExtractorRegistryOnce.Do(func() {
		defaultServiceExtractorRegistry, defaultServiceExtractorRegistryErr = LoadServiceExtractorRegistry([]byte(GetSafetyEnvelope(meetingExtractionRegistryAsset)))
	})
	return defaultServiceExtractorRegistry, defaultServiceExtractorRegistryErr
}
