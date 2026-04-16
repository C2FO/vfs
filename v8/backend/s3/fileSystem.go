// Package s3 implements [github.com/c2fo/vfs/v8.FileSystem] for AWS S3.
package s3

import (
	"errors"
	"path"

	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

// Scheme defines the file system type.
const Scheme = "s3"

const fsDisplayName = "AWS S3"

var (
	errFileSystemRequired       = errors.New("non-nil s3.FileSystem pointer is required")
	errAuthorityAndNameRequired = errors.New("non-empty strings for authority and name are required")
)

// FileSystem implements [vfs.FileSystem] for the S3 file system.
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

// NewFile returns the s3 implementation of [vfs.File].
func (fs *FileSystem) NewFile(authorityStr, name string, opts ...vfsopt.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if authorityStr == "" || name == "" {
		return nil, errAuthorityAndNameRequired
	}

	if err := utils.ValidateAbsoluteFilePath(name); err != nil {
		return nil, err
	}

	absLocPath := utils.EnsureTrailingSlash(path.Dir(name))
	loc, err := fs.NewLocation(authorityStr, absLocPath)
	if err != nil {
		return nil, err
	}

	filename := path.Base(name)
	return loc.NewFile(filename, opts...)
}

// NewLocation returns the s3 implementation of [vfs.Location].
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

// Name returns the display name of the file system.
func (fs *FileSystem) Name() string {
	return fsDisplayName
}

// Scheme returns "s3" as the initial part of a file URI ie: s3://
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
// Deprecated: Use [NewFileSystem] with [WithOptions] from [newFileSystemOption].
func (fs *FileSystem) WithOptions(opts any) *FileSystem {
	if opts, ok := opts.(Options); ok {
		fs.options = opts
		if opts.Region != "" || opts.ForcePathStyle || opts.Endpoint != "" || opts.Retry != nil ||
			opts.AccessKeyID != "" || opts.SecretAccessKey != "" || opts.SessionToken != "" {
			fs.client = nil
		}
	}
	return fs
}

// WithClient passes in an s3 client and returns the file system (chainable)
//
// Deprecated: Use [NewFileSystem] with [WithClient].
func (fs *FileSystem) WithClient(client any) *FileSystem {
	if c, ok := client.(Client); ok {
		fs.client = c
		fs.options = Options{}
	}
	return fs
}

var _ vfs.FileSystem = (*FileSystem)(nil)
