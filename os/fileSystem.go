package os

import (
	"github.com/c2fo/vfs"
)

//Scheme defines the filesystem type.
const (
	Scheme = "file"
)

// FileSystem implements vfs.Filesystem for the OS filesystem.
type FileSystem struct{}

// NewFile function returns the os implementation of vfs.File.
func (fs FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	file, err := newFile(name)
	return vfs.File(file), err
}

// NewLocation function returns the os implementation of vfs.Location.
func (fs FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	return &Location{
		fileSystem: vfs.FileSystem(fs),
		name:       vfs.AddTrailingSlash(name),
	}, nil
}

// Name returns "os"
func (fs FileSystem) Name() string {
	return "os"
}

// Scheme return "file" as the initial part of a file URI ie: file://
func (fs FileSystem) Scheme() string {
	return Scheme
}
