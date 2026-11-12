package iofs

import (
	"errors"
	"io"
	"io/fs"
	"time"

	"github.com/c2fo/vfs/v7"
)

const (
	defaultFilePerm fs.FileMode = 0o444
	defaultDirPerm  fs.FileMode = 0o555
)

// file is an open regular file. It deliberately exposes no write method, although the vfs.File it wraps has one.
type file struct {
	fsys *fsys
	vf   vfs.File
	e    vfs.Entry
	name string

	closed bool
}

// readerAtFile is a file whose underlying vfs.File also supports io.ReaderAt.
type readerAtFile struct {
	*file
	ra io.ReaderAt
}

func newFile(f *fsys, vf vfs.File, e vfs.Entry, name string) fs.File {
	fl := &file{fsys: f, vf: vf, e: e, name: name}
	if ra, ok := vf.(io.ReaderAt); ok {
		return &readerAtFile{file: fl, ra: ra}
	}
	return fl
}

var (
	_ io.Seeker   = (*file)(nil)
	_ io.ReaderAt = (*readerAtFile)(nil)
)

// Read reads from the file. A vfs.File reopens itself when used after Close, so a closed file is rejected here.
func (f *file) Read(p []byte) (int, error) {
	if f.closed {
		return 0, pathError("read", f.name, fs.ErrClosed)
	}
	n, err := f.vf.Read(p)
	return n, f.ioError("read", err)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, pathError("seek", f.name, fs.ErrClosed)
	}
	pos, err := f.vf.Seek(offset, whence)
	return pos, f.ioError("seek", err)
}

// Close closes the file. It is safe to call more than once.
func (f *file) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	return f.ioError("close", f.vf.Close())
}

// ioError reports err as a *fs.PathError naming the file's name in the filesystem, except io.EOF, which callers
// compare with ==.
func (f *file) ioError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) {
		return io.EOF
	}
	return pathError(op, f.name, err)
}

func (f *file) Stat() (fs.FileInfo, error) { return f.fsys.entryInfo(f.e, "stat", f.name) }

func (r *readerAtFile) ReadAt(p []byte, off int64) (int, error) {
	if r.closed {
		return 0, pathError("readat", r.name, fs.ErrClosed)
	}
	n, err := r.ra.ReadAt(p, off)
	return n, r.ioError("readat", err)
}

// filePerm returns the entry's mode, adding read permission when the backend reported none.
func filePerm(e vfs.Entry) fs.FileMode {
	m := e.Mode()
	if m.Perm() == 0 {
		m |= defaultFilePerm
	}
	return m
}

// fileInfo adapts a listed file's fs.FileInfo: it adds default permissions and reports no
// Sys value, so the underlying vfs.File, which can write, is not reachable.
type fileInfo struct {
	fs.FileInfo
	mode fs.FileMode
}

func (i fileInfo) Mode() fs.FileMode { return i.mode }

func (i fileInfo) Sys() any { return nil }

// dirInfo is the fs.FileInfo of a directory. Directories have no size or modification time in VFS.
type dirInfo struct {
	name string
	mode fs.FileMode
}

// newDirInfo returns the info for a directory listed as e, using the permissions the backend reported, if any.
func newDirInfo(name string, e *vfs.Entry) dirInfo {
	mode := defaultDirPerm
	if e != nil {
		if perm := e.Mode() &^ fs.ModeType; perm.Perm() != 0 {
			mode = perm
		}
	}
	return dirInfo{name: name, mode: fs.ModeDir | mode}
}

func (d dirInfo) Name() string       { return d.name }
func (d dirInfo) Size() int64        { return 0 }
func (d dirInfo) Mode() fs.FileMode  { return d.mode }
func (d dirInfo) ModTime() time.Time { return time.Time{} }
func (d dirInfo) IsDir() bool        { return true }
func (d dirInfo) Sys() any           { return nil }
