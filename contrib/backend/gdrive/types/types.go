// Package types defines the Google Drive client interfaces and data types, kept separate so that
// generated mocks can import them without an import cycle.
package types

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound is returned (wrapped) by Client methods when the requested item does not exist.
	ErrNotFound = errors.New("gdrive: item not found")

	// ErrAmbiguous is returned (wrapped) when a path matches more than one item, which Drive allows
	// but VFS paths cannot express.
	ErrAmbiguous = errors.New("gdrive: path matches more than one item")

	// ErrNotDownloadable is returned (wrapped) when an item has no byte content (a Google Doc, Sheet,
	// shortcut, ...): when reading it without an export mime type configured, and when writing,
	// copying or moving bytes over it.
	ErrNotDownloadable = errors.New("gdrive: item has no downloadable content")

	// ErrIsFolder is returned (wrapped) when a file operation is pointed at a folder.
	ErrIsFolder = errors.New("gdrive: path is a folder, not a file")

	// ErrNotFolder is returned (wrapped) when a path segment that must be a folder is a file.
	ErrNotFolder = errors.New("gdrive: path segment is a file, not a folder")
)

// FolderMimeType is the mime type Drive uses to mark a folder.
const FolderMimeType = "application/vnd.google-apps.folder"

// NativeMimePrefix prefixes the mime types of Google-native items, which have no byte content.
const NativeMimePrefix = "application/vnd.google-apps."

// Item is the subset of Drive file metadata used by this backend.
type Item struct {
	ID       string
	Name     string
	MimeType string
	Size     int64
	ModTime  time.Time
	IsFolder bool
}

// IsNative reports whether the item is a Google-native item without byte content.
func (i *Item) IsNative() bool {
	return !i.IsFolder && len(i.MimeType) >= len(NativeMimePrefix) && i.MimeType[:len(NativeMimePrefix)] == NativeMimePrefix
}

// Client defines the Drive operations used by this backend for a single drive.
// Item paths are drive-relative and slash-separated, with no leading slash; "" is the drive root.
type Client interface {
	// GetItem returns metadata for a file or folder. It returns ErrNotFound or ErrAmbiguous.
	GetItem(ctx context.Context, itemPath string) (*Item, error)

	// ListChildren returns every child of a folder, following pagination.
	ListChildren(ctx context.Context, folderPath string) ([]Item, error)

	// Download returns a reader for a file's content. The caller must close it.
	Download(ctx context.Context, itemPath string) (io.ReadCloser, error)

	// Upload replaces a file's content, or creates it, in a single multipart request. The content is
	// read from its start regardless of the reader's current position. Missing parent folders are created.
	Upload(ctx context.Context, itemPath string, content io.ReadSeeker) (*Item, error)

	// CreateUploadSession starts a resumable upload of totalSize bytes, replacing or creating the
	// file. Missing parent folders are created.
	CreateUploadSession(ctx context.Context, itemPath string, totalSize int64) (UploadSession, error)

	// Touch sets a file's modification time to now.
	Touch(ctx context.Context, itemPath string) error

	// Copy copies a file into an existing destination folder, replacing any existing file of the
	// same name.
	Copy(ctx context.Context, sourcePath, destParentPath, destName string) error

	// Move moves a file into an existing destination folder, replacing any existing file of the
	// same name.
	Move(ctx context.Context, sourcePath, destParentPath, destName string) (*Item, error)

	// Delete deletes a file (or trashes it, depending on configuration). Folders are refused with
	// ErrIsFolder.
	Delete(ctx context.Context, itemPath string) error
}

// UploadSession is an active resumable upload.
type UploadSession interface {
	// UploadChunk uploads bytes [offset, offset+len(data)) of the file. It returns the resulting item
	// once the final chunk is accepted, and nil before that.
	UploadChunk(ctx context.Context, data []byte, offset int64) (*Item, error)

	// Cancel abandons the upload session.
	Cancel(ctx context.Context) error
}
