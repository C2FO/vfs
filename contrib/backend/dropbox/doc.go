// Package dropbox implements vfs.FileSystem for Dropbox.
//
// # Usage
//
// Rely on github.com/c2fo/vfs/v7/backend
//
//	import(
//	    "github.com/c2fo/vfs/v7/backend"
//	    "github.com/c2fo/vfs/contrib/backend/dropbox"
//	)
//
//	func UseFs() error {
//	    fs := backend.Backend(dropbox.Scheme)
//	    ...
//	}
//
// Or call directly:
//
//	import "github.com/c2fo/vfs/contrib/backend/dropbox"
//
//	func DoSomething() {
//	    fs := dropbox.NewFileSystem(
//	        dropbox.WithAccessToken("your-oauth-token"),
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
// Dropbox backend requires an OAuth2 access token. You can obtain one by:
//
// 1. Creating a Dropbox App at https://www.dropbox.com/developers/apps
// 2. Generating an access token from the app console (for testing)
// 3. Implementing OAuth2 flow for production use
//
// Set the access token via the WithAccessToken option:
//
//	fs := dropbox.NewFileSystem(
//	    dropbox.WithAccessToken(os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")),
//	)
//
// # Limitations
//
// The Dropbox API has several limitations that affect VFS operations:
//
// 1. No Range Reads: Dropbox does not support HTTP range requests. Files must be downloaded
// entirely to support Seek operations. This is handled transparently via temp file buffering.
//
// 2. No Append Mode: Files must be uploaded in their entirety. Write operations use temp file
// buffering and upload on Close.
//
// 3. Upload Size Limit: Simple uploads are limited to 150MB. Larger files automatically use
// chunked upload sessions (4MB chunks by default).
//
// 4. Case Insensitive Paths: Dropbox paths are case-insensitive but case-preserving.
// /path/File.txt and /path/file.txt refer to the same file.
//
// 5. No Empty Directories: Dropbox doesn't store empty folders as entities. They exist only
// as path prefixes when files are present.
//
// 6. Touch Performance: Updating timestamps on existing files requires download and re-upload,
// which can be expensive for large files.
//
// # URI Format
//
//	dbx:///path/to/file.txt
//	dbx:///path/to/folder/
//
// The authority is always empty as Dropbox uses a single namespace per access token.
package dropbox
