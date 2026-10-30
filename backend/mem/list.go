package mem

import (
	"context"
	"io/fs"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// The listing is a snapshot taken when iteration starts: files written or deleted afterwards do not appear. Directories
// are implied by the files inside them, so an empty directory is not listed. Building the snapshot visits every file in
// the volume, so the cost grows with the volume, not with the location or any Limit. Each file entry carries the size and
// modification time held in memory.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := list.ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, &fs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		list.WalkDirs(ctx, l, q, l.snapshot().readDir, nil)(yield)
	}
}

// snapshot is the files under a location at one moment, grouped by directory.
type snapshot map[string][]list.Child

// readDir returns the children of relDir. A directory that holds no files does not exist as far as mem is concerned,
// so it has no children.
func (s snapshot) readDir(_ context.Context, relDir string) ([]list.Child, error) {
	return slices.Clone(s[relDir]), nil
}

// snapshot copies the files under l into a tree. A file is in the map only while it exists, because Close and Touch add
// it and every delete removes it, so the file's exists flag, which delete changes without the file's lock, is not read.
//
// It never holds the file system lock and a file's lock together: File.Close takes the file's lock first, so taking them
// in the other order could deadlock.
func (l *Location) snapshot() snapshot {
	prefix := l.Path()

	l.fileSystem.mu.Lock()
	var files []*memFile
	for key, obj := range l.fileSystem.fsMap[l.Authority().String()] {
		if obj == nil || !obj.isFile || !strings.HasPrefix(key, prefix) {
			continue
		}
		if f, ok := obj.i.(*memFile); ok {
			files = append(files, f)
		}
	}
	l.fileSystem.mu.Unlock()

	tree := snapshot{}
	seenDirs := map[string]bool{}
	for _, f := range files {
		f.Lock()
		size, modTime, filepath := len(f.contents), f.lastModified, f.filepath
		f.Unlock()
		if !strings.HasPrefix(filepath, prefix) {
			continue
		}
		tree.add(strings.TrimPrefix(filepath, prefix), uint64(size), modTime, seenDirs)
	}
	return tree
}

// add records the file at rel, and each directory leading to it once.
func (s snapshot) add(rel string, size uint64, modTime time.Time, seenDirs map[string]bool) {
	dir, name := "", rel
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		dir, name = rel[:i], rel[i+1:]
	}
	s[dir] = append(s[dir], list.Child{Name: name, Options: []vfs.EntryOption{vfs.WithEntryMetadata(size, modTime)}})

	for dir != "" && !seenDirs[dir] {
		seenDirs[dir] = true
		parent, child := "", dir
		if i := strings.LastIndexByte(dir, '/'); i >= 0 {
			parent, child = dir[:i], dir[i+1:]
		}
		s[parent] = append(s[parent], list.Child{Name: child, IsDir: true})
		dir = parent
	}
}
