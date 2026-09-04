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
		{name: "1A", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, Requirement: "Build a portal", DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}},
		{name: "1B", input: domain.DedicatedSquadInput{Model: domain.ModelSOWSmall, Requirement: "Build a portal", DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineCalendar, StartDate: &start}}},
		{name: "1C", input: domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, Requirement: "Provide engineering capacity", DevelopmentSquad: []domain.SquadLineInput{line}, KickoffDiscountBPS: 5000, PaymentCount: 1, PaymentAnchorDate: &start}},
		{name: "1D", input: domain.DedicatedSquadInput{Model: domain.ModelSprint, Requirement: "Fix checkout validation", DevelopmentSquad: []domain.SquadLineInput{sprintLine}, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}},
		{name: "unsupported model", input: domain.DedicatedSquadInput{Model: "1E", DevelopmentSquad: []domain.SquadLineInput{line}}, wantErr: "unsupported dedicated squad model"},
		{name: "missing requirement", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "requirement"},
		{name: "missing payment anchor", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, Requirement: "Build a portal", DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "payment_anchor_date"},
		{name: "zero quarter quantity", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, Requirement: "Build a portal", DevelopmentSquad: []domain.SquadLineInput{{Role: "web_programmer", Seniority: "senior", PeriodQuarters: 4}}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "development_squad[0].headcount_quarters"},
		{name: "relative timeline rejects start date", input: domain.DedicatedSquadInput{Model: domain.ModelSOWLarge, Requirement: "Build a portal", DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative, StartDate: &start}}, wantErr: "must be empty for relative"},
		{name: "1D missing dates", input: domain.DedicatedSquadInput{Model: domain.ModelSprint, Requirement: "Fix checkout validation", DevelopmentSquad: []domain.SquadLineInput{sprintLine}, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint}}, wantErr: "timeline.start_date"},
		{name: "1D workday count differs from dates", input: domain.DedicatedSquadInput{Model: domain.ModelSprint, Requirement: "Fix checkout validation", DevelopmentSquad: []domain.SquadLineInput{{Role: "web_programmer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 3}}, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}, wantErr: "development_squad[0].workday_count"},
		{name: "1C with timeline", input: domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, Requirement: "Provide engineering capacity", DevelopmentSquad: []domain.SquadLineInput{line}, KickoffDiscountBPS: 5000, PaymentCount: 1, PaymentAnchorDate: &start, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}, wantErr: "timeline.mode"},
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
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	monthlyLine := domain.SquadLineInput{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 8}
	inputs := []domain.DedicatedSquadInput{
		{Model: domain.ModelSOWLarge, Requirement: "Build quotation engine", DevelopmentSquad: []domain.SquadLineInput{monthlyLine}, PaymentCount: 2, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}},
		{Model: domain.ModelSOWSmall, Requirement: "Build quotation engine", DevelopmentSquad: []domain.SquadLineInput{monthlyLine}, PaymentCount: 2, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineCalendar, StartDate: &start}},
		{Model: domain.ModelStaffAugmentation, Requirement: "Provide engineering capacity", DevelopmentSquad: []domain.SquadLineInput{monthlyLine}, KickoffDiscountBPS: 5000, PaymentCount: 2, PaymentAnchorDate: &anchor},
		{Model: domain.ModelSprint, Requirement: "Fix checkout validation", DevelopmentSquad: []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 4}}, PaymentCount: 1, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}},
	}
	for _, input := range inputs {
		t.Run(string(input.Model), func(t *testing.T) {
			require.NoError(t, input.Validate())
			encoded, err := json.Marshal(input)
			require.NoError(t, err)
			var decoded domain.DedicatedSquadInput
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, input, decoded)
			require.NoError(t, decoded.Validate())
		})
	}
}

func TestDedicatedSquadInputAcceptsCalendarDateJSON(t *testing.T) {
	payload := []byte(`{"model":"1D","requirement":"Fix checkout","development_squad":[{"role":"backend_programmer","seniority":"senior","headcount_quarters":4,"period_quarters":0,"workday_count":5}],"maintenance_enabled":false,"payment_count":1,"payment_anchor_date":"2026-09-05","timeline":{"mode":"sprint","start_date":"2026-09-07","sprint_end_date":"2026-09-11"}}`)
	var input domain.DedicatedSquadInput
	require.NoError(t, json.Unmarshal(payload, &input))
	require.NotNil(t, input.PaymentAnchorDate)
	require.NotNil(t, input.Timeline.StartDate)
	require.Equal(t, "2026-09-05", input.PaymentAnchorDate.Format("2006-01-02"))
	require.NoError(t, input.Validate())
}

func TestDedicatedSquadInputRejectsValuesAboveOperationalLimits(t *testing.T) {
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	valid := domain.DedicatedSquadInput{
		Model: domain.ModelSOWLarge, Requirement: "Build a portal",
		DevelopmentSquad: []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}},
		PaymentCount:     2, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative},
	}
	tests := []struct {
		name    string
		mutate  func(*domain.DedicatedSquadInput)
		wantErr string
	}{
		{"requirement", func(in *domain.DedicatedSquadInput) {
			in.Requirement = string(make([]rune, domain.MaxDedicatedRequirementRunes+1))
		}, "requirement"},
		{"headcount", func(in *domain.DedicatedSquadInput) {
			in.DevelopmentSquad[0].HeadcountQuarters = domain.MaxHeadcountQuarters + 1
		}, "headcount_quarters"},
		{"period", func(in *domain.DedicatedSquadInput) {
			in.DevelopmentSquad[0].PeriodQuarters = domain.MaxPeriodQuarters + 1
		}, "period_quarters"},
		{"payments", func(in *domain.DedicatedSquadInput) { in.PaymentCount = domain.MaxPaymentCount + 1 }, "payment_count"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			input.DevelopmentSquad = append([]domain.SquadLineInput(nil), valid.DevelopmentSquad...)
			tt.mutate(&input)
			require.ErrorContains(t, input.Validate(), tt.wantErr)
		})
	}
}
