package s3

import vfsopt "github.com/c2fo/vfs/v8/options"

// FileContentType sets the Content-Type header on S3 PutObject and CopyObject when supported.
type FileContentType string

// NewFileOptionName implements [vfsopt.NewFileOption].
func (FileContentType) NewFileOptionName() string {
	return "s3.FileContentType"
}

var _ vfsopt.NewFileOption = FileContentType("")

// DeleteAllVersions requests deletion of all versions of an object (S3 versioning).
type DeleteAllVersions struct{}

// DeleteOptionName implements [vfsopt.DeleteOption].
func (DeleteAllVersions) DeleteOptionName() string {
	return "s3.DeleteAllVersions"
}

var _ vfsopt.DeleteOption = DeleteAllVersions{}
