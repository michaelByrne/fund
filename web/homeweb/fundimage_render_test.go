package homeweb

import (
	"strings"
	"testing"
	"time"

	"boardfund/service/donations"
	"boardfund/service/members"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func fundWithImage(t *testing.T) (donations.Fund, map[uuid.UUID]donations.FundImage) {
	t.Helper()

	fund := donations.Fund{ID: uuid.New(), Name: "human fund", Description: "d"}

	return fund, map[uuid.UUID]donations.FundImage{
		fund.ID: {FundID: fund.ID, SHA256: "abc123", ContentType: "image/jpeg", Width: 800, Height: 400},
	}
}

// The list is where somebody chooses a fund, so it is the place a picture does
// the most work.
func TestTheFundListShowsPictures(t *testing.T) {
	fund, images := fundWithImage(t)

	html := render(t, Funds([]donations.Fund{fund}, nil, images, nil, &members.Member{}, "/"))

	require.Contains(t, html, "/fund/"+fund.ID.String()+"/image/abc123")

	// Twice: the table for wide screens and the stacked cards for narrow ones are
	// both in the markup, and a picture in only one of them is missing from a real
	// device.
	require.Equal(t, 2, strings.Count(html, "/fund/"+fund.ID.String()+"/image/abc123"),
		"both the table and the small-screen cards should show it")
}

// Most funds have no picture, and the list has to look deliberate either way.
func TestAFundWithNoPictureStillLines(t *testing.T) {
	fund := donations.Fund{ID: uuid.New(), Name: "human fund"}

	html := render(t, Funds([]donations.Fund{fund}, nil, nil, nil, &members.Member{}, "/"))

	require.NotContains(t, html, "/image/")

	// The box is still there, so the names stay in a column instead of stepping in
	// and out depending on which funds have a picture.
	require.Contains(t, html, "w-10 h-10")
}

// The archive is the fund's page after it ends. It kept the notes; it should keep
// the picture too.
func TestTheArchiveShowsTheFundPicture(t *testing.T) {
	closedOn := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fund := donations.ClosedFund{
		Fund: donations.Fund{ID: uuid.New(), Name: "human fund", Expires: &closedOn},
	}

	image := &donations.FundImage{
		FundID: fund.ID, SHA256: "deadbeef", ContentType: "image/jpeg", Width: 800, Height: 400,
	}

	html := render(t, ClosedFundSummary(fund, donations.FundStats{}, nil, nil, image, nil, &members.Member{}, "/archive"))

	require.Contains(t, html, "/fund/"+fund.ID.String()+"/image/deadbeef")
}

// Closed funds are listed on the front page too, and read the same map.
func TestTheClosedListShowsPictures(t *testing.T) {
	closedOn := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fund := donations.ClosedFund{
		Fund: donations.Fund{ID: uuid.New(), Name: "winter fund", Expires: &closedOn},
	}

	images := map[uuid.UUID]donations.FundImage{
		fund.ID: {FundID: fund.ID, SHA256: "cafe", ContentType: "image/jpeg", Width: 400, Height: 400},
	}

	html := render(t, ClosedFunds([]donations.ClosedFund{fund}, images))

	require.Equal(t, 2, strings.Count(html, "/fund/"+fund.ID.String()+"/image/cafe"))
}

// A picture beside a name is decoration: the name is right there, and a screen
// reader announcing the fund twice is worse than it announcing it once.
func TestListPicturesAreNotAnnouncedTwice(t *testing.T) {
	fund, images := fundWithImage(t)

	html := render(t, Funds([]donations.Fund{fund}, nil, images, nil, &members.Member{}, "/"))

	require.Contains(t, html, `alt=""`)
	require.NotContains(t, html, `alt="human fund"`)
}

// Dimensions are given so the browser holds the space before the bytes arrive,
// rather than reflowing the list out from under somebody mid-scroll.
//
// Checked on each component rather than on the page: both are in the markup at
// once, so a page-wide assertion passes when only one of them carries them.
func TestListPicturesReserveTheirSpace(t *testing.T) {
	image := &donations.FundImage{
		FundID: uuid.New(), SHA256: "abc123", ContentType: "image/jpeg", Width: 800, Height: 400,
	}

	for name, html := range map[string]string{
		"thumbnail": render(t, FundThumbnail(image)),
		"card":      render(t, FundCardImage(image)),
	} {
		require.Containsf(t, html, `width="800"`, "%s should reserve its width", name)
		require.Containsf(t, html, `height="400"`, "%s should reserve its height", name)
		require.Containsf(t, html, `loading="lazy"`, "%s should not block the page on bytes", name)
	}
}

// The picture was a band across the top of the fund page: full width at h-48,
// which on a wide screen is about six to one. Almost nothing is that shape, so a
// portrait photograph kept a strip of its middle and a logo lost its top and
// bottom. The crop was the aspect ratio.
func TestTheFundPagePictureIsASquareAndIsNotCropped(t *testing.T) {
	fund := donations.Fund{ID: uuid.New(), Name: "human fund"}
	image := &donations.FundImage{
		FundID: fund.ID, SHA256: "abc", ContentType: "image/jpeg", Width: 600, Height: 1600,
	}

	html := render(t, FundImagePanel(fund, image))

	// Square: the same measurement both ways, at both sizes.
	require.Contains(t, html, "w-48 h-48")
	require.Contains(t, html, "md:w-64 md:h-64")

	// And nothing thrown away. object-cover fills the square by cropping, which is
	// a smaller version of the problem rather than a fix -- nothing here knows
	// where the subject of somebody's picture is.
	require.Contains(t, html, "object-contain")
	require.NotContains(t, html, "object-cover")

	// Not the full width of the page, which for a square means the height of the
	// screen.
	require.NotContains(t, html, "w-full h-48")
}

// The fund page names the fund beside the picture, but this one is the subject of
// the page rather than a decoration next to a label, so it is described.
func TestTheFundPagePictureIsDescribed(t *testing.T) {
	fund := donations.Fund{ID: uuid.New(), Name: "human fund"}
	image := &donations.FundImage{FundID: fund.ID, SHA256: "abc", Width: 10, Height: 10}

	require.Contains(t, render(t, FundImagePanel(fund, image)), `alt="human fund"`)
}

// A fund with no picture draws nothing at all -- no empty square holding space
// open on a page where nothing else lines up with it.
func TestTheFundPageDrawsNoEmptySquare(t *testing.T) {
	html := render(t, FundImagePanel(donations.Fund{ID: uuid.New(), Name: "human fund"}, nil))

	require.Empty(t, strings.TrimSpace(html))
}
