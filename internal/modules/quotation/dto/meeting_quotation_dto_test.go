package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestPromptQuotationStateView_RedactsMeetingNotes(t *testing.T) {
	state := domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		MeetingNotes:  "CANARY-MEETING-NOTES-MUST-NOT-LEAK",
		Scope:         []domain.Fact[string]{},
		Team:          []domain.PromptTeamLine{},
		Issues:        []domain.DraftIssue{},
		Readiness:     domain.ReadinessIncomplete,
	}

	body, err := json.Marshal(NewPromptQuotationStateView(state))
	require.NoError(t, err)
	require.NotContains(t, string(body), "meeting_notes")
	require.NotContains(t, string(body), "CANARY-MEETING-NOTES-MUST-NOT-LEAK")
}
