package service

import (
	"errors"
	"math"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var (
	ErrInvalidPricingMode = errors.New("invalid pricing mode")
	ErrNegativeValue      = errors.New("negative values are not allowed")
	ErrZeroMultiplier     = errors.New("multiplier (quantity, employee, or duration) cannot be zero")
	ErrIntegerOverflow    = errors.New("integer overflow during price calculation")
)

// PricingCalculator encapsulates the deterministic logic for pricing items.
type PricingCalculator struct{}

// NewPricingCalculator creates a new instance of the calculator.
func NewPricingCalculator() *PricingCalculator {
	return &PricingCalculator{}
}

// CalculateTotals evaluates a list of pricing sections and items, returning
// the calculated line_total for each item, and aggregating the overall
// one-time and recurring totals for the entire quotation.
func (c *PricingCalculator) CalculateTotals(sections []dto.PricingSectionDTO) (oneTimeTotal int64, recurringTotal int64, err error) {
	for i := range sections {
		for j := range sections[i].Items {
			lineTotal, err := c.CalculateItem(&sections[i].Items[j])
			if err != nil {
				return 0, 0, err
			}

			// Aggregate based on billing period
			if sections[i].Items[j].BillingPeriod == domain.BillingPeriodOneTime {
				oneTimeTotal, err = addSafe(oneTimeTotal, lineTotal)
				if err != nil {
					return 0, 0, err
				}
			} else {
				// Any period other than one_time goes into recurring total
				recurringTotal, err = addSafe(recurringTotal, lineTotal)
				if err != nil {
					return 0, 0, err
				}
			}
		}
	}
	return oneTimeTotal, recurringTotal, nil
}

// CalculateItem evaluates a single item's pricing inputs and updates its LineTotal field.
func (c *PricingCalculator) CalculateItem(item *dto.PricingItemDTO) (int64, error) {
	if item.UnitPrice < 0 {
		return 0, ErrNegativeValue
	}

	var total int64
	var err error

	switch item.PricingMode {
	case domain.PricingModeFixed:
		total = item.UnitPrice

	case domain.PricingModeUnit:
		if item.Quantity < 0 {
			return 0, ErrNegativeValue
		}
		if item.Quantity == 0 {
			return 0, ErrZeroMultiplier
		}
		total, err = multiplySafe(item.UnitPrice, item.Quantity)
		if err != nil {
			return 0, err
		}

	case domain.PricingModeResource:
		if item.EmployeeCount < 0 || item.Duration < 0 {
			return 0, ErrNegativeValue
		}
		if item.EmployeeCount == 0 || item.Duration == 0 {
			return 0, ErrZeroMultiplier
		}

		// price * employee
		temp, err := multiplySafe(item.UnitPrice, int64(item.EmployeeCount))
		if err != nil {
			return 0, err
		}
		// (price * employee) * duration
		total, err = multiplySafe(temp, int64(item.Duration))
		if err != nil {
			return 0, err
		}

	default:
		return 0, ErrInvalidPricingMode
	}

	// Update the item ignoring the client's line total
	item.LineTotal = total
	return total, nil
}

// multiplySafe multiplies two int64 values and returns an error on overflow.
func multiplySafe(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	res := a * b
	if a == 1 || b == 1 {
		return res, nil
	}
	if a == math.MinInt64 || b == math.MinInt64 {
		return 0, ErrIntegerOverflow
	}
	if res/b != a {
		return 0, ErrIntegerOverflow
	}
	return res, nil
}

// addSafe adds two int64 values and returns an error on overflow.
func addSafe(a, b int64) (int64, error) {
	res := a + b
	if (res > a) != (b > 0) {
		return 0, ErrIntegerOverflow
	}
	return res, nil
}
