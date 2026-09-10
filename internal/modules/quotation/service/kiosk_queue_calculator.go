package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CalculateKioskQueueSystem performs safe integer arithmetic to compute
// service subtotal, hardware subtotal, tax amounts, maintenance totals,
// payment target totals, and reconciles payment schedules.
func CalculateKioskQueueSystem(input domain.KioskQueueSystemInput) (domain.KioskQueueCalculation, error) {
	normalized := input.Normalize()
	if err := normalized.Validate(); err != nil {
		return domain.KioskQueueCalculation{}, err
	}

	calc := domain.KioskQueueCalculation{
		Variant:       normalized.Variant,
		ServiceGroups: make([]domain.KioskQueueServiceGroupCalculation, len(normalized.ServiceGroups)),
	}

	var err error
	for gIdx, group := range normalized.ServiceGroups {
		groupCalc := domain.KioskQueueServiceGroupCalculation{
			Title: group.Title,
			Items: make([]domain.KioskQueueServiceLineCalculation, len(group.Items)),
		}
		for iIdx, item := range group.Items {
			lineTotal, lErr := calculateQuarterLine(item.MonthlyRate, item.Headcount, item.PeriodQuarters)
			if lErr != nil {
				return domain.KioskQueueCalculation{}, fmt.Errorf("service_groups[%d].items[%d]: %w", gIdx, iIdx, lErr)
			}
			groupCalc.Items[iIdx] = domain.KioskQueueServiceLineCalculation{
				KioskQueueServiceItemInput: item,
				LineTotal:                  lineTotal,
			}
			calc.ServiceSubtotal, err = checkedKioskAdd(calc.ServiceSubtotal, lineTotal)
			if err != nil {
				return domain.KioskQueueCalculation{}, fmt.Errorf("service_subtotal: %w", err)
			}
		}
		calc.ServiceGroups[gIdx] = groupCalc
	}

	calc.ServiceDiscountAmount = normalized.ServiceDiscountAmount
	if calc.ServiceDiscountAmount > calc.ServiceSubtotal {
		return domain.KioskQueueCalculation{}, fmt.Errorf("service_discount_amount: %d exceeds service subtotal %d", calc.ServiceDiscountAmount, calc.ServiceSubtotal)
	}
	serviceAfterDiscount := calc.ServiceSubtotal - calc.ServiceDiscountAmount

	switch normalized.Variant {
	case domain.KioskQueueGovernmentMPP:
		calc.ServiceTaxAmount, err = calculatePPN11(calc.ServiceSubtotal)
		if err != nil {
			return domain.KioskQueueCalculation{}, fmt.Errorf("service_tax_amount: %w", err)
		}
		calc.ServiceTotal, err = checkedKioskAdd(serviceAfterDiscount, calc.ServiceTaxAmount)
		if err != nil {
			return domain.KioskQueueCalculation{}, fmt.Errorf("service_total: %w", err)
		}

		if normalized.HardwareProvision == domain.HardwareVenturoSupplied && len(normalized.HardwareItems) > 0 {
			calc.Hardware = make([]domain.KioskQueueHardwareLineCalculation, len(normalized.HardwareItems))
			for hIdx, item := range normalized.HardwareItems {
				lineTotal, hErr := checkedKioskMultiply(item.UnitPrice, int64(item.Quantity))
				if hErr != nil {
					return domain.KioskQueueCalculation{}, fmt.Errorf("hardware_items[%d]: %w", hIdx, hErr)
				}
				calc.Hardware[hIdx] = domain.KioskQueueHardwareLineCalculation{
					KioskQueueHardwareItemInput: item,
					LineTotal:                   lineTotal,
				}
				calc.HardwareSubtotal, err = checkedKioskAdd(calc.HardwareSubtotal, lineTotal)
				if err != nil {
					return domain.KioskQueueCalculation{}, fmt.Errorf("hardware_subtotal: %w", err)
				}
			}
			calc.HardwareTaxAmount, err = calculatePPN11(calc.HardwareSubtotal)
			if err != nil {
				return domain.KioskQueueCalculation{}, fmt.Errorf("hardware_tax_amount: %w", err)
			}
			calc.HardwareTotal, err = checkedKioskAdd(calc.HardwareSubtotal, calc.HardwareTaxAmount)
			if err != nil {
				return domain.KioskQueueCalculation{}, fmt.Errorf("hardware_total: %w", err)
			}
		}

		calc.PaymentTargetTotal, err = checkedKioskAdd(calc.ServiceTotal, calc.HardwareTotal)
		if err != nil {
			return domain.KioskQueueCalculation{}, fmt.Errorf("payment_target_total: %w", err)
		}

		for pIdx, p := range normalized.GovernmentPaymentSchedule {
			calc.PaymentTotal, err = checkedKioskAdd(calc.PaymentTotal, p.Nominal)
			if err != nil {
				return domain.KioskQueueCalculation{}, fmt.Errorf("government_payment_schedule[%d]: %w", pIdx, err)
			}
		}

	case domain.KioskQueuePrivateFoodCourt:
		calc.ServiceTaxAmount = 0
		calc.ServiceTotal = serviceAfterDiscount

		if len(normalized.MaintenanceItems) > 0 {
			calc.Maintenance = make([]domain.KioskQueueServiceLineCalculation, len(normalized.MaintenanceItems))
			for mIdx, item := range normalized.MaintenanceItems {
				lineTotal, mErr := calculateQuarterLine(item.MonthlyRate, item.Headcount, item.PeriodQuarters)
				if mErr != nil {
					return domain.KioskQueueCalculation{}, fmt.Errorf("maintenance_items[%d]: %w", mIdx, mErr)
				}
				calc.Maintenance[mIdx] = domain.KioskQueueServiceLineCalculation{
					KioskQueueServiceItemInput: item,
					LineTotal:                  lineTotal,
				}
				calc.MaintenanceTotal, err = checkedKioskAdd(calc.MaintenanceTotal, lineTotal)
				if err != nil {
					return domain.KioskQueueCalculation{}, fmt.Errorf("maintenance_total: %w", err)
				}
			}
		}

		calc.PaymentTargetTotal = calc.ServiceTotal

		for pIdx, p := range normalized.PrivatePaymentSchedule {
			calc.PaymentTotal, err = checkedKioskAdd(calc.PaymentTotal, p.Nominal)
			if err != nil {
				return domain.KioskQueueCalculation{}, fmt.Errorf("private_payment_schedule[%d]: %w", pIdx, err)
			}
		}

	default:
		return domain.KioskQueueCalculation{}, fmt.Errorf("unsupported variant: %q", normalized.Variant)
	}

	if calc.PaymentTotal != calc.PaymentTargetTotal {
		return domain.KioskQueueCalculation{}, fmt.Errorf("payment schedule total %d does not match payment target total %d", calc.PaymentTotal, calc.PaymentTargetTotal)
	}

	return calc, nil
}

