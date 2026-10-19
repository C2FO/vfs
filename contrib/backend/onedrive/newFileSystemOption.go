package onedrive

import (
	"context"
	"net/http"

	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameAccessToken        = "accessToken"
	optionNameClientCredentials  = "clientCredentials"
	optionNameDefaultCredential  = "defaultCredential"
	optionNameDeviceCode         = "deviceCode"
	optionNameInteractiveBrowser = "interactiveBrowser"
	optionNameUserDrive          = "userDrive"
	optionNameSiteDrive          = "siteDrive"
	optionNameDriveID            = "driveID"
	optionNameChunkSize          = "chunkSize"
	optionNameTempDir            = "tempDir"
	optionNameRetryCount         = "retryCount"
	optionNameClient             = "client"
	optionNameBaseURL            = "baseURL"
	optionNameHTTPClient         = "httpClient"
)

// WithAccessToken sets the OAuth2 access token for OneDrive API authentication.
func WithAccessToken(token string) options.NewFileSystemOption[FileSystem] {
	return &accessTokenOpt{token: token}
}

type accessTokenOpt struct {
	token string
}

func (o *accessTokenOpt) Apply(fs *FileSystem) {
	fs.options.AccessToken = o.token
}

func (o *accessTokenOpt) NewFileSystemOptionName() string {
	return optionNameAccessToken
}

