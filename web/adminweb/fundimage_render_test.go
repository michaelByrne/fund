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

// The picture is a setting now, so the card it appears in is the settings card,
// and the picture control is a part of that rather than a box of its own.
func TestTheDetailsCardIsTheSameCardAsTheRest(t *testing.T) {
	html := renderAdmin(t, FundDetails(donations.Fund{ID: uuid.New(), Name: "human fund"}, nil, "", ""))

	require.Contains(t, html, chrome(t, "fund details"),
		"fund details should be the same card as history and enrollments beside it")

	// The picture keeps its own id inside, because uploading swaps just that part
	// rather than redrawing the description and goal around it.
	require.Contains(t, html, `id="fund-image-control"`)
	require.Less(t, strings.Index(html, "fund details"), strings.Index(html, `id="fund-image-control"`),
		"the picture control belongs inside the card, not around it")
}

// Section carried id="enrollment-success" from whatever it was first cut from.
// Nothing referenced it, and the webhooks page draws three sections -- so that
// page had three elements sharing one id.
func TestSectionsDoNotAllShareAnID(t *testing.T) {
	require.NotContains(t, renderAdmin(t, common.Section("history")), "enrollment-success")

	// Two on a page is the case that was broken, so it is the case worth asserting.
	both := renderAdmin(t, FundDetails(donations.Fund{ID: uuid.New()}, nil, "", "")) +
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

	require.Contains(t, html, "not a jpeg")
	require.Contains(t, html, `id="fund-image-control"`)
}

// The picture is chosen while the fund is being described, so the form has to be
// able to carry a file at all.
func TestTheCreateFormCanCarryAPicture(t *testing.T) {
	html := renderAdmin(t, AddFund())

	// Without this htmx sends the filename and nothing else, and the server sees an
	// empty upload -- a fund created every time and never a picture with it.
	require.Contains(t, html, `hx-encoding="multipart/form-data"`)
	require.Contains(t, html, `type="file"`)
	require.Contains(t, html, `name="image"`)

	// Somewhere for a partial success to land. The response to this form is a row
	// for a different part of the page.
	require.Contains(t, html, `id="fund-create-notice"`)
}

// Creating a fund also creates the PayPal product and plan. A picture that will
// not upload must not read as though none of that happened, or somebody makes a
// second fund to replace the one they think failed.
func TestAFailedPictureStillReportsTheFund(t *testing.T) {
	fund := donations.Fund{ID: uuid.New(), Name: "human fund"}

	html := renderAdmin(t, FundCreatedWithoutPicture(fund, "that file is not a jpeg, png or webp image."))

	require.Contains(t, html, "human fund was created")
	require.Contains(t, html, "not a jpeg")

	// The row still goes to the list, so the fund appears where it belongs.
	require.Contains(t, html, "/admin/fund/audit?fund="+fund.ID.String())

	// Out of band, because the form's target is the list and this belongs by the
	// form.
	require.Contains(t, html, `hx-swap-oob="true"`)
}
