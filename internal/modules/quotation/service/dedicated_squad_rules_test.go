package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestDedicatedSquadRulesDecisionMatrix(t *testing.T) {
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	monthlyLine := domain.SquadLineInput{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}
	base := domain.DedicatedSquadInput{DevelopmentSquad: []domain.SquadLineInput{monthlyLine}, PaymentCount: 2, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative}}

	tests := []struct {
		name       string
		input      domain.DedicatedSquadInput
		subtotal   int64
		want       DedicatedSquadRules
		wantErr    string
		wantTitles []string
	}{
		{
			name: "1A threshold inclusive with optional maintenance",
			input: mutateInput(base, func(in *domain.DedicatedSquadInput) {
				in.Model = domain.ModelSOWLarge
				in.MaintenanceEnabled = true
				in.MaintenanceSquad = []domain.SquadLineInput{monthlyLine}
			}),
			subtotal:   150_000_000,
			want:       DedicatedSquadRules{PreparationKind: PreparationHiring, PaymentKind: PaymentNetPreparationDP, IncludeTimeline: true, IncludeMaintenance: true, IncludeK6: true, IncludePentest: true, PreparationDiscountBPS: 5000},
			wantTitles: []string{"Biaya Development", "Biaya Maintenance", "Lingkup Pekerjaan & Timeline", "Pembayaran", "Batasan dan Pengecualian"},
		},
		{
			name:     "1A rejects below threshold",
			input:    mutateInput(base, func(in *domain.DedicatedSquadInput) { in.Model = domain.ModelSOWLarge }),
			subtotal: 149_999_999,
			wantErr:  "development_subtotal: model 1A requires at least",
		},
		{
			name:       "1B tier 20",
			input:      mutateInput(base, func(in *domain.DedicatedSquadInput) { in.Model = domain.ModelSOWSmall }),
			subtotal:   20_000_000,
			want:       DedicatedSquadRules{PreparationKind: PreparationNone, PaymentKind: PaymentEqualInstallments, IncludeTimeline: true, ProjectTier: ProjectTier20},
			wantTitles: []string{"Biaya Development", "Lingkup Pekerjaan & Timeline", "Pembayaran", "Batasan dan Pengecualian"},
		},
		{
			name:     "1B tier 50",
			input:    mutateInput(base, func(in *domain.DedicatedSquadInput) { in.Model = domain.ModelSOWSmall }),
			subtotal: 50_000_000,
			want:     DedicatedSquadRules{PreparationKind: PreparationNone, PaymentKind: PaymentEqualInstallments, IncludeTimeline: true, ProjectTier: ProjectTier50},
		},
		{
			name:     "1B tier 100 includes security tests and no direct discount",
			input:    mutateInput(base, func(in *domain.DedicatedSquadInput) { in.Model = domain.ModelSOWSmall }),
			subtotal: 100_000_000,
			want:     DedicatedSquadRules{PreparationKind: PreparationNone, PaymentKind: PaymentEqualInstallments, IncludeTimeline: true, IncludeK6: true, IncludePentest: true, ProjectTier: ProjectTier100, DirectProjectDiscountBPS: 0},
		},
		{
			name:     "1B rejects threshold and maintenance",
			input:    mutateInput(base, func(in *domain.DedicatedSquadInput) { in.Model = domain.ModelSOWSmall }),
			subtotal: 150_000_000,
			wantErr:  "development_subtotal: model 1B requires less than",
		},
		{
			name: "1B rejects maintenance",
			input: mutateInput(base, func(in *domain.DedicatedSquadInput) {
				in.Model = domain.ModelSOWSmall
				in.MaintenanceEnabled = true
				in.MaintenanceSquad = []domain.SquadLineInput{monthlyLine}
			}),
			subtotal: 50_000_000,
			wantErr:  "maintenance_enabled: only supported for model 1A",
		},
		{
			name:       "1C monthly without timeline",
			input:      domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, DevelopmentSquad: []domain.SquadLineInput{{Role: "mobile_developer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 24}}, KickoffDiscountBPS: 10000, PaymentCount: 6, PaymentAnchorDate: &anchor},
			subtotal:   69_300_000,
			want:       DedicatedSquadRules{PreparationKind: PreparationEngineeringKickoff, PaymentKind: PaymentMonthly, PreparationDiscountBPS: 10000},
			wantTitles: []string{"Biaya Development", "Pembayaran", "Batasan dan Pengecualian"},
		},
		{
			name:    "1C payment count follows longest monthly period",
			input:   domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, DevelopmentSquad: []domain.SquadLineInput{{Role: "mobile_developer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 24}}, KickoffDiscountBPS: 5000, PaymentCount: 5, PaymentAnchorDate: &anchor},
			wantErr: "payment_count: model 1C requires 6 monthly payments",
		},
		{
			name:    "1C rejects unsupported kickoff discount",
			input:   domain.DedicatedSquadInput{Model: domain.ModelStaffAugmentation, DevelopmentSquad: []domain.SquadLineInput{{Role: "mobile_developer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}}, KickoffDiscountBPS: 2500, PaymentCount: 1, PaymentAnchorDate: &anchor},
			wantErr: "kickoff_discount_bps: must be 5000 or 10000",
		},
		{
			name:       "1D single settlement",
			input:      sprintInput(anchor, 1),
			subtotal:   10_000_000,
			want:       DedicatedSquadRules{PreparationKind: PreparationNone, PaymentKind: PaymentSingleSettlement, IncludeTimeline: true},
			wantTitles: []string{"Biaya Development", "Lingkup Pekerjaan & Timeline", "Pembayaran", "Batasan dan Pengecualian"},
		},
		{
			name:    "1D rejects multiple payments",
			input:   sprintInput(anchor, 2),
			wantErr: "payment_count: model 1D requires exactly one payment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateDedicatedSquadRules(tt.input, tt.subtotal)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			if tt.wantTitles != nil {
				sections := BuildSectionPlan(got)
				assert.Equal(t, tt.wantTitles, sectionTitles(sections))
				for i, section := range sections {
					assert.Equal(t, i+1, section.Number)
				}
			}
		})
	}
}

func mutateInput(input domain.DedicatedSquadInput, mutate func(*domain.DedicatedSquadInput)) domain.DedicatedSquadInput {
	mutate(&input)
	return input
}

func sprintInput(anchor time.Time, paymentCount int32) domain.DedicatedSquadInput {
	start := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	return domain.DedicatedSquadInput{Model: domain.ModelSprint, DevelopmentSquad: []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 4}}, PaymentCount: paymentCount, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}}
}

func sectionTitles(sections []domain.DocumentSection) []string {
	titles := make([]string, len(sections))
	for i, section := range sections {
		titles[i] = section.Title
	}
	return titles
}
