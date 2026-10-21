package gdrive

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameAccessToken        = "accessToken"
	optionNameTokenSource        = "tokenSource"
	optionNameServiceAccountJSON = "serviceAccountJSON"
	optionNameDefaultCredentials = "defaultCredentials"
	optionNameDeviceCode         = "deviceCode"
	optionNameScope              = "scope"
	optionNameDriveID            = "driveID"
	optionNameChunkSize          = "chunkSize"
	optionNameTempDir            = "tempDir"
	optionNameRetryCount         = "retryCount"
	optionNameBaseURL            = "baseURL"
	optionNameHTTPClient         = "httpClient"
	optionNameTrashOnDelete      = "trashOnDelete"
	optionNameExportMimeType     = "exportMimeType"
	optionNameRequestTimeout     = "requestTimeout"
	optionNameRequestLogger      = "requestLogger"
	optionNameClient             = "client"
)

// funcOpt adapts a function to options.NewFileSystemOption[FileSystem].
type funcOpt struct {
	name string
	fn   func(*FileSystem)
}

func (o funcOpt) Apply(fs *FileSystem)            { o.fn(fs) }
func (o funcOpt) NewFileSystemOptionName() string { return o.name }

func newOpt(name string, fn func(*FileSystem)) options.NewFileSystemOption[FileSystem] {
	return funcOpt{name: name, fn: fn}
}

// WithAccessToken sets a pre-obtained OAuth2 access token. It is never refreshed, so prefer
// WithTokenSource for anything long-running.
func WithAccessToken(token string) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameAccessToken, func(fs *FileSystem) { fs.options.AccessToken = token })
}

// WithTokenSource sets the OAuth2 token source. It takes precedence over every other credential option.
func WithTokenSource(ts oauth2.TokenSource) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameTokenSource, func(fs *FileSystem) { fs.options.TokenSource = ts })
}

// WithServiceAccountJSON authenticates as a service account using the key file contents. The service
// account must have been granted access to the files or shared drive to use.
func WithServiceAccountJSON(keyJSON []byte) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameServiceAccountJSON, func(fs *FileSystem) { fs.options.ServiceAccountJSON = keyJSON })
}

// WithDefaultCredentials authenticates with Google Application Default Credentials.
func WithDefaultCredentials() options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameDefaultCredentials, func(fs *FileSystem) { fs.options.UseDefaultCredentials = true })
}

// WithDeviceCode authenticates a user with the OAuth2 device authorization flow. The callback
// receives the verification URL and user code to show the user; if nil they are printed to stdout.
// The OAuth client must be of the "TVs and Limited Input devices" type.
func WithDeviceCode(
	clientID, clientSecret string,
	callback func(ctx context.Context, verificationURL, userCode string) error,
) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameDeviceCode, func(fs *FileSystem) {
		fs.options.DeviceCode = true
		fs.options.ClientID = clientID
		fs.options.ClientSecret = clientSecret
		fs.options.DeviceCodeCallback = callback
	})
}

// WithScope sets the OAuth2 scope: ScopeDrive (default) or ScopeDriveFile.
func WithScope(scope string) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameScope, func(fs *FileSystem) { fs.options.Scope = scope })
}

// WithDriveID sets the default shared drive ID used for locations with no authority.
func WithDriveID(driveID string) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameDriveID, func(fs *FileSystem) { fs.options.DriveID = driveID })
}

// WithChunkSize sets the resumable upload chunk size, rounded down to a multiple of 256 KiB.
func WithChunkSize(size int64) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameChunkSize, func(fs *FileSystem) { fs.options.ChunkSize = size })
}

// WithTempDir sets the directory for temporary files used during read/write operations.
func WithTempDir(dir string) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameTempDir, func(fs *FileSystem) { fs.options.TempDir = dir })
}

// WithRetryCount sets the number of retries for transient errors (default 3).
func WithRetryCount(count int) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameRetryCount, func(fs *FileSystem) { fs.options.RetryCount = count })
}

// WithBaseURL overrides the API host (default "https://www.googleapis.com"), e.g. for a mock server.
func WithBaseURL(baseURL string) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameBaseURL, func(fs *FileSystem) { fs.options.BaseURL = baseURL })
}

// WithHTTPClient sets the base *http.Client; OAuth2 credentials are layered over its transport.
func WithHTTPClient(client *http.Client) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameHTTPClient, func(fs *FileSystem) { fs.options.HTTPClient = client })
}

// WithTrashOnDelete moves deleted files to the Drive trash instead of deleting them permanently.
func WithTrashOnDelete() options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameTrashOnDelete, func(fs *FileSystem) { fs.options.TrashOnDelete = true })
}

// WithExportMimeType makes reads of Google-native files (Docs, Sheets, ...) export them as the
// given mime type, e.g. "application/pdf". Without it such reads fail with ErrNotDownloadable.
func WithExportMimeType(mimeType string) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameExportMimeType, func(fs *FileSystem) { fs.options.ExportMimeType = mimeType })
}

// WithRequestTimeout bounds each HTTP request attempt (default 2 minutes; 0 disables it). Downloads
// use it as an idle timeout once the response starts, and an upload chunk must be sent within it, so
// lower it together with WithChunkSize on slow links.
func WithRequestTimeout(timeout time.Duration) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameRequestTimeout, func(fs *FileSystem) { fs.options.RequestTimeout = timeout })
}

// WithRequestLogger calls logger once per HTTP attempt with a RequestLog. The caller decides how
// that maps onto its own logging library.
func WithRequestLogger(logger RequestLogger) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameRequestLogger, func(fs *FileSystem) { fs.options.RequestLogger = logger })
}

// WithClient sets a custom Client used for every drive. Useful for testing.
func WithClient(client Client) options.NewFileSystemOption[FileSystem] {
	return newOpt(optionNameClient, func(fs *FileSystem) { fs.client = client })
}

// compile-time interface check.
var _ options.NewFileSystemOption[FileSystem] = funcOpt{}
