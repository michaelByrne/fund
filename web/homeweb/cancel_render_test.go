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
	if err := MyDonationRow(donation, failure).Render(context.Background(), &out); err != nil {
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
