// Package types provides types and interfaces for Azure operations.
package types

import (
	"io"

	"github.com/c2fo/vfs/v7"
)

// The Client interface contains methods that perform specific operations to Azure Blob Storage.  This interface is
// here so we can write mocks over the actual functionality.
type Client interface {
	// Properties should return a BlobProperties struct for the blob specified by locationURI, and filePath.  If the
	// blob is not found an error should be returned.
	Properties(locationURI, filePath string) (*BlobProperties, error)

	// SetMetadata should add the metadata specified by the parameter metadata for the blob specified by the parameter
	// file.
	SetMetadata(file vfs.File, metadata map[string]*string) error

	// Upload should create or update the blob specified by the file parameter with the contents of the content
	// parameter
	Upload(file vfs.File, content io.ReadSeeker, contentType string) error

	// Download should return a reader for the blob specified by the file parameter
	Download(file vfs.File) (io.ReadCloser, error)

	// Copy should copy the file specified by srcFile to the file specified by tgtFile
	Copy(srcFile vfs.File, tgtFile vfs.File) error

	// List should return a listing for the specified location. Listings should include the full path for the file.
	List(l vfs.Location) ([]string, error)

	// Delete should delete the file specified by the parameter file.
	Delete(file vfs.File) error

	// DeleteAllVersions should delete all versions of the file specified by the parameter file.
	DeleteAllVersions(file vfs.File) error
}
