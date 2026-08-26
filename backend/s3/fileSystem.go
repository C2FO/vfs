// Package s3 implements vfs.FileSystem for AWS S3.
package s3

import (
	"errors"
	"fmt"
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
	errClientNotSupported       = errors.New("client does not implement s3.Client")
)

// FileSystem implements vfs.FileSystem for the S3 file system.
type FileSystem struct {
	client Client
	// clientErr defers reporting a client supplied through the chainable WithClient method, which
	// can't return an error itself, until the client is actually resolved.
	clientErr error
	options   Options
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
	if fs.clientErr != nil {
		return nil, fs.clientErr
	}

	if fs.client == nil {
		client, err := GetClient(fs.options)
		if err != nil {
			return nil, err
		}
		fs.client = client
	}

	return fs.client, nil
}

// backendClient returns the client as the contract the backend uses internally, adapting one that
// predates ListObjectsV2.
func (fs *FileSystem) backendClient() (backendClient, error) {
	client, err := fs.Client()
	if err != nil {
		return nil, err
	}
	return asBackendClient(client), nil
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
		// Options gives Client() an alternate way to resolve a client, so any previously latched
		// error (e.g. from WithClient) no longer applies.
		fs.clientErr = nil
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
// A client that doesn't implement Client is rejected, and the error is returned by the next call
// to Client rather than by this method, which can't return one.
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
	c, ok := client.(Client)
	if !ok {
		fs.clientErr = fmt.Errorf("%w: %T", errClientNotSupported, client)
		return fs
	}

	fs.client = c
	fs.clientErr = nil
	fs.options = Options{}

	return fs
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
