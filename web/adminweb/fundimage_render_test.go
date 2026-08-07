package adminweb

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"boardfund/service/donations"
	"boardfund/web/common"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func renderAdmin(t *testing.T, c templ.Component) string {
	t.Helper()

	var out strings.Builder
	require.NoError(t, c.Render(context.Background(), &out))

	return out.String()
}

// chrome is the card a section draws around whatever it holds: the wrapper and
// the heading tab, with the contents taken out.
//
// Derived from common.Section rather than written down here, so this compares the
// image control against what the other cards on the page actually are rather than
// against a copy of it that can drift.
func chrome(t *testing.T, title string) string {
	t.Helper()

	empty := renderAdmin(t, common.Section(title))

	body := regexp.MustCompile(`(?s)<div class="p-4 bg-even">.*`)

	return body.ReplaceAllString(empty, "")
}

// The image control was a box of its own invention sitting beside cards that all
// look the same, so it read as something bolted on rather than part of the page.
func TestTheImageControlIsTheSameCardAsTheRest(t *testing.T) {
	html := renderAdmin(t, FundImageControl(uuid.New(), nil, ""))

	require.Contains(t, html, chrome(t, "fund image"),
		"the image control should be the same card as history and enrollments beside it")

	// And still the thing htmx swaps, with the id outside the card so a reply
	// replaces the heading along with the body.
	require.Contains(t, html, `<div id="fund-image-control">`)
	require.Less(t, strings.Index(html, `id="fund-image-control"`), strings.Index(html, "fund image"),
		"the swap target should wrap the card, not sit inside it")
}

// Section carried id="enrollment-success" from whatever it was first cut from.
// Nothing referenced it, and the webhooks page draws three sections -- so that
// page had three elements sharing one id.
func TestSectionsDoNotAllShareAnID(t *testing.T) {
	require.NotContains(t, renderAdmin(t, common.Section("history")), "enrollment-success")

	// Two on a page is the case that was broken, so it is the case worth asserting.
	both := renderAdmin(t, FundImageControl(uuid.New(), nil, "")) +
		renderAdmin(t, FundHistory(nil))

	ids := regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(both, -1)

	seen := map[string]bool{}
	for _, id := range ids {
		require.False(t, seen[id[1]], "two elements share id %q", id[1])
		seen[id[1]] = true
	}
}

// A refused upload has to say so inside the card, not replace it.
func TestAFailureKeepsTheCard(t *testing.T) {
	html := renderAdmin(t, FundImageControl(uuid.New(), &donations.FundImage{
		SHA256: "abc", ContentType: "image/jpeg", Width: 10, Height: 10,
	}, "that file is not a jpeg, png or webp image."))

	require.Contains(t, html, chrome(t, "fund image"))
	require.Contains(t, html, "not a jpeg")
}
