package domain_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var arunikaFixturePath = filepath.Join("..", "service", "testdata", "prompt_first", "arunika-inventory.json")

func minimalPromptState() *domain.PromptQuotationState {
	return &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		Readiness:     domain.ReadinessIncomplete,
		MeetingNotes:  "Bu Nadia dari PT Arunika butuh aplikasi inventory.",
		ClientCompany: &domain.Fact[string]{
			Value:     "PT Arunika",
			Source:    domain.SourceMeeting,
			Evidence:  `"Bu Nadia dari PT Arunika"`,
			Confirmed: false,
		},
		ClientName: &domain.Fact[string]{
			Value:     "Nadia",
			Source:    domain.SourceMeeting,
			Evidence:  `"Bu Nadia dari PT Arunika"`,
			Confirmed: false,
		},
	}
}

func resolvedTeamLine() domain.PromptTeamLine {
	resolved := "venturo-std-2026.backend_programmer.intermediate"
	return domain.PromptTeamLine{
		RequestedRole:      "backend_programmer",
		ResolvedRateItemID: &resolved,
		Seniority:          "intermediate",
		HeadcountQuarters:  8,
		PeriodQuarters:     12,
		Source:             domain.SourceCatalog,
		Confirmed:          true,
	}
}

func confirmedPromptState() *domain.PromptQuotationState {
	confirmed := func(value string) *domain.Fact[string] {
		return &domain.Fact[string]{Value: value, Source: domain.SourceMeeting, Evidence: "meeting note", Confirmed: true}
	}
	months := 3
	return &domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		Readiness:        domain.ReadinessReadyForFinal,
		MeetingNotes:     "Bu Nadia dari PT Arunika butuh aplikasi inventory.",
		ClientName:       confirmed("Nadia"),
		ClientCompany:    confirmed("PT Arunika"),
		PICName:          confirmed("Nadia"),
		ProjectName:      confirmed("Arunika Inventory"),
		CommercialScheme: confirmed("dedicated_squad"),
		Scope:            []domain.Fact[string]{{Value: confirmed("Stok gudang").Value, Source: domain.SourceMeeting, Evidence: "meeting note", Confirmed: true}},
		DurationMonths:   &domain.Fact[int]{Value: months, Source: domain.SourceMeeting, Evidence: "meeting note", Confirmed: true},
		Team:             []domain.PromptTeamLine{resolvedTeamLine()},
		PaymentPattern:   confirmed("50:50"),
	}
}

