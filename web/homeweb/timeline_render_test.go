package homeweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/fundevents"
	"boardfund/service/members"
)

func renderTimeline(t *testing.T, events []fundevents.PublicEvent) string {
	t.Helper()

	var out strings.Builder

	if err := FundTimeline(events).Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

func on(day string) time.Time {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}

	return parsed
}

func cents(amount int32) *int32 {
	return &amount
}

// Newest-first is right for the admin feed, where the question is "what just
// happened". This page is read afterwards by someone asking what became of a
// fund that has ended, and that is a story: planned, approved, sent, closed.
func TestTheTimelineReadsForwardsInTime(t *testing.T) {
	// As the service returns them: newest first.
	page := renderTimeline(t, []fundevents.PublicEvent{
		{Kind: fundevents.KindFundClosed, OccurredAt: on("2026-06-01"), Automatic: true},
		{Kind: fundevents.KindBatchSettled, OccurredAt: on("2026-05-01"), Automatic: true},
		{Kind: fundevents.KindBatchPlanned, OccurredAt: on("2026-04-01"), Automatic: true},
	})

	planned := strings.Index(page, "payouts planned")
	settled := strings.Index(page, "payouts completed")
	closed := strings.Index(page, "fund closed")

	if planned < 0 || settled < 0 || closed < 0 {
		t.Fatalf("an event is missing from the page:\n%s", page)
	}

	if !(planned < settled && settled < closed) {
		t.Error("the timeline should read oldest first, so it tells the story in order")
	}
}

// A closed fund from before any of this was recorded has no timeline. An empty
// box sitting directly beneath a ledger reporting four completed payouts reads
// as a contradiction rather than as a gap.
func TestNoTimelineRatherThanAnEmptyOne(t *testing.T) {
	if page := renderTimeline(t, nil); strings.TrimSpace(page) != "" {
		t.Errorf("an empty timeline should render nothing at all, got:\n%s", page)
	}
}

// Approving a payout is the accountable act on this page. A donor asking who
// decided the money should move is entitled to an answer.
func TestTheApproverIsNamed(t *testing.T) {
	page := renderTimeline(t, []fundevents.PublicEvent{{
		Kind:        fundevents.KindBatchApproved,
		OccurredAt:  on("2026-05-01"),
		ActorName:   "treasurer",
		AmountCents: cents(75000),
		Detail:      "3 payees",
	}})

	for _, want := range []string{"payouts approved", "treasurer", "$750.00", "3 payees"} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q:\n%s", want, page)
		}
	}
}

// Expiry closes a fund with nobody involved. Rendering that as a blank where a
// name goes would read as a name being withheld.
func TestAnAutomaticStepSaysSo(t *testing.T) {
	page := renderTimeline(t, []fundevents.PublicEvent{{
		Kind: fundevents.KindFundClosed, OccurredAt: on("2026-06-01"), Automatic: true,
	}})

	if !strings.Contains(page, "automatic") {
		t.Errorf("a step nobody took should say so:\n%s", page)
	}
	if strings.Contains(page, "unnamed member") {
		t.Error("automatic and unnamed are different claims")
	}
}

// member.bco_name is nullable, so a person can act with no name to show. That is
// not the same as nobody acting.
func TestAnUnnamedActorIsNotShownAsAutomatic(t *testing.T) {
	page := renderTimeline(t, []fundevents.PublicEvent{{
		Kind: fundevents.KindBatchApproved, OccurredAt: on("2026-05-01"),
	}})

	if !strings.Contains(page, "unnamed member") {
		t.Errorf("an actor with no name should still read as a person:\n%s", page)
	}
	if strings.Contains(page, "automatic") {
		t.Error("a person acted, so the page must not credit a sweep")
	}
}

// The page exists to be trusted, so it should say what it is: recorded as it
// happened, never edited, and not a list of who gave or who received.
func TestThePageSaysWhatItIsAndWhatItLeavesOut(t *testing.T) {
	page := renderTimeline(t, []fundevents.PublicEvent{{
		Kind: fundevents.KindFundClosed, OccurredAt: on("2026-06-01"), Automatic: true,
	}})

	if !strings.Contains(page, "never edited") {
		t.Error("the page should say the record is append-only")
	}
	if !strings.Contains(page, "not named") {
		t.Error("the page should say that donors and recipients are not listed")
	}
}

// The whole timeline is built from PublicEvent, which has no field for the
// member an event was about -- so this checks the page it renders into, where a
// future edit could still reach for the notes or the ledger and print a name.
func TestTheArchivePageNamesNobodyFromTheTimeline(t *testing.T) {
	var out strings.Builder

	fund := donations.ClosedFund{Fund: donations.Fund{Name: "rent", Description: "help with rent"}}

	timeline := []fundevents.PublicEvent{
		{Kind: fundevents.KindBatchApproved, OccurredAt: on("2026-05-01"), ActorName: "treasurer"},
		{Kind: fundevents.KindFundClosed, OccurredAt: on("2026-06-01"), Automatic: true},
	}

	err := ClosedFundSummary(fund, donations.FundStats{}, nil, nil, timeline, &members.Member{}, "/archive").
		Render(context.Background(), &out)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	page := out.String()

	if !strings.Contains(page, "what happened") {
		t.Error("the archive page should carry the timeline")
	}
	if !strings.Contains(page, "treasurer") {
		t.Error("the approver should reach the page")
	}
}
