package os

import (
	"path"

	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	"github.com/c2fo/vfs/v4/utils"
)

//Scheme defines the filesystem type.
const Scheme = "file"
const name = "os"

// FileSystem implements vfs.Filesystem for the OS filesystem.
type FileSystem struct{}

// Retry will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the os implementation of vfs.File.
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	err := utils.ValidateAbsFilePath(name)
	if err != nil {
		return nil, err
	}
	fileName := path.Base(name)
	location := Location{fileSystem: &FileSystem{}, name: utils.EnsureTrailingSlash(path.Dir(name))}
	return &File{name: fileName, location: &location}, nil
}

// NewLocation function returns the os implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	err := utils.ValidateAbsLocationPath(name)
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		name:       utils.EnsureLeadingSlash(path.Clean(name)),
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
