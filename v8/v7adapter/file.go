package v7adapter

import (
	"fmt"
	"io"
	"io/fs"
	"time"

	vfsv7 "github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfsv8 "github.com/c2fo/vfs/v8"
)

// File wraps a v7 [vfsv7.File] as a v8 [vfsv8.File].
type File struct {
	inner vfsv7.File
}

// FileFromV7 wraps f. It returns nil if f is nil.
func FileFromV7(f vfsv7.File) vfsv8.File {
	if f == nil {
		return nil
	}
	return &File{inner: f}
}

// Read delegates to the v7 file.
func (f *File) Read(p []byte) (int, error) {
	return f.inner.Read(p)
}

// Write delegates to the v7 file.
func (f *File) Write(p []byte) (int, error) {
	return f.inner.Write(p)
}

// Close delegates to the v7 file.
func (f *File) Close() error {
	return f.inner.Close()
}

// String delegates to the v7 file.
func (f *File) String() string {
	return f.inner.String()
}

// URI returns the fully qualified URI for the file.
func (f *File) URI() string {
	return f.inner.URI()
}

// Path returns the absolute path including the file name.
func (f *File) Path() string {
	return f.inner.Path()
}

// Name returns the base name of the file.
func (f *File) Name() string {
	return f.inner.Name()
}

// Authority returns the URI authority from the file's location.
func (f *File) Authority() authority.Authority {
	return f.inner.Location().Authority()
}

// Location returns the parent location as a v8 [vfsv8.Location].
func (f *File) Location() vfsv8.Location {
	return LocationFromV7(f.inner.Location())
}

// Exists delegates to the v7 file.
func (f *File) Exists() (bool, error) {
	return f.inner.Exists()
}

// Stat synthesizes [fs.FileInfo] from v7 metadata.
func (f *File) Stat() (fs.FileInfo, error) {
	return statV7(f.inner)
}

// LastModified delegates to the v7 file.
func (f *File) LastModified() (*time.Time, error) {
	return f.inner.LastModified()
}

// Size delegates to the v7 file.
func (f *File) Size() (uint64, error) {
	return f.inner.Size()
}

// Touch delegates to the v7 file.
func (f *File) Touch() error {
	return f.inner.Touch()
}

var (
	_ vfsv8.File   = (*File)(nil)
	_ fmt.Stringer = (*File)(nil)
	_ io.Reader    = (*File)(nil)
	_ io.Writer    = (*File)(nil)
	_ io.Closer    = (*File)(nil)
)
