package service

import (
	"context"
	"fmt"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type DedicatedRateLookup interface {
	ResolveItem(ctx context.Context, companyID, rateCardID, role, seniority, termType string) (*domain.RateCardItem, error)
}

type DedicatedSquadCalculator struct {
	rates DedicatedRateLookup
}

func NewDedicatedSquadCalculator(rates DedicatedRateLookup) *DedicatedSquadCalculator {
	return &DedicatedSquadCalculator{rates: rates}
}

func (c *DedicatedSquadCalculator) Calculate(ctx context.Context, companyID, rateCardID string, input domain.DedicatedSquadInput) (domain.DedicatedSquadCalculation, error) {
	if err := input.Validate(); err != nil {
		return domain.DedicatedSquadCalculation{}, err
	}

	result := domain.DedicatedSquadCalculation{Model: input.Model}
	development, developmentMonthly, err := c.calculateLines(ctx, companyID, rateCardID, input.Model, input.DevelopmentSquad)
	if err != nil {
		return result, fmt.Errorf("development_squad: %w", err)
	}
	result.Development = development
	result.DevelopmentSubtotal, err = sumLineTotals(development)
	if err != nil {
		return result, fmt.Errorf("development subtotal: %w", err)
	}

	rules, err := EvaluateDedicatedSquadRules(input, result.DevelopmentSubtotal)
	if err != nil {
		return result, err
	}

	if rules.IncludeMaintenance {
		result.Maintenance, _, err = c.calculateLines(ctx, companyID, rateCardID, domain.ModelSOWLarge, input.MaintenanceSquad)
		if err != nil {
			return result, fmt.Errorf("maintenance_squad: %w", err)
		}
		result.MaintenanceSubtotal, err = sumLineTotals(result.Maintenance)
		if err != nil {
			return result, fmt.Errorf("maintenance subtotal: %w", err)
		}
	}

	result.Preparation, err = c.calculatePreparation(ctx, companyID, rateCardID, input, rules)
	if err != nil {
		return result, err
	}
	result.Total, err = checkedSum(result.DevelopmentSubtotal, result.MaintenanceSubtotal, result.Preparation.Gross)
	if err != nil {
		return result, fmt.Errorf("total: %w", err)
	}
	result.Discount = result.Preparation.Discount
	result.TotalInvestment, err = checkedAdd(result.Total, -result.Discount)
	if err != nil {
		return result, fmt.Errorf("total investment: %w", err)
	}

	result.Payments, err = buildDedicatedPayments(input, rules, result, developmentMonthly)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (c *DedicatedSquadCalculator) calculateLines(ctx context.Context, companyID, rateCardID string, model domain.SquadModel, inputs []domain.SquadLineInput) ([]domain.SquadLineCalculation, []int64, error) {
	lines := make([]domain.SquadLineCalculation, 0, len(inputs))
	var monthly []int64
	for i, line := range inputs {
		term := termForDedicatedLine(model, line)
		item, err := c.rates.ResolveItem(ctx, companyID, rateCardID, line.Role, line.Seniority, term)
		if err != nil {
			return nil, nil, fmt.Errorf("[%d] resolve rate: %w", i, err)
		}
		if item.AmountPerMonth <= 0 {
			return nil, nil, fmt.Errorf("[%d] amount_per_month: must be positive", i)
		}

		amountPerUnit := item.AmountPerMonth
		var total int64
		if model == domain.ModelSprint {
			amountPerUnit = item.AmountPerMonth / 22
			total, err = checkedProductDivide([]int64{amountPerUnit, int64(line.HeadcountQuarters), int64(line.WorkdayCount)}, 4)
		} else {
			total, err = checkedProductDivide([]int64{item.AmountPerMonth, int64(line.HeadcountQuarters), int64(line.PeriodQuarters)}, 16)
			if err == nil {
				monthly, err = addLineToMonthlyBuckets(monthly, item.AmountPerMonth, line, total)
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("[%d] line total: %w", i, err)
		}
		lines = append(lines, domain.SquadLineCalculation{SquadLineInput: line, TermType: term, AmountPerUnit: amountPerUnit, LineTotal: total})
	}
	return lines, monthly, nil
}

func (c *DedicatedSquadCalculator) calculatePreparation(ctx context.Context, companyID, rateCardID string, input domain.DedicatedSquadInput, rules DedicatedSquadRules) (domain.PreparationCalculation, error) {
	result := domain.PreparationCalculation{Kind: string(rules.PreparationKind)}
	if rules.PreparationKind == PreparationNone {
		return result, nil
	}
	for i, line := range input.DevelopmentSquad {
		term := termForDedicatedLine(input.Model, line)
		item, err := c.rates.ResolveItem(ctx, companyID, rateCardID, line.Role, line.Seniority, term)
		if err != nil {
			return result, fmt.Errorf("preparation[%d] resolve rate: %w", i, err)
		}
		fee := item.KickoffFee
		if rules.PreparationKind == PreparationHiring {
			fee, err = checkedAdd(item.RecruitmentFee, item.OnboardingFee)
			if err != nil {
				return result, fmt.Errorf("preparation[%d] fee: %w", i, err)
			}
		}
		lineFee, err := checkedProductDivide([]int64{fee, int64(line.HeadcountQuarters)}, 4)
		if err != nil {
			return result, fmt.Errorf("preparation[%d]: %w", i, err)
		}
		result.Gross, err = checkedAdd(result.Gross, lineFee)
		if err != nil {
			return result, fmt.Errorf("preparation gross: %w", err)
		}
	}
	var err error
	result.Discount, err = checkedProductDivide([]int64{result.Gross, int64(rules.PreparationDiscountBPS)}, 10_000)
	if err != nil {
		return result, fmt.Errorf("preparation discount: %w", err)
	}
	result.Net, err = checkedAdd(result.Gross, -result.Discount)
	return result, err
}

func termForDedicatedLine(model domain.SquadModel, line domain.SquadLineInput) string {
	if model == domain.ModelSprint {
		return "short_term"
	}
	if model == domain.ModelStaffAugmentation {
		return "long_term"
	}
	if line.PeriodQuarters < 12 {
		return "short_term"
	}
	return "long_term"
}

func buildDedicatedPayments(input domain.DedicatedSquadInput, rules DedicatedSquadRules, calculation domain.DedicatedSquadCalculation, monthly []int64) ([]domain.PaymentRow, error) {
	anchor := calculatorDateOnly(*input.PaymentAnchorDate)
	row := func(workPeriod, terms string, invoice time.Time, nominal int64) domain.PaymentRow {
		return domain.PaymentRow{WorkPeriod: workPeriod, Terms: terms, InvoiceDate: invoice, DueDate: invoice.AddDate(0, 0, 14), Nominal: nominal}
	}
	switch rules.PaymentKind {
	case PaymentSingleSettlement:
		invoice := anchor
		if input.Timeline.SprintEndDate != nil {
			invoice = calculatorDateOnly(*input.Timeline.SprintEndDate)
		}
		return []domain.PaymentRow{row("Penyelesaian pekerjaan", "Pelunasan", invoice, calculation.TotalInvestment)}, nil
	case PaymentEqualInstallments:
		return equalPaymentRows(calculation.TotalInvestment, input.PaymentCount, anchor, false)
	case PaymentNetPreparationDP:
		payments := []domain.PaymentRow{row("Hiring & Kick off", "Down Payment", anchor, calculation.Preparation.Net)}
		remaining, err := checkedAdd(calculation.TotalInvestment, -calculation.Preparation.Net)
		if err != nil {
			return nil, err
		}
		installments, err := equalPaymentRows(remaining, input.PaymentCount, anchor, true)
		return append(payments, installments...), err
	case PaymentMonthly:
		payments := []domain.PaymentRow{row("Engineering Kick-off", "Down Payment", anchor, calculation.Preparation.Net)}
		if len(monthly) != int(input.PaymentCount) {
			return nil, fmt.Errorf("payments: monthly buckets %d do not match payment_count %d", len(monthly), input.PaymentCount)
		}
		for i, nominal := range monthly {
			invoice := endOfMonth(anchor.AddDate(0, i+1, 0))
			payments = append(payments, row(monthRange(invoice), fmt.Sprintf("Termin %d", i+1), invoice, nominal))
		}
		return payments, nil
	default:
		return nil, fmt.Errorf("payments: unsupported kind %q", rules.PaymentKind)
	}
}

func equalPaymentRows(total int64, count int32, anchor time.Time, startNextMonth bool) ([]domain.PaymentRow, error) {
	if count <= 0 {
		return nil, fmt.Errorf("payments: count must be positive")
	}
	base := total / int64(count)
	rows := make([]domain.PaymentRow, count)
	for i := range rows {
		nominal := base
		if i == len(rows)-1 {
			nominal += total - base*int64(count)
		}
		monthOffset := i
		if startNextMonth {
			monthOffset++
		}
		invoice := anchor.AddDate(0, monthOffset, 0)
		rows[i] = domain.PaymentRow{WorkPeriod: fmt.Sprintf("Bulan %d", i+1), Terms: fmt.Sprintf("Termin %d", i+1), InvoiceDate: invoice, DueDate: invoice.AddDate(0, 0, 14), Nominal: nominal}
	}
	if !startNextMonth && len(rows) > 0 {
		rows[0].WorkPeriod, rows[0].Terms = "Hiring & Kick off", "Down Payment"
	}
	return rows, nil
}

func addLineToMonthlyBuckets(buckets []int64, rate int64, line domain.SquadLineInput, lineTotal int64) ([]int64, error) {
	months := int((line.PeriodQuarters + 3) / 4)
	for len(buckets) < months {
		buckets = append(buckets, 0)
	}
	remaining := line.PeriodQuarters
	var allocated int64
	for i := 0; i < months; i++ {
		quarters := int32(4)
		if remaining < quarters {
			quarters = remaining
		}
		amount, err := checkedProductDivide([]int64{rate, int64(line.HeadcountQuarters), int64(quarters)}, 16)
		if err != nil {
			return nil, err
		}
		if i == months-1 {
			amount, err = checkedAdd(lineTotal, -allocated)
			if err != nil {
				return nil, err
			}
		}
		buckets[i], err = checkedAdd(buckets[i], amount)
		if err != nil {
			return nil, err
		}
		allocated, err = checkedAdd(allocated, amount)
		if err != nil {
			return nil, err
		}
		remaining -= quarters
	}
	return buckets, nil
}

func checkedProductDivide(values []int64, divisor int64) (int64, error) {
	result := int64(1)
	var err error
	for _, value := range values {
		result, err = checkedMul(result, value)
		if err != nil {
			return 0, err
		}
	}
	return result / divisor, nil
}

func checkedSum(values ...int64) (int64, error) {
	var result int64
	var err error
	for _, value := range values {
		result, err = checkedAdd(result, value)
		if err != nil {
			return 0, err
		}
	}
	return result, nil
}

func sumLineTotals(lines []domain.SquadLineCalculation) (int64, error) {
	var result int64
	var err error
	for _, line := range lines {
		result, err = checkedAdd(result, line.LineTotal)
		if err != nil {
			return 0, err
		}
	}
	return result, nil
}

func endOfMonth(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month()+1, 0, 0, 0, 0, 0, value.Location())
}

func monthRange(end time.Time) string {
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())
	return fmt.Sprintf("%d - %d %s %d", start.Day(), end.Day(), end.Month(), end.Year())
}

func calculatorDateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}
