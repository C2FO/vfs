package sftp

import (
	"errors"
	"io"
	"os"
	"path"
	"reflect"
	"sync"
	"time"

	_sftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the filesystem type.
const Scheme = "sftp"
const name = "Secure File Transfer Protocol"

// defaultAutoDisconnectDuration is the number of idle seconds before a
// FileSystem's connection auto-disconnects, when Options.AutoDisconnect is
// unset. It's a var (rather than a const) so tests can raise it for the
// duration of the test binary, preventing the real background disconnect
// timer from firing into an unrelated, already-completed test later in the
// same process (see TestMain in fileSystem_test.go). Production behavior is
// unaffected: this is only ever mutated by test code.
var defaultAutoDisconnectDuration = 10

var defaultClientGetter func(authority.Authority, Options) (Client, io.Closer, error)

var (
	errFileSystemRequired       = errors.New("non-nil sftp.FileSystem pointer is required")
	errAuthorityAndPathRequired = errors.New("non-empty string for authority and path are required")
)

// FileSystem implements vfs.FileSystem for the SFTP filesystem.
type FileSystem struct {
	options    Options
	sftpclient Client
	sshConn    io.Closer
	timerMutex sync.Mutex
	connTimer  *time.Timer
}

// NewFileSystem initializer for fileSystem struct.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{}

	// apply options
	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry will return the default no-op retrier. The SFTP client provides its own retryer interface, and is available
// to override via the sftp.FileSystem Options type.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the SFTP implementation of vfs.File.
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

// NewLocation function returns the SFTP implementation of vfs.Location.
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
		path:       utils.EnsureTrailingSlash(path.Clean(locPath)),
		authority:  auth,
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
func (fs *FileSystem) Client(a authority.Authority) (Client, error) {
	// first stop connection timer, if any
	fs.connTimerStop()

	fs.timerMutex.Lock()
	defer fs.timerMutex.Unlock()

	if fs.sftpclient == nil || (reflect.ValueOf(fs.sftpclient).IsValid() && reflect.ValueOf(fs.sftpclient).IsNil()) {
		var err error
		fs.sftpclient, fs.sshConn, err = defaultClientGetter(a, fs.options)
		if err != nil {
			return nil, err
		}
	}
	return fs.sftpclient, nil
}

func (fs *FileSystem) connTimerStart() {
	fs.timerMutex.Lock()
	defer fs.timerMutex.Unlock()

	// Stop any previously-scheduled timer before replacing it. Without this,
	// repeated calls to connTimerStart (e.g. from successive file operations,
	// each deferring a call) leak an orphaned timer goroutine per call, since
	// only the most recently created timer is retained in fs.connTimer and
	// reachable by connTimerStop.
	if fs.connTimer != nil {
		fs.connTimer.Stop()
	}

	aliveSec := defaultAutoDisconnectDuration
	if fs.options.AutoDisconnect > 0 {
		aliveSec = fs.options.AutoDisconnect
	}

	// Capture this timer's own identity so the callback can detect whether it is
	// still the active timer when it eventually runs. time.Timer.Stop does not
	// wait for an already-fired callback, so a callback may still be parked on
	// timerMutex after connTimerStop niled fs.connTimer or connTimerStart
	// replaced it. Without this guard, such a stale callback would close a
	// client that a concurrent Client() call already handed back to a caller.
	var timer *time.Timer
	timer = time.AfterFunc(time.Duration(aliveSec)*time.Second, func() {
		// The timer fires asynchronously on its own goroutine, so we must hold the
		// same mutex used by connTimerStart/connTimerStop/Client to safely
		// read/mutate fs.sftpclient and fs.sshConn.
		fs.timerMutex.Lock()
		defer fs.timerMutex.Unlock()

		// If this is no longer the active timer, it was stopped or superseded
		// while already firing; do nothing so we don't close an in-use client.
		if fs.connTimer != timer {
			return
		}

		// close connection and nil-ify client to force lazy reconnect
		// Only close if we have a valid, non-nil client (not a typed-nil)
		if fs.sftpclient != nil && reflect.ValueOf(fs.sftpclient).IsValid() && !reflect.ValueOf(fs.sftpclient).IsNil() {
			_ = fs.sftpclient.Close()
			fs.sftpclient = nil
		}

		if fs.sshConn != nil && reflect.ValueOf(fs.sshConn).IsValid() && !reflect.ValueOf(fs.sshConn).IsNil() {
			_ = fs.sshConn.Close()
			fs.sshConn = nil
		}
	})
	fs.connTimer = timer
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
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithOptions option:
//
//	fs := sftp.NewFileSystem(sftp.WithOptions(opts))
//
// instead of:
//
//	fs := sftp.NewFileSystem().WithOptions(opts)
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
//
// Deprecated: This method is deprecated and will be removed in a future release.
// Use WithClient option:
//
//	fs := sftp.NewFileSystem(sftp.WithClient(client))
//
// instead of:
//
//	fs := sftp.NewFileSystem().WithClient(client)
func (fs *FileSystem) WithClient(client any) *FileSystem {
	switch client.(type) {
	case Client, *ssh.Client:
		fs.sftpclient = client.(Client)
		fs.options = Options{}
	}
	return fs
}

func init() {
	defaultClientGetter = func(auth authority.Authority, opts Options) (Client, io.Closer, error) {
		return GetClient(auth, opts)
	}

	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}

// Client is an interface to make it easier to test
type Client interface {
	Chmod(path string, mode os.FileMode) error
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
