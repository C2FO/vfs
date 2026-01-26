package dropbox

import (
	"os"
)

// Options holds configuration options for the Dropbox FileSystem.
type Options struct {
	// AccessToken is the OAuth2 access token for Dropbox API authentication (required).
	AccessToken string

	// ChunkSize is the size of chunks for uploading large files (default: 4MB).
	// Files larger than 150MB are automatically uploaded using chunked sessions.
	ChunkSize int64

	// TempDir is the directory for temporary files used during read/write operations.
	// Defaults to os.TempDir() if not specified.
	TempDir string

	// RetryCount is the number of retry attempts for transient errors (default: 3).
	RetryCount int
}

// NewOptions creates Options with default values.
func NewOptions() Options {
	return Options{
		ChunkSize:  4 * 1024 * 1024, // 4MB default chunk size
		TempDir:    os.TempDir(),
		RetryCount: 3,
	}
}
