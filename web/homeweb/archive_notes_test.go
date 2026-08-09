package homeweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/members"

	"github.com/google/uuid"
)

func renderArchive(t *testing.T, notes []donations.FundNote, member *members.Member) string {
	t.Helper()

	closedOn := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	fund := donations.ClosedFund{
		Fund: donations.Fund{
			ID: uuid.New(), Name: "human fund", Description: "money for people",
			Expires: &closedOn,
		},
	}

	var out strings.Builder
	if err := ClosedFundSummary(fund, donations.FundStats{}, notes, nil, nil, member, "/archive").
		Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

// A note is written to a fund's page, and the archive is that page after the fund
// ends. Dropping the notes at closing would delete what donors said at the exact
// moment it becomes the record of what the fund was.
func TestTheArchiveShowsNotes(t *testing.T) {
	html := renderArchive(t, []donations.FundNote{
		{ID: uuid.New(), Body: "this paid my rent", AuthorName: "ada", Created: time.Now()},
	}, &members.Member{})

	if !strings.Contains(html, "this paid my rent") {
		t.Error("a note written while the fund ran should survive it closing")
	}

	if !strings.Contains(html, "ada") {
		t.Error("the author should be named on the archive as they are on the fund page")
	}
}

// Nobody can give to a closed fund, so nobody can earn a note on one. Offering
// the form would take a note the server is bound to refuse.
func TestTheArchiveOffersNoNoteForm(t *testing.T) {
	html := renderArchive(t, []donations.FundNote{
		{ID: uuid.New(), Body: "this paid my rent", Created: time.Now()},
	}, &members.Member{})

	if strings.Contains(html, "fund-note-form") {
		t.Error("a closed fund must not invite a note it cannot accept")
	}
}

// The heading with nothing under it reads as a fund nobody had anything to say
// about, which is not the same as one that ended before the feature existed.
func TestTheArchiveHidesTheSectionWithNoNotes(t *testing.T) {
	if html := renderArchive(t, nil, &members.Member{}); strings.Contains(html, "notes from donors") {
		t.Error("with no notes there is no section to show")
	}
}

// Closing a fund is not a reason a note that should come down gets to stay up.
func TestTheArchiveKeepsAdminRemoval(t *testing.T) {
	note := donations.FundNote{ID: uuid.New(), Body: "abuse", Created: time.Now()}

	if html := renderArchive(t, []donations.FundNote{note}, &members.Member{}); strings.Contains(html, "/admin/note/remove/") {
		t.Error("an ordinary member must not be offered a removal control")
	}

	admin := &members.Member{Roles: []members.MemberRole{members.AdminRole}}
	if html := renderArchive(t, []donations.FundNote{note}, admin); !strings.Contains(html, "/admin/note/remove/"+note.ID.String()) {
		t.Error("an admin should still be able to take a note down after the fund ends")
	}
}
