package homeweb

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"boardfund/service/donations"
	"boardfund/service/enrollments"
	"boardfund/service/members"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func fundPage(t *testing.T, visible bool, recipients []enrollments.Recipient, notes []donations.FundNote) string {
	t.Helper()

	var out strings.Builder

	fund := donations.Fund{
		ID: uuid.New(), Name: "rent", Description: "help with rent",
		Active: true, EnrolleesVisible: visible,
	}

	require.NoError(t, Fund(fund, donations.FundStats{}, notes, recipients, nil,
		&members.Member{}, "/donate/x").Render(context.Background(), &out))

	return out.String()
}

func recipients(names ...string) []enrollments.Recipient {
	out := make([]enrollments.Recipient, 0, len(names))
	for _, name := range names {
		out = append(out, enrollments.Recipient{Name: name})
	}

	return out
}

func TestRecipientsAreListedWhenTheFundSaysSo(t *testing.T) {
	page := fundPage(t, true, recipients("ada", "grace"), nil)

	if !strings.Contains(page, "who this fund pays") {
		t.Errorf("the list should be headed:\n%s", page)
	}

	for _, name := range []string{"ada", "grace"} {
		if !strings.Contains(page, name) {
			t.Errorf("page is missing %q", name)
		}
	}
}

// The setting is off by default and off for every fund that existed before it.
// People enrolled without anyone telling them their name would appear on a page
// donors read, so the default has to be the quiet one.
func TestNobodyIsNamedWhenTheFundDoesNotSaySo(t *testing.T) {
	page := fundPage(t, false, recipients("ada", "grace"), nil)

	if strings.Contains(page, "who this fund pays") {
		t.Error("a fund with the setting off should not list anybody")
	}

	for _, name := range []string{"ada", "grace"} {
		if strings.Contains(page, name) {
			t.Errorf("%q was named on a fund that does not name its recipients:\n%s", name, page)
		}
	}
}

// The handler skips the query for a fund with the setting off, so this is
// belt-and-braces -- and it is the half that survives a future caller loading
// them anyway.
func TestTheTemplateRefusesEvenWhenHandedNames(t *testing.T) {
	var out strings.Builder

	fund := donations.Fund{ID: uuid.New(), Name: "rent", EnrolleesVisible: false}

	require.NoError(t, RecipientList(fund, recipients("ada")).Render(context.Background(), &out))

	if strings.Contains(out.String(), "ada") {
		t.Errorf("the list rendered a name for a fund with the setting off:\n%s", out.String())
	}
}

// A fund with the setting on and nobody enrolled yet would otherwise show a
// heading over nothing.
func TestAnEmptyListIsNotDrawn(t *testing.T) {
	page := fundPage(t, true, nil, nil)

	if strings.Contains(page, "who this fund pays") {
		t.Error("an empty recipient list should not be drawn")
	}
}

// Two long lists stacked down the page put the second below the fold. Side by
// side when both are there, and full width when only the notes are.
func TestRecipientsAndNotesSitSideBySide(t *testing.T) {
	notes := []donations.FundNote{{ID: uuid.New(), Body: "this paid my rent", AuthorName: "ada"}}

	together := fundPage(t, true, recipients("grace"), notes)

	if !strings.Contains(together, "lg:grid-cols-2") {
		t.Errorf("recipients and notes should share a two-column row:\n%s", together)
	}

	// Recipients first, so the page reads "who this helps" then "what they said".
	who := strings.Index(together, "who this fund pays")
	said := strings.Index(together, "notes from donors")

	if who < 0 || said < 0 || who > said {
		t.Error("recipients should come before the notes")
	}

	// Nothing to put beside the notes means no half-width column with a gap.
	alone := fundPage(t, false, recipients("grace"), notes)

	if strings.Contains(alone, "lg:grid-cols-2") {
		t.Error("notes on their own should not be squeezed into a column")
	}
}

// The projection is the guarantee, not the template. An Enrollment carries the
// address a person's payouts go to, and this type has no field that could carry
// it onto a page.
func TestARecipientCannotCarryAnythingButAName(t *testing.T) {
	shape := reflect.TypeOf(enrollments.Recipient{})

	if shape.NumField() != 1 || shape.Field(0).Name != "Name" {
		t.Fatalf("Recipient has %d fields; it exists to have exactly one", shape.NumField())
	}
}

// A member with no bco_name has nothing to show, and a blank row reads as a
// rendering fault rather than as somebody unnamed.
func TestAnUnnamedEnrolleeIsLeftOut(t *testing.T) {
	projected := enrollments.Recipients([]enrollments.Enrollment{
		{MemberBCOName: "ada", PaypalEmail: "ada@paypal.test"},
		{MemberBCOName: "", PaypalEmail: "nameless@paypal.test"},
	})

	if len(projected) != 1 || projected[0].Name != "ada" {
		t.Errorf("projected = %v, want just ada", projected)
	}
}
