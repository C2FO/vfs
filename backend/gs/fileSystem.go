package gs

import (
	"cloud.google.com/go/storage"
	"golang.org/x/net/context"

	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/backend"
	"github.com/c2fo/vfs/utils"
)

//Scheme defines the filesystem type.
const Scheme = "gs"
const name = "Google Cloud Storage"

// FileSystem implements vfs.Filesystem for the GCS filesystem.
type FileSystem struct {
	client  *storage.Client
	ctx     context.Context
	options vfs.Options
}

// NewFile function returns the gcs implementation of vfs.File.
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	return newFile(fs, volume, name)
}

// NewLocation function returns the GCS implementation of vfs.Location.
func (fs *FileSystem) NewLocation(volume string, path string) (loc vfs.Location, err error) {
	loc = &Location{
		fileSystem: fs,
		bucket:     volume,
		prefix:     utils.EnsureTrailingSlash(path),
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
		client, err := storage.NewClient(fs.ctx, gsClientOpts...)
		if err != nil {
			return nil, err
		}
		fs.client = client
	}
	return fs.client, nil
}

// WithOptions sets options for client and returns the filesystem (chainable)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	fs.options = opts
	//we set client to nil to ensure that a new client is created using the new context when Client() is called
	fs.client = nil
	return fs
}

// WithContext passes in user context and returns the filesystem (chainable)
func (fs *FileSystem) WithContext(ctx context.Context) *FileSystem {
	fs.ctx = ctx
	//we set client to nil to ensure that a new client is created using the new context when Client() is called
	fs.client = nil
	return fs
}

// WithClient passes in a google storage client and returns the filesystem (chainable)
func (fs *FileSystem) WithClient(client *storage.Client) *FileSystem {
	fs.client = client
	return fs
}

// NewFileSystem intializer for FileSystem struct accepts google cloud storage client and returns Filesystem or error.
func NewFileSystem() *FileSystem {
	fs := &FileSystem{}
	fs = fs.WithContext(context.Background())
	return fs
}

func init() {
	//registers a default Filesystem
	backend.Register(Scheme, NewFileSystem())
}
