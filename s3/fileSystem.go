package s3

import (
	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/utils"
)

//Scheme defines the filesystem type.
const Scheme = "s3"

// fileSystem implements vfs.Filesystem for the S3 filesystem.
type FileSystem struct {
	Client s3iface.S3API
}

// NewFile function returns the s3 implementation of vfs.File.
func (fs FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	file, err := newFile(&fs, volume, name)
	if err != nil {
		return nil, err
	}
	return vfs.File(file), nil
}

// NewLocation function returns the s3 implementation of vfs.Location.
func (fs FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	name = utils.CleanPrefix(name)
	return &Location{
		fileSystem: &fs,
		prefix:     name,
		bucket:     volume,
	}, nil
}

// Name returns "AWS S3"
func (fs FileSystem) Name() string {
	return "AWS S3"
}

// Scheme return "s3" as the initial part of a file URI ie: s3://
func (fs FileSystem) Scheme() string {
	return Scheme
}

// NewFileSystem intializer for fileSystem struct accepts aws-sdk s3iface.S3API client and returns Filesystem or error.
func NewFileSystem(client s3iface.S3API) (*FileSystem, error) {
	fs := &FileSystem{client}
	return fs, nil
}
