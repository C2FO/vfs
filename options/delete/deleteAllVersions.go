package delete

import "github.com/c2fo/vfs/v6/options"

const OptionNameDeleteAllVersions = "deleteAllVersions"

func WithDeleteAllVersions() options.DeleteOption {
	return DeleteAllVersions{}
}

type DeleteAllVersions struct{}

func (w DeleteAllVersions) DeleteOptionName() string {
	return OptionNameDeleteAllVersions
}
