package azure

import vfsopt "github.com/c2fo/vfs/v8/options"

// FileContentType sets the blob content type on Azure uploads when supported.
type FileContentType string

// NewFileOptionName implements [vfsopt.NewFileOption].
func (FileContentType) NewFileOptionName() string {
	return "azure.FileContentType"
}

var _ vfsopt.NewFileOption = FileContentType("")

// DeleteAllVersions requests deletion of all blob versions (Azure blob versioning / soft delete behavior).
type DeleteAllVersions struct{}

// DeleteOptionName implements [vfsopt.DeleteOption].
func (DeleteAllVersions) DeleteOptionName() string {
	return "azure.DeleteAllVersions"
}

var _ vfsopt.DeleteOption = DeleteAllVersions{}
