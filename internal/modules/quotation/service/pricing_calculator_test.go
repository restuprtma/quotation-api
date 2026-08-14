package service

import (
	"math"
	"testing"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func TestPricingCalculator_CalculateItem(t *testing.T) {
	calc := NewPricingCalculator()

	tests := []struct {
		name          string
		input         dto.PricingItemDTO
		expectedTotal int64
		expectedError error
	}{
		{
			name: "unit normal",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeUnit,
				UnitPrice:   4000,
				Quantity:    200,
				LineTotal:   9999999, // Fake client total, should be ignored
			},
			expectedTotal: 800000,
			expectedError: nil,
		},
		{
			name: "resource normal",
			input: dto.PricingItemDTO{
				PricingMode:   domain.PricingModeResource,
				UnitPrice:     11550000,
				EmployeeCount: 1,
				Duration:      1,
			},
			expectedTotal: 11550000,
			expectedError: nil,
		},
		{
			name: "fixed normal",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeFixed,
				UnitPrice:   25000000,
				Quantity:    10, // Should be ignored in fixed
			},
			expectedTotal: 25000000,
			expectedError: nil,
		},
		{
			name: "zero quantity in unit mode",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeUnit,
				UnitPrice:   4000,
				Quantity:    0,
			},
			expectedTotal: 0,
			expectedError: ErrZeroMultiplier,
		},
		{
			name: "zero duration in resource mode",
			input: dto.PricingItemDTO{
				PricingMode:   domain.PricingModeResource,
				UnitPrice:     11550000,
				EmployeeCount: 1,
				Duration:      0,
			},
			expectedTotal: 0,
			expectedError: ErrZeroMultiplier,
		},
		{
			name: "negative unit price",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeUnit,
				UnitPrice:   -4000,
				Quantity:    1,
			},
			expectedTotal: 0,
			expectedError: ErrNegativeValue,
		},
		{
			name: "negative quantity",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeUnit,
				UnitPrice:   4000,
				Quantity:    -5,
			},
			expectedTotal: 0,
			expectedError: ErrNegativeValue,
		},
		{
			name: "negative employee count",
			input: dto.PricingItemDTO{
				PricingMode:   domain.PricingModeResource,
				UnitPrice:     5000,
				EmployeeCount: -2,
				Duration:      1,
			},
			expectedTotal: 0,
			expectedError: ErrNegativeValue,
		},
		{
			name: "near limit valid integer",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeUnit,
				UnitPrice:   math.MaxInt64 / 2,
				Quantity:    2,
			},
			expectedTotal: (math.MaxInt64 / 2) * 2,
			expectedError: nil,
		},
		{
			name: "integer overflow in unit mode",
			input: dto.PricingItemDTO{
				PricingMode: domain.PricingModeUnit,
				UnitPrice:   math.MaxInt64,
				Quantity:    2,
			},
			expectedTotal: 0,
			expectedError: ErrIntegerOverflow,
		},
		{
			name: "invalid pricing mode",
			input: dto.PricingItemDTO{
				PricingMode: "random_mode",
				UnitPrice:   1000,
			},
			expectedTotal: 0,
			expectedError: ErrInvalidPricingMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, err := calc.CalculateItem(&tt.input)
			if err != tt.expectedError {
				t.Errorf("CalculateItem() error = %v, want %v", err, tt.expectedError)
			}
			if total != tt.expectedTotal {
				t.Errorf("CalculateItem() total = %v, want %v", total, tt.expectedTotal)
			}
			if err == nil && tt.input.LineTotal != tt.expectedTotal {
				t.Errorf("CalculateItem() did not update LineTotal properly. Got %v, want %v", tt.input.LineTotal, tt.expectedTotal)
			}
		})
	}
}

func TestPricingCalculator_CalculateTotals(t *testing.T) {
	calc := NewPricingCalculator()

	sections := []dto.PricingSectionDTO{
		{
			SectionName: "One Time Cost",
			Items: []dto.PricingItemDTO{
				{
					PricingMode:   domain.PricingModeUnit,
					UnitPrice:     5000,
					Quantity:      10,
					BillingPeriod: domain.BillingPeriodOneTime,
				},
				{
					PricingMode:   domain.PricingModeFixed,
					UnitPrice:     20000,
					BillingPeriod: domain.BillingPeriodOneTime,
				},
			},
		},
		{
			SectionName: "Recurring Cost",
			Items: []dto.PricingItemDTO{
				{
					PricingMode:   domain.PricingModeResource,
					UnitPrice:     3000,
					EmployeeCount: 2,
					Duration:      3,
					BillingPeriod: domain.BillingPeriodMonthly,
				},
				{
					PricingMode:   domain.PricingModeUnit,
					UnitPrice:     1000,
					Quantity:      12,
					BillingPeriod: domain.BillingPeriodYearly,
				},
			},
		},
	}

	oneTime, recurring, err := calc.CalculateTotals(sections)
	if err != nil {
		t.Fatalf("CalculateTotals() unexpected error: %v", err)
	}

	// One Time = (5000 * 10) + 20000 = 70000
	if oneTime != 70000 {
		t.Errorf("CalculateTotals() oneTime = %v, want 70000", oneTime)
	}

	// Recurring = (3000 * 2 * 3) + (1000 * 12) = 18000 + 12000 = 30000
	if recurring != 30000 {
		t.Errorf("CalculateTotals() recurring = %v, want 30000", recurring)
	}
}
