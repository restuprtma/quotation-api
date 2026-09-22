package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateCardItemHasBreakdownFields(t *testing.T) {
	item := RateCardItem{
		BaseSalary:    6800000,
		TechLeadFee:   1030000,
		AiRndFee:      500000,
		WorkspaceFee:  500000,
		ManagementFee: 2720000,
	}
	longTermTotal := item.BaseSalary + item.TechLeadFee + item.AiRndFee + item.WorkspaceFee + item.ManagementFee
	assert.Equal(t, int64(11550000), longTermTotal)
}

func TestRateCardItemHasNegotiationFields(t *testing.T) {
	item := RateCardItem{
		PublishedRate:  16700000,
		MaxDiscountPct: 5,
		FloorRate:      15865000,
	}
	expectedFloor := item.PublishedRate - (item.PublishedRate * int64(item.MaxDiscountPct) / 100)
	assert.Equal(t, item.FloorRate, expectedFloor)
}

func TestRateCardHasClientTier(t *testing.T) {
	rc := RateCard{
		ClientTier:            "commercial_national",
		ShortTermSurchargePct: 40,
		EnterpriseMarkupPct:   35,
	}
	assert.Equal(t, "commercial_national", rc.ClientTier)
	assert.Equal(t, 40, rc.ShortTermSurchargePct)
}

func TestRateCardItemDailyRate(t *testing.T) {
	item := RateCardItem{AmountPerMonth: 16170000}
	daily := item.DailyRate(16170000)
	assert.Equal(t, int64(735000), daily) // 16170000/22 = 735000
}

func TestRateCardItemHourlyRate(t *testing.T) {
	item := RateCardItem{AmountPerMonth: 16170000}
	hourly := item.HourlyRate()
	assert.Equal(t, int64(93468), hourly) // 16170000/173 = 93468
}

func TestRateCardItemTotalHiringFee(t *testing.T) {
	item := RateCardItem{
		RecruitmentFee: 3000000,
		OnboardingFee:  13600000,
	}
	assert.Equal(t, int64(16600000), item.TotalHiringFee())
}

func TestOvertimeRateConfig(t *testing.T) {
	cfg := OvertimeRateConfig{
		BaseMonthlyRate:             16170000,
		HourlyDivisor:               173,
		WorkdayFirstHourMultiplier:  150,
		WorkdaySubsequentMultiplier: 200,
		HolidayBaseMultiplier:       200,
		Holiday9thHourMultiplier:    300,
		Holiday10PlusMultiplier:     400,
	}

	assert.Equal(t, int64(93468), cfg.HourlyRate())
	assert.Equal(t, int64(140202), cfg.CalculateWorkdayOvertime(1))
	assert.Equal(t, int64(327138), cfg.CalculateWorkdayOvertime(2))
	assert.Equal(t, int64(1495488), cfg.CalculateHolidayOvertime(8))
	assert.Equal(t, int64(1775892), cfg.CalculateHolidayOvertime(9))
	assert.Equal(t, int64(2149764), cfg.CalculateHolidayOvertime(10))
}
