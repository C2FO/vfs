package gs

import vfsopt "github.com/c2fo/vfs/v8/options"

// FileContentType sets the Content-Type on GCS object writers and copies when supported.
type FileContentType string

// NewFileOptionName implements [vfsopt.NewFileOption].
func (FileContentType) NewFileOptionName() string {
	return "gs.FileContentType"
}

var _ vfsopt.NewFileOption = FileContentType("")

// DeleteAllVersions requests deletion of all generations/versions of an object (GCS object versioning).
type DeleteAllVersions struct{}

// DeleteOptionName implements [vfsopt.DeleteOption].
func (DeleteAllVersions) DeleteOptionName() string {
	return "gs.DeleteAllVersions"
}

var _ vfsopt.DeleteOption = DeleteAllVersions{}
