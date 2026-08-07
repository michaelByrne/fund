package paypal

import (
	"testing"
	"time"
)

// The reporting API returns UTC as a trailing Z, and this parsed with a layout
// demanding a numeric offset. Every transaction it returned failed to parse and
// the whole lookup errored -- which is why the audit had a provider column and
// nothing ever in it.
func TestParseProviderTime(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"utc as Z", "2026-08-06T22:15:20Z"},
		{"offset with a colon", "2026-08-06T22:15:20-07:00"},
		// What the old layout expected, and still arrives from some endpoints.
		{"offset without a colon", "2026-08-06T22:15:20-0700"},
		{"no zone at all", "2026-08-06T22:15:20"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parsed, err := parseProviderTime(c.value)
			if err != nil {
				t.Fatalf("parseProviderTime(%q): %v", c.value, err)
			}

			if parsed.IsZero() {
				t.Error("parsed to the zero time")
			}

			if got := parsed.UTC().Year(); got != 2026 {
				t.Errorf("year = %d, want 2026", got)
			}
		})
	}

	if _, err := parseProviderTime("not a timestamp"); err == nil {
		t.Error("something unparseable should be reported, not passed on as the zero time")
	}
}

// The zero time would render as a date in year one and read as real.
func TestParseProviderTimeKeepsTheInstant(t *testing.T) {
	parsed, err := parseProviderTime("2026-08-06T22:15:20Z")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	want := time.Date(2026, time.August, 6, 22, 15, 20, 0, time.UTC)
	if !parsed.Equal(want) {
		t.Errorf("got %s, want %s", parsed.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
