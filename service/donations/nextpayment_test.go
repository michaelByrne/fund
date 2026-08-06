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

// A daily fund rolls forward the same way, just in days. The distinction that
// matters is that it rolls forward at all: every one of these cases used to
// return the anchor unchanged, because the code asked whether the fund was
// monthly when it meant whether the fund repeats.
//
// `now` is deliberately off the hour here. at() pins everything to 12:00, which
// for a daily schedule anchored at 12:00 is always exactly a payout instant --
// so a table built only from at() would test the boundary and nothing else.
func TestNextPaymentAfterForDailyFunds(t *testing.T) {
	daily := func(anchor time.Time) Fund {
		return Fund{PayoutFrequency: PayoutFrequencyDaily, NextPayment: anchor}
	}

	afternoon := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 17, 0, 0, 0, time.UTC)
	}

	cases := []struct {
		name string
		fund Fund
		now  time.Time
		want time.Time
	}{
		{
			name: "anchor still ahead",
			fund: daily(at(2026, time.August, 7)),
			now:  at(2026, time.August, 6),
			want: at(2026, time.August, 7),
		},
		{
			// Due exactly now counts as due, matching the monthly behaviour: the
			// planner runs on the instant and must not skip the period it is in.
			name: "due exactly now",
			fund: daily(at(2026, time.August, 6)),
			now:  at(2026, time.August, 9),
			want: at(2026, time.August, 9),
		},
		{
			name: "part way through a period",
			fund: daily(at(2026, time.August, 6)),
			now:  afternoon(2026, time.August, 6),
			want: at(2026, time.August, 7),
		},
		{
			name: "a fortnight of missed runs",
			fund: daily(at(2026, time.August, 6)),
			now:  afternoon(2026, time.August, 20),
			want: at(2026, time.August, 21),
		},
		// Days need none of the clamping months do, but they still have to cross
		// month and year ends without landing on the 32nd of anything.
		{
			name: "across a month end",
			fund: daily(at(2026, time.August, 30)),
			now:  afternoon(2026, time.August, 31),
			want: at(2026, time.September, 1),
		},
		{
			name: "across a year end",
			fund: daily(at(2026, time.December, 31)),
			now:  afternoon(2027, time.January, 4),
			want: at(2027, time.January, 5),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.fund.NextPaymentAfter(c.now)
			if !got.Equal(c.want) {
				t.Errorf("NextPaymentAfter(%s) = %s, want %s",
					c.now.Format(time.RFC3339), got.Format(time.RFC3339), c.want.Format(time.RFC3339))
			}
		})
	}
}

// The time of day is the anchor's, not the caller's. A fund anchored at 09:00
// pays at 09:00 tomorrow, which is what makes the daily cron's window stable.
func TestDailyNextPaymentKeepsTheAnchorsTimeOfDay(t *testing.T) {
	anchor := time.Date(2026, time.August, 6, 9, 0, 0, 0, time.UTC)
	fund := Fund{PayoutFrequency: PayoutFrequencyDaily, NextPayment: anchor}

	got := fund.NextPaymentAfter(time.Date(2026, time.August, 9, 17, 42, 0, 0, time.UTC))
	want := time.Date(2026, time.August, 10, 9, 0, 0, 0, time.UTC)

	if !got.Equal(want) {
		t.Errorf("got %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestRecurringDistinguishesRepeatingFundsFromOneOffs(t *testing.T) {
	// Every call site that branches on this reads "does this fund pay more than
	// once", so a new frequency being absent here is the bug, not a style point.
	for freq, want := range map[PayoutFrequency]bool{
		PayoutFrequencyMonthly: true,
		PayoutFrequencyDaily:   true,
		PayoutFrequencyOnce:    false,
	} {
		if got := freq.Recurring(); got != want {
			t.Errorf("%s.Recurring() = %v, want %v", freq, got, want)
		}
	}
}
