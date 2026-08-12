package homeweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/members"
	"boardfund/service/notices"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func notice(body string) notices.Notice {
	return notices.Notice{
		ID:      uuid.New(),
		Body:    body,
		Active:  true,
		Created: time.Now(),
	}
}

func renderHome(t *testing.T, all []notices.Notice) string {
	t.Helper()

	var out strings.Builder

	page := Funds(
		[]donations.Fund{{ID: uuid.New(), Name: "rent", Active: true}},
		nil, nil, all, &members.Member{}, "/",
	)

	require.NoError(t, page.Render(context.Background(), &out))

	return out.String()
}

// Above the funds, which is the whole point: this is the first thing on the
// first page a member sees.
func TestNoticesSitAboveTheFunds(t *testing.T) {
	page := renderHome(t, []notices.Notice{notice("payouts are delayed this week")})

	board := strings.Index(page, "payouts are delayed this week")
	funds := strings.Index(page, "current active funds")

	if board < 0 || funds < 0 {
		t.Fatalf("something is missing: notice=%d funds=%d", board, funds)
	}

	if board > funds {
		t.Error("the notice should come before the funds table")
	}
}

// An empty box at the top of the front page is permanent furniture that says
// nothing, which is also how a reader learns to stop looking at it.
func TestNothingIsDrawnWhenThereAreNoNotices(t *testing.T) {
	page := renderHome(t, nil)

	if strings.Contains(page, ">notices<") {
		t.Error("an empty notice board should not be drawn at all")
	}

	// The rest of the page is unaffected.
	if !strings.Contains(page, "current active funds") {
		t.Error("the funds should still be there")
	}
}

func TestEveryActiveNoticeIsShown(t *testing.T) {
	page := renderHome(t, []notices.Notice{
		notice("payouts are delayed this week"),
		notice("a new fund opens on monday"),
	})

	for _, want := range []string{"payouts are delayed this week", "a new fund opens on monday"} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q", want)
		}
	}
}

// These were specified as plain text, and the card renders them escaped like
// every other member-facing string. An admin is trusted, but "trusted" is not a
// reason to hand one an HTML injection point into every member's front page --
// and a pasted-in fragment would otherwise render as markup by accident.
func TestANoticeCannotInjectMarkup(t *testing.T) {
	page := renderHome(t, []notices.Notice{
		notice(`<script>alert(1)</script> and <b>bold</b>`),
	})

	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Errorf("a notice injected a script tag:\n%s", page)
	}
	if strings.Contains(page, "<b>bold</b>") {
		t.Error("a notice injected markup")
	}

	// Escaped, not dropped: the admin still sees what they typed.
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Error("the text should survive as text")
	}
}

// A notice is a paragraph somebody typed into a textarea, so the line breaks
// they put in are part of it.
func TestLineBreaksSurvive(t *testing.T) {
	page := renderHome(t, []notices.Notice{notice("first line\nsecond line")})

	if !strings.Contains(page, "whitespace-pre-wrap") {
		t.Error("a notice should keep the breaks it was typed with")
	}
}
