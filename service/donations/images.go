package donations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"time"

	"golang.org/x/image/draw"
	// Registered for their decoders only. An upload's format is whatever decoding
	// it says it is, never what the filename or the browser claimed.
	//
	// GIF is registered deliberately and then refused by decodableFormat. Leaving
	// it out would refuse GIFs too, but by accident -- the absence of a decoder is
	// not a decision anybody wrote down, and the next import added for some other
	// reason would quietly make a format acceptable. Registered and named, the
	// allowlist is what decides. GIFs are refused because re-encoding one to a
	// still is a worse answer than saying no.
	_ "image/gif"

	_ "golang.org/x/image/webp"

	"github.com/google/uuid"
)

const (
	// MaxImageBytes is the largest upload accepted, before anything is decoded.
	//
	// A phone photograph is comfortably inside this. It is the first of two limits
	// because a byte count alone does not bound the work: see MaxImagePixels.
	MaxImageBytes = 8 << 20

	// MaxImagePixels bounds the decoded image, which is the number that actually
	// costs memory -- four bytes each.
	//
	// A byte limit cannot do this job. A few kilobytes of PNG can describe an image
	// of fifty thousand squared, and decoding it is how a small upload becomes a
	// dead process. The header is read on its own and checked against this before
	// any pixels are allocated.
	MaxImagePixels = 50_000_000

	// MaxImageDimension is how large a side is stored. Anything bigger is scaled
	// down to it, which is also what keeps the stored bytes small.
	MaxImageDimension = 1600

	// jpegQuality is a visible-quality/size tradeoff. 82 is where photographs stop
	// gaining much from more.
	jpegQuality = 82
)

// ErrImageTooLarge means the upload exceeded MaxImageBytes.
var ErrImageTooLarge = fmt.Errorf("an image can be at most %d megabytes", MaxImageBytes>>20)

// ErrImageTooManyPixels means the image decodes to more than MaxImagePixels.
//
// Separate from ErrImageTooLarge because it is a different refusal: the file was
// small and the picture it describes is not.
var ErrImageTooManyPixels = errors.New("that image is too many pixels to process")

// ErrImageUnreadable means it did not decode as an image we accept.
//
// Deliberately one error for "not an image", "a format we do not take" and
// "corrupt". The uploader gets the same sentence for all three because there is
// nothing useful to distinguish, and because SVG lands here -- the answer to
// which is no, not an explanation of what would make it acceptable.
var ErrImageUnreadable = errors.New("that file is not a jpeg, png or webp image")

// FundImage is a fund's picture, without the picture.
//
// The bytes are fetched by the one request that serves them. Everything that
// renders a page needs the URL and the shape and nothing else.
type FundImage struct {
	FundID      uuid.UUID
	ContentType string
	Width       int
	Height      int
	SHA256      string

	Created time.Time
	Updated time.Time
}

// URL is where the image is served, and it contains the hash of its contents.
//
// So a replaced image is a different URL rather than a stale one, and any cache
// TTL is correct at any layer -- a cached copy can only be of the bytes its URL
// names. The stylesheet that arrived 55 minutes out of date is the same lesson.
func (i FundImage) URL() string {
	return "/fund/" + i.FundID.String() + "/image/" + i.SHA256
}

// FundImageObject is where a picture is and what it is, without the picture.
type FundImageObject struct {
	S3Key       string
	ContentType string
	Updated     time.Time
}

