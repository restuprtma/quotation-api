package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// CalculateCloudInfrastructure calculates ordered server and setup lines, then
// reconciles one-time and recurring payments using integer-only arithmetic.
func CalculateCloudInfrastructure(in domain.CloudInfrastructureInput) (domain.CloudInfrastructureCalculation, error) {
	in = in.Normalize()
	if err := in.Validate(); err != nil {
		return domain.CloudInfrastructureCalculation{}, err
	}

	calc := domain.CloudInfrastructureCalculation{
		Packages: make([]domain.CloudPackageLineCalculation, 0, len(in.PackageItems)),
		Setup:    make([]domain.CloudSetupLineCalculation, 0, len(in.SetupItems)),
	}
	for i, item := range in.PackageItems {
		calc.Packages = append(calc.Packages, domain.CloudPackageLineCalculation{
			CloudPackageItemInput: item,
			LineTotal:             item.UnitRate,
		})
		var err error
		calc.MonthlySubtotal, err = checkedAddCloud(calc.MonthlySubtotal, item.UnitRate)
		if err != nil {
			return domain.CloudInfrastructureCalculation{}, fmt.Errorf("package_items[%d]: %w", i, err)
		}
	}

	for i, item := range in.SetupItems {
		calc.Setup = append(calc.Setup, domain.CloudSetupLineCalculation{
			CloudSetupItemInput: item,
			LineTotal:           item.Amount,
		})
		var err error
		calc.SetupSubtotal, err = checkedAddCloud(calc.SetupSubtotal, item.Amount)
		if err != nil {
			return domain.CloudInfrastructureCalculation{}, fmt.Errorf("setup_items[%d]: %w", i, err)
		}
	}

	for i, row := range in.PaymentSchedule {
		switch row.Component {
		case domain.CloudPaymentSetup:
			var err error
			calc.SetupPaymentTotal, err = checkedAddCloud(calc.SetupPaymentTotal, row.Nominal)
			if err != nil {
				return domain.CloudInfrastructureCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: %w", i, err)
			}
		case domain.CloudPaymentMonthly:
			if row.Nominal != calc.MonthlySubtotal {
				return domain.CloudInfrastructureCalculation{}, fmt.Errorf("payment_schedule[%d].nominal: monthly_service nominal %d must equal monthly subtotal %d", i, row.Nominal, calc.MonthlySubtotal)
			}
			calc.MonthlyPaymentRows++
		default:
			return domain.CloudInfrastructureCalculation{}, fmt.Errorf("payment_schedule[%d].component: unsupported value %q", i, row.Component)
		}
	}

	if calc.SetupPaymentTotal != calc.SetupSubtotal {
		return domain.CloudInfrastructureCalculation{}, fmt.Errorf("payment_schedule: setup payment total %d must equal setup subtotal %d", calc.SetupPaymentTotal, calc.SetupSubtotal)
	}
	if calc.MonthlyPaymentRows == 0 {
		return domain.CloudInfrastructureCalculation{}, fmt.Errorf("payment_schedule: missing monthly_service row")
	}

	var err error
	calc.ScheduledRecurringTotal, err = checkedMulCloud(calc.MonthlySubtotal, int64(calc.MonthlyPaymentRows))
	if err != nil {
		return domain.CloudInfrastructureCalculation{}, fmt.Errorf("scheduled recurring total overflow: %w", err)
	}
	calc.PaymentTotal, err = checkedAddCloud(calc.SetupPaymentTotal, calc.ScheduledRecurringTotal)
	if err != nil {
		return domain.CloudInfrastructureCalculation{}, fmt.Errorf("payment total overflow: %w", err)
	}
	return calc, nil
}

func checkedAddCloud(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("negative amount not allowed")
	}
	if a > domain.MaxCloudMoney-b {
		return 0, fmt.Errorf("amount exceeds maximum safe limit")
	}
	return a + b, nil
}

func checkedMulCloud(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, fmt.Errorf("negative multiplier not allowed")
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > domain.MaxCloudMoney/b {
		return 0, fmt.Errorf("multiplication exceeds maximum safe limit")
	}
	return a * b, nil
}
