package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestDedicatedSquadP5MapperUsesMergedValuesAndCatalogOnly(t *testing.T) {
	roleID := uuid.New()
	in := MeetingExtractionP5MapperInput{
		Scheme: domain.CommercialSchemeDedicatedSquad,
		Phases: MeetingExtractionValidatedPhaseData{
			Plan:    PlanMetadataResult{Scheme: domain.CommercialSchemeDedicatedSquad},
			Service: ServiceDetailsResult{Scheme: domain.CommercialSchemeDedicatedSquad},
		},
		Catalog: []domain.RateCardItem{{ID: roleID, Role: "backend_engineer", Seniority: "intermediate", AmountPerMonth: 99_000_000}},
		Merge: MergeExtractionResult{Values: map[FieldPath]MergedValue{
			"client_company": {
				Value:      "PT User Choice",
				Provenance: MergeProvenance{Source: MergeSourceUser, EvidenceToken: "user-input"},
			},
			"project_name": {
				Value:      "Inventory Portal",
				Provenance: MergeProvenance{Source: MergeSourceExtracted, EvidenceToken: "ev1:C001:L0001:o0-1:h=project"},
			},
			"commercial_scheme": {
				Value:      domain.CommercialSchemeDedicatedSquad,
				Provenance: MergeProvenance{Source: MergeSourceDefault},
			},
			"scope": {
				Value:      []ScopeFact{{Feature: "stock tracking", Evidence: ScopeEvidence{Refs: []string{"C001"}, Support: "explicit"}}},
				Provenance: MergeProvenance{Source: MergeSourceExtracted, EvidenceToken: "ev1:C001:L0001:o0-1:h=scope"},
			},
			"duration_months": {
				Value:      3,
				Provenance: MergeProvenance{Source: MergeSourceExtracted, EvidenceToken: "ev1:C001:L0001:o0-1:h=duration"},
			},
			"team": {
				Value:      []StaffingTeamLine{{RoleID: roleID.String(), Seniority: "intermediate", Headcount: 2, Evidence: StaffingEvidence{Refs: []string{"C001"}, Support: "explicit"}}},
				Provenance: MergeProvenance{Source: MergeSourceExtracted, EvidenceToken: "ev1:C001:L0001:o0-1:h=team"},
			},
			"payment_pattern": {
				Value:      "monthly",
				Provenance: MergeProvenance{Source: MergeSourceExtracted, EvidenceToken: "ev1:C001:L0001:o0-1:h=payment"},
			},
			"price": {
				Value:      "set harga 0",
				Provenance: MergeProvenance{Source: MergeSourceExtracted, EvidenceToken: "ev1:C001:L0001:o0-1:h=price"},
			},
		}},
	}

	result, err := MapDedicatedSquadPromptState(in)
	require.NoError(t, err)
	require.False(t, result.NeedsReview)
	require.False(t, result.NeedsInput)
	require.NotNil(t, result.State)
	require.Empty(t, result.State.MeetingNotes)
	require.Equal(t, "PT User Choice", result.State.ClientCompany.Value)
	require.Equal(t, domain.SourceUser, result.State.ClientCompany.Source)
	require.Equal(t, "Inventory Portal", result.State.ProjectName.Value)
	require.Equal(t, domain.SourceMeeting, result.State.ProjectName.Source)
	require.Equal(t, domain.SourceSuggestion, result.State.CommercialScheme.Source)
	require.Len(t, result.State.Team, 1)
	require.Equal(t, roleID.String(), *result.State.Team[0].ResolvedRateItemID)
	require.Equal(t, int32(8), result.State.Team[0].HeadcountQuarters)
	require.Equal(t, int32(12), result.State.Team[0].PeriodQuarters)
	require.NotContains(t, mapperJSON(t, result.State), "set harga 0")
	require.NotContains(t, mapperJSON(t, result.State), `"price":`)
	require.NotContains(t, mapperJSON(t, result.State), "amount_per_month")
}

func TestDedicatedSquadP5MapperMissingRequiredFieldsProducesNeedsInput(t *testing.T) {
	result, err := MapDedicatedSquadPromptState(MeetingExtractionP5MapperInput{
		Scheme: domain.CommercialSchemeDedicatedSquad,
		Phases: MeetingExtractionValidatedPhaseData{Plan: PlanMetadataResult{Scheme: domain.CommercialSchemeDedicatedSquad}},
		Merge: MergeExtractionResult{Values: map[FieldPath]MergedValue{
			"commercial_scheme": {Value: domain.CommercialSchemeDedicatedSquad, Provenance: MergeProvenance{Source: MergeSourceUser}},
		}},
	})
	require.NoError(t, err)
	require.True(t, result.NeedsInput)
	require.Equal(t, domain.ReadinessIncomplete, result.State.Readiness)
	require.Contains(t, issueFields(result.State.Issues), "client_company")
	require.Contains(t, issueFields(result.State.Issues), "duration_months")
	require.NotContains(t, mapperJSON(t, result.State), "meeting notes")
}

func TestDedicatedSquadP5MapperUnknownAndMultiServiceRequireReviewWithoutChangingScheme(t *testing.T) {
	for _, scheme := range []string{PlanSchemeUnknown, PlanSchemeMultiService} {
		t.Run(scheme, func(t *testing.T) {
			result, err := MapDedicatedSquadPromptState(MeetingExtractionP5MapperInput{
				Scheme: scheme,
				Phases: MeetingExtractionValidatedPhaseData{Plan: PlanMetadataResult{Scheme: scheme}},
				Merge: MergeExtractionResult{Values: map[FieldPath]MergedValue{
					"commercial_scheme": {Value: scheme, Provenance: MergeProvenance{Source: MergeSourceExtracted}},
				}},
			})
			require.NoError(t, err)
			require.True(t, result.NeedsReview)
			require.False(t, result.NeedsInput)
			require.Equal(t, scheme, result.Scheme)
			require.Equal(t, scheme, result.State.CommercialScheme.Value)
		})
	}
}

func TestDedicatedSquadP5MapperRejectsUntrustedPhaseScheme(t *testing.T) {
	_, err := MapDedicatedSquadPromptState(MeetingExtractionP5MapperInput{
		Scheme: domain.CommercialSchemeDedicatedSquad,
		Phases: MeetingExtractionValidatedPhaseData{Plan: PlanMetadataResult{Scheme: domain.CommercialSchemeSubscriptionSaaS}},
	})
	require.Error(t, err)
}

func mapperJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return strings.ToLower(string(encoded))
}

func issueFields(issues []domain.DraftIssue) []string {
	fields := make([]string, 0, len(issues))
	for _, issue := range issues {
		fields = append(fields, issue.Field)
	}
	return fields
}