// SaveFundImage decodes an upload, re-encodes it, and stores what it produced.
//
// Re-encoding rather than storing the upload is the point of this function. It
// strips EXIF, which on a photograph taken by a person carries where they were
// standing. It defeats a file that is a valid image and a valid something-else at
// once, because what comes out is written by us from decoded pixels. And it means
// the bytes served to every visitor are bytes this application produced, so the
// content type is a fact rather than a claim.
func (s DonationService) SaveFundImage(ctx context.Context, fundID uuid.UUID, upload io.Reader) (*FundImage, error) {
	// One byte past the limit, so a file exactly at it still passes and anything
	// larger is caught here rather than by reading all of it.
	raw, err := io.ReadAll(io.LimitReader(upload, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}

	if len(raw) > MaxImageBytes {
		return nil, ErrImageTooLarge
	}

	// The header first, and only the header. This is what stands between us and an
	// image whose dimensions are a denial of service.
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrImageUnreadable
	}

	if !decodableFormat(format) {
		return nil, ErrImageUnreadable
	}

	if config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return nil, ErrImageTooManyPixels
	}

	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrImageUnreadable
	}

	// Resampled once. This is the expensive step on a large upload, and it was
	// being done twice -- once to encode and once to ask the result its size.
	scaled := scaleDown(decoded)

	encoded, contentType, err := reencode(scaled)
	if err != nil {
		s.logger.Error("failed to re-encode a fund image", slog.String("error", err.Error()))

		return nil, err
	}

	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	bounds := scaled.Bounds()
	key := fundImageKey(fundID, digest, contentType)

	// What this is replacing, read before anything is written, so the object it
	// points at can be collected afterwards.
	previous, err := s.donationStore.GetFundImageKey(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to read the current fund image key", slog.String("error", err.Error()))

		return nil, err
	}

	// Bucket first, database second. This way round a failure between them leaves
	// an object nothing points at, which costs pennies and is invisible. The other
	// way round leaves a fund pointing at an object that is not there, which is a
	// broken picture on the page.
	if err = s.fundImages.PutFundImage(ctx, key, contentType, encoded); err != nil {
		return nil, err
	}

	image, err := s.donationStore.UpsertFundImage(ctx, UpsertFundImage{
		FundID:      fundID,
		S3Key:       key,
		ContentType: contentType,
		Width:       bounds.Dx(),
		Height:      bounds.Dy(),
		SHA256:      digest,
	})
	if err != nil {
		s.logger.Error("failed to store a fund image", slog.String("error", err.Error()))

		return nil, err
	}

	// Best effort, and after the row is committed. The new picture is already
	// live; failing the upload because the old bytes could not be tidied away
	// would undo work that succeeded to fix something nobody can see.
	//
	// Skipped when the key is unchanged, which happens when the same file is
	// uploaded twice -- the key is the hash, so that write replaced itself.
	if previous != "" && previous != key {
		if errDelete := s.fundImages.DeleteFundImage(ctx, previous); errDelete != nil {
			s.logger.Error("failed to remove the replaced fund image",
				slog.String("key", previous),
				slog.String("error", errDelete.Error()),
			)
		}
	}

	return image, nil
}

// fundImageKey is where an image lives in the bucket.
//
// Prefixed by fund so the bucket is browsable by a person, and named by the hash
// of its contents so a replacement is a new object rather than an overwrite:
// nothing can be holding a cached copy of a key whose bytes changed.
func fundImageKey(fundID uuid.UUID, digest, contentType string) string {
	extension := ".jpg"
	if contentType == "image/png" {
		extension = ".png"
	}

	return "fund/" + fundID.String() + "/" + digest + extension
}

// decodableFormat is what we accept, named rather than inferred from whatever
// decoders happen to be registered.
//
// SVG is not among them and must never be: it is a document that can run script,
// and this serves it from our own origin. It does not decode as an image anyway,
// so this is a second lock on a door that is already shut.
func decodableFormat(format string) bool {
	switch format {
	case "jpeg", "png", "webp":
		return true
	default:
		return false
	}
}

// scaleDown fits the image inside MaxImageDimension, keeping its proportions. An
// image already inside it is returned untouched.
func scaleDown(src image.Image) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	if width <= MaxImageDimension && height <= MaxImageDimension {
		return src
	}

	if width > height {
		height = height * MaxImageDimension / width
		width = MaxImageDimension
	} else {
		width = width * MaxImageDimension / height
		height = MaxImageDimension
	}

	// A side can round to zero on an extremely long thin image, and a zero-sided
	// image is not one.
	width = max(width, 1)
	height = max(height, 1)

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	return dst
}

