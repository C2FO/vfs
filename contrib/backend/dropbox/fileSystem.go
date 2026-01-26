package dropbox

import (
	"errors"
	"os"
	"path"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the file system type.
const Scheme = "dbx"

const name = "Dropbox"

var (
	errFileSystemRequired       = errors.New("non-nil dropbox.FileSystem pointer is required")
	errAuthorityAndNameRequired = errors.New("non-empty string for name is required")
	errAccessTokenRequired      = errors.New("access token is required for Dropbox authentication")
)

// FileSystem implements vfs.FileSystem for Dropbox.
type FileSystem struct {
	client  Client
	options Options
}

// NewFileSystem initializer for FileSystem struct.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: NewOptions(),
	}

	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry returns the default no-op retrier.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the Dropbox implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, name string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if name == "" {
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

// NewLocation function returns the Dropbox implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, name string) (vfs.Location, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if name == "" {
		return nil, errAuthorityAndNameRequired
	}

	if err := utils.ValidateAbsoluteLocationPath(name); err != nil {
		return nil, err
	}

	// Dropbox doesn't use authority (single namespace per token)
	// But we still create an authority object for consistency
	auth, err := authority.NewAuthority(utils.RemoveTrailingSlash(authorityStr))
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		path:       utils.EnsureTrailingSlash(path.Clean(name)),
		authority:  auth,
	}, nil
}

// Name returns "Dropbox"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme returns "dbx" as the initial part of a file URI ie: dbx://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying Dropbox client, creating it if necessary.
func (fs *FileSystem) Client() (Client, error) {
	if fs.client == nil {
		token := fs.options.AccessToken

		// If no token in options, try environment variable
		if token == "" {
			token = os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")
		}

		if token == "" {
			return nil, errAccessTokenRequired
		}

		// Create Dropbox config with access token
		config := dropbox.Config{
			Token:    token,
			LogLevel: dropbox.LogOff, // Can be made configurable if needed
		}

		// Create files client - files.Client implements our Client interface
		fs.client = files.New(config)
	}

	return fs.client, nil
}

func init() {
	// Register a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
