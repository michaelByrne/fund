package reporting

import (
	"context"
	"io"
)

type uploader interface {
	Upload(ctx context.Context, file io.Reader, filename string) error
}
