package adminweb

import (
	"context"
	"strings"
	"testing"
	"time"

	"boardfund/service/notices"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func adminNotice(body string, active bool) notices.Notice {
	return notices.Notice{
		ID:      uuid.New(),
		Body:    body,
		Active:  active,
		Created: time.Now(),
	}
}

func renderPanel(t *testing.T, all []notices.Notice, failure string) string {
	t.Helper()

	var out strings.Builder
	require.NoError(t, NoticePanel(all, failure).Render(context.Background(), &out))

	return out.String()
}

// The column the panel exists for: which of these is on the home page right now.
func TestTheTableSaysWhichNoticesAreShowing(t *testing.T) {
	page := renderPanel(t, []notices.Notice{
		adminNotice("this one is up", true),
		adminNotice("this one is not", false),
	}, "")

	if !strings.Contains(page, "on the home page") {
		t.Errorf("an active notice should say so:\n%s", page)
	}
	if !strings.Contains(page, "not shown") {
		t.Errorf("an inactive notice should say so:\n%s", page)
	}
}

// Everything, not just the active ones. A notice that has come down is the most
// likely thing somebody wants next, and a panel that hid them would mean
// retyping last month's message.
func TestNoticesThatCameDownAreStillListed(t *testing.T) {
	page := renderPanel(t, []notices.Notice{adminNotice("taken down last week", false)}, "")

	if !strings.Contains(page, "taken down last week") {
		t.Errorf("an inactive notice should still be in the panel:\n%s", page)
	}
	if !strings.Contains(page, "put back up") {
		t.Errorf("and it should be possible to put it back:\n%s", page)
	}
}

// A control labelled with the state it is in, rather than what it does, is the
// reliable way to make somebody click the wrong one.
func TestTheButtonSaysWhatItWillDo(t *testing.T) {
	up := renderPanel(t, []notices.Notice{adminNotice("currently up", true)}, "")

	if !strings.Contains(up, "take down") {
		t.Errorf("an active notice's control should offer to take it down:\n%s", up)
	}

	// And it asks for the state it wants rather than a flip, so two admins
	// clicking at once both get what they asked for.
	//
	// Escaped, because templ escapes attribute values and hx-vals is JSON inside
	// one. The browser decodes the entities before htmx ever sees the string, so
	// this is what correct looks like rather than a bug being pinned.
	if !strings.Contains(up, `hx-vals="{&#34;active&#34;: &#34;false&#34;}"`) {
		t.Errorf("the control should send the state it wants:\n%s", up)
	}

	down := renderPanel(t, []notices.Notice{adminNotice("currently down", false)}, "")

	if !strings.Contains(down, "put back up") {
		t.Errorf("an inactive notice's control should offer to restore it:\n%s", down)
	}
	if !strings.Contains(down, `hx-vals="{&#34;active&#34;: &#34;true&#34;}"`) {
		t.Errorf("the control should send the state it wants:\n%s", down)
	}
}

// The admin layout routes every error to #admin-error, which would put a message
// about this box above a form that still holds the text.
func TestARefusalIsReportedBesideTheBox(t *testing.T) {
	page := renderPanel(t, nil, "a notice needs something in it")

	if !strings.Contains(page, "a notice needs something in it") {
		t.Errorf("the failure should be shown:\n%s", page)
	}
	if !strings.Contains(page, `hx-target-error="#notices"`) {
		t.Errorf("the form should keep its own errors:\n%s", page)
	}
}

// An empty panel is the ordinary state before anybody has posted, and it should
// say so rather than showing a table with no rows.
func TestAnEmptyPanelSaysSo(t *testing.T) {
	page := renderPanel(t, nil, "")

	if !strings.Contains(page, "nothing has been posted yet") {
		t.Errorf("an empty panel should explain itself:\n%s", page)
	}

	// The form is still there, because posting the first one is the whole job.
	if !strings.Contains(page, "post notice") {
		t.Error("the form should be available with no notices")
	}
}

// The admin table shows the same text members see, and it is escaped the same
// way. This is the second render of admin-authored text, so it is the second
// place that could get it wrong.
func TestTheAdminTableEscapesTheBodyToo(t *testing.T) {
	page := renderPanel(t, []notices.Notice{adminNotice(`<script>alert(1)</script>`, true)}, "")

	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Errorf("the admin table rendered a notice as markup:\n%s", page)
	}
}
