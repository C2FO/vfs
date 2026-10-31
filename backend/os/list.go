package os

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"os"
	"path/filepath"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Each directory is read when the listing reaches it, so memory use is one directory per level of depth. File entries
// carry the size, modification time and mode the operating system reported. A symbolic link is listed as a file entry
// whose Type is fs.ModeSymlink, and is not followed, so a link to a directory is not descended into. A directory that
// cannot be read is yielded with its entry and an error, and the listing continues.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := list.ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, &fs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		root := osLocationPath(l)
		readDir := func(_ context.Context, relDir string) ([]list.Child, error) {
			dir := filepath.Join(root, filepath.FromSlash(relDir))
			if relDir == "" {
				if err := requireDirectory(dir); err != nil {
					return nil, err
				}
			}
			return readChildren(dir)
		}
		list.WalkDirs(ctx, l, q, readDir, nil)(yield)
	}
}

// errNotDirectory is the cause of a listing whose location is not a directory. It is not syscall.ENOTDIR, which on
// Windows is ERROR_PATH_NOT_FOUND and so would match fs.ErrNotExist and be taken for a location that does not exist.
var errNotDirectory = errors.New("not a directory")

// requireDirectory fails unless dir is a directory. Reading a regular file as a directory is an error on Unix but not on
// every platform, so the location of a listing is checked explicitly to make a location that is a file an error everywhere.
func requireDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &fs.PathError{Op: "readdir", Path: dir, Err: errNotDirectory}
	}
	return nil
}

// readChildren reads one directory. An entry that disappears between the directory read and its stat is left out.
func readChildren(dir string) ([]list.Child, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	children := make([]list.Child, 0, len(entries))
	for _, de := range entries {
		if de.IsDir() {
			children = append(children, list.Child{Name: de.Name(), IsDir: true})
			continue
		}
		info, err := de.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		children = append(children, list.Child{Name: de.Name(), Options: []vfs.EntryOption{
			vfs.WithEntryMetadata(uint64(max(info.Size(), 0)), info.ModTime()),
			vfs.WithEntryMode(info.Mode()),
			vfs.WithEntrySys(info),
		}})
	}
	return children, nil
}
