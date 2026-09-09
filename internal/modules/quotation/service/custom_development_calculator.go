package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CalculateCustomDevelopment calculates line totals, development subtotal,
// total investment, monthly total, and reconciles project and monthly payments.
func CalculateCustomDevelopment(in domain.CustomDevelopmentInput) (domain.CustomDevelopmentCalculation, error) {
	if err := in.Validate(); err != nil {
		return domain.CustomDevelopmentCalculation{}, err
	}

	calc := domain.CustomDevelopmentCalculation{
		DevelopmentGroups: make([]domain.CustomDevelopmentCostGroupCalculation, 0, len(in.DevelopmentGroups)),
		MonthlyLines:      make([]domain.CustomDevelopmentMonthlyLineCalculation, 0, len(in.MonthlyItems)),
	}

	for _, group := range in.DevelopmentGroups {
		groupCalc := domain.CustomDevelopmentCostGroupCalculation{
			Title: group.Title,
			Items: make([]domain.CustomDevelopmentCostLineCalculation, 0, len(group.Items)),
		}
		for _, item := range group.Items {
			lineCalc := domain.CustomDevelopmentCostLineCalculation{
				CustomDevelopmentCostItemInput: item,
				LineTotal:                      item.QuotedLineTotal,
			}
			groupCalc.Items = append(groupCalc.Items, lineCalc)

			if item.QuotedLineTotal != nil {
				var err error
				calc.DevelopmentSubtotal, err = checkedAddCustomDev(calc.DevelopmentSubtotal, *item.QuotedLineTotal)
				if err != nil {
					return domain.CustomDevelopmentCalculation{}, fmt.Errorf("development_groups: subtotal overflow: %w", err)
				}
			}
		}
		calc.DevelopmentGroups = append(calc.DevelopmentGroups, groupCalc)
	}

	calc.DiscountAmount = in.DiscountAmount
	if calc.DiscountAmount > calc.DevelopmentSubtotal {
		return domain.CustomDevelopmentCalculation{}, fmt.Errorf("discount_amount: cannot exceed development subtotal")
	}
	calc.TotalInvestment = calc.DevelopmentSubtotal - calc.DiscountAmount

	for i, item := range in.MonthlyItems {
		lineTotal, err := checkedMulCustomDev(item.UnitRate, int64(item.Quantity))
		if err != nil {
			return domain.CustomDevelopmentCalculation{}, fmt.Errorf("monthly_items[%d]: line total overflow: %w", i, err)
		}
		calc.MonthlyLines = append(calc.MonthlyLines, domain.CustomDevelopmentMonthlyLineCalculation{
			CustomDevelopmentMonthlyItemInput: item,
			LineTotal:                         lineTotal,
		})
		calc.MonthlyTotal, err = checkedAddCustomDev(calc.MonthlyTotal, lineTotal)
		if err != nil {
			return domain.CustomDevelopmentCalculation{}, fmt.Errorf("monthly_items: total overflow: %w", err)
		}
	}

	for i, row := range in.PaymentSchedule {
		switch row.Component {
		case domain.CustomDevelopmentPaymentProject:
			var err error
			calc.DevelopmentPaymentTotal, err = checkedAddCustomDev(calc.DevelopmentPaymentTotal, row.Nominal)
			if err != nil {
				return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: %w", i, err)
			}
		case domain.CustomDevelopmentPaymentMonthly:
			if calc.MonthlyTotal <= 0 {
				return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule[%d]: unexpected monthly recurring payment row when monthly total is zero", i)
			}
			if row.Nominal != calc.MonthlyTotal {
				return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: monthly payment nominal %d must equal monthly total %d", i, row.Nominal, calc.MonthlyTotal)
			}
			calc.MonthlyPaymentRows++
		default:
			return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule[%d].component: unsupported value %q", i, row.Component)
		}
	}

	if calc.DevelopmentPaymentTotal != calc.TotalInvestment {
		return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule: development payment total %d must equal total investment %d", calc.DevelopmentPaymentTotal, calc.TotalInvestment)
	}

	if calc.MonthlyTotal == 0 && calc.MonthlyPaymentRows > 0 {
		return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule: unexpected monthly recurring payment row when monthly total is zero")
	}

	if calc.MonthlyTotal > 0 && calc.MonthlyPaymentRows == 0 {
		return domain.CustomDevelopmentCalculation{}, fmt.Errorf("payment_schedule: missing monthly recurring payment row for monthly total %d", calc.MonthlyTotal)
	}

	return calc, nil
}

func checkedAddCustomDev(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("negative amount not allowed")
	}
	if a > domain.MaxCustomDevelopmentMoney-b {
		return 0, fmt.Errorf("amount exceeds maximum safe limit")
	}
	return a + b, nil
}

func checkedMulCustomDev(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("negative multiplier not allowed")
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > domain.MaxCustomDevelopmentMoney/b {
		return 0, fmt.Errorf("multiplication exceeds maximum safe limit")
	}
	return a * b, nil
}
