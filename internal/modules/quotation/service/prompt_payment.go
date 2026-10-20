package service

import (
	"fmt"
	"math"
	"regexp"
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

var nonPaymentKeywords = []string{
	"diskon", "discount", "potongan",
	"ppn", "pajak", "tax", "vat",
	"bunga", "interest",
}

var paymentKeywords = []string{
	"termin", "term", "dp", "down payment", "downpayment", "uang muka",
	"progress", "progres", "prestasi",
	"pelunasan", "lunas", "tahap", "tahapan",
	"bayar", "pembayaran", "payment",
	"uat", "bast", "golive", "go-live", "go live", "delivery", "selesai",
	"awal", "akhir",
}

var (
	monthlyRegex = regexp.MustCompile(`(?i)\b(?:monthly|bulanan):\s*(\d+)\b`)
	percentRegex = regexp.MustCompile(`(?i)\b(\d+(?:\.\d+)?)\s*(?:%|(?:\bpersen\b)|(?:\bpercent\b))`)
	ratioRegex   = regexp.MustCompile(`\b(\d{1,3}(?::\d{1,3})+)\b`)
)

// ParsePaymentPatternFromText extracts payment ratios from unstructured text,
// supporting dynamic percentage splits (e.g. "50:50", "40:30:30"), monthly payments,
// while safely ignoring non-payment percentages such as discounts or taxes.
func ParsePaymentPatternFromText(text string) (PaymentPattern, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("payment_pattern: empty text")
	}
	lower := strings.ToLower(trimmed)

	// 1. Direct monthly count (e.g. "monthly:3" or "bulanan:3")
	if m := monthlyRegex.FindStringSubmatch(lower); len(m) > 1 {
		return PaymentPattern(fmt.Sprintf("monthly:%s", m[1])), nil
	}
	if lower == "monthly" || lower == "bulanan" || lower == "equal_monthly" || lower == "equal monthly" {
		return PaymentPatternMonthly, nil
	}

	// 2. Extract percentages and filter out non-payment percentages (e.g. discount, tax)
	matches := percentRegex.FindAllStringSubmatchIndex(text, -1)
	if len(matches) > 0 {
		var paymentPercentages []int64
		for _, loc := range matches {
			numStr := text[loc[2]:loc[3]]
			val, err := strconv.ParseFloat(numStr, 64)
			if err != nil || val <= 0 {
				continue
			}

			if isNonPaymentPercentage(text, loc[0], loc[1]) {
				continue
			}

			paymentPercentages = append(paymentPercentages, int64(math.Round(val)))
		}

		if len(paymentPercentages) > 0 {
			var sum int64
			ratioStrs := make([]string, len(paymentPercentages))
			for i, p := range paymentPercentages {
				sum += p
				ratioStrs[i] = strconv.FormatInt(p, 10)
			}
			if sum == 100 {
				return PaymentPattern(strings.Join(ratioStrs, ":")), nil
			}
			return "", fmt.Errorf("payment_pattern: percentage sum is %d, expected 100", sum)
		}
	}

	// 3. Check for colon-separated ratio (e.g. "50:50", "40:30:30", "pola 50:50")
	ratioMatches := ratioRegex.FindAllString(text, -1)
	for _, m := range ratioMatches {
		if _, err := parseRatioPattern(m); err == nil {
			return PaymentPattern(m), nil
		}
	}

	// 4. Check for monthly keywords if no percentage or ratio matched
	if strings.Contains(lower, "monthly") || strings.Contains(lower, "bulanan") {
		return PaymentPatternMonthly, nil
	}

	return "", fmt.Errorf("payment_pattern: no valid payment pattern found in %q", text)
}

