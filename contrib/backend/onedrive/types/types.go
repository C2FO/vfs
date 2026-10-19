// Package types defines the OneDrive client interfaces and data types, kept separate so that
// generated mocks can import them without an import cycle.
package types

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned (wrapped) by Client methods when the requested item does not exist.
var ErrNotFound = errors.New("onedrive: item not found")

// DriveItem is the subset of Microsoft Graph driveItem metadata used by this backend.
type DriveItem struct {
	ID           string
	Name         string
	Size         int64
	LastModified time.Time
	IsFolder     bool
}

// Client defines the Microsoft Graph drive operations used by this backend.
// Item paths are drive-relative and slash-separated, with no leading slash; "" is the drive root.
type Client interface {
	// GetItem returns metadata for a file or folder.
	GetItem(ctx context.Context, itemPath string) (*DriveItem, error)

	// ListChildren returns every child of a folder, following pagination.
	ListChildren(ctx context.Context, folderPath string) ([]DriveItem, error)

	// Download returns a reader for a file's content. The caller must close it.
	Download(ctx context.Context, itemPath string) (io.ReadCloser, error)

	// Upload replaces a file's content in a single request. Graph limits this to 4MB.
	// Missing parent folders are created.
	Upload(ctx context.Context, itemPath string, content io.ReadSeeker) (*DriveItem, error)

	// CreateUploadSession starts a resumable upload for files larger than 4MB, replacing any existing file.
	CreateUploadSession(ctx context.Context, itemPath string) (UploadSession, error)

	// Copy copies an item into an existing destination folder, replacing any existing item of the
	// same name, and blocks until Graph reports the copy finished.
	Copy(ctx context.Context, sourcePath, destParentPath, destName string) error

	// Move moves an item into an existing destination folder, replacing any existing item of the same name.
	Move(ctx context.Context, sourcePath, destParentPath, destName string) (*DriveItem, error)

	// Delete deletes a file or folder.
	Delete(ctx context.Context, itemPath string) error
}

// UploadSession is an active resumable upload.
type UploadSession interface {
	// UploadChunk uploads bytes [offset, offset+len(data)) of a file totalSize bytes long.
	// It returns the resulting item once the final chunk is accepted, and nil before that.
	UploadChunk(ctx context.Context, data []byte, offset, totalSize int64) (*DriveItem, error)

	// Cancel abandons the upload session.
	Cancel(ctx context.Context) error
}
