// Package onedrive implements vfs.FileSystem for Microsoft OneDrive.
//
// # Usage
//
// Rely on github.com/c2fo/vfs/v7/backend
//
//	import(
//	    "github.com/c2fo/vfs/v7/backend"
//	    "github.com/c2fo/vfs/contrib/backend/onedrive"
//	)
//
//	func UseFs() error {
//	    fs := backend.Backend(onedrive.Scheme)
//	    ...
//	}
//
// Or call directly:
//
//	import "github.com/c2fo/vfs/contrib/backend/onedrive"
//
//	func DoSomething() {
//	    fs := onedrive.NewFileSystem(
//	        onedrive.WithClientCredentials("tenant-id", "client-id", "client-secret"),
//	    )
//	    location, err := fs.NewLocation("", "/path/to/folder/")
//	    if err != nil {
//	        return err
//	    }
//	    ...
//	}
//
// # Authentication
//
// The backend calls the Microsoft Graph REST API directly over net/http. WithBaseURL and
// WithHTTPClient override the endpoint and transport (for national clouds, proxies, or tests).
//
// OneDrive backend supports multiple authentication methods via Azure Identity:
//
// 1. Client Credentials (Service Principal) - for server-to-server scenarios:
//
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithClientCredentials("tenant-id", "client-id", "client-secret"),
//	)
//
// 2. Default Azure Credential - uses environment variables or managed identity:
//
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithDefaultCredential(),
//	)
//
// 3. Access Token - if you already have an OAuth2 token:
//
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithAccessToken("your-oauth-token"),
//	)
//
// # Drive Selection
//
// By default, the backend accesses the authenticated user's OneDrive.
// For application permissions, you must specify a drive:
//
//	// Access a specific user's drive
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithClientCredentials(...),
//	    onedrive.WithUserDrive("user@example.com"),
//	)
//
//	// Access a SharePoint site's drive
//	fs := onedrive.NewFileSystem(
//	    onedrive.WithClientCredentials(...),
//	    onedrive.WithSiteDrive("site-id", "drive-id"),
//	)
//
// # Limitations
//
// 1. No Range Writes: OneDrive upload sessions support range writes, but for simplicity
// this implementation uses temp file buffering like Dropbox.
//
// 2. Upload Size Limit: Simple uploads are limited to 4 MiB. Larger files automatically
// use upload sessions (5 MiB chunks by default).
//
// 3. Rate Limiting: Microsoft Graph API has rate limits. The backend retries 429
// (Too Many Requests) and 5xx responses, honoring Retry-After.
//
// 4. Path Length: OneDrive has a 400-character path length limit.
//
// 5. Touch Performance: Updating timestamps on existing files may require download
// and re-upload for some scenarios.
//
// # URI Format
//
//	onedrive:///path/to/file.txt
//	onedrive:///path/to/folder/
//
// The authority is always empty as OneDrive uses a single namespace per drive.
package onedrive
