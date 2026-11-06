package iofs

import (
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

func (f *file) Read(p []byte) (int, error) { return f.vf.Read(p) }

func (f *file) Seek(offset int64, whence int) (int64, error) { return f.vf.Seek(offset, whence) }

func (f *file) Close() error { return f.vf.Close() }

func (f *file) Stat() (fs.FileInfo, error) { return f.fsys.entryInfo(f.e, "stat", f.name) }

func (r *readerAtFile) ReadAt(p []byte, off int64) (int, error) { return r.ra.ReadAt(p, off) }

// filePerm returns the entry's mode, adding read permission when the backend reported none.
func filePerm(e vfs.Entry) fs.FileMode {
	m := e.Mode()
	if m.Perm() == 0 {
		m |= defaultFilePerm
	}
	return m
}

// fileInfo adapts a listed file's fs.FileInfo: it adds default permissions and exposes the vfs.File as Sys.
type fileInfo struct {
	fs.FileInfo
	mode fs.FileMode
	sys  vfs.File
}

func (i fileInfo) Mode() fs.FileMode { return i.mode }

func (i fileInfo) Sys() any { return i.sys }

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
