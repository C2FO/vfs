package gs

import (
	"cloud.google.com/go/storage"
	"golang.org/x/net/context"

	"github.com/c2fo/vfs"
)

//Scheme defines the filesystem type.
const Scheme = "gs"

// FileSystem implements vfs.Filesystem for the GCS filesystem.
type FileSystem struct {
	client *storage.Client
	ctx    context.Context
}

// NewFile function returns the gcs implementation of vfs.File.
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	file, err := newFile(fs, volume, name)
	return vfs.File(file), err
}

// NewLocation function returns the s3 implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume string, path string) (loc vfs.Location, err error) {
	loc = &Location{
		fileSystem: fs,
		bucket:     volume,
		prefix:     vfs.EnsureTrailingSlash(path),
	}
	return
}

// Name returns "Google Cloud Storage"
func (fs *FileSystem) Name() string {
	return "Google Cloud Storage"
}

// Scheme return "gs" as the initial part of a file URI ie: gs://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// NewFileSystem intializer for FileSystem struct accepts google cloud storage client and returns Filesystem or error.
func NewFileSystem(ctx context.Context, client *storage.Client) *FileSystem {
	return &FileSystem{
		client: client,
		ctx:    ctx,
	}
}
