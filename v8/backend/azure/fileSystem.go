package azure

import (
	"errors"
	"path"

	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

// Scheme defines the scheme for the azure implementation
const Scheme = "az"

// Name defines the name for the azure implementation
const Name = "azure"

var (
	errFileSystemRequired       = errors.New("azure.FileSystem receiver pointer must be non-nil")
	errContainerAndPathRequired = errors.New("non-empty strings for container and path are required")
)

// FileSystem implements [vfs.FileSystem] for Azure Blob Storage.
type FileSystem struct {
	options *Options
	client  Client
}

// NewFileSystem creates a new default FileSystem. This sets options.AccountName and
// options.AccountKey from the env variables AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_ACCESS_KEY when unset.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: NewOptions(),
	}

	options.ApplyOptions(fs, opts...)

	return fs
}

// WithOptions allows the caller to override the default options (chainable).
//
// Deprecated: Use [NewFileSystem] with [WithOptions] from [newFileSystemOption].
func (fs *FileSystem) WithOptions(opts any) *FileSystem {
	if azureOpts, ok := opts.(Options); ok {
		fs.options = &azureOpts
	}
	return fs
}

// WithClient allows the caller to specify a specific client to be used (chainable).
//
// Deprecated: Use [NewFileSystem] with [WithClient].
func (fs *FileSystem) WithClient(client Client) *FileSystem {
	fs.client = client
	return fs
}

// Client returns a Client to perform operations against Azure Blob Storage.
func (fs *FileSystem) Client() (Client, error) {
	if fs.client == nil {
		client, err := NewClient(fs.options)
		fs.client = client
		return fs.client, err
	}
	return fs.client, nil
}

// NewFile returns the azure implementation of [vfs.File].
func (fs *FileSystem) NewFile(container, absFilePath string, opts ...vfsopt.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if container == "" || absFilePath == "" {
		return nil, errContainerAndPathRequired
	}

	if err := utils.ValidateAbsoluteFilePath(absFilePath); err != nil {
		return nil, err
	}

	absLocPath := utils.EnsureTrailingSlash(path.Dir(absFilePath))
	loc, err := fs.NewLocation(container, absLocPath)
	if err != nil {
		return nil, err
	}
	filename := path.Base(absFilePath)
	return loc.NewFile(filename, opts...)
}

// NewLocation returns the azure implementation of [vfs.Location].
func (fs *FileSystem) NewLocation(container, absLocPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if container == "" || absLocPath == "" {
		return nil, errContainerAndPathRequired
	}

	if err := utils.ValidateAbsoluteLocationPath(absLocPath); err != nil {
		return nil, err
	}

	auth, err := authority.NewAuthority(container)
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		path:       path.Clean(absLocPath),
		authority:  auth,
	}, nil
}

// Name returns "azure"
func (fs *FileSystem) Name() string {
	return Name
}

// Scheme returns "az" as the initial part of the URI.
func (fs *FileSystem) Scheme() string {
	return Scheme
}

var _ vfs.FileSystem = (*FileSystem)(nil)