// WithClientCredentials sets the Azure AD client credentials for service principal authentication.
func WithClientCredentials(tenantID, clientID, clientSecret string) options.NewFileSystemOption[FileSystem] {
	return &clientCredentialsOpt{
		tenantID:     tenantID,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

type clientCredentialsOpt struct {
	tenantID     string
	clientID     string
	clientSecret string
}

func (o *clientCredentialsOpt) Apply(fs *FileSystem) {
	fs.options.TenantID = o.tenantID
	fs.options.ClientID = o.clientID
	fs.options.ClientSecret = o.clientSecret
}

func (o *clientCredentialsOpt) NewFileSystemOptionName() string {
	return optionNameClientCredentials
}

// WithDefaultCredential uses Azure Default Credential chain for authentication.
// This uses environment variables, managed identity, or other default credentials.
func WithDefaultCredential() options.NewFileSystemOption[FileSystem] {
	return &defaultCredentialOpt{}
}

type defaultCredentialOpt struct{}

func (o *defaultCredentialOpt) Apply(fs *FileSystem) {
	fs.options.UseDefaultCredential = true
}

func (o *defaultCredentialOpt) NewFileSystemOptionName() string {
	return optionNameDefaultCredential
}

// WithDeviceCode enables device code authentication flow.
// This works with both personal Microsoft accounts (consumer OneDrive) and work/school accounts.
// The user will be prompted to visit a URL and enter a code to authenticate.
//
// Parameters:
//   - clientID: The Azure AD application (client) ID. Required.
//   - tenantID: The tenant ID. Use "consumers" for personal accounts, "organizations" for
//     work/school only, or "common" for both. Defaults to "common" if empty.
//   - callback: Optional callback function that receives the device code message to display
//     to the user. If nil, the message is printed to stdout.
//
// Example for personal OneDrive:
//
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithDeviceCode("your-client-id", "consumers", nil),
//	)
func WithDeviceCode(clientID, tenantID string, callback func(context.Context, string) error) options.NewFileSystemOption[FileSystem] {
	return &deviceCodeOpt{clientID: clientID, tenantID: tenantID, callback: callback}
}

type deviceCodeOpt struct {
	clientID string
	tenantID string
	callback func(context.Context, string) error
}

func (o *deviceCodeOpt) Apply(fs *FileSystem) {
	fs.options.ClientID = o.clientID
	if o.tenantID != "" {
		fs.options.TenantID = o.tenantID
	} else {
		fs.options.TenantID = "common"
	}
	fs.options.UseDeviceCode = true
	fs.options.DeviceCodeCallback = o.callback
}

func (o *deviceCodeOpt) NewFileSystemOptionName() string {
	return optionNameDeviceCode
}

// WithInteractiveBrowser enables interactive browser authentication flow.
// This opens a browser window for the user to sign in.
// Works with both personal Microsoft accounts (consumer OneDrive) and work/school accounts.
//
// Parameters:
//   - clientID: The Azure AD application (client) ID. Required.
//   - tenantID: The tenant ID. Use "consumers" for personal accounts, "organizations" for
//     work/school only, or "common" for both. Defaults to "common" if empty.
//   - redirectURL: The redirect URL configured in Azure AD. Defaults to "http://localhost" if empty.
//
// Example for personal OneDrive:
//
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithInteractiveBrowser("your-client-id", "consumers", ""),
//	)
func WithInteractiveBrowser(clientID, tenantID, redirectURL string) options.NewFileSystemOption[FileSystem] {
	return &interactiveBrowserOpt{clientID: clientID, tenantID: tenantID, redirectURL: redirectURL}
}

type interactiveBrowserOpt struct {
	clientID    string
	tenantID    string
	redirectURL string
}

func (o *interactiveBrowserOpt) Apply(fs *FileSystem) {
	fs.options.ClientID = o.clientID
	if o.tenantID != "" {
		fs.options.TenantID = o.tenantID
	} else {
		fs.options.TenantID = "common"
	}
	fs.options.UseInteractiveBrowser = true
	if o.redirectURL != "" {
		fs.options.RedirectURL = o.redirectURL
	} else {
		fs.options.RedirectURL = "http://localhost"
	}
}

func (o *interactiveBrowserOpt) NewFileSystemOptionName() string {
	return optionNameInteractiveBrowser
}

// WithUserDrive configures the backend to access a specific user's OneDrive.
// This is required when using application permissions.
func WithUserDrive(userIDOrUPN string) options.NewFileSystemOption[FileSystem] {
	return &userDriveOpt{userID: userIDOrUPN}
}

type userDriveOpt struct {
	userID string
}

func (o *userDriveOpt) Apply(fs *FileSystem) {
	fs.options.UserID = o.userID
}

func (o *userDriveOpt) NewFileSystemOptionName() string {
	return optionNameUserDrive
}

// WithSiteDrive configures the backend to access a SharePoint site's drive.
func WithSiteDrive(siteID, driveID string) options.NewFileSystemOption[FileSystem] {
	return &siteDriveOpt{siteID: siteID, driveID: driveID}
}

type siteDriveOpt struct {
	siteID  string
	driveID string
}

func (o *siteDriveOpt) Apply(fs *FileSystem) {
	fs.options.SiteID = o.siteID
	fs.options.DriveID = o.driveID
}

func (o *siteDriveOpt) NewFileSystemOptionName() string {
	return optionNameSiteDrive
}

// WithDriveID sets a specific drive ID to access.
func WithDriveID(driveID string) options.NewFileSystemOption[FileSystem] {
	return &driveIDOpt{driveID: driveID}
}

type driveIDOpt struct {
	driveID string
}

func (o *driveIDOpt) Apply(fs *FileSystem) {
	fs.options.DriveID = o.driveID
}

func (o *driveIDOpt) NewFileSystemOptionName() string {
	return optionNameDriveID
}

// WithChunkSize sets the chunk size for uploading large files.
// Files larger than 4MB are uploaded using upload sessions with this chunk size.
// Default is 4MB.
func WithChunkSize(size int64) options.NewFileSystemOption[FileSystem] {
	return &chunkSizeOpt{size: size}
}

type chunkSizeOpt struct {
	size int64
}

func (o *chunkSizeOpt) Apply(fs *FileSystem) {
	fs.options.ChunkSize = o.size
}

func (o *chunkSizeOpt) NewFileSystemOptionName() string {
	return optionNameChunkSize
}

// WithTempDir sets the directory for temporary files used during read/write operations.
// Defaults to os.TempDir() if not specified.
func WithTempDir(dir string) options.NewFileSystemOption[FileSystem] {
	return &tempDirOpt{dir: dir}
}

type tempDirOpt struct {
	dir string
}

func (o *tempDirOpt) Apply(fs *FileSystem) {
	fs.options.TempDir = o.dir
}

func (o *tempDirOpt) NewFileSystemOptionName() string {
	return optionNameTempDir
}

// WithRetryCount sets the number of retry attempts for transient errors.
// Default is 3.
func WithRetryCount(count int) options.NewFileSystemOption[FileSystem] {
	return &retryCountOpt{count: count}
}

type retryCountOpt struct {
	count int
}

func (o *retryCountOpt) Apply(fs *FileSystem) {
	fs.options.RetryCount = o.count
}

func (o *retryCountOpt) NewFileSystemOptionName() string {
	return optionNameRetryCount
}

// WithBaseURL overrides the Microsoft Graph endpoint, e.g. for a national cloud
// ("https://graph.microsoft.us/v1.0") or a mock server in tests.
func WithBaseURL(baseURL string) options.NewFileSystemOption[FileSystem] {
	return &baseURLOpt{baseURL: baseURL}
}

type baseURLOpt struct {
	baseURL string
}

func (o *baseURLOpt) Apply(fs *FileSystem) {
	fs.options.BaseURL = o.baseURL
}

func (o *baseURLOpt) NewFileSystemOptionName() string {
	return optionNameBaseURL
}

// WithHTTPClient sets the *http.Client used for all Graph requests.
func WithHTTPClient(client *http.Client) options.NewFileSystemOption[FileSystem] {
	return &httpClientOpt{client: client}
}

type httpClientOpt struct {
	client *http.Client
}

func (o *httpClientOpt) Apply(fs *FileSystem) {
	fs.options.HTTPClient = o.client
}

func (o *httpClientOpt) NewFileSystemOptionName() string {
	return optionNameHTTPClient
}

// WithClient sets a custom OneDrive client. Useful for testing or when you need
// to provide a pre-configured client.
func WithClient(client Client) options.NewFileSystemOption[FileSystem] {
	return &clientOpt{client: client}
}

type clientOpt struct {
	client Client
}

func (o *clientOpt) Apply(fs *FileSystem) {
	fs.client = o.client
}

func (o *clientOpt) NewFileSystemOptionName() string {
	return optionNameClient
}
