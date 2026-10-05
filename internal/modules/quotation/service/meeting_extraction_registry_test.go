package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestServiceExtractorRegistryEmbeddedManifestMatchesDomainSchemes(t *testing.T) {
	registry, err := DefaultServiceExtractorRegistry()
	require.NoError(t, err)
	require.Equal(t, "meeting_extraction_registry_v1", registry.Version())
	require.Equal(t, sortedStrings(domain.CommercialSchemes()), registry.Schemes())

	for _, scheme := range domain.CommercialSchemes() {
		entry, ok := registry.Entry(scheme)
		require.True(t, ok)
		require.Equal(t, scheme, entry.RateCardSelectorKey)
		require.Equal(t, scheme, entry.ValidatorKey)
		require.Equal(t, scheme, entry.MapperKey)
		require.NotEmpty(t, entry.RequiredSections)
		require.True(t, entry.P2.Enabled)
		require.True(t, entry.P4.Enabled)
	}
}

func TestServiceExtractorRegistryRejectsMissingDuplicateAndUnknownSchemes(t *testing.T) {
	manifest := mustRegistryManifest(t)
	manifest.Entries = manifest.Entries[1:]
	_, err := loadRegistryManifest(t, manifest)
	require.ErrorIs(t, err, ErrServiceExtractorRegistry)

	manifest = mustRegistryManifest(t)
	manifest.Entries = append(manifest.Entries, manifest.Entries[0])
	_, err = loadRegistryManifest(t, manifest)
	require.ErrorIs(t, err, ErrServiceExtractorRegistry)

	manifest = mustRegistryManifest(t)
	manifest.Entries[0].Scheme = "unregistered_scheme"
	_, err = loadRegistryManifest(t, manifest)
	require.ErrorIs(t, err, ErrServiceExtractorRegistry)
}

func TestServiceExtractorRegistryRejectsUnknownRegistrationKeysAndBadAssetPaths(t *testing.T) {
	manifest := mustRegistryManifest(t)
	manifest.Entries[0].ValidatorKey = "not_registered"
	_, err := loadRegistryManifest(t, manifest)
	require.ErrorIs(t, err, ErrServiceExtractorRegistry)

	manifest = mustRegistryManifest(t)
	manifest.Entries[0].P4.SchemaAsset = "../outside.json"
	_, err = loadRegistryManifest(t, manifest)
	require.ErrorIs(t, err, ErrServiceExtractorRegistry)
}

func TestServiceExtractorRegistryRejectsOverlappingPhaseOwnership(t *testing.T) {
	manifest := mustRegistryManifest(t)
	_, err := loadRegistryManifest(t, manifest,
		registryTestPhase{key: "scope", owns: []FieldPath{"scope"}},
		registryTestPhase{key: "service", owns: []FieldPath{"scope"}},
	)
	require.ErrorIs(t, err, ErrServiceExtractorRegistry)
}

func TestServiceExtractorRegistryEntryIsDefensiveCopy(t *testing.T) {
	registry, err := DefaultServiceExtractorRegistry()
	require.NoError(t, err)
	entry, ok := registry.Entry(domain.CommercialSchemeDedicatedSquad)
	require.True(t, ok)
	entry.RequiredSections[0] = "changed"
	fresh, ok := registry.Entry(domain.CommercialSchemeDedicatedSquad)
	require.True(t, ok)
	require.NotEqual(t, "changed", fresh.RequiredSections[0])
}

func mustRegistryManifest(t *testing.T) serviceExtractorRegistryManifest {
	t.Helper()
	raw := GetSafetyEnvelope(meetingExtractionRegistryAsset)
	var manifest serviceExtractorRegistryManifest
	require.NoError(t, json.Unmarshal([]byte(raw), &manifest))
	return manifest
}

func loadRegistryManifest(t *testing.T, manifest serviceExtractorRegistryManifest, phases ...Phase) (*ServiceExtractorRegistry, error) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	return LoadServiceExtractorRegistry(raw, phases...)
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

type registryTestPhase struct {
	key  PhaseKey
	owns []FieldPath
}

func (p registryTestPhase) Key() PhaseKey     { return p.key }
func (p registryTestPhase) Owns() []FieldPath { return p.owns }
func (registryTestPhase) BuildPrompt(context.Context, PhaseInput) (ProviderRequest, error) {
	return ProviderRequest{}, nil
}
func (registryTestPhase) Schema(PhaseInput) (json.RawMessage, error) { return nil, nil }
func (p registryTestPhase) Validate(context.Context, PhaseInput, json.RawMessage) (PhaseResult, error) {
	return nil, nil
}
