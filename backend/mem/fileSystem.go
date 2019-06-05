package mem

import (
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	"github.com/c2fo/vfs/v4/utils"
)

//Scheme defines the filesystem type.
const Scheme = "file"
const name = "mem"

// FileSystem implements vfs.Filesystem for the mem filesystem.
type FileSystem struct{}

// FileSystem will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the mem implementation of vfs.File.  NOT DONE
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	file, _ := newFile(name)
	return file, nil
}

// NewLocation function returns the mem implementation of vfs.Location. NOT DONE
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	return &Location{
		fileSystem: fs,
		name:       utils.AddTrailingSlash(name),
	}, nil
}



// Name returns "mem"
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
