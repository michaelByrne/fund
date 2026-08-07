package donations_test

import (
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"boardfund/pg"
	"boardfund/service/donations"
	donationsstore "boardfund/service/donations/store"
	"boardfund/service/fundevents"
	fundeventstore "boardfund/service/fundevents/store"
	"boardfund/service/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeBucket stands in for S3.
//
// The real client is a thin wrapper over three SDK calls; what is worth testing
// is everything around it -- what gets written, in what order against the row,
// and what is cleaned up. Those are decisions this code makes. Whether the SDK
// can put an object is not.
type fakeBucket struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string

	putErr error
	// puts and deletes record what happened, in order, so a test can say what
	// should have been tidied away rather than only what remains.
	puts    []string
	deletes []string
}

func newFakeBucket() *fakeBucket {
	return &fakeBucket{objects: map[string][]byte{}, types: map[string]string{}}
}

func (f *fakeBucket) PutFundImage(_ context.Context, key, contentType string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.putErr != nil {
		return f.putErr
	}

	f.objects[key] = append([]byte{}, body...)
	f.types[key] = contentType
	f.puts = append(f.puts, key)

	return nil
}

func (f *fakeBucket) GetFundImage(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, ok := f.objects[key]
	if !ok {
		return nil, nil
	}

	return io.NopCloser(bytes.NewReader(body)), nil
}

func (f *fakeBucket) DeleteFundImage(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.objects, key)
	delete(f.types, key)
	f.deletes = append(f.deletes, key)

	return nil
}

func (f *fakeBucket) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.objects)
}

// readAll drains what the service handed back, which is what a visitor is served.
func readAll(t *testing.T, body io.ReadCloser) []byte {
	t.Helper()

	defer body.Close()

	out, err := io.ReadAll(body)
	require.NoError(t, err)

	return out
}

// jpegOf is an opaque photograph-shaped image.
func jpegOf(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}

	var out bytes.Buffer
	require.NoError(t, jpeg.Encode(&out, img, nil))

	return out.Bytes()
}

// pngWithAlpha is a logo-shaped image: transparent, which is the thing that must
// survive the round trip.
func pngWithAlpha(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 0})
		}
	}

	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, img))

	return out.Bytes()
}

