package iofs

import (
	"errors"
	"io"
	"io/fs"
	"iter"
	"sort"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
)

// dirFile is an open directory. Its entries are streamed from a non-recursive listing as ReadDir asks for them.
type dirFile struct {
	fsys *fsys
	loc  vfs.Location
	name string
	info dirInfo

	next   func() (vfs.Entry, error, bool)
	stop   func()
	done   bool
	files  map[string]struct{} // names of files already returned, so a same-named directory is dropped
	closed bool
}

var _ fs.ReadDirFile = (*dirFile)(nil)

func (d *dirFile) Stat() (fs.FileInfo, error) { return d.info, nil }

func (d *dirFile) Read([]byte) (int, error) {
	return 0, pathError("read", d.name, errors.New("is a directory"))
}

// Close stops any listing in progress. It is safe to call more than once.
func (d *dirFile) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	if d.stop != nil {
		d.stop()
	}
	return nil
}

// ReadDir implements fs.ReadDirFile. With n <= 0 it returns every remaining entry, sorted by filename. With n > 0 it
// returns at most n entries in listing order and io.EOF once none remain.
func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.closed {
		return nil, pathError("readdir", d.name, fs.ErrClosed)
	}
	if d.next == nil {
		d.next, d.stop = pull(list.Entries(d.fsys.ctx, d.loc))
	}

	var entries []fs.DirEntry
	for n <= 0 || len(entries) < n {
		if d.done {
			break
		}
		e, err, ok := d.next()
		if !ok {
			d.done = true
			break
		}
		if errors.Is(err, fs.ErrInvalid) {
			continue
		}
		if err != nil {
			d.done = true
			return entries, pathError("readdir", d.name, err)
		}
		if d.shadowed(e) {
			continue
		}
		entries = append(entries, dirEntry{Entry: e, fsys: d.fsys})
	}

	if n > 0 && len(entries) == 0 {
		return nil, io.EOF
	}
	if n <= 0 {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		if entries == nil {
			entries = []fs.DirEntry{}
		}
	}
	return entries, nil
}

// shadowed reports whether e is a directory with the name of a file already returned. Object stores allow an object
// "foo" and a prefix "foo/" together; the file wins, as it does for Open and Stat. Files precede a same-named
// directory in listing order, so one pass is enough.
func (d *dirFile) shadowed(e vfs.Entry) bool {
	if !e.IsDir() {
		if d.files == nil {
			d.files = make(map[string]struct{})
		}
		d.files[e.Name()] = struct{}{}
		return false
	}
	_, ok := d.files[e.Name()]
	return ok
}

func pull(seq iter.Seq2[vfs.Entry, error]) (func() (vfs.Entry, error, bool), func()) {
	return iter.Pull2(seq)
}

// dirEntry is the fs.DirEntry returned by ReadDir. Its Info agrees with Stat, including the default permissions.
type dirEntry struct {
	vfs.Entry
	fsys *fsys
}

func (e dirEntry) Info() (fs.FileInfo, error) {
	return e.fsys.entryInfo(e.Entry, "stat", e.RelPath())
}
