package payout

import "testing"

func TestDollars(t *testing.T) {
	cases := map[int32]string{
		0:       "$0.00",
		5:       "$0.05",
		125:     "$1.25",
		100:     "$1.00",
		999999:  "$9999.99",
		-5:      "-$0.05",
		-125:    "-$1.25",
		-100:    "-$1.00",
		-999999: "-$9999.99",
	}

	for cents, want := range cases {
		got := dollars(cents)
		if got != want {
			t.Errorf("dollars(%d) = %q, want %q", cents, got, want)
		}
	}
}
