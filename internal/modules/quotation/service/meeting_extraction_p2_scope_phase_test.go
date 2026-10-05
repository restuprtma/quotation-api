package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestP2ScopeSchemaIsStrictBoundedAndContainsNoQuotes(t *testing.T) {
	phase := NewScopePhase()
	schema, err := phase.Schema(PhaseInput{})
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(schema)), "quote")

	var node any
	require.NoError(t, json.Unmarshal(schema, &node))
	assertP2SchemaBoundedAndStrict(t, node)
}

func TestP2ScopeValidationDedupesAndRejectsRawEvidenceFields(t *testing.T) {
	doc := mustP2Document(t, "Portal inventory\nMobile reporting")
	phase := NewScopePhase()
	in := p2Input(doc, []string{"L0001"})
	candidate := []byte(`{"scope":[{"feature":"Inventory portal","evidence":{"refs":["L0001"],"support":"explicit"}},{"feature":" inventory   portal ","evidence":{"refs":["L0001"],"support":"explicit"}}]}`)
	result, err := phase.Validate(context.Background(), in, candidate)
	require.NoError(t, err)
	scope := result.(ScopeResult)
	require.Len(t, scope.Scope, 1)
	require.Equal(t, []string{"L0001"}, scope.EvidenceRefs)
	persisted, err := scope.SafeResultJSON()
	require.NoError(t, err)
	require.NotContains(t, string(persisted), "quote")

	_, err = phase.Validate(context.Background(), in, []byte(`{"scope":[{"feature":"Inventory","evidence":{"refs":["L0001"],"support":"explicit","quote":"secret"}}]}`))
	require.Error(t, err)
}

func TestP2ScopeShortDocumentUsesFullLabelledContextAndOptionalSkip(t *testing.T) {
	doc := mustP2Document(t, "Inventory portal\nMobile reporting")
	phase := NewScopePhase()
	in := p2Input(doc, []string{"L0001"})
	skip, err := phase.ShouldSkip(in)
	require.NoError(t, err)
	require.False(t, skip)
	request, err := phase.BuildPrompt(context.Background(), in)
	require.NoError(t, err)
	require.Contains(t, request.Prompt, "[C001]")
	require.Contains(t, request.Prompt, "[L0001] Inventory portal")
	require.Contains(t, request.Prompt, "[L0002] Mobile reporting")

	skip, err = phase.ShouldSkip(p2Input(doc, nil))
	require.NoError(t, err)
	require.True(t, skip)
}

func TestP2ScopeLongDocumentRoutesReferencesWithNeighborAndFallback(t *testing.T) {
	doc, err := NewMeetingNoteDocument("one-one\ntwo-two\nthree\nfour-four\nfive-five", MeetingNoteDocumentConfig{MaxNotesRunes: 200, ChunkMaxRunes: 14, FullDocumentMaxRunes: 10})
	require.NoError(t, err)
	phase := NewScopePhase()
	request, err := phase.BuildPrompt(context.Background(), p2Input(doc, []string{"C002"}))
	require.NoError(t, err)
	require.Contains(t, request.Prompt, "[C001]")
	require.Contains(t, request.Prompt, "[C002]")
	require.Contains(t, request.Prompt, "[C003]")

	request, err = phase.BuildPrompt(context.Background(), p2Input(doc, []string{"C999"}))
	require.NoError(t, err)
	require.Contains(t, request.Prompt, "Routing warning")
	require.Contains(t, request.Prompt, "[C001]")
}

func p2Input(doc *MeetingNoteDocument, refs []string) PhaseInput {
	sections := "[]"
	if refs != nil {
		encoded, _ := json.Marshal([]PlanAvailableSection{{Section: "scope", Refs: refs}})
		sections = string(encoded)
	}
	return PhaseInput{Document: doc, Dependencies: map[string]json.RawMessage{
		string(PhaseKeyPlanMetadata): json.RawMessage(`{"scheme":"custom_development","confidence":0.9,"available_sections":` + sections + `,"metadata":{"client_company":null,"project_name":null,"pic_name":null,"client_name":null}}`),
	}}
}

func mustP2Document(t *testing.T, notes string) *MeetingNoteDocument {
	t.Helper()
	doc, err := NewMeetingNoteDocument(notes, MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	return doc
}

func assertP2SchemaBoundedAndStrict(t *testing.T, value any) {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	if object["type"] == "object" {
		require.Equal(t, false, object["additionalProperties"])
	}
	if object["type"] == "array" {
		require.NotNil(t, object["maxItems"])
	}
	if object["type"] == "string" {
		require.NotNil(t, object["maxLength"])
	}
	for _, child := range object {
		assertP2SchemaBoundedAndStrict(t, child)
	}
}
