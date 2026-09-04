package domain

import (
	"time"

	"github.com/google/uuid"
)

type OvertimeRateConfig struct {
	ID                          uuid.UUID `json:"id" db:"id"`
	CompanyID                   uuid.UUID `json:"company_id" db:"company_id"`
	RateCardID                  uuid.UUID `json:"rate_card_id" db:"rate_card_id"`
	Role                        string    `json:"role" db:"role"`
	Seniority                   string    `json:"seniority" db:"seniority"`
	TermType                    string    `json:"term_type" db:"term_type"`
	BaseMonthlyRate             int64     `json:"base_monthly_rate" db:"base_monthly_rate"`
	HourlyDivisor               int       `json:"hourly_divisor" db:"hourly_divisor"`
	WorkdayFirstHourMultiplier  int       `json:"workday_first_hour_multiplier" db:"workday_first_hour_multiplier"`
	WorkdaySubsequentMultiplier int       `json:"workday_subsequent_multiplier" db:"workday_subsequent_multiplier"`
	HolidayBaseMultiplier       int       `json:"holiday_base_multiplier" db:"holiday_base_multiplier"`
	Holiday9thHourMultiplier    int       `json:"holiday_9th_hour_multiplier" db:"holiday_9th_hour_multiplier"`
	Holiday10PlusMultiplier     int       `json:"holiday_10plus_multiplier" db:"holiday_10plus_multiplier"`
	CreatedAt                   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at" db:"updated_at"`
}

// HourlyRate returns BaseMonthlyRate / HourlyDivisor.
func (o *OvertimeRateConfig) HourlyRate() int64 {
	if o.HourlyDivisor <= 0 {
		return 0
	}
	return o.BaseMonthlyRate / int64(o.HourlyDivisor)
}

// CalculateWorkdayOvertime computes total overtime cost for N hours on a workday.
// Hour 1: 1.5x hourly rate. Hour 2+: 2.0x hourly rate.
func (o *OvertimeRateConfig) CalculateWorkdayOvertime(hours int) int64 {
	if hours <= 0 {
		return 0
	}
	hourly := o.HourlyRate()
	total := hourly * int64(o.WorkdayFirstHourMultiplier) / 100
	if hours > 1 {
		total += hourly * int64(o.WorkdaySubsequentMultiplier) / 100 * int64(hours-1)
	}
	return total
}

// CalculateHolidayOvertime computes total overtime cost for N hours on a holiday.
// Hours 1-8: 2.0x, Hour 9: 3.0x, Hours 10-12: 4.0x.
func (o *OvertimeRateConfig) CalculateHolidayOvertime(hours int) int64 {
	if hours <= 0 {
		return 0
	}
	hourly := o.HourlyRate()
	var total int64

	baseHours := hours
	if baseHours > 8 {
		baseHours = 8
	}
	total += hourly * int64(o.HolidayBaseMultiplier) / 100 * int64(baseHours)

	if hours >= 9 {
		total += hourly * int64(o.Holiday9thHourMultiplier) / 100
	}
	if hours >= 10 {
		extraHours := hours - 9
		total += hourly * int64(o.Holiday10PlusMultiplier) / 100 * int64(extraHours)
	}

	return total
}
