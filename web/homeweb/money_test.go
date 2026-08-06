package homeweb

import (
	"math"
	"testing"
)

// Payout totals are summed as bigint, so this must not narrow. The first version
// of this function delegated to the int32 formatter, which wraps silently on
// exactly the large totals it exists to display.
func TestCentsToDecimalString64(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{50, "0.50"},
		{99, "0.99"},
		{100, "1.00"},
		{2500, "25.00"},
		{999, "9.99"},
		{100000, "1,000.00"},
		{123456789, "1,234,567.89"},
		{-2500, "-25.00"},
		{-5, "-0.05"},
		// Just past int32: $21,474,836.48. The narrowing version returned
		// "-21,474,836.48" here -- a negative total for a fund that took money in.
		{int64(math.MaxInt32) + 1, "21,474,836.48"},
		{int64(math.MaxInt32) * 100, "2,147,483,647.00"},
	}

	for _, c := range cases {
		if got := centsToDecimalString64(c.cents); got != c.want {
			t.Errorf("centsToDecimalString64(%d) = %q, want %q", c.cents, got, c.want)
		}
	}
}

// The two formatters have to agree, or the same figure reads differently
// depending on which side of the ledger it is on.
func TestFormattersAgreeOnSharedRange(t *testing.T) {
	for _, cents := range []int32{0, 1, 99, 100, 2500, 100000, 123456789, math.MaxInt32} {
		wide := centsToDecimalString64(int64(cents))
		narrow := centsToDecimalString(cents)

		if wide != narrow {
			t.Errorf("cents=%d: int64 form %q, int32 form %q", cents, wide, narrow)
		}
	}
}
