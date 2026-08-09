package aws

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// FundImages stores fund pictures in one bucket.
//
// One bucket with a key per object, rather than the bucket-per-fund-per-type
// shape the report code uses. That shape needs a CreateBucket at fund creation,
// runs into the account bucket limit, and gives nothing back: a key is free and
// a bucket is not.
//
// Objects are private. They are read back through the application, which is what
// keeps them behind the same URL and the same cache as everything else, and means
// there is no public bucket policy to get wrong.
type FundImages struct {
	s3Client *s3.Client
	bucket   string

	logger *slog.Logger
}

func NewFundImages(s3Client *s3.Client, bucket string, logger *slog.Logger) *FundImages {
	return &FundImages{
		s3Client: s3Client,
		bucket:   bucket,
		logger:   logger,
	}
}

// PutFundImage writes the bytes and returns nothing: the caller already knows the
// key, because the key is derived from the contents.
//
// The content type is stored on the object as well as in the database. Nothing
// reads it from here -- the database is what the serving path consults -- but an
// object that describes itself is worth the field to whoever is looking in the
// bucket at three in the morning.
func (f FundImages) PutFundImage(ctx context.Context, key, contentType string, body []byte) error {
	_, err := f.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &f.bucket,
		Key:         &key,
		ContentType: &contentType,
		Body:        newSeekableReader(body),
	})
	if err != nil {
		f.logger.ErrorContext(ctx, "failed to put fund image",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)

		return err
	}

	return nil
}

// GetFundImage reads an object back.
//
// The caller closes it. It is streamed rather than buffered because the only
// caller is writing it straight to a response.
func (f FundImages) GetFundImage(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := f.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &f.bucket,
		Key:    &key,
	})
	if err != nil {
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			// The database says there should be an object here and there is not.
			// Worth knowing about, but it is a 404 to whoever asked, not a 500.
			f.logger.ErrorContext(ctx, "fund image is recorded but not in the bucket", slog.String("key", key))

			return nil, nil
		}

		f.logger.ErrorContext(ctx, "failed to get fund image",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)

		return nil, err
	}

	return output.Body, nil
}

// DeleteFundImage removes an object.
//
// S3 treats deleting something that is not there as success, which is the right
// answer here too: the caller wanted it gone.
func (f FundImages) DeleteFundImage(ctx context.Context, key string) error {
	_, err := f.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &f.bucket,
		Key:    &key,
	})
	if err != nil {
		f.logger.ErrorContext(ctx, "failed to delete fund image",
			slog.String("key", key),
			slog.String("error", err.Error()),
		)

		return err
	}

	return nil
}

// newSeekableReader gives the SDK something it can rewind.
//
// PutObject signs the payload and may retry, and a plain reader cannot be read
// twice. bytes.Reader can, so a retried upload sends the same bytes rather than
// nothing.
func newSeekableReader(body []byte) *bytes.Reader {
	return bytes.NewReader(body)
}
