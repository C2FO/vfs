package delete

import "github.com/c2fo/vfs/v6/options"

const optionNameDeleteAllVersions = "deleteAllVersions"

// WithDeleteAllVersions returns DeleteAllVersions implementation of delete.DeleteOption
func WithDeleteAllVersions() options.DeleteOption {
	return DeleteAllVersions{}
}

// DeleteAllVersions represents the DeleteOption that is used to remove all versions of files when deleted.
// This will remove all versions of files for the filesystems that support file versioning.
type DeleteAllVersions struct{}

// DeleteOptionName returns the name of DeleteAllVersions option
func (w DeleteAllVersions) DeleteOptionName() string {
	return optionNameDeleteAllVersions
}
