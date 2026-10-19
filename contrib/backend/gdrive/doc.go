// Package gdrive implements vfs.FileSystem for Google Drive.
//
// # Usage
//
// Rely on github.com/c2fo/vfs/v7/backend
//
//	import(
//	    "github.com/c2fo/vfs/v7/backend"
//	    "github.com/c2fo/vfs/contrib/backend/gdrive"
//	)
//
//	func UseFs() error {
//	    fs := backend.Backend(gdrive.Scheme)
//	    ...
//	}
//
// Or call directly:
//
//	fs := gdrive.NewFileSystem(gdrive.WithAccessToken("ya29...."))
//	location, err := fs.NewLocation("", "/path/to/folder/")
//
// URIs look like gdrive:///path/to/file.txt for My Drive, and gdrive://<shared drive ID>/path/to/file.txt
// for a shared drive.
//
// # Authentication
//
// The backend calls the Drive v3 REST API directly over net/http and authenticates with OAuth2:
//
//   - WithAccessToken: a pre-obtained token (never refreshed)
//   - WithTokenSource: any oauth2.TokenSource, which handles refresh
//   - WithServiceAccountJSON: a service account key; share the target folder or drive with its email
//   - WithDefaultCredentials: Google Application Default Credentials
//   - WithDeviceCode: the OAuth2 device flow for a user, with an OAuth client of the "TVs and Limited
//     Input devices" type
//
// With no option, VFS_GDRIVE_ACCESS_TOKEN is used if set. The default scope is
// https://www.googleapis.com/auth/drive; with WithScope(gdrive.ScopeDriveFile) Drive only exposes files the
// application created, so existing paths cannot be addressed.
//
// # Semantics
//
// Drive addresses items by ID, allows duplicate names in a folder, and treats folders as files. See
// DESIGN.md for the rules this backend applies. In short: a path that matches more than one item fails
// with ErrAmbiguous, writes replace the existing file in place, file operations refuse folders, deletes are
// permanent unless WithTrashOnDelete is set, and Google-native files (Docs, Sheets) cannot be read unless
// WithExportMimeType is set.
package gdrive
