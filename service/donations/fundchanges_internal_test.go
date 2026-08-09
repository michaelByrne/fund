package donations

import (
	"strings"
	"testing"
	"time"
)

func day(value string) *time.Time {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}

	return &parsed
}

func openFund() Fund {
	return Fund{
		Name:            "rent",
		Description:     "help with rent",
		GoalCents:       50000,
		PayoutFrequency: PayoutFrequencyMonthly,
		Expires:         day("2026-09-01"),
	}
}

// The details form posts every field on every save, so the common case is a save
// that changed nothing. Describing that as an update would put a line in the
// feed saying the fund was edited when it was not, and a reader who sees a few
// of those stops believing the ones that mean something.
func TestAnUnchangedSaveDescribesNothing(t *testing.T) {
	if got := describeFundChanges(openFund(), openFund()); got != "" {
		t.Errorf("describeFundChanges = %q, want empty", got)
	}
}

// The old value is the part that cannot be recovered afterwards: the new one is
// in the fund row, and the previous one is overwritten by the write this
// describes.
func TestAChangeNamesWhatItWasAndWhatItBecame(t *testing.T) {
	before := openFund()
	after := before
	after.GoalCents = 75000

	got := describeFundChanges(before, after)

	for _, want := range []string{"goal", "$500.00", "$750.00"} {
		if !strings.Contains(got, want) {
			t.Errorf("describeFundChanges = %q, want it to contain %q", got, want)
		}
	}
}

// The end date decides when the fund stops taking money and closes itself, which
// makes it the edit most worth recording -- and the one whose absence from the
// log would be least noticed.
func TestMovingTheEndDateIsDescribed(t *testing.T) {
	before := openFund()
	after := before
	after.Expires = day("2026-12-25")

	got := describeFundChanges(before, after)

	if !strings.Contains(got, "2026-09-01") || !strings.Contains(got, "2026-12-25") {
		t.Errorf("describeFundChanges = %q, want both dates", got)
	}
}

// The stored expiry is a timestamp with a time of day on it -- CreateFund takes
// whatever it is given. The form is an <input type="date">, which renders
// expires.UTC() as a day and submits a day, so resubmitting it unchanged parses
// to midnight and loses the hours.
//
// Comparing those two as instants reports a change on every single save of a
// date nobody touched, which is exactly the empty event the caller's guard
// exists to prevent. Comparing them as UTC days is what matches the round-trip
// the form actually performs.
func TestResubmittingAnUntouchedDateIsNotAChange(t *testing.T) {
	stored := time.Date(2026, time.September, 1, 14, 23, 11, 0, time.UTC)

	before := openFund()
	before.Expires = &stored

	// What comes back from the form: the same day, midnight, because that is all
	// a date input can carry.
	after := before
	after.Expires = day(stored.UTC().Format("2006-01-02"))

	if got := describeFundChanges(before, after); got != "" {
		t.Errorf("describeFundChanges = %q, want empty -- the date did not move", got)
	}
}

// The other direction: a day that really did move must still be reported, so the
// comparison above cannot be loosened into one that never sees a change.
func TestAnAdjacentDayIsAChange(t *testing.T) {
	stored := time.Date(2026, time.September, 1, 14, 23, 11, 0, time.UTC)

	before := openFund()
	before.Expires = &stored

	after := before
	after.Expires = day("2026-09-02")

	if got := describeFundChanges(before, after); got == "" {
		t.Error("moving the end date by a day is a change")
	}
}

// The recorded line has to name the same day the comparison used and the same
// day the admin saw in the field they edited. sameDay compares UTC days and the
// form's date input renders expires.UTC(), so formatting here in the value's own
// location would let all three disagree.
//
// Reachable with an ordinary date: 18:00 in Los Angeles is the next day in UTC.
func TestTheRecordedDateIsTheOneTheFormShowed(t *testing.T) {
	pacific := time.FixedZone("America/Los_Angeles", -7*3600)

	// 2026-09-01 18:00 -0700 is 2026-09-02 01:00 UTC. Formatted locally this says
	// September 1st; every other part of the round-trip says the 2nd.
	stored := time.Date(2026, time.September, 1, 18, 0, 0, 0, pacific)

	before := openFund()
	before.Expires = &stored

	after := before
	after.Expires = day("2026-09-10")

	got := describeFundChanges(before, after)

	if !strings.Contains(got, "2026-09-02") {
		t.Errorf("describeFundChanges = %q, want the utc day the form and the comparison both use", got)
	}
	if strings.Contains(got, "2026-09-01") {
		t.Errorf("describeFundChanges = %q, want no local-zone day, which nothing else agrees with", got)
	}
}

// Cent-exact by construction rather than by the rounding happening to land
// right. A negative is reachable -- the goal arrives as a parsed float from a
// form and nothing rejects a minus sign -- and must not render as "$-5.-50".
func TestGoalAmountsAreRenderedExactly(t *testing.T) {
	for _, tc := range []struct {
		cents int32
		want  string
	}{
		{0, "none"},
		{5, "$0.05"},
		{50, "$0.50"},
		{100, "$1.00"},
		{50000, "$500.00"},
		{123456789, "$1234567.89"},
		{-550, "-$5.50"},
	} {
		if got := goalDescription(tc.cents); got != tc.want {
			t.Errorf("goalDescription(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func TestSettingAndClearingOptionalFieldsReadsAsSuch(t *testing.T) {
	before := openFund()

	cleared := before
	cleared.GoalCents = 0
	cleared.Expires = nil

	got := describeFundChanges(before, cleared)

	// "goal $500.00 to $0.00" would say a fund is trying to raise nothing rather
	// than that it stopped having a target.
	if !strings.Contains(got, "goal $500.00 to none") {
		t.Errorf("describeFundChanges = %q, want the cleared goal read as none", got)
	}
	if !strings.Contains(got, "end date 2026-09-01 to none") {
		t.Errorf("describeFundChanges = %q, want the cleared date read as none", got)
	}

	// And back the other way, since a nil on either side is its own comparison.
	if back := describeFundChanges(cleared, before); !strings.Contains(back, "none to") {
		t.Errorf("describeFundChanges = %q, want a value being set to read as from none", back)
	}
}

// Several fields can move in one save, and a line naming only the first would be
// worse than none: it reports an edit and understates it.
func TestEveryChangedFieldIsListed(t *testing.T) {
	before := openFund()

	after := before
	after.Description = "help with rent and utilities"
	after.GoalCents = 75000
	after.Expires = day("2026-12-25")

	got := describeFundChanges(before, after)

	for _, want := range []string{"description", "goal", "end date"} {
		if !strings.Contains(got, want) {
			t.Errorf("describeFundChanges = %q, want it to mention %q", got, want)
		}
	}
}

// The details form locks these today. A field that cannot currently be edited is
// not one that never will be, and this function is where the coverage would
// quietly stop.
func TestLockedFieldsAreStillCompared(t *testing.T) {
	before := openFund()

	renamed := before
	renamed.Name = "rent and utilities"

	if got := describeFundChanges(before, renamed); !strings.Contains(got, "name") {
		t.Errorf("describeFundChanges = %q, want a rename to be described", got)
	}

	requeued := before
	requeued.PayoutFrequency = PayoutFrequencyOnce

	if got := describeFundChanges(before, requeued); !strings.Contains(got, "payouts") {
		t.Errorf("describeFundChanges = %q, want a frequency change to be described", got)
	}
}
