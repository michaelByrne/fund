package homeweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/members"

	"github.com/a-h/templ"
	"github.com/google/uuid"
)

func render(t *testing.T, c templ.Component) string {
	t.Helper()

	var out strings.Builder
	if err := c.Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

// The fund page is where the form used to be, and the reason it moved: sitting
// there it invited everybody and the server accepted from almost nobody.
func TestTheFundPageOffersNoNoteForm(t *testing.T) {
	fund := donations.Fund{ID: uuid.New(), Name: "human fund", Description: "d"}
	notes := []donations.FundNote{
		{ID: uuid.New(), Body: "this paid my rent", AuthorName: "ada", Created: time.Now()},
	}

	html := render(t, Fund(fund, donations.FundStats{}, notes, &members.Member{}, "/donate"))

	if strings.Contains(html, "fund-note-form") {
		t.Error("the fund page must not offer a box the server refuses almost everyone who uses it")
	}

	if !strings.Contains(html, "this paid my rent") {
		t.Error("it should still show what donors said -- that is the point of them")
	}
}

// The thank-you screen is where it went. Somebody who has just given is the one
// person entitled to leave a note, and this is the moment they did it.
func TestTheThankYouScreenAsksADonor(t *testing.T) {
	fundID := uuid.New()

	html := render(t, ThankYou(members.Member{}, fundID, true, "/donation/success"))

	if !strings.Contains(html, "fund-note-form") {
		t.Error("a donor should be asked for a note at the moment they earned one")
	}

	if !strings.Contains(html, "/fund/"+fundID.String()+"/note") {
		t.Error("the note should be posted against the fund just given to")
	}

	// Somebody has just parted with money. The screen must not read as another
	// form standing between them and being finished, so there is always a way on
	// that does not involve writing anything.
	if !strings.Contains(html, "take me to the fund") {
		t.Error("the ask should be visibly skippable")
	}

	// Not "no thanks". The form swaps itself when a note is saved and this is its
	// sibling, so a decline would still be sitting there offering to skip
	// something the donor has already done.
	if strings.Contains(html, "no thanks") {
		t.Error("the way out should read as a destination, which stays true after a note is left")
	}
}

// Eligibility is the server's to decide, and the screen is reached with a fund id
// out of the query string. Asking regardless would put a box in front of somebody
// whose every submission is refused.
func TestTheThankYouScreenAsksNobodyElse(t *testing.T) {
	html := render(t, ThankYou(members.Member{}, uuid.New(), false, "/donation/success"))

	if strings.Contains(html, "fund-note-form") {
		t.Error("a note was offered to somebody the server would refuse")
	}

	// Still a thank-you.
	if !strings.Contains(html, "Thank you") {
		t.Error("the thank-you is the page, and it should survive having nothing to ask")
	}
}

// Two donations to one fund is ordinary -- a one-off, then a monthly. Both tiles
// carry an editor for the one note, and two elements with one id would have htmx
// swapping whichever it found first.
func TestEachDonationEditorHasItsOwnID(t *testing.T) {
	fundID := uuid.New()
	notes := map[uuid.UUID]donations.FundNote{
		fundID: {ID: uuid.New(), FundID: fundID, Body: "kept me going", Created: time.Now()},
	}

	first := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundID: fundID, FundName: "human fund", Active: true,
	})
	second := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundID: fundID, FundName: "human fund",
		Active: true, Recurring: true, HasSubscription: true,
	})

	html := render(t, MyDonations([]donations.MemberDonation{first, second}, notes, &members.Member{}, "/donations"))

	for _, donation := range []donations.MemberDonation{first, second} {
		id := `id="fund-note-form-` + donation.ID.String() + `"`
		if strings.Count(html, id) != 1 {
			t.Errorf("expected exactly one editor keyed to donation %s", donation.ID)
		}
	}

	// Both editors hold the one note, because there is one note per fund.
	if strings.Count(html, "kept me going") != 2 {
		t.Error("each editor should show the note that is actually up for that fund")
	}
}

// A donor who skipped the ask, or who never had one, still gets an editor here --
// that is the whole reason this control exists on this page.
func TestMyDonationsOffersANoteEitherWay(t *testing.T) {
	donation := donations.NewMemberDonation(donations.MemberDonationRow{
		ID: uuid.New(), FundID: uuid.New(), FundName: "human fund", Active: true,
	})

	blank := render(t, MyDonationRow(donation, nil, ""))
	if !strings.Contains(blank, "leave a note on this fund") {
		t.Error("a donor with no note should be able to write one from here")
	}

	written := render(t, MyDonationRow(donation, &donations.FundNote{
		ID: uuid.New(), FundID: donation.FundID, Body: "kept me going", Created: time.Now(),
	}, ""))

	if !strings.Contains(written, "kept me going") {
		t.Error("a donor with a note should see it rather than an empty box that overwrites it")
	}

	if !strings.Contains(written, "/fund/"+donation.FundID.String()+"/note/remove") {
		t.Error("and should be able to take their own words down without asking an admin")
	}
}

