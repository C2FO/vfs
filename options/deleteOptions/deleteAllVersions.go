package deleteOptions

import "github.com/c2fo/vfs/v6/options"

const OptionNameDeleteAllVersions = "deleteAllVersions"

func WithDeleteAllVersion() options.DeleteOption {
	return DeleteAllVersions{}
}

type DeleteAllVersions struct{}

func (w DeleteAllVersions) DeleteOptionName() string {
	return OptionNameDeleteAllVersions
}
