// Package s3 implements vfs.FileSystem for AWS S3.
package s3

import (
	"errors"
	"path"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the file system type.
const Scheme = "s3"
const name = "AWS S3"

var (
	errFileSystemRequired       = errors.New("non-nil s3.FileSystem pointer is required")
	errAuthorityAndNameRequired = errors.New("non-empty strings for authority and name are required")
)

// FileSystem implements vfs.FileSystem for the S3 file system.
type FileSystem struct {
	client  Client
	options Options
}

// NewFileSystem initializer for FileSystem struct accepts aws-sdk client and returns Filesystem or error.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: Options{},
	}

	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry will return the default no-op retrier. The S3 client provides its own retryer interface, and is available
// to override via the s3.FileSystem Options type.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the s3 implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, name string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if authorityStr == "" || name == "" {
		return nil, errAuthorityAndNameRequired
	}

	if err := utils.ValidateAbsoluteFilePath(name); err != nil {
		return nil, err
	}

	// get location path
	absLocPath := utils.EnsureTrailingSlash(path.Dir(name))
	loc, err := fs.NewLocation(authorityStr, absLocPath)
	if err != nil {
		return nil, err
	}

	filename := path.Base(name)
	return loc.NewFile(filename, opts...)
}

// NewLocation function returns the s3 implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, name string) (vfs.Location, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if authorityStr == "" || name == "" {
		return nil, errAuthorityAndNameRequired
	}

	if err := utils.ValidateAbsoluteLocationPath(name); err != nil {
		return nil, err
	}

	auth, err := authority.NewAuthority(utils.RemoveTrailingSlash(authorityStr))
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		prefix:     utils.EnsureTrailingSlash(path.Clean(name)),
		authority:  auth,
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
func (fs *FileSystem) Client() (Client, error) {
	if fs.client == nil {
		var err error
		fs.client, err = GetClient(fs.options)
		if err != nil {
			return nil, err
		}
	}
	return fs.client, nil
}

// WithOptions sets options for client and returns the file system (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithOptions option:
//
//	fs := s3.NewFileSystem(WithOptions(opts))
//
// instead of:
//
//	fs := s3.NewFileSystem().WithOptions(opts)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	// only set options if vfs.Options is s3.Options
	if opts, ok := opts.(Options); ok {
		fs.options = opts
		// we set client to nil to ensure that a new client is created using the new context when Client() is called
		if opts.Region != "" || opts.ForcePathStyle || opts.Endpoint != "" || opts.Retry != nil ||
			opts.AccessKeyID != "" || opts.SecretAccessKey != "" || opts.SessionToken != "" {
			fs.client = nil
		}
	}
	return fs
}

// WithClient passes in an s3 client and returns the file system (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithClient option:
//
//	fs := s3.NewFileSystem(WithClient(client))
//
// instead of:
//
//	fs := s3.NewFileSystem().WithClient(client)
func (fs *FileSystem) WithClient(client any) *FileSystem {
	if c, ok := client.(Client); ok {
		fs.client = c
		fs.options = Options{}
	}
	return fs
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