func TestPromptQuotationValidate(t *testing.T) {
	assert.Equal(t, "VENTURO-STD-2026", domain.DefaultPromptRateCardCode)

	tests := []struct {
		name    string
		mutate  func(*domain.PromptQuotationState)
		wantErr string
	}{
		{
			name:    "valid minimal incomplete state",
			mutate:  func(state *domain.PromptQuotationState) {},
			wantErr: "",
		},
		{
			name:    "schema version is pinned",
			mutate:  func(state *domain.PromptQuotationState) { state.SchemaVersion = "prompt_quotation_v0" },
			wantErr: "schema_version",
		},
		{
			name:    "empty meeting notes are rejected",
			mutate:  func(state *domain.PromptQuotationState) { state.MeetingNotes = "  " },
			wantErr: "meeting_notes",
		},
		{
			name:    "meeting fact without evidence is rejected",
			mutate:  func(state *domain.PromptQuotationState) { state.ClientCompany.Evidence = "" },
			wantErr: "client_company.evidence",
		},
		{
			name:    "unknown value source is rejected",
			mutate:  func(state *domain.PromptQuotationState) { state.ClientCompany.Source = "guess" },
			wantErr: "client_company.source",
		},
		{
			name:    "empty fact value is rejected",
			mutate:  func(state *domain.PromptQuotationState) { state.ClientCompany.Value = " " },
			wantErr: "client_company.value",
		},
		{
			name: "zero month duration is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				state.DurationMonths = &domain.Fact[int]{Value: 0, Source: domain.SourceUser}
			},
			wantErr: "duration_months.value",
		},
		{
			name: "team line without requested role is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				line := resolvedTeamLine()
				line.RequestedRole = ""
				state.Team = []domain.PromptTeamLine{line}
			},
			wantErr: "team[0].requested_role",
		},
		{
			name: "team line without seniority is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				line := resolvedTeamLine()
				line.Seniority = ""
				state.Team = []domain.PromptTeamLine{line}
			},
			wantErr: "team[0].seniority",
		},
		{
			name: "team line with zero headcount is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				line := resolvedTeamLine()
				line.HeadcountQuarters = 0
				state.Team = []domain.PromptTeamLine{line}
			},
			wantErr: "team[0].headcount_quarters",
		},
		{
			name: "team line with unbounded period is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				line := resolvedTeamLine()
				line.PeriodQuarters = domain.MaxPromptPeriodQuarters + 1
				state.Team = []domain.PromptTeamLine{line}
			},
			wantErr: "team[0].period_quarters",
		},
		{
			name: "team line with blank resolved rate item id is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				line := resolvedTeamLine()
				blank := " "
				line.ResolvedRateItemID = &blank
				state.Team = []domain.PromptTeamLine{line}
			},
			wantErr: "team[0].resolved_rate_item_id",
		},
		{
			name: "team line meeting fact without evidence is rejected",
			mutate: func(state *domain.PromptQuotationState) {
				line := resolvedTeamLine()
				line.Source = domain.SourceMeeting
				line.Evidence = ""
				state.Team = []domain.PromptTeamLine{line}
			},
			wantErr: "team[0].evidence",
		},
		{
			name:    "unknown readiness is rejected",
			mutate:  func(state *domain.PromptQuotationState) { state.Readiness = "done" },
			wantErr: "readiness",
		},
		{
			name:    "ready_for_final with a blocking issue is rejected",
			mutate:  func(state *domain.PromptQuotationState) { state.Readiness = domain.ReadinessReadyForFinal },
			wantErr: "readiness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := minimalPromptState()
			tt.mutate(state)
			err := state.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestPromptQuotationReadinessGate(t *testing.T) {
	t.Run("missing pricing inputs keep the state incomplete", func(t *testing.T) {
		state := minimalPromptState()
		require.Equal(t, domain.ReadinessIncomplete, state.ComputeReadiness())
		assert.Equal(t, []string{
			"commercial_scheme.missing",
			"duration_months.missing",
			"team.empty",
		}, blockingCodes(state.ComputeIssues()))
	})

	t.Run("missing client company is a blocking issue", func(t *testing.T) {
		state := minimalPromptState()
		state.ClientCompany = nil
		require.Equal(t, domain.ReadinessIncomplete, state.ComputeReadiness())
		assert.Contains(t, blockingCodes(state.ComputeIssues()), "client_company.missing")
	})

	t.Run("missing pic name is only a warning issue", func(t *testing.T) {
		state := confirmedPromptState()
		state.PICName = nil
		require.Equal(t, domain.ReadinessPriceable, state.ComputeReadiness())
		assert.NotContains(t, blockingCodes(state.ComputeIssues()), "pic_name.missing")
		var warningCodes []string
		for _, issue := range state.ComputeIssues() {
			if issue.Severity == domain.IssueWarning {
				warningCodes = append(warningCodes, issue.Code)
			}
		}
		assert.Contains(t, warningCodes, "pic_name.missing")
	})

	t.Run("unresolved team role keeps the state incomplete", func(t *testing.T) {
		state := minimalPromptState()
		state.DurationMonths = &domain.Fact[int]{Value: 3, Source: domain.SourceMeeting, Evidence: "meeting note"}
		state.CommercialScheme = &domain.Fact[string]{Value: "dedicated_squad", Source: domain.SourceSuggestion, Evidence: "default squad scheme"}
		state.Team = []domain.PromptTeamLine{{
			RequestedRole:     "programmer",
			Seniority:         "intermediate",
			HeadcountQuarters: 8,
			PeriodQuarters:    12,
			Source:            domain.SourceMeeting,
			Evidence:          "dua programmer intermediate",
		}}
		state.PaymentPattern = &domain.Fact[string]{Value: "50:50", Source: domain.SourceMeeting, Evidence: "meeting note"}
		require.Equal(t, domain.ReadinessIncomplete, state.ComputeReadiness())
		assert.Equal(t, []string{"team.rate_unresolved"}, blockingCodes(state.ComputeIssues()))
		state.Readiness = domain.ReadinessReadyForFinal
		require.ErrorContains(t, state.Validate(), "readiness")
	})

	t.Run("unconfirmed facts are priceable but not final", func(t *testing.T) {
		state := confirmedPromptState()
		state.Readiness = domain.ReadinessPriceable
		state.ClientCompany.Confirmed = false
		assert.Equal(t, domain.ReadinessPriceable, state.ComputeReadiness())
		require.NoError(t, state.Validate())
	})

	t.Run("confirmed complete state is ready for final", func(t *testing.T) {
		state := confirmedPromptState()
		assert.Empty(t, state.ComputeIssues())
		assert.Equal(t, domain.ReadinessReadyForFinal, state.ComputeReadiness())
		require.NoError(t, state.Validate())
	})
}

func blockingCodes(issues []domain.DraftIssue) []string {
	codes := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			codes = append(codes, issue.Code)
		}
	}
	return codes
}

func blockingIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	filtered := make([]domain.DraftIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

func issueTriples(issues []domain.DraftIssue) []string {
	triples := make([]string, 0, len(issues))
	for _, issue := range issues {
		triples = append(triples, string(issue.Severity)+"|"+issue.Code+"|"+issue.Field)
	}
	return triples
}

func TestMoneyInputValidate(t *testing.T) {
	amount := int64(150_000_000)

	t.Run("nil amount is a blocking issue", func(t *testing.T) {
		err := domain.MoneyInput{Source: domain.SourceCatalog}.Validate()
		require.Error(t, err)
		var validation *domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Equal(t, domain.IssueBlocking, validation.Issue.Severity)
		assert.Equal(t, "amount", validation.Issue.Field)
	})

	t.Run("negative amount is a blocking issue", func(t *testing.T) {
		negative := int64(-1)
		err := domain.MoneyInput{Amount: &negative, Source: domain.SourceUser}.Validate()
		require.Error(t, err)
		var validation *domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Equal(t, domain.IssueBlocking, validation.Issue.Severity)
	})

	t.Run("catalog source without reference is a blocking issue", func(t *testing.T) {
		err := domain.MoneyInput{Amount: &amount, Source: domain.SourceCatalog}.Validate()
		require.Error(t, err)
		var validation *domain.ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Equal(t, "catalog_ref", validation.Issue.Field)
	})

	t.Run("recorded zero amount is accepted", func(t *testing.T) {
		zero := int64(0)
		assert.NoError(t, domain.MoneyInput{Amount: &zero, Source: domain.SourceUser}.Validate())
	})

	t.Run("priced catalog input is accepted", func(t *testing.T) {
		ref := "venturo-std-2026.backend_programmer.intermediate"
		assert.NoError(t, domain.MoneyInput{Amount: &amount, Source: domain.SourceCatalog, CatalogRef: &ref}.Validate())
	})
}

func TestArunikaFixtureRoundtrip(t *testing.T) {
	raw, err := os.ReadFile(arunikaFixturePath)
	require.NoError(t, err)

	state := &domain.PromptQuotationState{}
	require.NoError(t, json.Unmarshal(raw, state))
	require.NoError(t, state.Validate())

	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	roundtripped := map[string]any{}
	require.NoError(t, json.Unmarshal(encoded, &roundtripped))

	fixture := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	assert.Equal(t, fixture, roundtripped)

	expectedKeys := []string{
		"schema_version", "meeting_notes", "client_name", "client_company", "pic_name",
		"project_name", "scope", "duration_months", "team", "payment_pattern", "issues",
		"readiness",
	}
	assert.ElementsMatch(t, expectedKeys, mapKeys(fixture))

	assert.Equal(t, state.Readiness, state.ComputeReadiness())
	assert.ElementsMatch(t, issueTriples(blockingIssues(state.Issues)), issueTriples(blockingIssues(state.ComputeIssues())))
	assert.Subset(t, issueTriples(state.Issues), issueTriples(state.ComputeIssues()))

	assertNoCommercialValues(t, fixture, "arunika-inventory.json")
}

func TestArunikaFixtureSurfacesUnresolvedProgrammerRole(t *testing.T) {
	raw, err := os.ReadFile(arunikaFixturePath)
	require.NoError(t, err)

	state := &domain.PromptQuotationState{}
	require.NoError(t, json.Unmarshal(raw, state))

	require.Len(t, state.Team, 1)
	assert.Equal(t, "programmer", state.Team[0].RequestedRole)
	assert.Nil(t, state.Team[0].ResolvedRateItemID)
	assert.Equal(t, domain.ReadinessIncomplete, state.ComputeReadiness())

	fields := make([]string, 0, len(state.Issues))
	for _, issue := range state.Issues {
		if issue.Severity == domain.IssueBlocking {
			fields = append(fields, issue.Field)
		}
	}
	assert.Contains(t, fields, "team[0].resolved_rate_item_id")

	assert.Equal(t, domain.ReadinessIncomplete, state.Readiness)
	assert.NoError(t, state.Validate())
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

var commercialKeys = map[string]bool{
	"amount": true, "amount_per_unit": true, "discount": true, "line_total": true,
	"price": true, "prices": true, "rate": true, "rates": true, "subtotal": true,
	"tax": true, "total": true, "unit_price": true,
}

func assertNoCommercialValues(t *testing.T, value any, path string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if commercialKeys[key] {
				t.Errorf("fixture must not carry a commercial value at %s.%s", path, key)
			}
			assertNoCommercialValues(t, child, fmt.Sprintf("%s.%s", path, key))
		}
	case []any:
		for index, child := range typed {
			assertNoCommercialValues(t, child, fmt.Sprintf("%s[%d]", path, index))
		}
	}
}
