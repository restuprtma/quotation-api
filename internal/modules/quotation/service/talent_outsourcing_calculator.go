package service

import (
	"fmt"
	"math"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func checkedAddTalent(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a > domain.MaxTalentMoney-b {
		return 0, fmt.Errorf("money total exceeds maximum safe amount")
	}
	return a + b, nil
}
func checkedMulTalent(a, b int64) (int64, error) {
	if a < 0 || b < 0 || (a != 0 && b > domain.MaxTalentMoney/a) {
		return 0, fmt.Errorf("money multiplication exceeds maximum safe amount")
	}
	return a * b, nil
}
func checkedMulTalentRaw(a, b int64) (int64, error) {
	if a < 0 || b < 0 || (a != 0 && b > math.MaxInt64/a) {
		return 0, fmt.Errorf("money intermediate multiplication overflows int64")
	}
	return a * b, nil
}
func mulDivExactTalent(a, b, d int64) (int64, error) {
	n, e := checkedMulTalentRaw(a, b)
	if e != nil {
		return 0, e
	}
	if d <= 0 || n%d != 0 {
		return 0, fmt.Errorf("calculation must produce whole rupiah")
	}
	r := n / d
	if r > domain.MaxTalentMoney {
		return 0, fmt.Errorf("money result exceeds maximum safe amount")
	}
	return r, nil
}
func roundHalfUpTalent(n, d int64) (int64, error) {
	if n < 0 || d <= 0 {
		return 0, fmt.Errorf("invalid rounding input")
	}
	if n > math.MaxInt64-d/2 {
		return 0, fmt.Errorf("rounding intermediate overflows int64")
	}
	r := (n + d/2) / d
	if r > domain.MaxTalentMoney {
		return 0, fmt.Errorf("rounded result exceeds maximum safe amount")
	}
	return r, nil
}

func CalculateTalentOutsourcing(raw domain.TalentOutsourcingInput) (domain.TalentOutsourcingCalculation, error) {
	in := domain.NormalizeTalentOutsourcingInput(raw)
	if err := in.Validate(); err != nil {
		return domain.TalentOutsourcingCalculation{}, err
	}
	calc := domain.TalentOutsourcingCalculation{EngagementMode: in.EngagementMode}
	if in.EngagementMode == domain.TalentModeManpowerSupply {
		calc.CostGroups = make([]domain.TalentCostGroupCalculation, 0, len(in.CostGroups))
		for gi, g := range in.CostGroups {
			gc := domain.TalentCostGroupCalculation{Category: g.Category, Title: g.Title, Items: make([]domain.TalentCostLineCalculation, 0, len(g.Items))}
			for ii, item := range g.Items {
				base, err := checkedMulTalentRaw(item.UnitRate, int64(item.Quantity))
				if err != nil {
					return calc, fmt.Errorf("cost_groups[%d].items[%d]: %w", gi, ii, err)
				}
				var line int64
				if item.PeriodUnit == domain.TalentPeriodWorkday {
					line, err = mulDivExactTalent(base, int64(item.PeriodCount), 20)
				} else {
					line, err = checkedMulTalent(base, int64(item.PeriodCount))
				}
				if err != nil {
					return calc, fmt.Errorf("cost_groups[%d].items[%d]: %w", gi, ii, err)
				}
				gc.Items = append(gc.Items, domain.TalentCostLineCalculation{TalentCostItemInput: item, LineTotal: line})
				if g.Category == domain.TalentCategoryPersonnel {
					calc.PersonnelSubtotal, err = checkedAddTalent(calc.PersonnelSubtotal, line)
				} else {
					calc.NonPersonnelSubtotal, err = checkedAddTalent(calc.NonPersonnelSubtotal, line)
				}
				if err != nil {
					return calc, err
				}
			}
			calc.CostGroups = append(calc.CostGroups, gc)
		}
		var err error
		calc.TotalBeforeTax, err = checkedAddTalent(calc.PersonnelSubtotal, calc.NonPersonnelSubtotal)
		if err != nil {
			return calc, err
		}
	} else {
		for i, item := range in.PreparationItems {
			calc.Preparation = append(calc.Preparation, domain.TalentPreparationLineCalculation{TalentPreparationItemInput: item, LineTotal: item.QuotedLineTotal})
			var err error
			calc.PreparationSubtotal, err = checkedAddTalent(calc.PreparationSubtotal, item.QuotedLineTotal)
			if err != nil {
				return calc, fmt.Errorf("preparation_items[%d]: %w", i, err)
			}
		}
		if in.PreparationDiscountAmount > calc.PreparationSubtotal {
			return calc, fmt.Errorf("preparation_discount_amount exceeds preparation subtotal")
		}
		calc.PreparationDiscountAmount = in.PreparationDiscountAmount
		for i, item := range in.SquadItems {
			a, err := checkedMulTalent(item.MonthlyRate, int64(item.Headcount))
			if err != nil {
				return calc, fmt.Errorf("squad_items[%d]: %w", i, err)
			}
			line, err := checkedMulTalent(a, int64(item.PeriodMonths))
			if err != nil {
				return calc, fmt.Errorf("squad_items[%d]: %w", i, err)
			}
			calc.Squad = append(calc.Squad, domain.TalentSquadLineCalculation{TalentSquadItemInput: item, LineTotal: line})
			calc.SquadSubtotal, err = checkedAddTalent(calc.SquadSubtotal, line)
			if err != nil {
				return calc, err
			}
		}
		var err error
		calc.TotalBeforeTax, err = checkedAddTalent(calc.PreparationSubtotal-in.PreparationDiscountAmount, calc.SquadSubtotal)
		if err != nil {
			return calc, err
		}
	}
	var err error
	if in.TaxTreatment == domain.TalentTaxBPJSDPP12 {
		n, e := checkedMulTalentRaw(calc.TotalBeforeTax, 11)
		if e != nil {
			return calc, fmt.Errorf("dpp: %w", e)
		}
		calc.DPPAmount = n / 12
		n, e = checkedMulTalentRaw(calc.DPPAmount, 12)
		if e != nil {
			return calc, fmt.Errorf("tax: %w", e)
		}
		calc.TaxAmount, err = roundHalfUpTalent(n, 100)
		if err != nil {
			return calc, err
		}
	}
	calc.GrandTotal, err = checkedAddTalent(calc.TotalBeforeTax, calc.TaxAmount)
	if err != nil {
		return calc, err
	}
	for i, p := range in.PaymentSchedule {
		calc.PaymentTotal, err = checkedAddTalent(calc.PaymentTotal, p.Nominal)
		if err != nil {
			return calc, fmt.Errorf("payment_schedule[%d]: %w", i, err)
		}
	}
	if calc.PaymentTotal != calc.GrandTotal {
		return calc, fmt.Errorf("payment total %d must equal grand total %d", calc.PaymentTotal, calc.GrandTotal)
	}
	return calc, nil
}
