package vfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"math"
	"path"
	"strings"
	"time"

	"github.com/c2fo/vfs/v7/options"
)

// EntryLister is an optional interface a Location may implement to list its contents natively as a stream of
// Entry values. The list package uses it to implement list.Entries.
//
// An implementation MUST:
//   - yield entries in ascending byte order of relative path, where directories compare with a trailing "/"
//   - yield directories for recursive listings, synthesizing them where the backend has none
//   - make no further backend requests once yield returns false, and have no request in flight while yield runs
//   - honor ctx, and wrap each backend request with FileSystem().Retry()
//   - yield a failure to read one subdirectory together with that directory's Entry, and continue; yield any other
//     failure with a zero Entry and then stop
//
// Options are hints. The list package applies every option itself, so an implementation may ignore any option it
// cannot push down to the backend. A Location that does not implement EntryLister is listed through
// Location.List: non-recursive, files only, without metadata.
type EntryLister interface {
	ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[Entry, error]
}

// Entry describes one file or directory found by a listing. It implements fs.DirEntry. Entries are values: they are
// safe to copy and to use after the listing that produced them has ended.
//
// Entries are not comparable with == and must not be used as map keys: the value returned by Sys() is backend
// specific and may not be comparable, which would make the comparison panic. Compare Path() instead.
//
// The zero Entry is not a valid entry. Listings yield it alongside an error that ended the listing.
type Entry struct {
	parent   Location
	rel      string // slash-separated, relative to parent, no leading or trailing slash
	dir      bool
	mode     fs.FileMode
	size     uint64
	modTime  time.Time
	hasMeta  bool
	sys      any
	typeBits fs.FileMode
}

// EntryOption configures an Entry built by NewFileEntry or NewDirEntry.
type EntryOption func(*Entry)

// WithEntryMetadata records the size and modification time reported by the backend's listing, so Info() needs no
// further request.
func WithEntryMetadata(size uint64, modTime time.Time) EntryOption {
	return func(e *Entry) {
		e.size = size
		e.modTime = modTime
		e.hasMeta = true
	}
}

// WithEntryMode records the mode reported by the backend: the permission bits, the setuid, setgid and sticky bits,
// and, for files, the type bits such as fs.ModeSymlink. fs.ModeDir is always derived from the entry's kind, never
// from mode.
func WithEntryMode(mode fs.FileMode) EntryOption {
	return func(e *Entry) {
		e.mode = mode &^ fs.ModeType
		e.typeBits = mode & fs.ModeType &^ fs.ModeDir
	}
}

// WithEntrySys records the backend's native listing object, returned by Sys().
func WithEntrySys(sys any) EntryOption {
	return func(e *Entry) { e.sys = sys }
}

// NewFileEntry builds the Entry for a file found at relPath beneath parent. It is for backend implementers.
//
// relPath must be slash-separated, relative to parent, and satisfy fs.ValidPath other than "." (no empty, "." or ".."
// segments, no leading or trailing slash). Otherwise NewFileEntry returns a *fs.PathError wrapping fs.ErrInvalid. It
// returns an error, not a panic, because object stores can hold keys that no Entry can represent, such as "a//b".
func NewFileEntry(parent Location, relPath string, opts ...EntryOption) (Entry, error) {
	return newEntry(parent, relPath, false, opts)
}

// NewDirEntry builds the Entry for a directory found at relPath beneath parent. It is for backend implementers.
//
// relPath follows the rules of NewFileEntry, except that one trailing slash is allowed and removed.
func NewDirEntry(parent Location, relPath string, opts ...EntryOption) (Entry, error) {
	return newEntry(parent, relPath, true, opts)
}

func newEntry(parent Location, relPath string, dir bool, opts []EntryOption) (Entry, error) {
	if parent == nil {
		return Entry{}, &fs.PathError{Op: "list", Path: relPath, Err: errors.New("entry requires a parent location")}
	}
	rel := relPath
	if dir {
		rel = strings.TrimSuffix(rel, "/")
	}
	if rel == "." || !fs.ValidPath(rel) || rel == "" {
		return Entry{}, &fs.PathError{Op: "list", Path: relPath, Err: fs.ErrInvalid}
	}
	e := Entry{parent: parent, rel: rel, dir: dir}
	for _, opt := range opts {
		opt(&e)
	}
	return e, nil
}

// Name returns the entry's base name, without a trailing slash.
func (e Entry) Name() string {
	if e.rel == "" {
		return ""
	}
	return path.Base(e.rel)
}

// IsDir reports whether the entry is a directory.
func (e Entry) IsDir() bool { return e.dir }

// Type returns the entry's type bits: fs.ModeDir for directories, fs.ModeSymlink for symbolic links the backend
// reports, and 0 otherwise.
func (e Entry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return e.typeBits
}

// Mode returns the entry's type bits plus the permission, setuid, setgid and sticky bits, when the backend reports
// them. Otherwise only the type bits are set.
func (e Entry) Mode() fs.FileMode { return e.Type() | e.mode }

