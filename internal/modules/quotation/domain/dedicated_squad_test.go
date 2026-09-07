package domain_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestDedicatedSquadProfileJSONAndLegacyCompatibility(t *testing.T) {
	legacy := validDedicatedInput(domain.ModelSOWLarge)
	require.NoError(t, legacy.Validate())
	assert.Nil(t, legacy.QuotationProfile)

	discount := int32(2500)
	next := legacy
	next.QuotationProfile = &domain.QuotationProfileInput{
		PreparationMode:        domain.PreparationProfileHiring,
		PreparationDiscountBPS: &discount,
		MaintenanceMode:        domain.MaintenanceProfileIncluded,
		ScopeTimelineMode:      domain.ScopeTimelineCalendar,
		IncludedPhases:         []string{domain.PhaseRequirement, domain.PhaseDevelopment, domain.PhaseSIT, domain.PhaseUAT, domain.PhaseGoLive},
		PaymentMode:            domain.PaymentProfileRows,
		PaymentRows: []domain.PaymentRowInput{
			{WorkPeriod: "Kick off", Terms: "Down Payment", AllocationKind: domain.PaymentAllocationNetPreparation},
			{WorkPeriod: "Bulan 1", Terms: "Termin 1", AllocationKind: domain.PaymentAllocationRemainingBPS, AllocationBPS: 10000, InvoiceMonthOffset: 1},
		},
		DocumentOptions: domain.DocumentOptionsInput{LetterPrefix: "VEN", TaxTreatment: domain.TaxExclusivePPN, ClauseCodes: []string{"out_of_pocket", "overtime"}},
	}
	next.MaintenanceEnabled = true
	next.MaintenanceSquad = []domain.SquadLineInput{{Role: "quality_assurance", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}}
	require.NoError(t, next.Validate())

	encoded, err := json.Marshal(next)
	require.NoError(t, err)
	var decoded domain.DedicatedSquadInput
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, next, decoded)
	require.NoError(t, decoded.Validate())
}

func TestDedicatedSquadProfileRejectsInvalidCombinations(t *testing.T) {
	valid := validDedicatedInput(domain.ModelSOWLarge)
	discount := int32(2500)
	validProfile := func() *domain.QuotationProfileInput {
		return &domain.QuotationProfileInput{
			PreparationMode:        domain.PreparationProfileHiring,
			PreparationDiscountBPS: &discount,
			ScopeTimelineMode:      domain.ScopeTimelineRelative,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*domain.DedicatedSquadInput)
		wantErr string
	}{
		{"unknown preparation mode", func(in *domain.DedicatedSquadInput) { in.QuotationProfile.PreparationMode = "other" }, "preparation_mode"},
		{"unknown phase", func(in *domain.DedicatedSquadInput) { in.QuotationProfile.IncludedPhases = []string{"other"} }, "included_phases"},
		{"duplicate phase", func(in *domain.DedicatedSquadInput) {
			in.QuotationProfile.IncludedPhases = []string{domain.PhaseSIT, domain.PhaseSIT}
		}, "duplicate"},
		{"unknown clause", func(in *domain.DedicatedSquadInput) {
			in.QuotationProfile.DocumentOptions.ClauseCodes = []string{"other"}
		}, "clause_codes"},
		{"discount over maximum", func(in *domain.DedicatedSquadInput) {
			value := int32(10001)
			in.QuotationProfile.PreparationDiscountBPS = &value
		}, "preparation_discount_bps"},
		{"maintenance without lines", func(in *domain.DedicatedSquadInput) {
			in.QuotationProfile.MaintenanceMode = domain.MaintenanceProfileSeparate
		}, "maintenance_squad"},
		{"phases without timeline", func(in *domain.DedicatedSquadInput) {
			in.QuotationProfile.ScopeTimelineMode = domain.ScopeTimelineNone
			in.QuotationProfile.IncludedPhases = []string{domain.PhaseDevelopment}
		}, "included_phases"},
		{"empty explicit payments", func(in *domain.DedicatedSquadInput) { in.QuotationProfile.PaymentMode = domain.PaymentProfileRows }, "payment_rows"},
		{"invalid payment bps sum", func(in *domain.DedicatedSquadInput) {
			in.QuotationProfile.PaymentMode = domain.PaymentProfileRows
			in.QuotationProfile.PaymentRows = []domain.PaymentRowInput{{WorkPeriod: "Bulan 1", Terms: "Termin", AllocationKind: domain.PaymentAllocationInvestmentBPS, AllocationBPS: 9999}}
		}, "totaling 10000"},
		{"net preparation without preparation", func(in *domain.DedicatedSquadInput) {
			in.QuotationProfile.PreparationMode = domain.PreparationProfileNone
			in.QuotationProfile.PaymentMode = domain.PaymentProfileRows
			in.QuotationProfile.PaymentRows = []domain.PaymentRowInput{
				{WorkPeriod: "Kick off", Terms: "DP", AllocationKind: domain.PaymentAllocationNetPreparation},
				{WorkPeriod: "Bulan 1", Terms: "Termin", AllocationKind: domain.PaymentAllocationRemainingBPS, AllocationBPS: 10000},
			}
		}, "preparation_mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			input.QuotationProfile = validProfile()
			tt.mutate(&input)
			require.ErrorContains(t, input.Validate(), tt.wantErr)
		})
	}
}

