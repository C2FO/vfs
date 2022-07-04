package ftp

import (
	"errors"
	"fmt"
	"io"
	"path"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the filesystem type.
const Scheme = "ftp"
const name = "Secure File Transfer Protocol"

// FileSystem implements vfs.Filesystem for the FTP filesystem.
type FileSystem struct {
	options   vfs.Options
	ftpclient Client
}

// Retry will return the default no-op retrier. The FTP client provides its own retryer interface, and is available
// to override via the ftp.FileSystem Options type.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the FTP implementation of vfs.File.
func (fs *FileSystem) NewFile(authority string, filePath string) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New("non-nil ftp.FileSystem pointer is required")
	}
	if filePath == "" {
		return nil, errors.New("non-empty string for path is required")
	}
	if err := utils.ValidateAbsoluteFilePath(filePath); err != nil {
		return nil, err
	}

	auth, err := utils.NewAuthority(authority)
	if err != nil {
		return nil, err
	}

	return &File{
		fileSystem: fs,
		Authority:  auth,
		path:       path.Clean(filePath),
	}, nil
}

// NewLocation function returns the FTP implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authority string, locPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New("non-nil ftp.FileSystem pointer is required")
	}
	if err := utils.ValidateAbsoluteLocationPath(locPath); err != nil {
		return nil, err
	}

	auth, err := utils.NewAuthority(authority)
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		path:       utils.EnsureTrailingSlash(path.Clean(locPath)),
		Authority:  auth,
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

// Client returns the underlying ftp client, creating it, if necessary
// See Overview for authentication resolution
func (fs *FileSystem) Client(authority utils.Authority) (Client, error) {
	if fs.ftpclient == nil {
		if fs.options == nil {
			fs.options = Options{}
		}

		if opts, ok := fs.options.(Options); ok {
			var err error
			fs.ftpclient, err = fs.getClient(authority, opts)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unable to create client, vfs.Options must be an ftp.Options")
		}
	}
	return fs.ftpclient, nil
}

// WithOptions sets options for client and returns the filesystem (chainable)
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
func (fs *FileSystem) WithClient(client Client) *FileSystem {
	switch client.(type) {
	case *_ftp.ServerConn:
		fs.ftpclient = client.(*_ftp.ServerConn)
		fs.options = nil
	}
	return fs
}

// NewFileSystem initializer for fileSystem struct.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func init() {
	// registers a default Filesystem
	backend.Register(Scheme, NewFileSystem())
}

func (fs *FileSystem) getClient(authority utils.Authority, opts Options) (client Client, err error) {
	if fs.ftpclient != nil {
		return fs.ftpclient.(Client), nil
	}
	return getClient(authority, opts)
}

// Client is an interface to make it easier to test
type Client interface {
	Delete(path string) error
	List(p string) ([]*_ftp.Entry, error) // NLST for just names
	Login(user string, password string) error
	MakeDir(path string) error
	Quit() error
	Rename(from, to string) error
	RetrFrom(path string, offset uint64) (*_ftp.Response, error) // is a ReadCloser (with Deadline)
	StorFrom(path string, r io.Reader, offset uint64) error      // hint use io.Pipe() if io.Writer is required.
}