// reencode writes the image out again, as PNG when it has transparency and JPEG
// otherwise.
//
// Always-JPEG would put a black rectangle behind every logo with a transparent
// background, and always-PNG would store photographs at several times the size.
func reencode(img image.Image) ([]byte, string, error) {
	var out bytes.Buffer

	if hasAlpha(img) {
		if err := png.Encode(&out, img); err != nil {
			return nil, "", err
		}

		return out.Bytes(), "image/png", nil
	}

	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", err
	}

	return out.Bytes(), "image/jpeg", nil
}

// hasAlpha reports whether any pixel is not fully opaque.
//
// The colour model is not enough to decide: RGBA is what everything becomes after
// scaling, and almost all of those are opaque throughout. Asking the pixels costs
// one pass over an image already bounded to MaxImageDimension squared.
func hasAlpha(img image.Image) bool {
	if opaque, ok := img.(interface{ Opaque() bool }); ok {
		return !opaque.Opaque()
	}

	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}

	return false
}

// GetFundImage is what a page needs to render the tag.
func (s DonationService) GetFundImage(ctx context.Context, fundID uuid.UUID) (*FundImage, error) {
	image, err := s.donationStore.GetFundImageMeta(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to read a fund image", slog.String("error", err.Error()))

		return nil, err
	}

	return image, nil
}

// GetFundImages is the same for many funds at once, keyed by fund.
//
// The home page lists every fund there is, and one query per tile to decide
// whether to draw a picture is a query per tile too many.
func (s DonationService) GetFundImages(ctx context.Context, fundIDs []uuid.UUID) (map[uuid.UUID]FundImage, error) {
	if len(fundIDs) == 0 {
		return map[uuid.UUID]FundImage{}, nil
	}

	images, err := s.donationStore.GetFundImageMetaForFunds(ctx, fundIDs)
	if err != nil {
		s.logger.Error("failed to read fund images", slog.String("error", err.Error()))

		return nil, err
	}

	return images, nil
}

// OpenFundImage is the picture itself, for the request that serves it.
//
// The hash is part of the lookup rather than checked afterwards, so a URL naming
// bytes that are no longer there finds nothing instead of finding their
// replacement. A nil reader with no error means exactly that: no such picture.
//
// The caller closes it. It is streamed rather than read into memory because the
// only caller is copying it straight to a response.
func (s DonationService) OpenFundImage(ctx context.Context, fundID uuid.UUID, sha256 string) (io.ReadCloser, *FundImageObject, error) {
	object, err := s.donationStore.GetFundImageObject(ctx, fundID, sha256)
	if err != nil {
		return nil, nil, err
	}

	if object == nil {
		return nil, nil, nil
	}

	body, err := s.fundImages.GetFundImage(ctx, object.S3Key)
	if err != nil {
		return nil, nil, err
	}

	if body == nil {
		// Recorded but not in the bucket. Already logged where it was noticed; to
		// whoever asked it is a picture that is not there.
		return nil, nil, nil
	}

	return body, object, nil
}

// RemoveFundImage takes a fund's picture down.
//
// A hard delete, unlike a note. There is no decision here worth keeping a record
// of -- it is the fund's own picture, replaced or withdrawn by the people who put
// it there, and nothing was published in somebody else's name.
//
// The row goes first. Once it is gone the picture is off every page, which is
// what was asked for; the object is then tidied up, and a failure to do that
// leaves bytes in a bucket that nothing points at rather than a picture still
// showing on the site.
func (s DonationService) RemoveFundImage(ctx context.Context, fundID uuid.UUID) error {
	key, err := s.donationStore.GetFundImageKey(ctx, fundID)
	if err != nil {
		s.logger.Error("failed to read the fund image key", slog.String("error", err.Error()))

		return err
	}

	if err = s.donationStore.DeleteFundImage(ctx, fundID); err != nil {
		s.logger.Error("failed to remove a fund image", slog.String("error", err.Error()))

		return err
	}

	if key == "" {
		return nil
	}

	if err = s.fundImages.DeleteFundImage(ctx, key); err != nil {
		s.logger.Error("failed to remove the fund image object",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)
	}

	return nil
}