func TestDedicatedSquadProfileRejectsForbiddenAndUnknownJSONFields(t *testing.T) {
	base := `{"model":"1A","requirement":"Build portal","development_squad":[{"role":"backend_programmer","seniority":"senior","headcount_quarters":4,"period_quarters":4}],"payment_count":1,"payment_anchor_date":"2026-09-05","timeline":{"mode":"relative"},"quotation_profile":{"payment_mode":"explicit_rows","payment_rows":[{"work_period":"Kick off","terms":"DP","allocation_kind":"investment_bps","allocation_bps":10000%s}]}}`
	for _, field := range []string{` ,"nominal":1000`, ` ,"due_date":"2026-09-19"`, ` ,"rate":1000`, ` ,"html":"<p>unsafe</p>"`} {
		t.Run(field, func(t *testing.T) {
			var input domain.DedicatedSquadInput
			payload := []byte(fmt.Sprintf(base, field))
			require.Error(t, json.Unmarshal(payload, &input))
		})
	}

	var input domain.DedicatedSquadInput
	require.Error(t, json.Unmarshal([]byte(`{"model":"1A","requirement":"Build portal","development_squad":[{"role":"backend_programmer","seniority":"senior","headcount_quarters":4,"period_quarters":4}],"payment_count":1,"payment_anchor_date":"2026-09-05","timeline":{"mode":"relative","unexpected":true}}`), &input))
}

func TestDedicatedSquadProfileValidatesSprintDates(t *testing.T) {
	input := validDedicatedInput(domain.ModelSprint)
	input.Timeline.SprintEndDate = input.Timeline.StartDate
	input.QuotationProfile = &domain.QuotationProfileInput{ScopeTimelineMode: domain.ScopeTimelineSprint}
	require.ErrorContains(t, input.Validate(), "workday_count")
}

func TestDedicatedSquadProfileCanUseProfileSupportedCombinationsOutsideLegacyModelGates(t *testing.T) {
	input := validDedicatedInput(domain.ModelSOWSmall)
	input.KickoffDiscountBPS = 0 // retained legacy field is not the profile discount.
	discount := int32(2500)
	input.MaintenanceEnabled = true
	input.MaintenanceSquad = []domain.SquadLineInput{{Role: "quality_assurance", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}}
	input.QuotationProfile = &domain.QuotationProfileInput{
		PreparationMode:        domain.PreparationProfileHiring,
		PreparationDiscountBPS: &discount,
		MaintenanceMode:        domain.MaintenanceProfileIncluded,
		ScopeTimelineMode:      domain.ScopeTimelineRelative,
	}
	require.NoError(t, input.Validate())
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

func validDedicatedInput(model domain.SquadModel) domain.DedicatedSquadInput {
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	line := domain.SquadLineInput{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}
	input := domain.DedicatedSquadInput{
		Model: model, Requirement: "Build a portal", DevelopmentSquad: []domain.SquadLineInput{line}, PaymentCount: 2, PaymentAnchorDate: &anchor,
	}
	switch model {
	case domain.ModelSOWLarge, domain.ModelSOWSmall:
		input.Timeline = domain.TimelineInput{Mode: domain.TimelineRelative}
	case domain.ModelStaffAugmentation:
		input.KickoffDiscountBPS = 5000
		input.Requirement = "Provide engineering capacity"
	case domain.ModelSprint:
		input.Requirement = "Fix checkout validation"
		input.PaymentCount = 1
		input.DevelopmentSquad = []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 4}}
		input.Timeline = domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}
	}
	return input
}
