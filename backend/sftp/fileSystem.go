package sftp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"

	_sftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the filesystem type.
const Scheme = "sftp"
const name = "Secure File Transfer Protocol"
const defaultAutoDisconnectDuration = 10

var defaultClientGetter func(utils.Authority, Options) (Client, io.Closer, error)

// FileSystem implements vfs.Filesystem for the SFTP filesystem.
type FileSystem struct {
	options    vfs.Options
	sftpclient Client
	sshConn    io.Closer
	timerMutex sync.Mutex
	connTimer  *time.Timer
}

// Retry will return the default no-op retrier. The SFTP client provides its own retryer interface, and is available
// to override via the sftp.FileSystem Options type.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the SFTP implementation of vfs.File.
func (fs *FileSystem) NewFile(authority, filePath string) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New("non-nil sftp.FileSystem pointer is required")
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

// NewLocation function returns the SFTP implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authority, locPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New("non-nil sftp.FileSystem pointer is required")
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

// Scheme return "sftp" as the initial part of a file URI ie: sftp://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying sftp client, creating it, if necessary
// See Overview for authentication resolution
func (fs *FileSystem) Client(authority utils.Authority) (Client, error) {
	// first stop connection timer, if any
	fs.connTimerStop()
	if fs.sftpclient == nil {
		if fs.options == nil {
			fs.options = Options{}
		}

		opts, ok := fs.options.(Options)
		if !ok {
			return nil, fmt.Errorf("unable to create client, vfs.Options must be an sftp.Options")
		}
		var err error
		fs.sftpclient, fs.sshConn, err = defaultClientGetter(authority, opts)
		if err != nil {
			return nil, err
		}
	}
	return fs.sftpclient, nil
}

func (fs *FileSystem) connTimerStart() {
	fs.timerMutex.Lock()
	defer fs.timerMutex.Unlock()

	aliveSec := defaultAutoDisconnectDuration
	if fs.options != nil {
		if v, ok := fs.options.(Options); ok && v.AutoDisconnect != 0 {
			aliveSec = v.AutoDisconnect
		}
	}

	fs.connTimer = time.AfterFunc(time.Duration(aliveSec)*time.Second, func() {
		// close connection and nil-ify client to force lazy reconnect
		if fs.sftpclient != nil {
			_ = fs.sftpclient.Close()
			fs.sftpclient = nil
		}

		if fs.sshConn != nil {
			_ = fs.sshConn.Close()
			fs.sshConn = nil
		}
	})
}

func (fs *FileSystem) connTimerStop() {
	fs.timerMutex.Lock()
	defer fs.timerMutex.Unlock()
	if fs.connTimer != nil {
		fs.connTimer.Stop()
		fs.connTimer = nil
	}
}

// WithOptions sets options for client and returns the filesystem (chainable)
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {

	// only set options if vfs.Options is sftp.Options
	if opts, ok := opts.(Options); ok {
		fs.options = opts
		// we set client to nil to ensure that a new client is created using the new context when Client() is called
		fs.sftpclient = nil
	}
	return fs
}

// WithClient passes in an sftp client and returns the filesystem (chainable)
func (fs *FileSystem) WithClient(client interface{}) *FileSystem {
	switch client.(type) {
	case Client, *ssh.Client:
		fs.sftpclient = client.(Client)
		fs.options = nil
	}
	return fs
}

// NewFileSystem initializer for fileSystem struct.
func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func init() {
	defaultClientGetter = getClient

	// registers a default Filesystem
	backend.Register(Scheme, NewFileSystem())
}

// Client is an interface to make it easier to test
type Client interface {
	Chtimes(path string, atime, mtime time.Time) error
	Create(path string) (*_sftp.File, error)
	MkdirAll(path string) error
	OpenFile(path string, f int) (*_sftp.File, error)
	ReadDir(p string) ([]os.FileInfo, error)
	Remove(path string) error
	Rename(oldname, newname string) error
	Stat(p string) (os.FileInfo, error)
	Close() error
}
