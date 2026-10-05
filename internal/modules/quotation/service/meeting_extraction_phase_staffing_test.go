package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestStaffingPhaseP3NarrowedCatalogAndNoMoney(t *testing.T) {
	frontendID := uuid.New()
	phase := NewStaffingPhase([]domain.RateCardItem{{ID: frontendID, Role: "frontend_programmer", Seniority: "senior", AmountPerMonth: 99_000_000}})
	doc, err := NewMeetingNoteDocument("Need one senior frontend developer for six months.", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	in := PhaseInput{Document: doc}
	require.Equal(t, []FieldPath{"duration_months", "team"}, phase.Owns())

	schema, err := phase.Schema(in)
	require.NoError(t, err)
	require.Contains(t, string(schema), frontendID.String())
	for _, forbidden := range []string{"rate", "amount", "total", "price", "nominal", "discount"} {
		require.NotContains(t, strings.ToLower(string(schema)), forbidden)
	}
	prompt, err := phase.BuildPrompt(context.Background(), in)
	require.NoError(t, err)
	for _, forbidden := range []string{"rate", "amount", "total", "price", "nominal", "discount", "99000000"} {
		require.NotContains(t, strings.ToLower(prompt.Prompt), forbidden)
	}

	candidate := []byte(`{"duration_months":{"value":6,"evidence":{"refs":["L0001"],"support":"explicit"}},"team":[{"role_id":"` + frontendID.String() + `","seniority":"senior","headcount":1,"evidence":{"refs":["L0001"],"support":"explicit"}}]}`)
	result, err := phase.Validate(context.Background(), in, candidate)
	require.NoError(t, err)
	staffing := result.(StaffingResult)
	require.Equal(t, 6, staffing.DurationMonths.Value)
	require.Equal(t, frontendID.String(), staffing.Team[0].RoleID)
	require.Equal(t, []string{"L0001"}, staffing.EvidenceReferenceIDs())
	persisted, err := staffing.SafeResultJSON()
	require.NoError(t, err)
	for _, forbidden := range []string{"rate", "amount", "total", "price", "nominal", "discount", "99000000"} {
		require.NotContains(t, strings.ToLower(string(persisted)), forbidden)
	}
}

func TestStaffingPhaseP3RejectsForgedRoleAndSeniorityMismatch(t *testing.T) {
	frontendID := uuid.New()
	phase := NewStaffingPhase([]domain.RateCardItem{{ID: frontendID, Role: "frontend_programmer", Seniority: "senior"}})
	doc, err := NewMeetingNoteDocument("Need one frontend developer.", MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	in := PhaseInput{Document: doc}

	forged := []byte(`{"duration_months":null,"team":[{"role_id":"` + uuid.New().String() + `","seniority":"senior","headcount":1,"evidence":{"refs":["L0001"],"support":"explicit"}}]}`)
	_, err = phase.Validate(context.Background(), in, forged)
	require.ErrorIs(t, err, ErrStaffingPhaseValidation)

	mismatch := []byte(`{"duration_months":null,"team":[{"role_id":"` + frontendID.String() + `","seniority":"associate","headcount":1,"evidence":{"refs":["L0001"],"support":"explicit"}}]}`)
	_, err = phase.Validate(context.Background(), in, mismatch)
	require.ErrorIs(t, err, ErrStaffingPhaseValidation)
}
