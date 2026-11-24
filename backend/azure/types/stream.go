package types

import (
	"context"
	"io"

	"github.com/c2fo/vfs/v7"
)

// BlobStreamer is an optional interface a Client implements to stream block blob uploads without a seekable local
// buffer and to read blobs from an arbitrary byte offset. Client itself is unchanged so that existing implementations
// keep working.
type BlobStreamer interface {
	// UploadStream uploads content to the block blob for file without requiring an io.ReadSeeker, committing only after
	// content reaches io.EOF without error or context cancellation.
	UploadStream(ctx context.Context, file vfs.File, content io.Reader, contentType string) error

	// DownloadRange returns a reader for the blob for file starting at byte offset (0 reads the whole blob).
	DownloadRange(ctx context.Context, file vfs.File, offset int64) (io.ReadCloser, error)
}
