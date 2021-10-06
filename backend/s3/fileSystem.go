package s3

import (
	"errors"
	"fmt"
	"path"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the file system type.
const Scheme = "s3"
const name = "AWS S3"

// FileSystem implements vfs.FileSystem for the S3 file system.
type FileSystem struct {
	client  s3iface.S3API
	options vfs.Options
}

// Retry will return the default no-op retrier. The S3 client provides its own retryer interface, and is available
// to override via the s3.FileSystem Options type.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the s3 implementation of vfs.File.
func (fs *FileSystem) NewFile(volume, name string) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New("non-nil s3.FileSystem pointer is required")
	}
	if volume == "" || name == "" {
		return nil, errors.New("non-empty strings for bucket and key are required")
	}
	if err := utils.ValidateAbsoluteFilePath(name); err != nil {
		return nil, err
	}

	return &File{
		fileSystem: fs,
		bucket:     utils.RemoveTrailingSlash(volume),
		key:        path.Clean(name),
	}, nil
}

// NewLocation function returns the s3 implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume, name string) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New("non-nil s3.FileSystem pointer is required")
	}
	if volume == "" || name == "" {
		return nil, errors.New("non-empty strings for bucket and key are required")
	}
	if err := utils.ValidateAbsoluteLocationPath(name); err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		prefix:     utils.EnsureTrailingSlash(path.Clean(name)),
		bucket:     utils.RemoveTrailingSlash(volume),
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

// Client returns the underlying aws s3 client, creating it, if necessary
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

// WithOptions sets options for client and returns the file system (chainable)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	// only set options if vfs.Options is s3.Options
	if opts, ok := opts.(Options); ok {
		fs.options = opts
		// we set client to nil to ensure that a new client is created using the new context when Client() is called
		fs.client = nil
	}
	return fs
}

// WithClient passes in an s3 client and returns the file system (chainable)
func (fs *FileSystem) WithClient(client interface{}) *FileSystem {
	switch client.(type) {
	case *s3.S3, s3iface.S3API:
		fs.client = client.(s3iface.S3API)
		fs.options = nil
	}
	return fs
}

// NewFileSystem initializer for FileSystem struct accepts aws-sdk s3iface.S3API client and returns Filesystem or error.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
