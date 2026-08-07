package adminweb

import (
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// One list headed "current funds" held both open and closed ones, which made the
// heading wrong about half its contents.
func TestClosedFundsAreListedApartFromCurrentOnes(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)

	open := donations.Fund{ID: uuid.New(), Name: "open fund", Active: true}
	deactivated := donations.Fund{ID: uuid.New(), Name: "deactivated fund", Active: false}
	expired := donations.Fund{ID: uuid.New(), Name: "expired fund", Active: true, Expires: &past}

	html := renderAdmin(t, FundsList([]donations.Fund{open, deactivated, expired}))

	require.Contains(t, html, "current funds")
	require.Contains(t, html, "closed funds")

	current := html[strings.Index(html, "current funds"):strings.Index(html, "closed funds")]
	closed := html[strings.Index(html, "closed funds"):]

	require.Contains(t, current, "open fund")
	require.NotContains(t, current, "deactivated fund")

	// Expired counts as closed even though it was never deactivated -- it is past
	// its end date, and it is not taking money.
	require.NotContains(t, current, "expired fund")

	require.Contains(t, closed, "deactivated fund")
	require.Contains(t, closed, "expired fund")
}

// They stay on this tab. A fund vanishing the day it expired took its payouts and
// its payees out of reach on the very day a treasurer would go looking.
func TestClosedFundsAreStillOnThePage(t *testing.T) {
	closed := donations.Fund{ID: uuid.New(), Name: "deactivated fund", Active: false}

	html := renderAdmin(t, FundsList([]donations.Fund{closed}))

	require.Contains(t, html, "deactivated fund")
	require.Contains(t, html, "/admin/fund/audit?fund="+closed.ID.String())
}

// An empty list under a heading reads as a fault on a new instance.
func TestNoClosedHeadingWithoutClosedFunds(t *testing.T) {
	open := donations.Fund{ID: uuid.New(), Name: "open fund", Active: true}

	html := renderAdmin(t, FundsList([]donations.Fund{open}))

	require.Contains(t, html, "current funds")
	require.NotContains(t, html, "closed funds")
}

// A new fund is prepended to the open list by the create form's target, so that
// id has to be on the list a new fund belongs in.
func TestNewFundsLandInTheCurrentList(t *testing.T) {
	closed := donations.Fund{ID: uuid.New(), Name: "deactivated fund", Active: false}
	open := donations.Fund{ID: uuid.New(), Name: "open fund", Active: true}

	html := renderAdmin(t, FundsList([]donations.Fund{closed, open}))

	target := strings.Index(html, `id="admin-funds"`)
	closedList := strings.Index(html, `id="admin-closed-funds"`)

	require.Positive(t, target)
	require.Less(t, target, closedList, "new funds must land above the closed list, not in it")
}

// Typing a fund's details, submitting, and finding them still in the boxes is how
// the same fund gets created twice.
func TestTheCreateFormEmptiesItselfOnSuccess(t *testing.T) {
	html := renderAdmin(t, AddFund())

	require.Contains(t, html, "this.reset()")

	// Only on success. A refusal comes back as this form carrying a message, and
	// emptying it would take the values from the person about to fix them.
	require.Contains(t, html, "event.detail.successful")
}
