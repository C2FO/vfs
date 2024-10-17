package delete

import "github.com/c2fo/vfs/v6/options"

const optionNameDeleteAllVersions = "deleteAllVersions"

// WithAllVersions returns AllVersions implementation of DeleteOption
func WithAllVersions() options.DeleteOption {
	return AllVersions{}
}

// AllVersions represents the DeleteOption that is used to remove all versions of files when deleted.
// This will remove all versions of files for the filesystems that support file versioning.
type AllVersions struct{}

// DeleteOptionName returns the name of AllVersions option
func (w AllVersions) DeleteOptionName() string {
	return optionNameDeleteAllVersions
}

// WithDeleteAllVersions returns DeleteAllVersions implementation of options.DeleteOption
// Deprecated: use WithAllVersions instead
func WithDeleteAllVersions() options.DeleteOption {
	return DeleteAllVersions{}
}

// DeleteAllVersions represents the DeleteOption that is used to remove all versions of files when deleted.
// This will remove all versions of files for the filesystems that support file versioning.
// Deprecated: use AllVersions instead
type DeleteAllVersions struct{}

// DeleteOptionName returns the name of DeleteAllVersions option
func (w DeleteAllVersions) DeleteOptionName() string {
	return optionNameDeleteAllVersions
}
