package gs

import (
	"errors"
	"path"

	"cloud.google.com/go/storage"
	"golang.org/x/net/context"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the file system type.
const Scheme = "gs"
const name = "Google Cloud Storage"

// FileSystem implements vfs.FileSystem for the GCS file system.
type FileSystem struct {
	client        *storage.Client
	ctx           context.Context
	options       vfs.Options
	clientCreator ClientCreator
}

// Retry will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	if opts, _ := fs.options.(Options); opts.Retry != nil {
		return opts.Retry
	}
	return vfs.DefaultRetryer()
}

// NewFile function returns the gcs implementation of vfs.File.
func (fs *FileSystem) NewFile(volume, name string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New("non-nil gs.FileSystem pointer is required")
	}
	if volume == "" || name == "" {
		return nil, errors.New("non-empty strings for Bucket and Key are required")
	}
	if err := utils.ValidateAbsoluteFilePath(name); err != nil {
		return nil, err
	}
	return &File{
		fileSystem: fs,
		bucket:     volume,
		key:        path.Clean(name),
		opts:       opts,
	}, nil
}

// NewLocation function returns the GCS implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume, name string) (loc vfs.Location, err error) {
	if fs == nil {
		return nil, errors.New("non-nil gs.FileSystem pointer is required")
	}
	if volume == "" || name == "" {
		return nil, errors.New("non-empty strings for bucket and key are required")
	}
	if err := utils.ValidateAbsoluteLocationPath(name); err != nil {
		return nil, err
	}
	loc = &Location{
		fileSystem: fs,
		bucket:     volume,
		prefix:     utils.EnsureTrailingSlash(path.Clean(name)),
	}
	return
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
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	fs.options = opts
	// we set client to nil to ensure that a new client is created using the new context when Client() is called
	fs.client = nil
	return fs
}

// WithContext passes in user context and returns the file system (chainable)
func (fs *FileSystem) WithContext(ctx context.Context) *FileSystem {
	fs.ctx = ctx
	// we set client to nil to ensure that a new client is created using the new context when Client() is called
	fs.client = nil
	return fs
}

// WithClient passes in a google storage client and returns the file system (chainable)
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

// NewFileSystem initializer for FileSystem struct accepts google cloud storage client and returns FileSystem or error.
func NewFileSystem() *FileSystem {
	fs := &FileSystem{
		ctx:           context.Background(),
		clientCreator: &defaultClientCreator{},
	}

	return fs
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