// The editor names itself so the reply can wear the same id. Without it the
// second edit has nothing to swap: htmx looks for an element that the first
// reply's response replaced with a differently-named one.
func TestTheEditorSendsItsOwnID(t *testing.T) {
	fundID := uuid.New()
	html := render(t, FundNoteForm("fund-note-form-abc", fundID, nil, "", ""))

	if !strings.Contains(html, `name="editor"`) || !strings.Contains(html, `value="fund-note-form-abc"`) {
		t.Error("the editor should tell the server which element it is, so the reply can match")
	}

	if !strings.Contains(html, `hx-target="#fund-note-form-abc"`) {
		t.Error("and should target itself")
	}

	if !strings.Contains(html, `hx-target-error="#fund-note-form-abc"`) {
		t.Error("including on failure -- htmx discards 4xx bodies with nowhere to put them")
	}
}

// The note cards sit on a page of things that all have the same drop shadow, and
// without one they read as a flat list rather than as cards.
//
// shadow-blue-boxy, not blue-boxy-filter. The filter version is a drop-shadow on
// everything inside the element, which is what put a second copy of the caption
// behind the audit page's text.
func TestNoteCardsCarryTheBoxShadow(t *testing.T) {
	notes := []donations.FundNote{
		{ID: uuid.New(), Body: "this paid my rent", AuthorName: "ada", Created: time.Now()},
	}

	html := render(t, FundNotes(notes, false))

	if !strings.Contains(html, "shadow-blue-boxy") {
		t.Error("a note card should carry the same box shadow as everything else on the page")
	}

	if strings.Contains(html, "blue-boxy-filter") {
		t.Error("the filter shadow applies to text inside it too, which doubles the words")
	}
}

// The bouncing thank-you and "donate more money" were bounded by #donation, the
// whole page container, so they wandered over everything below them. With a note
// form on this page that is a moving target crossing the box somebody is trying to
// type in, and taking the click that would have focused it.
func TestTheAnimationStaysInItsOwnBox(t *testing.T) {
	html := render(t, ThankYou(members.Member{}, uuid.New(), true, "/donation/success"))

	if strings.Contains(html, `getElementById('donation')`) {
		t.Error("bounding the animation to the page container is what let it cross the form")
	}

	if !strings.Contains(html, `getElementById('bouncing-elements')`) {
		t.Error("the animation should be bounded by its own arena")
	}

	arena := html[strings.Index(html, `id="bouncing-elements"`):]
	arena = arena[:strings.Index(arena, ">")]

	// The children are absolutely positioned, so without a height this collapses
	// to nothing and the walls it is measured against are zero apart.
	if !strings.Contains(arena, "h-72") {
		t.Error("the arena needs a height of its own or it collapses")
	}

	// A resize mid-flight remeasures the walls. Anything already outside them
	// would otherwise be loose on the page.
	if !strings.Contains(arena, "overflow-hidden") {
		t.Error("nothing should be able to escape the arena")
	}
}

// The ask reads as a card like everything else it sits among, rather than as
// text floating on the page background.
func TestTheNoteAskIsACard(t *testing.T) {
	html := render(t, ThankYouNote(uuid.New()))

	heading := html[strings.Index(html, "want to say why?"):]
	if !strings.Contains(html[:strings.Index(html, "want to say why?")], "shadow-blue-boxy-thin") {
		t.Error("the heading should carry a shadow like the other heading cards")
	}

	if !strings.Contains(heading, "shadow-blue-boxy") {
		t.Error("the editor should be a card in its own right")
	}

	// bg-fore is what the page container behind it already is, so a card in it
	// would be defined by its shadow alone.
	if strings.Contains(heading, "bg-fore") {
		t.Error("the editor should not be the same colour as the page behind it")
	}
}

// Same treatment on the fund page, where the notes are read rather than written.
func TestTheFundNotesHeadingIsACard(t *testing.T) {
	html := render(t, FundNotes([]donations.FundNote{
		{ID: uuid.New(), Body: "this paid my rent", Created: time.Now()},
	}, false))

	heading := html[:strings.Index(html, "notes from donors")]
	if !strings.Contains(heading, "shadow-blue-boxy-thin") {
		t.Error("the notes heading should carry a shadow like the other heading cards")
	}
}
