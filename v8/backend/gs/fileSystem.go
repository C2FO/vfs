package gs

import (
	"context"
	"errors"
	"path"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

// Scheme defines the file system type.
const Scheme = "gs"

const fsDisplayName = "Google Cloud Storage"

// FileSystem implements [vfs.FileSystem] for Google Cloud Storage.
type FileSystem struct {
	client        *storage.Client
	ctx           context.Context
	options       Options
	clientCreator ClientCreator
	retryer       Retryer
}

var noOpRetryer Retryer = func(wrapped func() error) error {
	return wrapped()
}

var (
	errFileSystemRequired       = errors.New("non-nil gs.FileSystem pointer is required")
	errAuthorityAndPathRequired = errors.New("non-empty strings for authority and path are required")
)

// NewFileSystem initializer for FileSystem struct accepts google cloud storage client and returns FileSystem or error.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		ctx:           context.Background(),
		clientCreator: &defaultClientCreator{},
		retryer:       noOpRetryer,
	}

	options.ApplyOptions(fs, opts...)

	return fs
}

// NewFile returns the GCS implementation of [vfs.File].
func (fs *FileSystem) NewFile(authorityStr, filePath string, opts ...vfsopt.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if authorityStr == "" || filePath == "" {
		return nil, errAuthorityAndPathRequired
	}

	if err := utils.ValidateAbsoluteFilePath(filePath); err != nil {
		return nil, err
	}

	absLocPath := utils.EnsureTrailingSlash(path.Dir(filePath))
	loc, err := fs.NewLocation(authorityStr, absLocPath)
	if err != nil {
		return nil, err
	}

	filename := path.Base(filePath)
	return loc.NewFile(filename, opts...)
}

// NewLocation returns the GCS implementation of [vfs.Location].
func (fs *FileSystem) NewLocation(authorityStr, locPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if authorityStr == "" || locPath == "" {
		return nil, errAuthorityAndPathRequired
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
		prefix:     utils.EnsureTrailingSlash(path.Clean(locPath)),
		authority:  auth,
	}, nil
}

// Name returns the display name of the file system.
func (fs *FileSystem) Name() string {
	return fsDisplayName
}

// Scheme returns "gs" as the initial part of a file URI (gs://).
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying Google Cloud Storage client, creating it if necessary.
func (fs *FileSystem) Client() (*storage.Client, error) {
	if fs.client == nil {
		gsClientOpts := parseClientOptions(fs.options)
		client, err := fs.clientCreator.NewClient(fs.ctx, gsClientOpts...)
		if err != nil {
			return nil, err
		}
		fs.client = client
	}
	return fs.client, nil
}

// WithOptions sets options for the client and returns the file system (chainable).
//
// Deprecated: Use [NewFileSystem] with [WithOptions] from [newFileSystemOption].
func (fs *FileSystem) WithOptions(opts any) *FileSystem {
	if o, ok := opts.(Options); ok {
		fs.options = o
		fs.client = nil
	}
	return fs
}

// WithClient passes in a storage client and returns the file system (chainable).
//
// Deprecated: Use [NewFileSystem] with [WithClient].
func (fs *FileSystem) WithClient(client any) *FileSystem {
	if c, ok := client.(*storage.Client); ok {
		fs.client = c
	}
	return fs
}

// ClientCreator defines an interface for creating a new Google Cloud Storage client.
type ClientCreator interface {
	NewClient(ctx context.Context, opts ...option.ClientOption) (*storage.Client, error)
}

// defaultClientCreator is the default implementation of ClientCreator.
type defaultClientCreator struct{}

// NewClient creates a new Google Cloud Storage client.
func (d *defaultClientCreator) NewClient(ctx context.Context, opts ...option.ClientOption) (*storage.Client, error) {
	return storage.NewClient(ctx, opts...)
}

// GetClient returns a new Google Cloud Storage client with the provided context and Options.
func GetClient(ctx context.Context, opts Options) (*storage.Client, error) {
	gsClientOpts := parseClientOptions(opts)
	creator := &defaultClientCreator{}
	return creator.NewClient(ctx, gsClientOpts...)
}

var _ vfs.FileSystem = (*FileSystem)(nil)
