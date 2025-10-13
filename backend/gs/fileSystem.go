package gs

import (
	"errors"
	"path"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the file system type.
const Scheme = "gs"
const name = "Google Cloud Storage"

// FileSystem implements vfs.FileSystem for the GCS file system.
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

	// apply options
	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry will return a retryer provided via options, or a no-op if none is provided.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.Retry(fs.retryer)
}

// NewFile function returns the gcs implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, filePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if authorityStr == "" || filePath == "" {
		return nil, errAuthorityAndPathRequired
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

// NewLocation function returns the GCS implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, locPath string) (loc vfs.Location, err error) {
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

// Name returns "Google Cloud Storage"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "gs" as the initial part of a file URI ie: gs://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying google storage client, creating it, if necessary
// See Overview for authentication resolution
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

// WithOptions sets options for client and returns the file system (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithClient option:
//
//	fs := gs.NewFileSystem(gs.WithClient(client))
//
// instead of:
//
//	fs := gs.NewFileSystem().WithClient(client)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	fs.options = opts.(Options)
	// we set client to nil to ensure that a new client is created using the new context when Client() is called
	fs.client = nil
	return fs
}

// WithContext passes in user context and returns the file system (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithContext option:
//
//	fs := gs.NewFileSystem(WithContext(ctx))
//
// instead of:
//
//	fs := s3.NewFileSystem().WithContext(ctx)
func (fs *FileSystem) WithContext(ctx context.Context) *FileSystem {
	fs.ctx = ctx
	// we set client to nil to ensure that a new client is created using the new context when Client() is called
	fs.client = nil
	return fs
}

// WithClient passes in a google storage client and returns the file system (chainable)
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithOptions option:
//
//	fs := gs.NewFileSystem(gs.WithOptions(opts))
//
// instead of:
//
//	fs := gs.NewFileSystem().WithOptions(opts)
func (fs *FileSystem) WithClient(client *storage.Client) *FileSystem {
	fs.client = client
	return fs
}

// ClientCreator defines an interface for creating a new Google Cloud Storage client.
type ClientCreator interface {
	NewClient(ctx context.Context, opts ...option.ClientOption) (*storage.Client, error)
}

// defaultClientCreator is the default implementation of ClientCreator.
type defaultClientCreator struct{}

// NewClient is a function that creates a new Google Cloud Storage client.
func (d *defaultClientCreator) NewClient(ctx context.Context, opts ...option.ClientOption) (*storage.Client, error) {
	return storage.NewClient(ctx, opts...)
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}

// GetClient returns a new Google Cloud Storage client with the provided context and Options.
func GetClient(ctx context.Context, opts Options) (*storage.Client, error) {
	gsClientOpts := parseClientOptions(opts)
	creator := &defaultClientCreator{}
	return creator.NewClient(ctx, gsClientOpts...)
}