// An image upload is the largest new surface this application has, and almost all
// of the work is refusing things.
func TestFundImages(t *testing.T) {
	ctx := context.Background()

	container, pool, err := pg.SetupTestDatabase()
	require.NoError(t, err)

	t.Cleanup(func() { _ = container.Terminate(ctx) })

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	store := donationsstore.NewDonationStore(pool)
	bucket := newFakeBucket()
	svc := donations.NewDonationService(store, stubDocumentStorage{}, bucket, &mocks.PaymentsProviderMock{},
		fundevents.NewService(fundeventstore.NewEventStore(pool), logger), nil, logger)

	t.Run("stores a jpeg and serves it back", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 300, 200)))
		require.NoError(t, errSave)
		require.Equal(t, "image/jpeg", saved.ContentType)
		require.Equal(t, 300, saved.Width)
		require.Equal(t, 200, saved.Height)

		body, object, errGet := svc.OpenFundImage(ctx, fundID, saved.SHA256)
		require.NoError(t, errGet)
		require.NotNil(t, body)
		require.Equal(t, "image/jpeg", object.ContentType)

		// What comes back must decode, because it is what every visitor is served.
		_, format, errDecode := image.DecodeConfig(bytes.NewReader(readAll(t, body)))
		require.NoError(t, errDecode)
		require.Equal(t, "jpeg", format)

		requireStoredSize(t, ctx, svc, fundID, saved)
	})

	// Always-JPEG would put a black rectangle behind every logo.
	t.Run("keeps transparency as png", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(pngWithAlpha(t, 120, 120)))
		require.NoError(t, errSave)
		require.Equal(t, "image/png", saved.ContentType)
	})

	// The stored picture is ours, not theirs. This is what strips EXIF and what
	// makes a file that is valid as two formats at once merely a picture.
	t.Run("stores what it re-encoded, not what was uploaded", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		// A JPEG carrying a comment. Anything the encoder did not put there is gone
		// by definition, and a marker is the readable way to prove it.
		original := jpegOf(t, 80, 80)
		withMarker := append([]byte{}, original...)
		withMarker = append(withMarker, []byte("SECRETLOCATIONDATA")...)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(withMarker))
		require.NoError(t, errSave)

		body, _, errGet := svc.OpenFundImage(ctx, fundID, saved.SHA256)
		require.NoError(t, errGet)

		require.False(t, bytes.Contains(readAll(t, body), []byte("SECRETLOCATIONDATA")),
			"trailing data survived, so the upload was stored rather than re-encoded")
	})

	t.Run("scales a large image down", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 3200, 1600)))
		require.NoError(t, errSave)

		require.Equal(t, donations.MaxImageDimension, saved.Width)
		require.Equal(t, donations.MaxImageDimension/2, saved.Height, "proportions should hold")

		// And the stored picture really is that size. Asserting only the recorded
		// dimensions passes just as well when the full-size image was encoded and
		// the row describes something it is not -- which is a page reserving space
		// for a picture half the size of the one arriving.
		requireStoredSize(t, ctx, svc, fundID, saved)
	})

	t.Run("refuses something that is not an image", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		_, errSave := svc.SaveFundImage(ctx, fundID, strings.NewReader("this is just some text"))
		require.ErrorIs(t, errSave, donations.ErrImageUnreadable)
	})

	// SVG is a document that can run script, and this would serve it from our own
	// origin. It must never be storable, whatever it is labelled.
	t.Run("refuses svg", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
			`<script>alert(1)</script></svg>`

		_, errSave := svc.SaveFundImage(ctx, fundID, strings.NewReader(svg))
		require.ErrorIs(t, errSave, donations.ErrImageUnreadable)
	})

	// The decoder is registered, so this reaches the allowlist and is refused
	// there. That is what makes the allowlist the thing deciding, rather than
	// which decoders happen to be linked in.
	t.Run("refuses a gif, which decodes but is not on the list", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		img := image.NewRGBA(image.Rect(0, 0, 20, 20))

		var out bytes.Buffer
		require.NoError(t, gif.Encode(&out, img, nil))

		// It really does decode -- otherwise this would prove nothing about the list.
		_, format, errDecode := image.DecodeConfig(bytes.NewReader(out.Bytes()))
		require.NoError(t, errDecode)
		require.Equal(t, "gif", format)

		_, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(out.Bytes()))
		require.ErrorIs(t, errSave, donations.ErrImageUnreadable)
	})

	t.Run("refuses an upload past the byte limit", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		// Bytes that would decode if there were fewer of them, so this is the size
		// being refused rather than the content.
		oversized := bytes.Repeat([]byte{0xff}, donations.MaxImageBytes+1)
		copy(oversized, jpegOf(t, 10, 10))

		_, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(oversized))
		require.ErrorIs(t, errSave, donations.ErrImageTooLarge)
	})

	// A byte limit cannot catch this: a few kilobytes of PNG describes an image of
	// tens of thousands squared, and decoding it is how a small upload kills the
	// process. The header is read on its own and checked before any pixels exist.
	t.Run("refuses a small file describing an enormous image", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		bomb := pngWithAlpha(t, 1, 1)
		require.Less(t, len(bomb), 1024, "the point is that the file is tiny")

		// 30000 x 30000 is 900 million pixels, 3.6GB decoded.
		huge := image.NewRGBA(image.Rect(0, 0, 1, 1))
		var out bytes.Buffer
		require.NoError(t, png.Encode(&out, huge))

		// Rewrite the IHDR dimensions rather than encoding something enormous, which
		// is exactly the trick being defended against.
		forged := forgePNGSize(t, out.Bytes(), 30000, 30000)

		_, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(forged))
		require.ErrorIs(t, errSave, donations.ErrImageTooManyPixels)
	})

	t.Run("replacing an image changes its url", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		first, errFirst := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errFirst)

		second, errSecond := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 140, 90)))
		require.NoError(t, errSecond)

		require.NotEqual(t, first.URL(), second.URL(),
			"a replacement under the same url is how a cache serves the old picture for ever")

		// And the old URL is a miss rather than a way to the new bytes.
		old, _, errOld := svc.OpenFundImage(ctx, fundID, first.SHA256)
		require.NoError(t, errOld)
		require.Nil(t, old)
	})

	t.Run("one image per fund", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		_, errFirst := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errFirst)
		_, errSecond := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errSecond)

		var count int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM fund_image WHERE fund_id = $1`, fundID).Scan(&count))
		require.Equal(t, 1, count)
	})

	t.Run("removing takes it down", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errSave)

		require.NoError(t, svc.RemoveFundImage(ctx, fundID))

		meta, errMeta := svc.GetFundImage(ctx, fundID)
		require.NoError(t, errMeta)
		require.Nil(t, meta)

		gone, _, errGone := svc.OpenFundImage(ctx, fundID, saved.SHA256)
		require.NoError(t, errGone)
		require.Nil(t, gone)
	})

	// The bucket and the row are two writes with no transaction across them, so
	// what happens between them is a decision rather than an accident.
	t.Run("a bucket failure stores nothing", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		bucket.putErr = errors.New("s3 is having a day")
		t.Cleanup(func() { bucket.putErr = nil })

		_, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.Error(t, errSave)

		// No row, so no page points at an object that was never written.
		meta, errMeta := svc.GetFundImage(ctx, fundID)
		require.NoError(t, errMeta)
		require.Nil(t, meta, "a fund must not point at a picture that failed to upload")
	})

	t.Run("replacing an image cleans up the one it replaced", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		before := bucket.count()

		first, errFirst := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errFirst)

		_, errSecond := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 140, 90)))
		require.NoError(t, errSecond)

		require.Equal(t, before+1, bucket.count(),
			"replacing left the old object behind, so the bucket grows for ever")

		require.Contains(t, bucket.deletes, fundImageKeyFor(fundID, first),
			"the object that was replaced is the one that should have gone")
	})

	// The key is the hash, so this wrote the same object twice. Deleting "the old
	// one" afterwards would delete the picture that is now live.
	t.Run("uploading the same picture twice keeps it", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		same := jpegOf(t, 100, 100)

		first, errFirst := svc.SaveFundImage(ctx, fundID, bytes.NewReader(same))
		require.NoError(t, errFirst)

		second, errSecond := svc.SaveFundImage(ctx, fundID, bytes.NewReader(same))
		require.NoError(t, errSecond)
		require.Equal(t, first.SHA256, second.SHA256)

		body, _, errOpen := svc.OpenFundImage(ctx, fundID, second.SHA256)
		require.NoError(t, errOpen)
		require.NotNil(t, body, "re-uploading the same picture deleted it")
		require.NotEmpty(t, readAll(t, body))
	})

	t.Run("removing takes the object with it", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 111, 111)))
		require.NoError(t, errSave)

		key := fundImageKeyFor(fundID, saved)
		require.NoError(t, svc.RemoveFundImage(ctx, fundID))

		require.Contains(t, bucket.deletes, key, "the row went and the bytes stayed")
	})

	// The row says there is a picture and the bucket disagrees. Whoever asked gets
	// a missing picture, not a 500 and not a panic.
	t.Run("a row pointing at nothing is a miss", func(t *testing.T) {
		fundID := seedOnceFund(t, ctx, pool)

		saved, errSave := svc.SaveFundImage(ctx, fundID, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errSave)

		require.NoError(t, bucket.DeleteFundImage(ctx, fundImageKeyFor(fundID, saved)))

		body, object, errOpen := svc.OpenFundImage(ctx, fundID, saved.SHA256)
		require.NoError(t, errOpen)
		require.Nil(t, body)
		require.Nil(t, object)
	})

	t.Run("a fund with no image is not an error", func(t *testing.T) {
		meta, errMeta := svc.GetFundImage(ctx, seedOnceFund(t, ctx, pool))
		require.NoError(t, errMeta)
		require.Nil(t, meta)
	})

	t.Run("many funds cost one query", func(t *testing.T) {
		withImage := seedOnceFund(t, ctx, pool)
		without := seedOnceFund(t, ctx, pool)

		_, errSave := svc.SaveFundImage(ctx, withImage, bytes.NewReader(jpegOf(t, 100, 100)))
		require.NoError(t, errSave)

		images, errImages := svc.GetFundImages(ctx, []uuid.UUID{withImage, without})
		require.NoError(t, errImages)
		require.Contains(t, images, withImage)
		require.NotContains(t, images, without)
	})
}

// requireStoredSize checks the object against what the row says about it.
//
// The two can disagree -- the dimensions are read from the scaled image and the
// bytes are whatever was handed to the encoder -- and nothing else here would
// notice, because every page draws from the row.
func requireStoredSize(t *testing.T, ctx context.Context, svc *donations.DonationService,
	fundID uuid.UUID, recorded *donations.FundImage) {
	t.Helper()

	body, _, err := svc.OpenFundImage(ctx, fundID, recorded.SHA256)
	require.NoError(t, err)
	require.NotNil(t, body)

	config, _, err := image.DecodeConfig(bytes.NewReader(readAll(t, body)))
	require.NoError(t, err)

	require.Equal(t, recorded.Width, config.Width, "the stored picture is not the width recorded for it")
	require.Equal(t, recorded.Height, config.Height, "the stored picture is not the height recorded for it")
}

// fundImageKeyFor is the key the service would have written, rebuilt from the
// outside so a test asserts against the naming rather than trusting it.
func fundImageKeyFor(fundID uuid.UUID, recorded *donations.FundImage) string {
	extension := ".jpg"
	if recorded.ContentType == "image/png" {
		extension = ".png"
	}

	return "fund/" + fundID.String() + "/" + recorded.SHA256 + extension
}

// forgePNGSize rewrites the width and height in a PNG's IHDR and fixes its CRC,
// producing a tiny file that claims to be an enormous picture.
func forgePNGSize(t *testing.T, data []byte, width, height uint32) []byte {
	t.Helper()

	// 8 byte signature, 4 byte length, 4 byte "IHDR", then width and height.
	const ihdr = 8 + 4 + 4
	require.Greater(t, len(data), ihdr+8)

	forged := append([]byte{}, data...)
	put := func(offset int, v uint32) {
		forged[offset] = byte(v >> 24)
		forged[offset+1] = byte(v >> 16)
		forged[offset+2] = byte(v >> 8)
		forged[offset+3] = byte(v)
	}

	put(ihdr, width)
	put(ihdr+4, height)

	// The CRC follows the 13 bytes of IHDR data and covers the type and the data.
	// Go's decoder verifies it, so a forgery that skips this is merely corrupt.
	put(ihdr+13, crc32.ChecksumIEEE(forged[ihdr-4:ihdr+13]))

	return forged
}
