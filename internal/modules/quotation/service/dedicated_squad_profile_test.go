package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func int32Pointer(value int32) *int32 { return &value }

func TestResolveDedicatedSquadProfileKeepsLegacyPreset(t *testing.T) {
	in := baseInput(domain.ModelSOWLarge, []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}})
	profile, err := ResolveDedicatedSquadProfile(in)
	require.NoError(t, err)
	assert.Equal(t, PreparationHiring, profile.PreparationKind)
	assert.Equal(t, int32(5000), profile.PreparationDiscountBPS)
	assert.True(t, profile.HasPhase(PhaseK6))
	assert.True(t, profile.HasPhase(PhasePentest))
	assert.True(t, profile.UsesLegacyDefaults)
}

func TestResolveDedicatedSquadProfileDoesNotUseSubtotalThreshold(t *testing.T) {
	in := baseInput(domain.ModelSOWSmall, []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}})
	in.QuotationProfile = &domain.QuotationProfileInput{
		PreparationMode:        domain.PreparationProfileHiring,
		PreparationDiscountBPS: int32Pointer(2500),
		ScopeTimelineMode:      domain.ScopeTimelineRelative,
		IncludedPhases:         []string{domain.PhaseRequirement, domain.PhaseDevelopment, domain.PhaseSIT, domain.PhaseUAT, domain.PhaseGoLive},
	}
	profile, err := ResolveDedicatedSquadProfile(in)
	require.NoError(t, err)
	assert.Equal(t, PreparationHiring, profile.PreparationKind)
	assert.Equal(t, int32(2500), profile.PreparationDiscountBPS)
	rules, err := EvaluateDedicatedSquadRulesWithProfile(in, profile, 250_000_000)
	require.NoError(t, err)
	assert.Equal(t, PreparationHiring, rules.PreparationKind)
	assert.False(t, rules.IncludeK6)
	assert.False(t, rules.IncludePentest)
}

func TestResolveDedicatedSquadProfileSupportsEmbeddedMaintenanceAndSelectedPhases(t *testing.T) {
	line := domain.SquadLineInput{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}
	in := baseInput(domain.ModelSOWSmall, []domain.SquadLineInput{line})
	in.MaintenanceSquad = []domain.SquadLineInput{line}
	in.QuotationProfile = &domain.QuotationProfileInput{
		MaintenanceMode:   domain.MaintenanceProfileIncluded,
		ScopeTimelineMode: domain.ScopeTimelineRelative,
		IncludedPhases:    []string{domain.PhaseRequirement, domain.PhaseDevelopment, domain.PhaseK6, domain.PhaseGoLive},
	}
	profile, err := ResolveDedicatedSquadProfile(in)
	require.NoError(t, err)
	rules, err := EvaluateDedicatedSquadRulesWithProfile(in, profile, 1)
	require.NoError(t, err)
	assert.True(t, rules.MaintenanceEmbedded)
	assert.False(t, rules.IncludeMaintenanceSection)
	assert.True(t, rules.IncludeK6)
	assert.False(t, rules.IncludePentest)
	assert.Equal(t, []string{"Biaya Development", "Lingkup Pekerjaan & Timeline", "Pembayaran", "Batasan dan Pengecualian"}, sectionTitles(BuildSectionPlan(rules)))
}

func TestDedicatedSquadCalculatorUsesExplicitProfilePayments(t *testing.T) {
	lookup := &fakeDedicatedRateLookup{items: map[string]domain.RateCardItem{
		"backend_programmer|senior|short_term": {AmountPerMonth: 10_000_001, RecruitmentFee: 3_000_000, OnboardingFee: 13_600_000},
	}}
	in := baseInput(domain.ModelSOWSmall, []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}})
	in.QuotationProfile = &domain.QuotationProfileInput{
		PreparationMode:        domain.PreparationProfileHiring,
		PreparationDiscountBPS: int32Pointer(2500),
		ScopeTimelineMode:      domain.ScopeTimelineRelative,
		IncludedPhases:         []string{domain.PhaseRequirement, domain.PhaseDevelopment, domain.PhaseGoLive},
		PaymentMode:            domain.PaymentProfileRows,
		PaymentRows: []domain.PaymentRowInput{
			{WorkPeriod: "Kick off", Terms: "Termin 1", AllocationKind: domain.PaymentAllocationInvestmentBPS, AllocationBPS: 3333},
			{WorkPeriod: "Bulan 1", Terms: "Termin 2", AllocationKind: domain.PaymentAllocationInvestmentBPS, AllocationBPS: 3333, InvoiceMonthOffset: 1},
			{WorkPeriod: "Bulan 2", Terms: "Termin 3", AllocationKind: domain.PaymentAllocationInvestmentBPS, AllocationBPS: 3334, InvoiceMonthOffset: 2},
		},
	}
	profile, err := ResolveDedicatedSquadProfile(in)
	require.NoError(t, err)
	got, err := NewDedicatedSquadCalculator(lookup).CalculateWithProfile(context.Background(), "company", "card", in, profile)
	require.NoError(t, err)
	require.Len(t, got.Payments, 3)
	assert.Equal(t, got.TotalInvestment, sumDedicatedPayments(got.Payments))
	assert.Equal(t, got.Payments[0].InvoiceDate.AddDate(0, 0, 14), got.Payments[0].DueDate)
	assert.Greater(t, got.Payments[2].Nominal, got.Payments[0].Nominal)
}
