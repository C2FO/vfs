package os

import (
	"github.com/c2fo/vfs/v3"
	"github.com/c2fo/vfs/v3/backend"
	"github.com/c2fo/vfs/v3/backend/all"
	"github.com/c2fo/vfs/v3/utils"
)

//Scheme defines the filesystem type.
const Scheme = "file"
const name = "os"

// FileSystem implements vfs.Filesystem for the OS filesystem.
type FileSystem struct{}

// FileSystem will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return all.DefaultRetrier()
}

// NewFile function returns the os implementation of vfs.File.
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	file, err := newFile(name)
	return file, err
}

// NewLocation function returns the os implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	return &Location{
		fileSystem: fs,
		name:       utils.AddTrailingSlash(name),
	}, nil
}

// Name returns "os"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "file" as the initial part of a file URI ie: file://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

func init() {
	backend.Register(Scheme, &FileSystem{})
}
