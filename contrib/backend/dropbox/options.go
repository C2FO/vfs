package dropbox

import (
	"os"
)

// Options holds configuration options for the Dropbox FileSystem.
type Options struct {
	// AccessToken is the OAuth2 access token for Dropbox API authentication (required).
	AccessToken string

	// ChunkSize is the size of chunks for uploading large files (default: 4MB).
	// Files larger than MaxSimpleUploadSize are automatically uploaded using chunked sessions.
	ChunkSize int64

	// MaxSimpleUploadSize is the threshold above which chunked uploads are used.
	// Default is 150MB (Dropbox API limit). This can be lowered for testing purposes.
	MaxSimpleUploadSize int64

	// TempDir is the directory for temporary files used during read/write operations.
	// Defaults to os.TempDir() if not specified.
	TempDir string
}

// NewOptions creates Options with default values.
func NewOptions() Options {
	return Options{
		ChunkSize:           4 * 1024 * 1024,   // 4MB default chunk size
		MaxSimpleUploadSize: 150 * 1024 * 1024, // 150MB Dropbox API limit
		TempDir:             os.TempDir(),
	}
}
