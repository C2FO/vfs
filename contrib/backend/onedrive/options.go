package onedrive

import (
	"context"
	"net/http"
	"os"
)

// Options holds configuration options for the OneDrive FileSystem.
type Options struct {
	// TenantID is the Azure AD tenant ID for authentication.
	// For personal Microsoft accounts, use "consumers" or "common".
	// For work/school accounts, use your organization's tenant ID.
	TenantID string

	// ClientID is the Azure AD application (client) ID.
	// Required for device code and interactive browser flows.
	ClientID string

	// ClientSecret is the Azure AD application client secret.
	// Only needed for client credentials flow (work/school accounts only).
	ClientSecret string

	// AccessToken is a pre-obtained OAuth2 access token (alternative to client credentials).
	AccessToken string

	// UseDefaultCredential uses Azure Default Credential chain (env vars, managed identity, etc).
	UseDefaultCredential bool

	// UseDeviceCode enables device code flow for personal/work accounts.
	// User will be prompted to visit a URL and enter a code to authenticate.
	// Requires ClientID to be set. TenantID defaults to "common" if not set.
	UseDeviceCode bool

	// DeviceCodeCallback is called when a device code is received.
	// If nil, the default behavior prints the message to stdout.
	DeviceCodeCallback func(context.Context, string) error

	// UseInteractiveBrowser enables interactive browser authentication.
	// Opens a browser window for user to sign in.
	// Requires ClientID to be set. TenantID defaults to "common" if not set.
	UseInteractiveBrowser bool

	// RedirectURL is the redirect URL for interactive browser flow.
	// Defaults to "http://localhost" if not specified.
	RedirectURL string

	// DriveID is the specific drive ID to access (optional).
	// If not set, uses the authenticated user's default drive.
	DriveID string

	// UserID is the user ID or UPN for accessing a specific user's drive (for app permissions).
	UserID string

	// SiteID is the SharePoint site ID for accessing site drives.
	SiteID string

	// ChunkSize is the upload-session chunk size for files larger than 4MB (default: 5MiB).
	// Graph requires a multiple of 320KiB, so it is rounded down to one (minimum 320KiB), and
	// rejects chunks above 60MiB, which fail the upload locally.
	ChunkSize int64

	// TempDir is the directory for temporary files used during read/write operations.
	// Defaults to os.TempDir() if not specified.
	TempDir string

	// RetryCount is the number of times a throttled (429) or transient 5xx request is retried (default: 3).
	RetryCount int

	// BaseURL is the Microsoft Graph endpoint (default: DefaultBaseURL). Override it to point the
	// backend at a mock server or proxy; token acquisition still uses the public cloud.
	BaseURL string

	// HTTPClient is used for all Graph requests (default: http.DefaultClient).
	HTTPClient *http.Client
}

// NewOptions creates Options with default values.
func NewOptions() Options {
	return Options{
		ChunkSize:  5 * 1024 * 1024,
		TempDir:    os.TempDir(),
		RetryCount: 3,
		BaseURL:    DefaultBaseURL,
	}
}
