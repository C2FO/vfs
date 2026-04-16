package os

import (
	"path"
	"path/filepath"

	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

// Scheme defines the URI scheme for this backend (RFC 8089 file URI scheme).
const Scheme = "file"

const fsDisplayName = "os"

// FileSystem implements [vfs.FileSystem] for the local operating system.
type FileSystem struct {
	tempDir string
}

// NewFileSystem creates a new instance of the OS file system.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{}
	options.ApplyOptions(fs, opts...)
	return fs
}

// NewFile returns an OS-backed [vfs.File] for the given authority and absolute file path.
func (fs *FileSystem) NewFile(authorityStr, filePath string, _ ...vfsopt.NewFileOption) (vfs.File, error) {
	filePath = normalizeOSPath(filePath)
	if err := utils.ValidateAbsoluteFilePath(filePath); err != nil {
		return nil, err
	}

	loc, err := fs.NewLocation(authorityStr, utils.EnsureTrailingSlash(path.Dir(filePath)))
	if err != nil {
		return nil, err
	}

	return &File{
		name:     filePath,
		location: loc.(*Location),
		tempDir:  fs.tempDir,
	}, nil
}

// NewLocation returns an OS-backed [vfs.Location] for the given authority and absolute location path.
func (fs *FileSystem) NewLocation(authorityStr, locPath string) (vfs.Location, error) {
	locPath = normalizeOSPath(locPath)
	if err := utils.ValidateAbsoluteLocationPath(locPath); err != nil {
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

// Name returns the display name of the file system.
func (fs *FileSystem) Name() string {
	return fsDisplayName
}

// Scheme returns [Scheme] (the file URI scheme).
func (fs *FileSystem) Scheme() string {
	return Scheme
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

var _ vfs.FileSystem = (*FileSystem)(nil)
