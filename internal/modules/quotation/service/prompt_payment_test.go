package service

import "testing"

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
	if got := SplitPayment(1, PaymentPattern("30:70")); got != nil {
		t.Fatalf("unknown pattern returned %#v", got)
	}
	if got := SplitPayment(-1, PaymentPattern5050); got != nil {
		t.Fatalf("negative total returned %#v", got)
	}
}
