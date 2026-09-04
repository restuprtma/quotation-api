package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestDedicatedSquadInputValidateMatrix(t *testing.T) {
	start := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	line := domain.SquadLineInput{Role: "web_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}
	sprintLine := domain.SquadLineInput{Role: "web_programmer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 4}

	tests := []struct {
		name    string
		input   domain.DedicatedSquadInput
		wantErr string
	}{
		{name: "1A", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}},
		{name: "1B", input: domain.DedicatedSquadInput{Model: domain.ModelSOWSmall, DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineCalendar, StartDate: &start}}},
		{name: "1C", input: domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, DevelopmentSquad: []domain.SquadLineInput{line}, KickoffDiscountBPS: 5000, PaymentCount: 1, PaymentAnchorDate: &start}},
		{name: "1D", input: domain.DedicatedSquadInput{Model: domain.ModelSprint, DevelopmentSquad: []domain.SquadLineInput{sprintLine}, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}},
		{name: "unsupported model", input: domain.DedicatedSquadInput{Model: "1E", DevelopmentSquad: []domain.SquadLineInput{line}}, wantErr: "unsupported dedicated squad model"},
		{name: "missing payment anchor", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "payment_anchor_date"},
		{name: "zero quarter quantity", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, DevelopmentSquad: []domain.SquadLineInput{{Role: "web_programmer", Seniority: "senior", PeriodQuarters: 4}}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "development_squad[0].headcount_quarters"},
		{name: "1D missing dates", input: domain.DedicatedSquadInput{Model: domain.ModelSprint, DevelopmentSquad: []domain.SquadLineInput{sprintLine}, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint}}, wantErr: "timeline.start_date"},
		{name: "1D workday count differs from dates", input: domain.DedicatedSquadInput{Model: domain.ModelSprint, DevelopmentSquad: []domain.SquadLineInput{{Role: "web_programmer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 3}}, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}, wantErr: "development_squad[0].workday_count"},
		{name: "1C with timeline", input: domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, DevelopmentSquad: []domain.SquadLineInput{line}, KickoffDiscountBPS: 5000, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "timeline.mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestDedicatedSquadInputJSONRoundTrip(t *testing.T) {
	for _, model := range []domain.SquadModel{domain.ModelSOWLarge, domain.ModelSOWSmall, domain.ModelStaffAugmentation, domain.ModelSprint} {
		t.Run(string(model), func(t *testing.T) {
			input := domain.DedicatedSquadInput{Model: model, Requirement: "Build quotation engine", DevelopmentSquad: []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 8}}, PaymentCount: 2}
			encoded, err := json.Marshal(input)
			require.NoError(t, err)
			var decoded domain.DedicatedSquadInput
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, input, decoded)
		})
	}
}
