package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeExtractionRejectsOwnershipCollision(t *testing.T) {
	_, err := MergeExtraction(MergeExtractionInput{Phases: []Phase{
		mergeTestPhase{key: "one", owns: []FieldPath{"duration_months"}},
		mergeTestPhase{key: "two", owns: []FieldPath{"duration_months"}},
	}})
	require.ErrorIs(t, err, ErrMergeOwnershipCollision)
}

func TestMergeExtractionUsesDeterministicPrecedence(t *testing.T) {
	result, err := MergeExtraction(MergeExtractionInput{
		Candidates: []MergeCandidate{
			{Path: "client_company", Value: "Default Co", Source: MergeSourceDefault},
			{Path: "client_company", Value: "Extracted Co", Source: MergeSourceExtracted, EvidenceValid: true},
			{Path: "client_company", Value: "User Co", Source: MergeSourceUser},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "User Co", result.Values["client_company"].Value)
	require.Equal(t, MergeSourceUser, result.Values["client_company"].Provenance.Source)
}

func TestMergeExtractionDropsInvalidEvidenceAndRequestsOneRepair(t *testing.T) {
	result, err := MergeExtraction(MergeExtractionInput{Candidates: []MergeCandidate{{
		Path: "client_company", Value: "Unsupported", Source: MergeSourceExtracted, Required: true,
		EvidenceValid: false,
	}}})
	require.NoError(t, err)
	require.NotContains(t, result.Values, FieldPath("client_company"))
	require.Equal(t, []FieldPath{"client_company"}, result.RepairFields)
	require.Contains(t, result.WarningCodes(), "EVIDENCE_MISMATCH")

	result, err = MergeExtraction(MergeExtractionInput{Candidates: []MergeCandidate{{
		Path: "client_company", Value: "Unsupported", Source: MergeSourceExtracted, Required: true,
		EvidenceValid: false,
	}}, RepairAttempts: map[FieldPath]int{"client_company": 1}})
	require.NoError(t, err)
	require.Empty(t, result.RepairFields)
	require.True(t, result.NeedsInput)
}

func TestNewExtractedTextMergeCandidateUsesEvidenceChecker(t *testing.T) {
	doc, err := NewMeetingNoteDocument("Acme\n", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	evidence := MeetingEvidence{
		Refs:             []string{"C001", "L0001"},
		StartOffset:      0,
		EndOffset:        4,
		NormalizedSHA256: normalizedEvidenceHash("Acme"),
		Support:          EvidenceSupportExplicit,
	}
	candidate := NewExtractedTextMergeCandidate(NewMeetingEvidenceChecker(doc), "client_company", PhaseKeyPlanMetadata, "Different", evidence, true)
	require.False(t, candidate.EvidenceValid)
	result, err := MergeExtraction(MergeExtractionInput{Candidates: []MergeCandidate{candidate}})
	require.NoError(t, err)
	require.Equal(t, []FieldPath{"client_company"}, result.RepairFields)

	candidate = NewExtractedTextMergeCandidate(NewMeetingEvidenceChecker(doc), "client_company", PhaseKeyPlanMetadata, "Acme", evidence, true)
	require.True(t, candidate.EvidenceValid)
	require.Contains(t, candidate.EvidenceToken, "ev1:")
}

func TestMergeExtractionConflictNullsCanonicalValueAndUsesSafeCandidates(t *testing.T) {
	result, err := MergeExtraction(MergeExtractionInput{
		Candidates: []MergeCandidate{{Path: "duration_months", Value: 6, Source: MergeSourceExtracted, EvidenceValid: true}},
		Consistency: []MergeConsistencyObservation{{
			Path: "duration_months", Value: 12, Source: MergeSourceExtracted, EvidenceValid: true,
		}},
	})
	require.NoError(t, err)
	require.NotContains(t, result.Values, FieldPath("duration_months"))
	require.True(t, result.NeedsInput)
	require.Equal(t, "CONFLICT", result.Warnings[0].Code)
	require.Equal(t, "duration_months", string(result.Warnings[0].Field))
	require.Len(t, result.Warnings[0].Candidates, 2)
	for _, candidate := range result.Warnings[0].Candidates {
		require.Empty(t, candidate.SourceText)
	}
}

func TestMergeExtractionUserValueIsNotOverriddenByConsistencyObservation(t *testing.T) {
	result, err := MergeExtraction(MergeExtractionInput{
		Candidates: []MergeCandidate{
			{Path: "duration_months", Value: 6, Source: MergeSourceExtracted, EvidenceValid: true},
			{Path: "duration_months", Value: 9, Source: MergeSourceUser},
		},
		Consistency: []MergeConsistencyObservation{{Path: "duration_months", Value: 12, Source: MergeSourceExtracted, EvidenceValid: true}},
	})
	require.NoError(t, err)
	require.Equal(t, 9, result.Values["duration_months"].Value)
	require.False(t, result.NeedsInput)
}

type mergeTestPhase struct {
	key  PhaseKey
	owns []FieldPath
}

func (p mergeTestPhase) Key() PhaseKey     { return p.key }
func (p mergeTestPhase) Owns() []FieldPath { return p.owns }
func (mergeTestPhase) BuildPrompt(_ context.Context, _ PhaseInput) (ProviderRequest, error) {
	return ProviderRequest{}, errors.New("not used")
}
func (mergeTestPhase) Schema(PhaseInput) (json.RawMessage, error) { return nil, errors.New("not used") }
func (mergeTestPhase) Validate(context.Context, PhaseInput, json.RawMessage) (PhaseResult, error) {
	return nil, errors.New("not used")
}
