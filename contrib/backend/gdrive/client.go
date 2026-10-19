package gdrive

import (
	"errors"

	"github.com/c2fo/vfs/contrib/backend/gdrive/types"
)

var (
	// ErrNotFound is returned (wrapped) by Client methods when the requested item does not exist.
	ErrNotFound = types.ErrNotFound

	// ErrAmbiguous is returned (wrapped) when a path matches more than one Drive item.
	ErrAmbiguous = types.ErrAmbiguous

	// ErrNotDownloadable is returned (wrapped) when reading an item that has no byte content, such as a
	// Google Doc, without an export mime type configured.
	ErrNotDownloadable = types.ErrNotDownloadable

	// ErrIsFolder is returned (wrapped) when a file operation is pointed at a folder.
	ErrIsFolder = types.ErrIsFolder

	// ErrNotFolder is returned (wrapped) when a path segment that must be a folder is a file.
	ErrNotFolder = types.ErrNotFolder
)

// Client defines the Drive operations used by this backend. See types.Client.
type Client = types.Client

// UploadSession is an active resumable upload. See types.UploadSession.
type UploadSession = types.UploadSession

// Item is the subset of Drive file metadata used by this backend.
type Item = types.Item

const folderMimeType = types.FolderMimeType

// isMissing reports whether err means nothing exists at the path, including when a parent segment
// is a file.
func isMissing(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotFolder)
}
