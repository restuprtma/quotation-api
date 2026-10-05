package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestPlanPhaseP1SchemaAndValidation(t *testing.T) {
	doc, err := NewMeetingNoteDocument("PT Example\nProject Portal\nPIC: Ana", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	phase := NewPlanMetadataPhase("")
	in := PhaseInput{Document: doc, PipelineVersion: "pipeline-v1", PromptVersion: "p1", SchemaVersion: "p1-schema"}

	schema, err := phase.Schema(in)
	require.NoError(t, err)
	for _, scheme := range append(domain.CommercialSchemes(), "unknown", "multi_service") {
		require.Contains(t, string(schema), scheme)
	}
	require.Contains(t, string(schema), `"additionalProperties":false`)
	require.NotContains(t, string(schema), "quote")

	candidate := []byte(`{"scheme":"dedicated_squad","confidence":0.91,"available_sections":[{"section":"metadata","refs":["L0001"]}],"metadata":{"client_company":{"value":"PT Example","evidence":{"refs":["L0001"],"support":"explicit"}},"project_name":{"value":"Portal","evidence":{"refs":["L0002"],"support":"explicit"}},"pic_name":null,"client_name":null}}`)
	result, err := phase.Validate(context.Background(), in, candidate)
	require.NoError(t, err)
	plan := result.(PlanMetadataResult)
	require.Equal(t, "dedicated_squad", plan.Scheme)
	require.Equal(t, "PT Example", plan.Metadata.ClientCompany.Value)
	require.Equal(t, []string{"L0001", "L0002"}, plan.EvidenceRefs)

	_, err = phase.Validate(context.Background(), in, []byte(strings.Replace(string(candidate), `"dedicated_squad"`, `"not-a-scheme"`, 1)))
	require.Error(t, err)
	_, err = phase.Validate(context.Background(), in, []byte(strings.Replace(string(candidate), `"support":"explicit"`, `"support":"explicit","quote":"secret"`, 1)))
	require.Error(t, err)
}

func TestPlanPhaseP1ManualSchemeOmitsClassificationAndRetainsMetadata(t *testing.T) {
	doc, err := NewMeetingNoteDocument("PT Example", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	phase := NewPlanMetadataPhase(domain.CommercialSchemeWhiteLabel)
	in := PhaseInput{Document: doc}
	schema, err := phase.Schema(in)
	require.NoError(t, err)
	require.NotContains(t, string(schema), `"scheme"`)
	require.NotContains(t, string(schema), `"confidence"`)

	result, err := phase.Validate(context.Background(), in, []byte(`{"available_sections":[{"section":"metadata","refs":["L0001"]}],"metadata":{"client_company":{"value":"PT Example","evidence":{"refs":["L0001"],"support":"explicit"}},"project_name":null,"pic_name":null,"client_name":null}}`))
	require.NoError(t, err)
	plan := result.(PlanMetadataResult)
	require.Equal(t, domain.CommercialSchemeWhiteLabel, plan.Scheme)
	require.Equal(t, PlanSchemeSourceManual, plan.SchemeSource)

	_, err = phase.Validate(context.Background(), in, []byte(`{"scheme":"dedicated_squad","available_sections":[],"metadata":{"client_company":null,"project_name":null,"pic_name":null,"client_name":null}}`))
	require.Error(t, err)
}

func TestPlanPhaseP1ManualSchemeFromInputUsesMetadataOnlyContract(t *testing.T) {
	doc, err := NewMeetingNoteDocument("PT Example", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	phase := NewPlanMetadataPhase("")
	in := PhaseInput{Document: doc, ManualScheme: domain.CommercialSchemeWhiteLabel}
	schema, err := phase.Schema(in)
	require.NoError(t, err)
	require.NotContains(t, string(schema), `"scheme"`)
	prompt, err := phase.BuildPrompt(context.Background(), in)
	require.NoError(t, err)
	require.Contains(t, prompt.Prompt, "Do not classify a scheme")
	result, err := phase.Validate(context.Background(), in, []byte(`{"available_sections":[],"metadata":{"client_company":null,"project_name":null,"pic_name":null,"client_name":null}}`))
	require.NoError(t, err)
	require.Equal(t, domain.CommercialSchemeWhiteLabel, result.(PlanMetadataResult).Scheme)
}

func TestPlanPhaseP1UserHintsAndReviewDisposition(t *testing.T) {
	doc, err := NewMeetingNoteDocument("notes", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	phase := NewPlanMetadataPhase("")
	hint := "User Company"
	in := PhaseInput{Document: doc, UserHints: PlanUserHints{ClientCompany: &hint}}
	candidate := []byte(`{"scheme":"unknown","confidence":0.2,"available_sections":[],"metadata":{"client_company":{"value":"Provider Company","evidence":{"refs":["L0001"],"support":"explicit"}},"project_name":null,"pic_name":null,"client_name":null}}`)
	result, err := phase.Validate(context.Background(), in, candidate)
	require.NoError(t, err)
	plan := result.(PlanMetadataResult)
	require.Equal(t, hint, plan.Metadata.ClientCompany.Value)
	require.Equal(t, PlanSchemeSourceUser, plan.Metadata.ClientCompany.Source)
	require.Equal(t, PlanDispositionNeedsReview, plan.Disposition)
	require.False(t, plan.ContinuePipeline())
}

func TestPlanPhaseP1FingerprintChangesWithEveryInput(t *testing.T) {
	doc, err := NewMeetingNoteDocument("notes", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	base := PhaseInput{Document: doc, Dependencies: map[string]json.RawMessage{"x": json.RawMessage(`{"ok":true}`)}, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "e"}
	baseFP := InputFingerprint(PhaseKeyPlanMetadata, base)
	variants := []PhaseInput{
		{Document: mustPlanDocument(t, "changed"), Dependencies: base.Dependencies, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "e"},
		{Document: doc, Dependencies: map[string]json.RawMessage{"x": json.RawMessage(`{"ok":false}`)}, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "e"},
		{Document: doc, Dependencies: base.Dependencies, PipelineVersion: "z", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "e"},
		{Document: doc, Dependencies: base.Dependencies, PipelineVersion: "a", PromptVersion: "z", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "e"},
		{Document: doc, Dependencies: base.Dependencies, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "z", CatalogSnapshotHash: "d", RegistryEntryVersion: "e"},
		{Document: doc, Dependencies: base.Dependencies, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "z", RegistryEntryVersion: "e"},
		{Document: doc, Dependencies: base.Dependencies, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "z"},
		{Document: doc, Dependencies: base.Dependencies, PipelineVersion: "a", PromptVersion: "b", SchemaVersion: "c", CatalogSnapshotHash: "d", RegistryEntryVersion: "e", ManualScheme: domain.CommercialSchemeDedicatedSquad},
	}
	for _, variant := range variants {
		require.NotEqual(t, baseFP, InputFingerprint(PhaseKeyPlanMetadata, variant))
	}
	hint := "client override"
	withHint := base
	withHint.UserHints.ClientName = &hint
	require.NotEqual(t, baseFP, InputFingerprint(PhaseKeyPlanMetadata, withHint))
}

func TestInputFingerprintCanonicalizesPersistedJSONWhitespace(t *testing.T) {
	doc, err := NewMeetingNoteDocument("notes", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	compact := PhaseInput{Document: doc, Dependencies: map[string]json.RawMessage{
		"plan_metadata": json.RawMessage(`{"scheme":"dedicated_squad","confidence":0.95}`),
	}}
	jsonbReloaded := compact
	jsonbReloaded.Dependencies = map[string]json.RawMessage{
		"plan_metadata": json.RawMessage(`{"scheme": "dedicated_squad", "confidence": 0.95}`),
	}
	require.Equal(t, InputFingerprint(PhaseKeyScope, compact), InputFingerprint(PhaseKeyScope, jsonbReloaded))
}

func mustPlanDocument(t *testing.T, notes string) *MeetingNoteDocument {
	t.Helper()
	d, err := NewMeetingNoteDocument(notes, MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	return d
}
