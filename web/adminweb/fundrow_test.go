package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"

	"github.com/google/uuid"
)

func TestFundRowMarksClosedFunds(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	cases := []struct {
		name      string
		fund      donations.Fund
		wantLabel string
		wantClose bool
	}{
		{"open", donations.Fund{ID: uuid.New(), Name: "open", Active: true, Expires: &future}, "", true},
		{"expired", donations.Fund{ID: uuid.New(), Name: "exp", Active: true, Expires: &past}, "expired", false},
		{"deactivated", donations.Fund{ID: uuid.New(), Name: "dead", Active: false}, "closed", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			if err := FundRow(c.fund).Render(context.Background(), &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			html := out.String()

			if c.wantLabel != "" && !strings.Contains(html, ">"+c.wantLabel+"<") {
				t.Errorf("row should be labelled %q", c.wantLabel)
			}

			// The close control on an already-closed fund offers an action that does
			// nothing, behind a confirm prompt saying it will.
			hasClose := strings.Contains(html, "/admin/fund/deactivate/")
			if hasClose != c.wantClose {
				t.Errorf("close control present=%v, want %v", hasClose, c.wantClose)
			}
		})
	}
}

// The audit was reached through a magnifying glass that opened a dropdown listing
// a fund's stored reports. There are no stored reports now -- one live view per
// fund -- so the dropdown had nothing to list and was swapping a whole page into
// itself.
func TestFundRowLinksToTheAudit(t *testing.T) {
	fund := donations.Fund{ID: uuid.New(), Name: "human fund", Active: true}

	var out strings.Builder
	if err := FundRow(fund).Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	html := out.String()

	if !strings.Contains(html, "view audit") {
		t.Error("the audit should be reachable by a link that says what it is")
	}

	if !strings.Contains(html, `href="/admin/fund/audit?fund=`+fund.ID.String()+`"`) {
		t.Error("a real href, so it can be opened in a tab and shows where it goes")
	}

	// It has to look like a link. text-links resolves to #333 on this theme, which
	// is the colour of the text beside it, so the only thing marking this as
	// clickable was the pointer cursor -- and the whole row has one of those.
	if !strings.Contains(html, "text-blue-500") {
		t.Error("the audit link should be a colour that says it is a link")
	}

	// The row carries its own hx-get to the fund page. Without stopping the click
	// here, one click does both.
	if !strings.Contains(html, "event.stopPropagation()") {
		t.Error("clicking the link should not also open the fund")
	}

	for _, gone := range []string{"toggleDropdown", "audit-dropdown", "audit-content", "gear-icon"} {
		if strings.Contains(html, gone) {
			t.Errorf("%s belonged to the dropdown and should be gone with it", gone)
		}
	}
}
