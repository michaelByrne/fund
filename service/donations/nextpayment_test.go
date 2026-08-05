package donations

import (
	"testing"
	"time"
)

func at(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

// Nothing advances next_payment, so a stored date is only ever the schedule's
// anchor. Read literally it goes stale the moment it passes, which is what made
// the fund page tell donors "next payment: paid" indefinitely.
func TestNextPaymentAfter(t *testing.T) {
	monthly := func(anchor time.Time) Fund {
		return Fund{PayoutFrequency: PayoutFrequencyMonthly, NextPayment: anchor}
	}

	cases := []struct {
		name string
		fund Fund
		now  time.Time
		want time.Time
	}{
		{
			name: "anchor still ahead",
			fund: monthly(at(2026, time.September, 5)),
			now:  at(2026, time.August, 5),
			want: at(2026, time.September, 5),
		},
		{
			name: "anchor is today",
			fund: monthly(at(2026, time.September, 5)),
			now:  at(2026, time.September, 5),
			want: at(2026, time.September, 5),
		},
		{
			name: "one period past",
			fund: monthly(at(2026, time.September, 5)),
			now:  at(2026, time.September, 20),
			want: at(2026, time.October, 5),
		},
		{
			// The case that mattered: an anchor from a year ago still answers.
			name: "long stale anchor rolls all the way forward",
			fund: monthly(at(2025, time.March, 5)),
			now:  at(2026, time.August, 20),
			want: at(2026, time.September, 5),
		},
		{
			name: "crosses a year boundary",
			fund: monthly(at(2026, time.December, 5)),
			now:  at(2027, time.January, 2),
			want: at(2027, time.January, 5),
		},
		{
			// time.AddDate would give 3 March here and drift further every month.
			name: "31st clamps in a short month",
			fund: monthly(at(2026, time.January, 31)),
			now:  at(2026, time.February, 15),
			want: at(2026, time.February, 28),
		},
		{
			// Clamping must not become permanent: March has a 31st again.
			name: "clamping does not stick",
			fund: monthly(at(2026, time.January, 31)),
			now:  at(2026, time.March, 15),
			want: at(2026, time.March, 31),
		},
		{
			name: "29th in a non-leap February",
			fund: monthly(at(2026, time.January, 29)),
			now:  at(2026, time.February, 10),
			want: at(2026, time.February, 28),
		},
		{
			name: "a one-off fund has a single date and does not roll",
			fund: Fund{PayoutFrequency: PayoutFrequencyOnce, NextPayment: at(2026, time.March, 5)},
			now:  at(2026, time.August, 1),
			want: at(2026, time.March, 5),
		},
		{
			name: "no anchor at all",
			fund: monthly(time.Time{}),
			now:  at(2026, time.August, 1),
			want: time.Time{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.fund.NextPaymentAfter(c.now)
			if !got.Equal(c.want) {
				t.Errorf("NextPaymentAfter(%s) = %s, want %s",
					c.now.Format("2006-01-02"), got.Format("2006-01-02"), c.want.Format("2006-01-02"))
			}
		})
	}
}

// Whatever the anchor and whenever you ask, a monthly fund's next payout is in
// the future. That is the property the fund page depends on.
func TestNextPaymentIsNeverInThePastForMonthlyFunds(t *testing.T) {
	now := at(2026, time.August, 20)

	for day := 1; day <= 31; day++ {
		for monthsBack := 0; monthsBack <= 24; monthsBack++ {
			anchor := addMonths(at(2026, time.August, day), -monthsBack)

			fund := Fund{PayoutFrequency: PayoutFrequencyMonthly, NextPayment: anchor}

			if next := fund.NextPaymentAfter(now); next.Before(now) {
				t.Fatalf("anchor %s gave a next payment of %s, before now",
					anchor.Format("2006-01-02"), next.Format("2006-01-02"))
			}
		}
	}
}
