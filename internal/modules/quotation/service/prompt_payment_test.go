package service

import (
	"testing"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestSplitPaymentReconcilesPresets(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pattern PaymentPattern
		want    []int64
	}{
		{"50:50", PaymentPattern5050, []int64{50000, 50000}},
		{"40:40:20", PaymentPattern404020, []int64{40000, 40000, 20000}},
		{"monthly remainder", PaymentPattern("monthly:3"), []int64{33333, 33333, 33334}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := SplitPayment(100000, tc.pattern)
			if len(parts) != len(tc.want) {
				t.Fatalf("got %d parts, want %d", len(parts), len(tc.want))
			}
			var sum int64
			for i, part := range parts {
				if part.Amount != tc.want[i] {
					t.Errorf("part %d = %d, want %d", i, part.Amount, tc.want[i])
				}
				sum += part.Amount
			}
			if sum != 100000 {
				t.Fatalf("sum = %d, want 100000", sum)
			}
		})
	}
}

func TestSplitPaymentRejectsUnknownAndOverflowingTotal(t *testing.T) {
	if got := SplitPayment(1, PaymentPattern("30:60")); got != nil {
		t.Fatalf("invalid pattern returned %#v", got)
	}
	if got := SplitPayment(1, PaymentPattern("invalid")); got != nil {
		t.Fatalf("invalid pattern returned %#v", got)
	}
	if got := SplitPayment(-1, PaymentPattern5050); got != nil {
		t.Fatalf("negative total returned %#v", got)
	}
}

func TestParsePaymentPatternFromText_FreeTextPercent(t *testing.T) {
	input := "Termin 1 (DP): 50%; Termin 2: 50%"
	got, err := ParsePaymentPatternFromText(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "50:50" {
		t.Fatalf("got %q, want %q", got, "50:50")
	}
}

func TestParsePaymentPatternFromText_ThreeTerms(t *testing.T) {
	input := "3 termin: 40% (DP), 30% (Progress), 30% (Pelunasan)"
	got, err := ParsePaymentPatternFromText(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "40:30:30" {
		t.Fatalf("got %q, want %q", got, "40:30:30")
	}
}

func TestParsePaymentPatternFromText_IgnoreNonPaymentPercent(t *testing.T) {
	input := "Diskon 10%, termin DP 50%, pelunasan 50%"
	got, err := ParsePaymentPatternFromText(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "50:50" {
		t.Fatalf("got %q, want %q", got, "50:50")
	}
}

func TestSplitPayment_DynamicRatio(t *testing.T) {
	total := int64(100_000_000)
	pattern := PaymentPattern("40:30:30")
	parts := SplitPayment(total, pattern)
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(parts))
	}
	expected := []int64{40_000_000, 30_000_000, 30_000_000}
	var sum int64
	for i, p := range parts {
		if p.Amount != expected[i] {
			t.Errorf("part %d amount = %d, want %d", i, p.Amount, expected[i])
		}
		sum += p.Amount
	}
	if sum != total {
		t.Fatalf("sum = %d, want %d", sum, total)
	}
}

func TestSplitPayment_InvalidSumRejection(t *testing.T) {
	total := int64(100_000_000)
	for _, invalid := range []PaymentPattern{
		"40:40",
		"50:60",
		"Termin 1: 40%, Termin 2: 40%",
		"invalid",
	} {
		parts := SplitPayment(total, invalid)
		if parts != nil {
			t.Fatalf("expected nil for invalid pattern %q, got %#v", invalid, parts)
		}
	}
}

func TestParsePaymentPatternFromText_MonthlyAndColon(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want PaymentPattern
	}{
		{"pola 50:50", "pola 50:50", "50:50"},
		{"exact ratio", "40:30:30", "40:30:30"},
		{"four terms", "25:25:25:25", "25:25:25:25"},
		{"monthly", "monthly", PaymentPatternMonthly},
		{"bulanan", "Pembayaran bulanan", PaymentPatternMonthly},
		{"monthly with count", "monthly:4", "monthly:4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePaymentPatternFromText(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPaymentPatternRows_Draft0085And0086(t *testing.T) {
	// Draft 0085: 3 terms: 40% (DP), 30% (Progress), 30% (Pelunasan)
	draft0085Pattern := PaymentPattern("3 termin: 40% (DP), 30% (Progress), 30% (Pelunasan)")
	rows, err := paymentPatternRows(draft0085Pattern, &domain.Fact[int]{Value: 3})
	if err != nil {
		t.Fatalf("draft 0085 paymentPatternRows error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("draft 0085 expected 3 rows, got %d", len(rows))
	}
	if rows[0].AllocationBPS != 4000 || rows[1].AllocationBPS != 3000 || rows[2].AllocationBPS != 3000 {
		t.Fatalf("draft 0085 rows BPS incorrect: %+v", rows)
	}

	// Draft 0086: pola 50:50
	draft0086Pattern := PaymentPattern("pola 50:50")
	rows0086, err := paymentPatternRows(draft0086Pattern, &domain.Fact[int]{Value: 3})
	if err != nil {
		t.Fatalf("draft 0086 paymentPatternRows error: %v", err)
	}
	if len(rows0086) != 2 {
		t.Fatalf("draft 0086 expected 2 rows, got %d", len(rows0086))
	}
	if rows0086[0].AllocationBPS != 5000 || rows0086[1].AllocationBPS != 5000 {
		t.Fatalf("draft 0086 rows BPS incorrect: %+v", rows0086)
	}
}
