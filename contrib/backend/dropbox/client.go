package dropbox

import (
	"io"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
)

// Client defines the subset of Dropbox SDK methods used by this backend.
// This interface limits the API surface and enables efficient mocking in tests.
// The files.Client from the Dropbox SDK automatically implements this interface.
type Client interface {
	// GetMetadata returns metadata for a file or folder.
	GetMetadata(arg *files.GetMetadataArg) (files.IsMetadata, error)

	// ListFolder lists the contents of a folder.
	ListFolder(arg *files.ListFolderArg) (*files.ListFolderResult, error)

	// ListFolderContinue continues a paginated list operation.
	ListFolderContinue(arg *files.ListFolderContinueArg) (*files.ListFolderResult, error)

	// Download downloads a file.
	Download(arg *files.DownloadArg) (*files.FileMetadata, io.ReadCloser, error)

	// Upload uploads a file (max 150MB).
	Upload(arg *files.UploadArg, content io.Reader) (*files.FileMetadata, error)

	// UploadSessionStart starts a chunked upload session.
	UploadSessionStart(arg *files.UploadSessionStartArg, content io.Reader) (*files.UploadSessionStartResult, error)

	// UploadSessionAppendV2 appends data to an upload session.
	UploadSessionAppendV2(arg *files.UploadSessionAppendArg, content io.Reader) error

	// UploadSessionFinish completes an upload session.
	UploadSessionFinish(arg *files.UploadSessionFinishArg, content io.Reader) (*files.FileMetadata, error)

	// CopyV2 copies a file or folder.
	CopyV2(arg *files.RelocationArg) (*files.RelocationResult, error)

	// MoveV2 moves a file or folder.
	MoveV2(arg *files.RelocationArg) (*files.RelocationResult, error)

	// DeleteV2 deletes a file or folder.
	DeleteV2(arg *files.DeleteArg) (*files.DeleteResult, error)
}
