package os

import (
	"path"
	"path/filepath"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the file system type.
const Scheme = "file"
const name = "os"

// FileSystem implements vfs.FileSystem for the OS file system.
type FileSystem struct {
	tempDir string
}

// NewFileSystem creates a new instance of the OS file system.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{}

	// apply options
	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry will return a retriever provided via options, or a no-op if none is provided.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the os implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, filePath string, opts ...options.NewFileOption) (vfs.File, error) {
	filePath = normalizeOSPath(filePath)
	err := utils.ValidateAbsoluteFilePath(filePath)
	if err != nil {
		return nil, err
	}

	loc, err := fs.NewLocation(authorityStr, utils.EnsureTrailingSlash(path.Dir(filePath)))
	if err != nil {
		return nil, err
	}

	return &File{
		name:     filePath,
		location: loc.(*Location),
		opts:     opts,
		tempDir:  fs.tempDir,
	}, nil
}

// NewLocation function returns the os implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, locPath string) (vfs.Location, error) {
	locPath = normalizeOSPath(locPath)
	err := utils.ValidateAbsoluteLocationPath(locPath)
	if err != nil {
		return nil, err
	}

	auth, err := authority.NewAuthority(authorityStr)
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		authority:  auth,
		name:       utils.EnsureTrailingSlash(path.Clean(locPath)),
	}, nil
}

// Name returns "os"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "file" as the initial part of a file URI ie: file://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

func init() {
	backend.Register(Scheme, &FileSystem{})
}

// normalizeOSPath converts a native OS path to the canonical forward-slash form
// used internally. On Windows, drive-letter paths like "C:\foo" become "/C:/foo"
// so the drive letter stays in the path (not the URI authority).
// UNC paths like "\\server\share\path" become "//server/share/path" via
// filepath.ToSlash and already satisfy the leading-slash requirement.
func normalizeOSPath(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.ToSlash(p)
	if len(p) >= 2 && p[1] == ':' && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) {
		p = "/" + p
	}
	return p
}
