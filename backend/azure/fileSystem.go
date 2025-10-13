package azure

import (
	"context"
	"errors"
	"path"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newlocation"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the scheme for the azure implementation
const Scheme = "az"

// Name defines the name for the azure implementation
const Name = "azure"

const errNilFileSystemReceiver = "azure.FileSystem receiver pointer must be non-nil"

// FileSystem implements the vfs.FileSystem interface for Azure Blob Storage
type FileSystem struct {
	options *Options
	client  Client
	ctx     context.Context
}

// NewFileSystem creates a new default FileSystem.  This will set the options options.AccountName and
// options.AccountKey with the env variables AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_ACCESS_KEY respectively.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: NewOptions(),
		ctx:     context.Background(),
	}

	// apply options
	options.ApplyOptions(fs, opts...)

	return fs
}

// WithOptions allows the caller to override the default options
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithOptions option:
//
//	fs := azure.NewFileSystem(azure.WithOptions(opts))
//
// instead of:
//
//	fs := azure.NewFileSystem().WithOptions(opts)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	azureOpts, _ := opts.(Options)
	fs.options = &azureOpts
	return fs
}

// WithClient allows the caller to specify a specific client to be used
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithClient option:
//
//	fs := azure.NewFileSystem(azure.WithClient(client))
//
// instead of:
//
//	azure := s3.NewFileSystem().WithClient(client)
func (fs *FileSystem) WithClient(client Client) *FileSystem {
	fs.client = client
	return fs
}

// Client returns a Client to perform operations against Azure Blob Storage
func (fs *FileSystem) Client() (Client, error) {
	if fs.client == nil {
		client, err := NewClient(fs.options)
		fs.client = client
		return fs.client, err
	}
	return fs.client, nil
}

// NewFile returns the azure implementation of vfs.File
func (fs *FileSystem) NewFile(container, absFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New(errNilFileSystemReceiver)
	}

	if container == "" || absFilePath == "" {
		return nil, errors.New("non-empty strings for container and path are required")
	}

	if err := utils.ValidateAbsoluteFilePath(absFilePath); err != nil {
		return nil, err
	}

	// get location path
	absLocPath := utils.EnsureTrailingSlash(path.Dir(absFilePath))
	loc, err := fs.NewLocation(container, absLocPath)
	if err != nil {
		return nil, err
	}
	filename := path.Base(absFilePath)
	return loc.NewFile(filename, opts...)
}

// NewLocation returns the azure implementation of vfs.Location
func (fs *FileSystem) NewLocation(container, absLocPath string, opts ...options.NewLocationOption) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New(errNilFileSystemReceiver)
	}

	if container == "" || absLocPath == "" {
		return nil, errors.New("non-empty strings for container and path are required")
	}

	if err := utils.ValidateAbsoluteLocationPath(absLocPath); err != nil {
		return nil, err
	}

	auth, err := authority.NewAuthority(container)
	if err != nil {
		return nil, err
	}

	ctx := fs.ctx
	for _, o := range opts {
		switch o := o.(type) {
		case *newlocation.Context:
			ctx = context.Context(o)
		default:
		}
	}

	return &Location{
		fileSystem: fs,
		path:       path.Clean(absLocPath),
		authority:  auth,
		ctx:        ctx,
	}, nil
}

// Name returns "azure"
func (fs *FileSystem) Name() string {
	return Name
}

// Scheme returns "az" as the initial part of the URI i.e. https://..
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Retry returns the default retry function.  This is overridable via the WithOptions function.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	if fs.options.RetryFunc != nil {
		return fs.options.RetryFunc
	}
	return vfs.DefaultRetryer()
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