func isNonPaymentPercentage(text string, matchStart, matchEnd int) bool {
	lower := strings.ToLower(text)

	// 1. Check clause bounded by delimiters , ; \n \r
	delimStart := strings.LastIndexAny(lower[:matchStart], ",;\n\r")
	clauseStart := 0
	if delimStart != -1 {
		clauseStart = delimStart + 1
	}

	delimEnd := strings.IndexAny(lower[matchEnd:], ",;\n\r")
	clauseEnd := len(lower)
	if delimEnd != -1 {
		clauseEnd = matchEnd + delimEnd
	}

	clause := strings.TrimSpace(lower[clauseStart:clauseEnd])

	// If there were delimiters defining a sub-clause:
	if delimStart != -1 || delimEnd != -1 {
		hasNonPayment := false
		for _, kw := range nonPaymentKeywords {
			if strings.Contains(clause, kw) {
				hasNonPayment = true
				break
			}
		}
		if !hasNonPayment {
			return false
		}
		// If clause contains non-payment keyword, check if clause also has payment keyword
		hasPayment := false
		for _, pw := range paymentKeywords {
			if strings.Contains(clause, pw) {
				hasPayment = true
				break
			}
		}
		if !hasPayment {
			return true
		}
	}

	// 2. Proximity check (immediate prefix / suffix up to 25 chars)
	preStart := matchStart - 25
	if preStart < clauseStart {
		preStart = clauseStart
	}
	preText := lower[preStart:matchStart]

	postEnd := matchEnd + 25
	if postEnd > clauseEnd {
		postEnd = clauseEnd
	}
	postText := lower[matchEnd:postEnd]

	hasNonPaymentNear := false
	for _, kw := range nonPaymentKeywords {
		if strings.Contains(preText, kw) || strings.Contains(postText, kw) {
			hasNonPaymentNear = true
			break
		}
	}
	if !hasNonPaymentNear {
		return false
	}

	// Compare proximity of payment keyword vs non-payment keyword
	lastNP := -1
	for _, nw := range nonPaymentKeywords {
		if idx := strings.LastIndex(preText, nw); idx > lastNP {
			lastNP = idx
		}
	}
	lastP := -1
	for _, pw := range paymentKeywords {
		if idx := strings.LastIndex(preText, pw); idx > lastP {
			lastP = idx
		}
	}
	if lastP > lastNP {
		return false
	}

	return true
}

func parseRatioPattern(pattern string) ([]int64, error) {
	parts := strings.Split(pattern, ":")
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty ratio")
	}
	values := make([]int64, len(parts))
	var totalPercent int64
	for i, p := range parts {
		val, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || val <= 0 {
			return nil, fmt.Errorf("invalid ratio part %q", p)
		}
		values[i] = val * 100 // convert percent to basis points (bps)
		totalPercent += val
	}
	if totalPercent != 100 {
		return nil, fmt.Errorf("ratio parts sum to %d, expected 100", totalPercent)
	}
	return values, nil
}

// SplitPayment reconciles every supported preset and dynamic ratio to total.
// The final part receives the integer remainder, including for negative totals
// (which are rejected so callers cannot accidentally create a credit schedule).
func SplitPayment(total int64, pattern PaymentPattern) []PaymentPart {
	if total < 0 {
		return nil
	}
	raw := strings.ToLower(strings.TrimSpace(string(pattern)))
	if raw == "" {
		return nil
	}

	values := []int64{}
	monthlyCount := int64(0)

	switch raw {
	case "monthly", "equal_monthly", "equal monthly":
		values = []int64{3333, 3333, 3334}
	default:
		const prefix = "monthly:"
		if strings.HasPrefix(raw, prefix) {
			count, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(raw, prefix)), 10, 32)
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
		} else if bpsValues, err := parseRatioPattern(raw); err == nil {
			values = bpsValues
		} else if parsed, err := ParsePaymentPatternFromText(string(pattern)); err == nil {
			parsedRaw := strings.ToLower(strings.TrimSpace(string(parsed)))
			if parsedRaw != raw {
				return SplitPayment(total, parsed)
			}
			return nil
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
	parsedPattern := pattern
	if parsed, err := ParsePaymentPatternFromText(string(pattern)); err == nil {
		parsedPattern = parsed
	}
	normalized := strings.ToLower(strings.TrimSpace(string(parsedPattern)))
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
		rows[i] = domain.PaymentRowInput{
			WorkPeriod:          fmt.Sprintf("Termin %d", i+1),
			Terms:               fmt.Sprintf("Termin %d", i+1),
			AllocationKind:      domain.PaymentAllocationInvestmentBPS,
			AllocationBPS:       int32(part.AllocationBPS),
			InvoiceMonthOffset:  int32(i),
		}
	}
	return rows, nil
}