func checkedKioskAdd(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("money addition cannot accept negative values: %d, %d", a, b)
	}
	if a > domain.KioskQueueMaxMoney-b {
		return 0, fmt.Errorf("money addition exceeds maximum safe amount: %d + %d", a, b)
	}
	return a + b, nil
}

func checkedKioskMultiply(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("money multiplication cannot accept negative values: %d, %d", a, b)
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > domain.KioskQueueMaxMoney/b {
		return 0, fmt.Errorf("money multiplication exceeds maximum safe amount: %d * %d", a, b)
	}
	return a * b, nil
}

func calculateQuarterLine(rate int64, headcount, periodQuarters int32) (int64, error) {
	if rate < 0 || headcount < 0 || periodQuarters < 0 {
		return 0, fmt.Errorf("quarter line parameters must be non-negative")
	}
	if rate == 0 || headcount == 0 || periodQuarters == 0 {
		return 0, nil
	}
	mult, err := checkedKioskMultiply(int64(headcount), int64(periodQuarters))
	if err != nil {
		return 0, err
	}
	product, err := checkedKioskMultiply(rate, mult)
	if err != nil {
		return 0, err
	}
	if product%4 != 0 {
		return 0, fmt.Errorf("line total must be a whole integer rupiah, got remainder %d", product%4)
	}
	return product / 4, nil
}

func calculatePPN11(subtotal int64) (int64, error) {
	if subtotal < 0 {
		return 0, fmt.Errorf("subtotal must be non-negative")
	}
	if subtotal == 0 {
		return 0, nil
	}
	prod, err := checkedKioskMultiply(subtotal, 1100)
	if err != nil {
		return 0, err
	}
	if prod%10000 != 0 {
		return 0, fmt.Errorf("fractional rupiah in PPN 11%%: %d * 1100 %% 10000 != 0", subtotal)
	}
	return prod / 10000, nil
}
