package os

import "github.com/c2fo/vfs/v7/options"

// WithTempDir specifies a custom directory for intermediate temporary files used during write operations.
// This is useful when the system temp directory is on a different device or volume than the target file,
// which would otherwise cause cross-device rename errors.
type WithTempDir struct {
	TempDir string
}

// Apply applies the temp dir option to the given FileSystem.
func (o WithTempDir) Apply(fs *FileSystem) {
	fs.tempDir = o.TempDir
}

// NewFileSystemOptionName returns the option name.
func (o WithTempDir) NewFileSystemOptionName() string {
	return "WithTempDir"
}

var _ options.NewFileSystemOption[FileSystem] = WithTempDir{}
