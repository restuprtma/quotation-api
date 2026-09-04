package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type fakeDedicatedRateLookup struct {
	items map[string]domain.RateCardItem
	calls []string
}

func (f *fakeDedicatedRateLookup) ResolveItem(_ context.Context, _, _, role, seniority, term string) (*domain.RateCardItem, error) {
	key := role + "|" + seniority + "|" + term
	f.calls = append(f.calls, key)
	item, ok := f.items[key]
	if !ok {
		return nil, assert.AnError
	}
	return &item, nil
}

func anchorDate() *time.Time {
	v := time.Date(2026, time.June, 17, 0, 0, 0, 0, time.UTC)
	return &v
}

func baseInput(model domain.SquadModel, lines []domain.SquadLineInput) domain.DedicatedSquadInput {
	return domain.DedicatedSquadInput{
		Model: model, Requirement: "Sistem reservasi", DevelopmentSquad: lines,
		PaymentCount: 1, PaymentAnchorDate: anchorDate(),
		Timeline: domain.TimelineInput{Mode: domain.TimelineRelative},
	}
}

func TestDedicatedSquadCalculatorLargeSOWPreparation(t *testing.T) {
	lookup := &fakeDedicatedRateLookup{items: map[string]domain.RateCardItem{}}
	roles := []struct {
		role                    string
		recruitment, onboarding int64
	}{
		{"backend_programmer", 3_000_000, 13_600_000},
		{"frontend_programmer", 1_500_000, 11_000_000},
		{"mobile_developer", 3_000_000, 13_600_000},
		{"automated_tester", 1_500_000, 10_000_000},
	}
	var lines []domain.SquadLineInput
	for _, role := range roles {
		lookup.items[role.role+"|senior|long_term"] = domain.RateCardItem{AmountPerMonth: 12_500_000, RecruitmentFee: role.recruitment, OnboardingFee: role.onboarding}
		lines = append(lines, domain.SquadLineInput{Role: role.role, Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 12})
	}
	in := baseInput(domain.ModelSOWLarge, lines)
	in.PaymentCount = 3

	got, err := NewDedicatedSquadCalculator(lookup).Calculate(context.Background(), "company", "card", in)
	require.NoError(t, err)
	assert.Equal(t, int64(57_200_000), got.Preparation.Gross)
	assert.Equal(t, int64(28_600_000), got.Preparation.Discount)
	assert.Equal(t, int64(28_600_000), got.Preparation.Net)
	assert.Equal(t, int64(150_000_000), got.DevelopmentSubtotal)
	assert.Equal(t, int64(178_600_000), got.TotalInvestment)
	require.Len(t, got.Payments, 4)
	assert.Equal(t, int64(28_600_000), got.Payments[0].Nominal)
	assert.Equal(t, got.TotalInvestment, sumDedicatedPayments(got.Payments))
}

func TestDedicatedSquadCalculatorSmallSOWRemainderOnLastInstallment(t *testing.T) {
	lookup := &fakeDedicatedRateLookup{items: map[string]domain.RateCardItem{
		"backend_programmer|senior|short_term": {AmountPerMonth: 10_000_001},
	}}
	in := baseInput(domain.ModelSOWSmall, []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}})
	in.PaymentCount = 3

	got, err := NewDedicatedSquadCalculator(lookup).Calculate(context.Background(), "company", "card", in)
	require.NoError(t, err)
	require.Len(t, got.Payments, 3)
	assert.Equal(t, []int64{3_333_333, 3_333_333, 3_333_335}, paymentNominals(got.Payments))
	assert.Equal(t, got.TotalInvestment, sumDedicatedPayments(got.Payments))
	assert.Equal(t, "short_term", got.Development[0].TermType)
}

func TestDedicatedSquadCalculatorStaffAugmentationMonthlyAndKickoff(t *testing.T) {
	lookup := &fakeDedicatedRateLookup{items: map[string]domain.RateCardItem{
		"mobile_developer|senior|long_term": {AmountPerMonth: 11_550_000, KickoffFee: 11_550_000},
	}}
	in := baseInput(domain.ModelStaffAugmentation, []domain.SquadLineInput{{Role: "mobile_developer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 8}})
	in.Timeline = domain.TimelineInput{}
	in.KickoffDiscountBPS = 5000
	in.PaymentCount = 2

	got, err := NewDedicatedSquadCalculator(lookup).Calculate(context.Background(), "company", "card", in)
	require.NoError(t, err)
	assert.Equal(t, int64(5_775_000), got.Preparation.Net)
	require.Len(t, got.Payments, 3)
	assert.Equal(t, []int64{5_775_000, 11_550_000, 11_550_000}, paymentNominals(got.Payments))
	assert.Equal(t, time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC), got.Payments[1].InvoiceDate)
	assert.Equal(t, got.TotalInvestment, sumDedicatedPayments(got.Payments))
}

func TestDedicatedSquadCalculatorSprintUsesDailyRateAndEndDate(t *testing.T) {
	lookup := &fakeDedicatedRateLookup{items: map[string]domain.RateCardItem{
		"mobile_developer|senior|short_term": {AmountPerMonth: 16_050_000},
	}}
	start := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC)
	in := baseInput(domain.ModelSprint, []domain.SquadLineInput{{Role: "mobile_developer", Seniority: "senior", HeadcountQuarters: 4, WorkdayCount: 4}})
	in.Timeline = domain.TimelineInput{Mode: domain.TimelineSprint, StartDate: &start, SprintEndDate: &end}

	got, err := NewDedicatedSquadCalculator(lookup).Calculate(context.Background(), "company", "card", in)
	require.NoError(t, err)
	assert.Equal(t, int64(729_545), got.Development[0].AmountPerUnit)
	assert.Equal(t, int64(2_918_180), got.Development[0].LineTotal)
	require.Len(t, got.Payments, 1)
	assert.Equal(t, end, got.Payments[0].InvoiceDate)
	assert.Equal(t, end.AddDate(0, 0, 14), got.Payments[0].DueDate)
	assert.Equal(t, "Pelunasan", got.Payments[0].Terms)
}

func TestDedicatedSquadCalculatorRejectsOverflow(t *testing.T) {
	lookup := &fakeDedicatedRateLookup{items: map[string]domain.RateCardItem{
		"backend_programmer|senior|short_term": {AmountPerMonth: math.MaxInt64},
	}}
	in := baseInput(domain.ModelSOWSmall, []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 8}})
	_, err := NewDedicatedSquadCalculator(lookup).Calculate(context.Background(), "company", "card", in)
	require.ErrorContains(t, err, "overflow")
}

func paymentNominals(payments []domain.PaymentRow) []int64 {
	result := make([]int64, len(payments))
	for i := range payments {
		result[i] = payments[i].Nominal
	}
	return result
}

func sumDedicatedPayments(payments []domain.PaymentRow) int64 {
	var result int64
	for _, payment := range payments {
		result += payment.Nominal
	}
	return result
}
