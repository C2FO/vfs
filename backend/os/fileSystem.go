package os

import (
	"path"
	"path/filepath"
	"runtime"

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
type FileSystem struct{}

// Retry will return a retriever provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the os implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, filePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if runtime.GOOS == "windows" && filepath.IsAbs(filePath) {
		if v := filepath.VolumeName(filePath); v != "" {
			authorityStr = v
			filePath = filePath[len(v):]
		}
	}

	filePath = filepath.ToSlash(filePath)
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
	}, nil
}

// NewLocation function returns the os implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, locPath string) (vfs.Location, error) {
	if runtime.GOOS == "windows" && filepath.IsAbs(locPath) {
		if v := filepath.VolumeName(locPath); v != "" {
			authorityStr = v
			locPath = locPath[len(v):]
		}
	}

	locPath = filepath.ToSlash(locPath)
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
