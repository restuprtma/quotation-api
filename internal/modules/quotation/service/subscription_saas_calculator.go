package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CalculateSubscriptionSaaS calculates recurring subtotals, one-time totals,
// grand total contract amount, and reconciles the payment schedule for a SaaS quotation.
func CalculateSubscriptionSaaS(input domain.SubscriptionSaaSInput) (domain.SubscriptionSaaSCalculation, error) {
	if err := input.Validate(); err != nil {
		return domain.SubscriptionSaaSCalculation{}, err
	}
	out := domain.SubscriptionSaaSCalculation{
		OneTimeItems: append([]domain.SubscriptionOneTimeItemInput(nil), input.OneTimeItems...),
		Payments:     append([]domain.SubscriptionPaymentRowInput(nil), input.PaymentSchedule...),
	}
	for _, period := range input.PricePeriods {
		subtotal, err := checkedMul(period.MonthlyRate, int64(period.DurationMonths))
		if err != nil {
			return out, fmt.Errorf("price_periods: subtotal overflow")
		}
		out.RecurringTotal, err = checkedAdd(out.RecurringTotal, subtotal)
		if err != nil {
			return out, fmt.Errorf("price_periods: total overflow")
		}
		out.PricePeriods = append(out.PricePeriods, domain.SubscriptionPricePeriodCalculation{
			SubscriptionPricePeriodInput: period,
			Subtotal:                     subtotal,
		})
	}
	for _, item := range input.OneTimeItems {
		var err error
		out.OneTimeTotal, err = checkedAdd(out.OneTimeTotal, item.Amount)
		if err != nil {
			return out, fmt.Errorf("one_time_items: total overflow")
		}
	}
	total, err := checkedAdd(out.RecurringTotal, out.OneTimeTotal)
	if err != nil {
		return out, fmt.Errorf("total_contract: overflow")
	}
	if total > domain.MaxSubscriptionSafeAmount {
		return out, fmt.Errorf("total_contract: exceeds JSON safe integer limit")
	}
	out.TotalContract = total
	var paymentTotal int64
	for _, row := range out.Payments {
		paymentTotal, err = checkedAdd(paymentTotal, row.Nominal)
		if err != nil {
			return out, fmt.Errorf("payment_schedule: total overflow")
		}
	}
	if paymentTotal != out.TotalContract {
		return out, fmt.Errorf("payment_schedule: nominal total must equal total contract")
	}
	return out, nil
}
