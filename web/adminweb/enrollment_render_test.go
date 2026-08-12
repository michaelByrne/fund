package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/enrollments"
	"boardfund/service/members"

	"github.com/google/uuid"
)

func enrollment(name, email string, firstPayout time.Time) enrollments.Enrollment {
	return enrollments.Enrollment{
		ID:              uuid.New(),
		MemberID:        uuid.New(),
		MemberBCOName:   name,
		PaypalEmail:     email,
		FirstPayoutDate: firstPayout,
		Created:         time.Now().Add(-30 * 24 * time.Hour),
	}
}

// Both exclusions are silent in the payout path -- PlanBatch skips an enrollee
// with no address, the eligibility query skips one dated in the future -- so the
// row has to say which applies.
func TestEnrollmentRowShowsWhoCannotBePaid(t *testing.T) {
	ctx := context.Background()

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(30 * 24 * time.Hour)

	cases := []struct {
		name       string
		enrollment enrollments.Enrollment
		want       []string
		notWant    []string
	}{
		{
			name:       "payable",
			enrollment: enrollment("tester", "tester@paypal.test", past),
			want:       []string{"tester", "tester@paypal.test", "enrolled"},
			notWant:    []string{"no paypal address", "payouts cannot be sent", "from "},
		},
		{
			name:       "no paypal address",
			enrollment: enrollment("nomail", "", past),
			want:       []string{"nomail", "no paypal address", "payouts cannot be sent"},
		},
		{
			name:       "not yet eligible",
			enrollment: enrollment("waiting", "waiting@paypal.test", future),
			want:       []string{"waiting", "waiting@paypal.test", "from " + future.Format("01-02-2006")},
			notWant:    []string{"no paypal address"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			if err := EnrollmentRow(c.enrollment, false).Render(ctx, &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			for _, want := range c.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("row missing %q", want)
				}
			}

			for _, notWant := range c.notWant {
				if strings.Contains(out.String(), notWant) {
					t.Errorf("row should not contain %q", notWant)
				}
			}
		})
	}
}

func TestUnpayableNoticeOnlyWhenSomethingIsWrong(t *testing.T) {
	ctx := context.Background()

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(30 * 24 * time.Hour)

	allPayable := []enrollments.Enrollment{
		enrollment("a", "a@paypal.test", past),
		enrollment("b", "b@paypal.test", past),
	}

	var quiet strings.Builder
	if err := UnpayableNotice(coverageOf(allPayable)).Render(ctx, &quiet); err != nil {
		t.Fatalf("render: %v", err)
	}

	// Saying "2 of 2" every time trains the reader to skip the line.
	if strings.TrimSpace(quiet.String()) != "" {
		t.Errorf("notice shown when nothing is wrong: %q", quiet.String())
	}

	mixed := append(allPayable,
		enrollment("c", "", past),
		enrollment("d", "d@paypal.test", future),
	)

	var loud strings.Builder
	if err := UnpayableNotice(coverageOf(mixed)).Render(ctx, &loud); err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(loud.String(), "cover 2 of 4 enrollees") {
		t.Errorf("expected a 2-of-4 summary, got %q", loud.String())
	}
}

// The notice's condition and its message must come from the same count. They
// were derived separately, each taking its own time.Now(), so across the
// eligibility boundary one could say nothing is wrong while the other named a
// shortfall.
func TestPayoutCoverageIsOneSnapshot(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(30 * 24 * time.Hour)

	coverage := coverageOf([]enrollments.Enrollment{
		enrollment("a", "a@paypal.test", past),
		enrollment("b", "", past),
		enrollment("c", "c@paypal.test", future),
	})

	if coverage.Total != 3 {
		t.Errorf("Total = %d, want 3", coverage.Total)
	}

	if coverage.Unpayable != 2 {
		t.Errorf("Unpayable = %d, want 2", coverage.Unpayable)
	}

	if !coverage.Incomplete() {
		t.Error("Incomplete() should be true when anyone is left out")
	}

	if !strings.Contains(coverage.Summary(), "cover 1 of 3 enrollees") {
		t.Errorf("Summary() = %q", coverage.Summary())
	}

	empty := coverageOf(nil)
	if empty.Incomplete() {
		t.Error("no enrollees means nothing is missing, not a shortfall")
	}
}

func TestPayable(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		e    enrollments.Enrollment
		want bool
	}{
		{"address and date passed", enrollment("a", "a@test", past), true},
		{"no address", enrollment("a", "", past), false},
		{"date in the future", enrollment("a", "a@test", future), false},
		{"neither", enrollment("a", "", future), false},
		// The eligibility query uses <=, so a date of exactly now is included.
		{"date is exactly now", enrollment("a", "a@test", now), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.e.Payable(now); got != c.want {
				t.Errorf("Payable = %v, want %v", got, c.want)
			}
		})
	}
}

// Current enrollments sit beside the form that adds one, and the history
// follows underneath. The grid places these in source order, so the order here
// is the layout -- and swapping two adjacent divs is an easy thing to undo by
// accident while editing the ones around them.
func TestEnrollmentsSitBesideTheFormThatAddsThem(t *testing.T) {
	var out strings.Builder

	page := Enrollments(
		donations.Fund{Name: "rent", Active: true},
		nil, knownPending(nil), nil, nil,
		&members.Member{}, "/admin/fund",
	)

	if err := page.Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	html := out.String()

	added := strings.Index(html, "add enrollment")
	current := strings.Index(html, "current enrollments")
	history := strings.Index(html, ">history<")

	if added < 0 || current < 0 || history < 0 {
		t.Fatalf("a panel is missing: add=%d current=%d history=%d", added, current, history)
	}

	if !(added < current && current < history) {
		t.Error("want add enrollment, then current enrollments, then history")
	}
}
