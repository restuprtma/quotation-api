package service

import (
	"fmt"
	"strconv"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// PaymentPattern is the small, non-monetary vocabulary accepted from a
// meeting note. Nominals are always derived from the calculated total.
type PaymentPattern string

const (
	PaymentPattern5050    PaymentPattern = "50:50"
	PaymentPattern404020  PaymentPattern = "40:40:20"
	PaymentPatternMonthly PaymentPattern = "monthly"
)

type PaymentPart struct {
	AllocationBPS int64
	Amount        int64
}

// SplitPayment reconciles every supported preset to total. The final part
// receives the integer remainder, including for negative totals (which are
// rejected so callers cannot accidentally create a credit schedule).
func SplitPayment(total int64, pattern PaymentPattern) []PaymentPart {
	if total < 0 {
		return nil
	}
	values := []int64{}
	monthlyCount := int64(0)
	switch strings.ToLower(strings.TrimSpace(string(pattern))) {
	case "50:50":
		values = []int64{5000, 5000}
	case "40:40:20":
		values = []int64{4000, 4000, 2000}
	case "monthly", "equal_monthly", "equal monthly":
		values = []int64{3333, 3333, 3334}
	default:
		const prefix = "monthly:"
		value := strings.ToLower(strings.TrimSpace(string(pattern)))
		if strings.HasPrefix(value, prefix) {
			count, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(value, prefix)), 10, 32)
			if err != nil || count <= 0 || count > 60 {
				return nil
			}
			monthlyCount = count
			values = make([]int64, count)
			base := int64(10000) / count
			for i := range values {
				values[i] = base
			}
			values[len(values)-1] += 10000 - base*count
		} else {
			return nil
		}
	}
	parts := make([]PaymentPart, len(values))
	var allocated int64
	for i, bps := range values {
		amount := splitAmount(total, bps)
		if monthlyCount > 0 {
			amount = total / monthlyCount
		}
		if i == len(values)-1 {
			amount = total - allocated
		}
		parts[i] = PaymentPart{AllocationBPS: bps, Amount: amount}
		allocated += amount
	}
	if allocated != total {
		return nil
	}
	return parts
}

// splitAmount avoids multiplying a full int64 Rupiah total by basis points.
func splitAmount(total, bps int64) int64 {
	return (total/10000)*bps + ((total%10000)*bps)/10000
}

func paymentPatternRows(pattern PaymentPattern, durationMonths *domain.Fact[int]) ([]domain.PaymentRowInput, error) {
	normalized := strings.ToLower(strings.TrimSpace(string(pattern)))
	if normalized == "monthly" || normalized == "equal_monthly" || normalized == "equal monthly" {
		if durationMonths == nil || durationMonths.Value <= 0 {
			return nil, fmt.Errorf("payment_pattern: monthly payment requires a positive duration")
		}
		normalized = fmt.Sprintf("monthly:%d", durationMonths.Value)
	}
	parts := SplitPayment(10000, PaymentPattern(normalized))
	if len(parts) == 0 {
		return nil, fmt.Errorf("payment_pattern: unsupported value %q", pattern)
	}
	rows := make([]domain.PaymentRowInput, len(parts))
	for i, part := range parts {
		rows[i] = domain.PaymentRowInput{WorkPeriod: fmt.Sprintf("Termin %d", i+1), Terms: fmt.Sprintf("Termin %d", i+1), AllocationKind: domain.PaymentAllocationInvestmentBPS, AllocationBPS: int32(part.AllocationBPS), InvoiceMonthOffset: int32(i)}
	}
	return rows, nil
}
