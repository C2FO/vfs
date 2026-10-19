package gdrive

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// ScopeDrive grants access to all of a user's Drive files. It is the default scope.
const ScopeDrive = "https://www.googleapis.com/auth/drive"

// ScopeDriveFile grants access only to files the application created. With this scope, files that
// already exist in Drive cannot be addressed by path.
const ScopeDriveFile = "https://www.googleapis.com/auth/drive.file"

// Default option values.
const (
	// DefaultChunkSize is the resumable upload chunk size (a multiple of 256 KiB).
	DefaultChunkSize = 8 * 1024 * 1024

	// DefaultRetryCount is the number of retries for transient errors.
	DefaultRetryCount = 3

	// DefaultRequestTimeout bounds each HTTP request. It must leave room to send a whole upload chunk.
	DefaultRequestTimeout = 2 * time.Minute
)

// Options holds configuration options for the Google Drive FileSystem.
type Options struct {
	// AccessToken is a pre-obtained OAuth2 access token. It is never refreshed.
	AccessToken string

	// TokenSource supplies OAuth2 tokens and takes precedence over every other credential option.
	TokenSource oauth2.TokenSource

	// ServiceAccountJSON is a service account key file's contents.
	ServiceAccountJSON []byte

	// UseDefaultCredentials uses Google Application Default Credentials.
	UseDefaultCredentials bool

	// DeviceCode enables the OAuth2 device authorization flow with ClientID/ClientSecret.
	DeviceCode         bool
	ClientID           string
	ClientSecret       string
	DeviceCodeCallback func(ctx context.Context, verificationURL, userCode string) error

	// Scope is the OAuth2 scope requested (default ScopeDrive).
	Scope string

	// DriveID is the default shared drive ID used when a location has no authority. Empty means My Drive.
	DriveID string

	// ChunkSize is the resumable upload chunk size (default 8 MiB, rounded down to a multiple of 256 KiB).
	ChunkSize int64

	// TempDir is the directory for temporary files (default os.TempDir()).
	TempDir string

	// RetryCount is the number of retries for transient errors (default 3).
	RetryCount int

	// BaseURL overrides the API host, e.g. "http://localhost:8080" for a mock server.
	BaseURL string

	// HTTPClient is the underlying HTTP client; OAuth2 credentials are layered over its transport.
	HTTPClient *http.Client

	// TrashOnDelete moves files to the trash instead of deleting them permanently.
	TrashOnDelete bool

	// ExportMimeType, when set, is used to export Google-native files on read (e.g. "application/pdf").
	ExportMimeType string

	// RequestTimeout bounds each HTTP request attempt (default 2 minutes; 0 disables it). Downloads
	// apply it to the wait for response headers and then as an idle timeout between body reads, so
	// large downloads only fail when they stall. Uploads must send a whole chunk within it.
	RequestTimeout time.Duration

	// RequestLogger, when set, is called once per HTTP attempt. The record's URL has upload session
	// IDs redacted, and credentials are never included.
	RequestLogger RequestLogger
}

// NewOptions returns Options with defaults applied.
func NewOptions() Options {
	return Options{
		Scope:          ScopeDrive,
		ChunkSize:      DefaultChunkSize,
		RetryCount:     DefaultRetryCount,
		RequestTimeout: DefaultRequestTimeout,
	}
}
