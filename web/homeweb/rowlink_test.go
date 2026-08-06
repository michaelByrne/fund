package homeweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"

	"github.com/google/uuid"
)

func TestClosedFundRowsAreLinks(t *testing.T) {
	id := uuid.New()
	closed := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)

	fund := donations.ClosedFund{
		Fund: donations.Fund{
			ID:      id,
			Name:    "human fund",
			Expires: &closed,
		},
	}

	var out strings.Builder
	if err := ClosedFunds([]donations.ClosedFund{fund}).Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	html := out.String()

	// A real href is what makes middle-click, open-in-new-tab and copy-link work,
	// and what shows the destination on hover. hx-get gave none of that.
	if want := `href="/fund/` + id.String() + `/summary"`; !strings.Contains(html, want) {
		t.Errorf("archive rows should link to the summary page")
	}

	// The desktop row and the mobile card are separate markup, so both need one.
	if got := strings.Count(html, "/summary"); got != 2 {
		t.Errorf("found %d links, want one for the table row and one for the card", got)
	}

	// hx-get on a row swapped the whole summary page into the row itself, since
	// the handler renders a full page and the row was its own swap target.
	if strings.Contains(html, "hx-get") {
		t.Error("rows should navigate, not swap")
	}
}

func TestRowLinkOverlaysTheWholeRow(t *testing.T) {
	var out strings.Builder
	if err := RowLink("/fund/abc/summary", "human fund").Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	html := out.String()

	// An anchor cannot wrap a <tr>, so the row is covered by stretching the
	// anchor's ::after over it. Without all three utilities the clickable area
	// collapses back to the width of the name text.
	for _, class := range []string{"after:absolute", "after:inset-0", "after:content-"} {
		if !strings.Contains(html, class) {
			t.Errorf("missing %s, so the link would not cover the row", class)
		}
	}
}