// Path returns the absolute path of the entry within its authority, ending in "/" for directories. It matches the
// Path() of the File or Location the entry resolves to.
func (e Entry) Path() string {
	p := path.Join("/", e.parentPath(), e.rel)
	if e.dir && p != "/" {
		p += "/"
	}
	return p
}

// RelPath returns the slash-separated path of the entry relative to the listed location, with no leading or
// trailing slash. It is always a valid io/fs name (see fs.ValidPath).
func (e Entry) RelPath() string { return e.rel }

// Size returns the file size reported by the listing. It returns 0 when HasMetadata is false and for directories.
func (e Entry) Size() uint64 { return e.size }

// ModTime returns the modification time reported by the listing. It returns the zero time when HasMetadata is false.
func (e Entry) ModTime() time.Time { return e.modTime }

// HasMetadata reports whether Size and ModTime came from the listing.
func (e Entry) HasMetadata() bool { return e.hasMeta }

// Sys returns the backend's native listing object for the entry, or nil.
func (e Entry) Sys() any { return e.sys }

// File returns the File the entry describes. It makes no request. It returns an error for a directory entry.
func (e Entry) File() (File, error) {
	if e.parent == nil {
		return nil, errors.New("vfs: entry has no parent location")
	}
	if e.dir {
		return nil, fmt.Errorf("vfs: entry %q is a directory, not a file", e.rel)
	}
	return e.parent.NewFile(e.rel)
}

// Location returns the Location the entry describes. It makes no request. It returns an error for a file entry.
func (e Entry) Location() (Location, error) {
	if e.parent == nil {
		return nil, errors.New("vfs: entry has no parent location")
	}
	if !e.dir {
		return nil, fmt.Errorf("vfs: entry %q is a file, not a directory", e.rel)
	}
	return e.parent.NewLocation(e.rel + "/")
}

// Info returns the entry's fs.FileInfo. When the entry has listed metadata, and for directories, it makes no
// request. Otherwise it asks the backend for the file's size and modification time.
//
// Failures are returned as a *fs.PathError with Op "stat". The error matches fs.ErrNotExist, as well as the
// backend's own error, when the file no longer exists. A size that doesn't fit an int64 is also an error.
//
// Info is InfoContext with context.Background(); it exists to satisfy fs.DirEntry.
func (e Entry) Info() (fs.FileInfo, error) {
	return e.InfoContext(context.Background())
}

// InfoContext is Info with a context. It returns ctx.Err(), wrapped in a *fs.PathError, if ctx is already done
// before a request would be made. The File methods it calls don't accept a context yet, so a request already in
// flight is not interrupted.
func (e Entry) InfoContext(ctx context.Context) (fs.FileInfo, error) {
	if e.dir || e.hasMeta {
		return e.fileInfo(e.size, e.modTime)
	}
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{Op: "stat", Path: e.Path(), Err: err}
	}
	f, err := e.File()
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: e.Path(), Err: err}
	}
	size, sizeErr := f.Size()
	var modTime *time.Time
	var modErr error
	if sizeErr == nil {
		if err := ctx.Err(); err != nil {
			return nil, &fs.PathError{Op: "stat", Path: e.Path(), Err: err}
		}
		modTime, modErr = f.LastModified()
	}
	if err := errors.Join(sizeErr, modErr); err != nil {
		// A canceled context is reported as such, without starting another request to find out whether the file exists.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, &fs.PathError{Op: "stat", Path: e.Path(), Err: errors.Join(ctxErr, err)}
		}
		if exists, existsErr := f.Exists(); existsErr == nil && !exists {
			err = errors.Join(fs.ErrNotExist, err)
		}
		return nil, &fs.PathError{Op: "stat", Path: e.Path(), Err: err}
	}
	var mt time.Time
	if modTime != nil {
		mt = *modTime
	}
	return e.fileInfo(size, mt)
}

func (e Entry) fileInfo(size uint64, modTime time.Time) (fs.FileInfo, error) {
	if size > math.MaxInt64 {
		return nil, &fs.PathError{Op: "stat", Path: e.Path(), Err: fmt.Errorf("size %d overflows int64", size)}
	}
	return entryInfo{e: e, size: int64(size), modTime: modTime}, nil
}

func (e Entry) parentPath() string {
	if e.parent == nil {
		return ""
	}
	return e.parent.Path()
}

// entryInfo is the fs.FileInfo returned by Entry.Info.
type entryInfo struct {
	e       Entry
	size    int64
	modTime time.Time
}

func (i entryInfo) Name() string       { return i.e.Name() }
func (i entryInfo) Size() int64        { return i.size }
func (i entryInfo) Mode() fs.FileMode  { return i.e.Mode() }
func (i entryInfo) ModTime() time.Time { return i.modTime }
func (i entryInfo) IsDir() bool        { return i.e.dir }
func (i entryInfo) Sys() any           { return i.e.sys }

var (
	_ fs.DirEntry = Entry{}
	_ fs.FileInfo = entryInfo{}
)
