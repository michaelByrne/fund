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
