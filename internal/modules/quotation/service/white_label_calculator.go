package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CalculateWhiteLabel calculates subtotal per line, component subtotals,
// total investment, and reconciles payment schedule per component.
func CalculateWhiteLabel(input domain.WhiteLabelInput) (domain.WhiteLabelCalculation, error) {
	if err := input.Validate(); err != nil {
		return domain.WhiteLabelCalculation{}, err
	}

	calc := domain.WhiteLabelCalculation{}

	if len(input.LicenseItems) > 0 {
		calc.License = make([]domain.WhiteLabelLicenseLineCalculation, 0, len(input.LicenseItems))
		for i, item := range input.LicenseItems {
			lineTotal, err := checkedMulWhiteLabel(item.UnitRate, int64(item.Quantity))
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("license_items[%d]: %w", i, err)
			}
			calc.License = append(calc.License, domain.WhiteLabelLicenseLineCalculation{
				WhiteLabelLicenseItemInput: item,
				LineTotal:                  lineTotal,
			})
			calc.LicenseSubtotal, err = checkedAddWhiteLabel(calc.LicenseSubtotal, lineTotal)
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("license_subtotal: %w", err)
			}
		}
	}

	if len(input.MaintenanceItems) > 0 {
		calc.Maintenance = make([]domain.WhiteLabelMaintenanceLineCalculation, 0, len(input.MaintenanceItems))
		for i, item := range input.MaintenanceItems {
			numerator, err := checkedMulWhiteLabel(item.MonthlyRate, int64(item.FTEQuarters), int64(item.PeriodMonths))
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("maintenance_items[%d]: %w", i, err)
			}
			if numerator%4 != 0 {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("maintenance_items[%d]: line calculation must be a whole rupiah amount (divisible by 4)", i)
			}
			lineTotal := numerator / 4
			calc.Maintenance = append(calc.Maintenance, domain.WhiteLabelMaintenanceLineCalculation{
				WhiteLabelMaintenanceItemInput: item,
				LineTotal:                      lineTotal,
			})
			calc.MaintenanceSubtotal, err = checkedAddWhiteLabel(calc.MaintenanceSubtotal, lineTotal)
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("maintenance_subtotal: %w", err)
			}
		}
	}

	if len(input.HostingItems) > 0 {
		calc.Hosting = make([]domain.WhiteLabelHostingLineCalculation, 0, len(input.HostingItems))
		for i, item := range input.HostingItems {
			lineTotal, err := checkedMulWhiteLabel(item.MonthlyRate, int64(item.Quantity), int64(item.PeriodMonths))
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("hosting_items[%d]: %w", i, err)
			}
			calc.Hosting = append(calc.Hosting, domain.WhiteLabelHostingLineCalculation{
				WhiteLabelHostingItemInput: item,
				LineTotal:                  lineTotal,
			})
			calc.HostingSubtotal, err = checkedAddWhiteLabel(calc.HostingSubtotal, lineTotal)
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("hosting_subtotal: %w", err)
			}
		}
	}

	var err error
	calc.MaintenanceHostingSubtotal, err = checkedAddWhiteLabel(calc.MaintenanceSubtotal, calc.HostingSubtotal)
	if err != nil {
		return domain.WhiteLabelCalculation{}, fmt.Errorf("maintenance_hosting_subtotal: %w", err)
	}

	calc.TotalInvestment, err = checkedAddWhiteLabel(calc.LicenseSubtotal, calc.MaintenanceHostingSubtotal)
	if err != nil {
		return domain.WhiteLabelCalculation{}, fmt.Errorf("total_investment: %w", err)
	}

	for i, row := range input.PaymentSchedule {
		switch row.Component {
		case domain.WhiteLabelPaymentLicense:
			calc.LicensePaymentTotal, err = checkedAddWhiteLabel(calc.LicensePaymentTotal, row.Nominal)
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: %w", i, err)
			}
		case domain.WhiteLabelPaymentMaintenanceHosting:
			calc.MaintenanceHostingPaymentTotal, err = checkedAddWhiteLabel(calc.MaintenanceHostingPaymentTotal, row.Nominal)
			if err != nil {
				return domain.WhiteLabelCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: %w", i, err)
			}
		}
	}

	calc.PaymentTotal, err = checkedAddWhiteLabel(calc.LicensePaymentTotal, calc.MaintenanceHostingPaymentTotal)
	if err != nil {
		return domain.WhiteLabelCalculation{}, fmt.Errorf("payment_total: %w", err)
	}

	if calc.LicensePaymentTotal != calc.LicenseSubtotal {
		return domain.WhiteLabelCalculation{}, fmt.Errorf("payment_schedule.license: total %d must equal license subtotal %d", calc.LicensePaymentTotal, calc.LicenseSubtotal)
	}
	if calc.MaintenanceHostingPaymentTotal != calc.MaintenanceHostingSubtotal {
		return domain.WhiteLabelCalculation{}, fmt.Errorf("payment_schedule.maintenance_hosting: total %d must equal maintenance and hosting subtotal %d", calc.MaintenanceHostingPaymentTotal, calc.MaintenanceHostingSubtotal)
	}

	return calc, nil
}

func checkedMulWhiteLabel(values ...int64) (int64, error) {
	result := int64(1)
	for _, value := range values {
		if value <= 0 || result > domain.MaxWhiteLabelMoney/value {
			return 0, fmt.Errorf("money multiplication exceeds limit")
		}
		result *= value
	}
	return result, nil
}

func checkedAddWhiteLabel(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > domain.MaxWhiteLabelMoney-right {
		return 0, fmt.Errorf("money addition exceeds limit")
	}
	return left + right, nil
}
