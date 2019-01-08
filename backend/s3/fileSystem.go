package s3

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/backend"
	"github.com/c2fo/vfs/utils"
)

// Scheme defines the filesystem type.
const Scheme = "s3"
const name = "AWS S3"

// FileSystem implements vfs.Filesystem for the S3 filesystem.
type FileSystem struct {
	client  s3iface.S3API
	options vfs.Options
}

// NewFile function returns the s3 implementation of vfs.File.
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	return newFile(fs, volume, name)
}

// NewLocation function returns the s3 implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	name = utils.CleanPrefix(name)
	return &Location{
		fileSystem: fs,
		prefix:     name,
		bucket:     volume,
	}, nil
}

// Name returns "AWS S3"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "s3" as the initial part of a file URI ie: s3://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying aws s3 client, creating (lazily) it if necessary
// See Overview for authentication resolution
func (fs *FileSystem) Client() (s3iface.S3API, error) {
	if fs.client == nil {
		if fs.options == nil {
			fs.options = Options{}
		}

		if opts, ok := fs.options.(Options); ok {
			var err error
			fs.client, err = getClient(opts)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unable to create client, vfs.Options must be an s3.Options")
		}
	}
	return fs.client, nil
}

// WithOptions sets options for client and returns the filesystem (chainable)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {

	// only set options if vfs.Options is s3.Options
	if opts, ok := fs.options.(Options); ok {
		fs.options = opts
		//we set client to nil to ensure that a new client is created using the new context when Client() is called
		fs.client = nil
	}
	return fs
}

// WithClient passes in an s3 client and returns the filesystem (chainable)
func (fs *FileSystem) WithClient(client interface{}) *FileSystem {
	switch client.(type) {
	case s3iface.S3API, *s3.S3:
		fs.client = client.(s3iface.S3API)
		fs.options = nil
	}
	return fs
}

// NewFileSystem intializer for fileSystem struct accepts aws-sdk s3iface.S3API client and returns Filesystem or error.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func init() {
	//registers a default Filesystem
	backend.Register(Scheme, NewFileSystem())
}
