package vfs

import (
	"context"
	"io"
)

// Copy streams bytes from src to dst. It honors ctx cancellation before starting
// the copy; ongoing copies may continue until the underlying Read observes the
// cancellation (backend-dependent). For server-side or same-bucket optimized
// transfers, use backend-specific APIs when available.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return io.Copy(dst, src)
}
