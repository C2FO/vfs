package os

import "github.com/c2fo/vfs/v7/options"

// WithTempDir provides an option to set a custom temporary directory path for the OS backend.
type WithTempDir struct {
	TempDir string
}

// Apply applies the temp dir option to the given FileSystem options.
func (o WithTempDir) Apply(fs *FileSystem) {
	fs.tempDir = o.TempDir
}

// NewFileSystemOptionName returns the option name.
func (o WithTempDir) NewFileSystemOptionName() string {
	return "WithTempDir"
}

// Ensure WithTempDir implements the NewFileSystemOption interface.
var _ options.NewFileSystemOption[FileSystem] = WithTempDir{}
