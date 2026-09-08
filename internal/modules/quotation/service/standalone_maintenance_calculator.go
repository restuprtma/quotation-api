package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CalculateStandaloneMaintenance calculates line totals, subtotal, total investment,
// and reconciles the payment schedule for a standalone maintenance quotation.
func CalculateStandaloneMaintenance(input domain.StandaloneMaintenanceInput) (domain.StandaloneMaintenanceCalculation, error) {
	if err := input.Validate(); err != nil {
		return domain.StandaloneMaintenanceCalculation{}, err
	}

	calc := domain.StandaloneMaintenanceCalculation{
		Mode:           input.Mode,
		DiscountAmount: input.DiscountAmount,
	}

	switch input.Mode {
	case domain.StandaloneMaintenanceRetainer:
		calc.Retainer = make([]domain.MaintenanceRetainerLineCalculation, 0, len(input.RetainerItems))
		for i, item := range input.RetainerItems {
			numerator, err := checkedMulStandaloneMaintenance(item.MonthlyRate, int64(item.FTEQuarters), int64(item.PeriodMonths))
			if err != nil {
				return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("retainer_items[%d]: %w", i, err)
			}
			if numerator%4 != 0 {
				return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("retainer_items[%d]: line calculation must be a whole rupiah amount (divisible by 4)", i)
			}
			lineTotal := numerator / 4
			calc.Retainer = append(calc.Retainer, domain.MaintenanceRetainerLineCalculation{
				MaintenanceRetainerItemInput: item,
				LineTotal:                    lineTotal,
			})
			calc.Subtotal, err = checkedAddStandaloneMaintenance(calc.Subtotal, lineTotal)
			if err != nil {
				return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("subtotal: %w", err)
			}
		}

	case domain.StandaloneMaintenanceHourlyTask:
		calc.Hourly = make([]domain.MaintenanceHourlyLineCalculation, 0, len(input.HourlyCostItems))
		for i, item := range input.HourlyCostItems {
			if item.QuotedLineTotal <= 0 {
				return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("hourly_cost_items[%d].quoted_line_total: must be greater than zero", i)
			}
			if item.QuotedLineTotal > domain.MaxStandaloneMaintenanceMoney {
				return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("hourly_cost_items[%d].quoted_line_total: exceeds maximum safe amount", i)
			}
			calc.Hourly = append(calc.Hourly, domain.MaintenanceHourlyLineCalculation{
				MaintenanceHourlyCostItemInput: item,
				LineTotal:                      item.QuotedLineTotal,
			})
			var err error
			calc.Subtotal, err = checkedAddStandaloneMaintenance(calc.Subtotal, item.QuotedLineTotal)
			if err != nil {
				return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("subtotal: %w", err)
			}
		}

	default:
		return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("mode: unsupported value %q", input.Mode)
	}

	if input.DiscountAmount < 0 {
		return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("discount_amount: must be greater than or equal to zero")
	}
	if input.DiscountAmount > domain.MaxStandaloneMaintenanceMoney {
		return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("discount_amount: exceeds maximum safe amount")
	}
	if input.DiscountAmount > calc.Subtotal {
		return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("discount_amount: cannot exceed subtotal")
	}

	calc.TotalInvestment = calc.Subtotal - input.DiscountAmount

	for i, row := range input.PaymentSchedule {
		if row.Nominal <= 0 {
			return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: must be greater than zero", i)
		}
		if row.Nominal > domain.MaxStandaloneMaintenanceMoney {
			return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: exceeds maximum safe amount", i)
		}
		var err error
		calc.PaymentTotal, err = checkedAddStandaloneMaintenance(calc.PaymentTotal, row.Nominal)
		if err != nil {
			return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("payment_schedule: %w", err)
		}
	}

	if calc.PaymentTotal != calc.TotalInvestment {
		return domain.StandaloneMaintenanceCalculation{}, fmt.Errorf("payment_schedule: total %d must equal total investment %d", calc.PaymentTotal, calc.TotalInvestment)
	}

	return calc, nil
}

func checkedMulStandaloneMaintenance(values ...int64) (int64, error) {
	result := int64(1)
	for _, value := range values {
		if value <= 0 || result > domain.MaxStandaloneMaintenanceMoney/value {
			return 0, fmt.Errorf("money multiplication exceeds limit")
		}
		result *= value
	}
	return result, nil
}

func checkedAddStandaloneMaintenance(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > domain.MaxStandaloneMaintenanceMoney-right {
		return 0, fmt.Errorf("money addition exceeds limit")
	}
	return left + right, nil
}

// CheckedMulStandaloneMaintenance exposes checked multiplication for testing and reuse.
func CheckedMulStandaloneMaintenance(values ...int64) (int64, error) {
	return checkedMulStandaloneMaintenance(values...)
}

// CheckedAddStandaloneMaintenance exposes checked addition for testing and reuse.
func CheckedAddStandaloneMaintenance(left, right int64) (int64, error) {
	return checkedAddStandaloneMaintenance(left, right)
}
