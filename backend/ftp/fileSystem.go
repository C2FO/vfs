package ftp

import (
	"context"
	"errors"
	"path"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the filesystem type.
const Scheme = "ftp"
const name = "File Transfer Protocol"

var dataConnGetterFunc func(context.Context, authority.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error)
var defaultClientGetter func(context.Context, authority.Authority, Options) (client types.Client, err error)

// FileSystem implements vfs.FileSystem for the FTP filesystem.
type FileSystem struct {
	options   Options
	ftpclient types.Client
	dataconn  types.DataConn
	resetConn bool
}

// NewFileSystem initializer for fileSystem struct.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: Options{},
	}

	// apply options
	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry will return the default no-op retrier. The FTP client provides its own retryer interface, and is available
// to override via the ftp.FileSystem Options type.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the FTP implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, filePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New("non-nil ftp.FileSystem pointer is required")
	}

	if authorityStr == "" || filePath == "" {
		return nil, errors.New("non-empty string for authority and path is required")
	}

	if err := utils.ValidateAbsoluteFilePath(filePath); err != nil {
		return nil, err
	}

	// get location path
	absLocPath := utils.EnsureTrailingSlash(path.Dir(filePath))
	loc, err := fs.NewLocation(authorityStr, absLocPath)
	if err != nil {
		return nil, err
	}
	filename := path.Base(filePath)
	return loc.NewFile(filename, opts...)
}

// NewLocation function returns the FTP implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, locPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New("non-nil ftp.FileSystem pointer is required")
	}

	if authorityStr == "" || locPath == "" {
		return nil, errors.New("non-empty string for authority and path is required")
	}

	if err := utils.ValidateAbsoluteLocationPath(locPath); err != nil {
		return nil, err
	}

	auth, err := authority.NewAuthority(authorityStr)
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		path:       utils.EnsureTrailingSlash(path.Clean(locPath)),
		authority:  auth,
	}, nil
}

// Name returns "Secure File Transfer Protocol"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "ftp" as the initial part of a file URI ie: ftp://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// DataConn returns the underlying ftp data connection, creating it, if necessary
// See Overview for authentication resolution
func (fs *FileSystem) DataConn(ctx context.Context, authority authority.Authority, t types.OpenType, f *File) (types.DataConn, error) {
	if t != types.SingleOp && f == nil {
		return nil, errors.New("can not create DataConn for read or write for a nil file")
	}
	return dataConnGetterFunc(ctx, authority, fs, f, t)
}

// Client returns the underlying ftp client, creating it, if necessary
// See Overview for authentication resolution
func (fs *FileSystem) Client(ctx context.Context, authority authority.Authority) (types.Client, error) {
	if fs.ftpclient == nil {
		var err error
		fs.ftpclient, err = defaultClientGetter(ctx, authority, fs.options)
		if err != nil {
			return nil, err
		}
	}

	return fs.ftpclient, nil
}

// WithOptions sets options for client and returns the filesystem (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithOptions option:
//
//	fs := ftp.NewFileSystem(ftp.WithOptions(opts))
//
// instead of:
//
//	fs := ftp.NewFileSystem().WithOptions(opts)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	// only set options if vfs.Options is ftp.Options
	if opts, ok := opts.(Options); ok {
		fs.options = opts
		// we set client to nil to ensure that a new client is created using the new context when Client() is called
		fs.ftpclient = nil
	}
	return fs
}

// WithClient passes in an ftp client and returns the filesystem (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithClient option:
//
//	fs := ftp.NewFileSystem(ftp.WithClient(client))
//
// instead of:
//
//	fs := ftp.NewFileSystem().WithClient(client)
func (fs *FileSystem) WithClient(client types.Client) *FileSystem {
	fs.ftpclient = client
	fs.options = Options{}

	return fs
}

func init() {
	defaultClientGetter = getClient
	dataConnGetterFunc = getDataConn
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
