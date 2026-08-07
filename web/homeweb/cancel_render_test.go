package homeweb

import (
	"context"
	"strings"
	"testing"

	"boardfund/service/donations"

	"github.com/google/uuid"
)

func renderRow(t *testing.T, donation donations.MemberDonation, failure string) string {
	t.Helper()

	var out strings.Builder
	if err := MyDonationRow(donation, nil, failure).Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

// htmx discards 4xx and 5xx bodies unless an error target is declared. Without
// one every failure here is silent: the donor presses cancel, the row does not
// change, and the most reasonable reading is that it worked.
func TestTheCancelButtonDeclaresAnErrorTarget(t *testing.T) {
	donation := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundName: "human fund",
		Active: true, Recurring: true, HasSubscription: true,
	})

	html := renderRow(t, donation, "")

	target := "#donation-" + donation.ID.String()

	if !strings.Contains(html, `hx-target-error="`+target+`"`) {
		t.Error("without an error target the donor is told nothing when cancelling fails")
	}

	if !strings.Contains(html, `hx-post="/donation/cancel/`+donation.ID.String()+`"`) {
		t.Error("the button should post to the cancel route")
	}
}

// The failure is rendered inside the row rather than replacing it, so the
// donation and its button survive: replacing the row would take away the control
// the donor needs to try again.
func TestAFailureKeepsTheRowAndItsButton(t *testing.T) {
	donation := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundName: "human fund",
		Active: true, Recurring: true, HasSubscription: true,
	})

	const message = "we could not stop this donation with PayPal, so it is still running. please try again."

	html := renderRow(t, donation, message)

	if !strings.Contains(html, message) {
		t.Error("the donor should be told what happened")
	}

	if !strings.Contains(html, "cancel this donation") {
		t.Error("the button should survive a failure, or there is no way to retry")
	}

	// The id has to be stable or the next swap has nowhere to land.
	if !strings.Contains(html, `id="donation-`+donation.ID.String()+`"`) {
		t.Error("the row should keep its id")
	}
}

// A successful redraw carries no banner, or every donation would look like it had
// just failed.
func TestASuccessfulRedrawShowsNoFailure(t *testing.T) {
	donation := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundName: "human fund", Active: false, Recurring: true,
	})

	html := renderRow(t, donation, "")

	if strings.Contains(html, "please try again") {
		t.Error("a row with no failure should carry no failure text")
	}

	if strings.Contains(html, "cancel this donation") {
		t.Error("an ended donation should not offer cancellation")
	}
}

// Colour says whether a donation is live, not where it sits in the list.
// Alternating odd and even meant two active donations next to each other looked
// like two different kinds of thing.
func TestTileColourFollowsState(t *testing.T) {
	live := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundName: "human fund",
		Active: true, Recurring: true, HasSubscription: true,
	})

	ended := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundName: "human fund",
		Active: false, Recurring: true, InactiveReason: "CANCELLED",
	})

	expired := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundName: "human fund",
		Active: false, Recurring: true, InactiveReason: "EXPIRED",
	})

	liveHTML := renderRow(t, live, "")

	if !strings.Contains(liveHTML, "bg-odd") {
		t.Error("a live donation should take the lighter tile")
	}

	if strings.Contains(liveHTML, "bg-even") {
		t.Error("a live donation should not take the ended tile as well")
	}

	// Cancelled and expired are the same thing to a donor: it has stopped.
	for name, donation := range map[string]donations.MemberDonation{
		"cancelled": ended, "expired": expired,
	} {
		html := renderRow(t, donation, "")

		if !strings.Contains(html, "bg-even") {
			t.Errorf("a %s donation should take the darker tile", name)
		}

		if strings.Contains(html, "bg-odd") {
			t.Errorf("a %s donation should not take the live tile", name)
		}
	}

	// The note editor is drawn inside the tile, so it must not use the two colours
	// that say what state the tile is in. This caught exactly that.
	if strings.Contains(renderRow(t, live, ""), "bg-even") {
		t.Error("something inside a live tile is using the ended colour")
	}

	// Nothing should depend on position any more.
	if strings.Contains(liveHTML, "odd:bg-odd") || strings.Contains(liveHTML, "even:bg-even") {
		t.Error("tiles still alternate by position")
	}
}
