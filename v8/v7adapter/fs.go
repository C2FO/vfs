package v7adapter

import (
	vfsv7 "github.com/c2fo/vfs/v7"
	vfsv8 "github.com/c2fo/vfs/v8"
	vfsv8opt "github.com/c2fo/vfs/v8/options"
)

// FileSystem wraps a v7 [vfsv7.FileSystem] as a v8 [vfsv8.FileSystem].
type FileSystem struct {
	inner vfsv7.FileSystem
}

// FileSystemFromV7 wraps fs. It returns nil if fs is nil.
func FileSystemFromV7(fs vfsv7.FileSystem) vfsv8.FileSystem {
	if fs == nil {
		return nil
	}
	return &FileSystem{inner: fs}
}

// NewFile delegates to the v7 file system without forwarding v8 options to v7 yet.
func (s *FileSystem) NewFile(authority, absFilePath string, _ ...vfsv8opt.NewFileOption) (vfsv8.File, error) {
	f, err := s.inner.NewFile(authority, absFilePath)
	if err != nil {
		return nil, err
	}
	return FileFromV7(f), nil
}

// NewLocation delegates to the v7 file system and wraps the result.
func (s *FileSystem) NewLocation(authority, absLocPath string) (vfsv8.Location, error) {
	loc, err := s.inner.NewLocation(authority, absLocPath)
	if err != nil {
		return nil, err
	}
	return LocationFromV7(loc), nil
}

// Name delegates to the v7 file system.
func (s *FileSystem) Name() string {
	return s.inner.Name()
}

// Scheme delegates to the v7 file system.
func (s *FileSystem) Scheme() string {
	return s.inner.Scheme()
}

var _ vfsv8.FileSystem = (*FileSystem)(nil)
