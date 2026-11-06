package iofs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
)

var errNotDir = errors.New("not a directory")

// Option configures the filesystem returned by New.
type Option func(*fsys)

// WithContext sets the context used for the listing requests behind Open, Stat and ReadDir. The default is
// context.Background().
func WithContext(ctx context.Context) Option {
	return func(f *fsys) {
		if ctx != nil {
			f.ctx = ctx
		}
	}
}

// New returns a read-only fs.FS rooted at loc. The name "." refers to loc itself.
//
// The result also implements fs.StatFS, fs.ReadDirFS, fs.ReadFileFS and fs.SubFS.
func New(loc vfs.Location, opts ...Option) fs.FS {
	f := &fsys{loc: loc, ctx: context.Background()}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

type fsys struct {
	loc vfs.Location
	ctx context.Context
}

var (
	_ fs.FS         = (*fsys)(nil)
	_ fs.StatFS     = (*fsys)(nil)
	_ fs.ReadDirFS  = (*fsys)(nil)
	_ fs.ReadFileFS = (*fsys)(nil)
	_ fs.SubFS      = (*fsys)(nil)
)

// pathError builds the *fs.PathError returned for name. A failure already reported by the list package is
// unwrapped first, so the message names the caller's path rather than the backend's.
func pathError(op, name string, err error) error {
	if pe, ok := err.(*fs.PathError); ok && pe.Err != nil { //nolint:errorlint // only unwrap an error that is itself a PathError
		err = pe.Err
	}
	return &fs.PathError{Op: op, Path: name, Err: err}
}

// lookup finds the entry for name, which must not be ".". Entries are listed in byte order of their order key, where
// directories carry a trailing "/", so a file named "foo" is seen before a directory "foo/" and wins.
func (f *fsys) lookup(op, name string) (vfs.Entry, error) {
	dir, base := path.Split(name)
	parent := f.loc
	if dir != "" {
		var err error
		if parent, err = f.loc.NewLocation(dir); err != nil {
			return vfs.Entry{}, pathError(op, name, fmt.Errorf("%w: %w", fs.ErrInvalid, err))
		}
	}

	for e, err := range list.Entries(f.ctx, parent, list.Prefix(base)) {
		if errors.Is(err, fs.ErrInvalid) {
			continue
		}
		if err != nil {
			return vfs.Entry{}, pathError(op, name, err)
		}
		switch n := e.Name(); {
		case n == base:
			return e, nil
		case n > base+"/":
			return vfs.Entry{}, pathError(op, name, fs.ErrNotExist)
		}
	}
	return vfs.Entry{}, pathError(op, name, fs.ErrNotExist)
}

// Open opens the named file or directory.
func (f *fsys) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathError("open", name, fs.ErrInvalid)
	}
	if name == "." {
		return &dirFile{fsys: f, loc: f.loc, name: name, info: newDirInfo(name, nil)}, nil
	}

	e, err := f.lookup("open", name)
	if err != nil {
		return nil, err
	}
	if e.IsDir() {
		loc, err := e.Location()
		if err != nil {
			return nil, pathError("open", name, err)
		}
		return &dirFile{fsys: f, loc: loc, name: name, info: newDirInfo(e.Name(), &e)}, nil
	}

	vf, err := e.File()
	if err != nil {
		return nil, pathError("open", name, err)
	}
	return newFile(f, vf, e, name), nil
}

// Stat returns the fs.FileInfo for the named file or directory.
func (f *fsys) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, pathError("stat", name, fs.ErrInvalid)
	}
	if name == "." {
		return newDirInfo(name, nil), nil
	}

	e, err := f.lookup("stat", name)
	if err != nil {
		return nil, err
	}
	return f.entryInfo(e, "stat", name)
}

// entryInfo builds the fs.FileInfo for a listed entry. Listed metadata is used when the backend provided it.
func (f *fsys) entryInfo(e vfs.Entry, op, name string) (fs.FileInfo, error) {
	if e.IsDir() {
		return newDirInfo(e.Name(), &e), nil
	}
	info, err := e.InfoContext(f.ctx)
	if err != nil {
		return nil, pathError(op, name, err)
	}
	vf, err := e.File()
	if err != nil {
		return nil, pathError(op, name, err)
	}
	return fileInfo{FileInfo: info, mode: filePerm(e), sys: vf}, nil
}

// ReadDir reads the named directory and returns its entries sorted by filename.
func (f *fsys) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, pathErrorOp("readdir", err)
	}
	defer file.Close() //nolint:errcheck // nothing useful to do if closing a listing fails

	dir, ok := file.(*dirFile)
	if !ok {
		return nil, pathError("readdir", name, errNotDir)
	}
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// ReadFile reads the named file and returns its contents.
func (f *fsys) ReadFile(name string) ([]byte, error) {
	file, err := f.Open(name)
	if err != nil {
		return nil, pathErrorOp("readfile", err)
	}
	defer file.Close() //nolint:errcheck // the contents were already read

	if _, ok := file.(*dirFile); ok {
		return nil, pathError("readfile", name, errors.New("is a directory"))
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, pathError("readfile", name, err)
	}
	return data, nil
}

// Sub returns an fs.FS rooted at the named directory. It makes no request.
func (f *fsys) Sub(dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, pathError("sub", dir, fs.ErrInvalid)
	}
	if dir == "." {
		return f, nil
	}
	loc, err := f.loc.NewLocation(strings.TrimSuffix(dir, "/") + "/")
	if err != nil {
		return nil, pathError("sub", dir, fmt.Errorf("%w: %w", fs.ErrInvalid, err))
	}
	return &fsys{loc: loc, ctx: f.ctx}, nil
}

// pathErrorOp re-labels the operation of a *fs.PathError returned by Open.
func pathErrorOp(op string, err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return &fs.PathError{Op: op, Path: pe.Path, Err: pe.Err}
	}
	return err
}
