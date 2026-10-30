package list

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"slices"
	"strings"

	"github.com/c2fo/vfs/v7"
)

// Child describes one child of a directory, as a hierarchical backend reads it.
type Child struct {
	// Name is the child's base name. A name that cannot be part of an Entry's path is reported as a non-fatal error
	// matching fs.ErrInvalid, not as an entry.
	Name string
	// IsDir reports whether the child is a directory. A symbolic link is not a directory here, even if it points to
	// one, so listings never follow links; report it with vfs.WithEntryMode(fs.ModeSymlink | perm).
	IsDir bool
	// Options supply the listed size and modification time, mode and native object.
	Options []vfs.EntryOption
}

// ReadDirFunc reads the children of the directory at relDir, a slash-separated path relative to the listed location
// ("" for the location itself), in any order. A directory that does not exist is reported with an error matching
// fs.ErrNotExist, and one that cannot be read with fs.ErrPermission when the cause is known.
type ReadDirFunc func(ctx context.Context, relDir string) ([]Child, error)

// WalkDirs lists a hierarchical backend, one directory read at a time. It is the engine behind the native listing of
// backends that read a directory per request, such as os, sftp and ftp.
//
// Each directory's children are sorted by order key and walked depth first, which yields the byte order of full paths
// that every listing guarantees. Descent stops at q.Depth, skips directories outside q.Prefix and subtrees that sort
// entirely before q.StartAfter, and honors q.Descend in a recursive listing, calling it for each directory about to be read. WalkDirs
// does not apply Kind, Glob, Match, Filter or Limit; list.Entries does.
//
// If reading the location itself fails the error is yielded with a zero Entry and the listing ends, except that a
// location which does not exist yields nothing. If reading a subdirectory fails, the error is yielded together with
// that directory's Entry and the walk continues with the next entry outside it. A child with an invalid name is yielded
// as a zero Entry with an error matching fs.ErrInvalid, before the valid entries of its directory, and the walk
// continues.
//
// Each read is wrapped in retry, which may be nil, and is not attempted once ctx is done. No read is in progress while
// the caller's loop body runs, and none is started after the caller stops iterating.
func WalkDirs(ctx context.Context, parent vfs.Location, q Query, readDir ReadDirFunc, retry RetryFunc) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		w := &walker{
			emitter: emitter{ctx: ctx, parent: parent, yield: yield},
			win:     newWindow(q),
			descend: descender(q),
			readDir: readDir,
			retry:   retry,
		}
		w.walk("", vfs.Entry{})
	}
}

type walker struct {
	emitter
	win     window
	descend EntryFunc
	readDir ReadDirFunc
	retry   RetryFunc
}

type walkItem struct {
	rel   string
	key   string
	dir   bool
	entry vfs.Entry
}

// walk reads and yields the contents of the directory at relDir. dir is that directory's Entry, or the zero Entry for
// the location itself. It returns false once the listing must end.
func (w *walker) walk(relDir string, dir vfs.Entry) bool {
	if w.canceled() {
		return false
	}
	var children []Child
	err := attempt(w.ctx, w.retry, func() (err error) {
		children, err = w.readDir(w.ctx, relDir)
		return err
	})
	if err != nil {
		return w.readFailed(relDir, dir, err)
	}

	items, invalid := w.items(relDir, children)
	for _, err := range invalid {
		if !w.fail(vfs.Entry{}, err) {
			return false
		}
	}
	slices.SortFunc(items, func(a, b walkItem) int { return strings.Compare(a.key, b.key) })
	for i := range items {
		if !w.visit(&items[i]) {
			return false
		}
	}
	return true
}

// readFailed handles a failed directory read and returns whether the walk goes on.
func (w *walker) readFailed(relDir string, dir vfs.Entry, err error) bool {
	if w.canceled() {
		return false
	}
	if relDir != "" {
		return w.fail(dir, listError(dir.Path(), err))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		w.fail(vfs.Entry{}, listError(w.parent.Path(), err))
	}
	return false
}

// items turns children into entries, and returns an error for each child with an invalid name that lies in the window.
func (w *walker) items(relDir string, children []Child) (items []walkItem, invalid []error) {
	items = make([]walkItem, 0, len(children))
	for i := range children {
		c := &children[i]
		rel := c.Name
		if relDir != "" {
			rel = relDir + "/" + c.Name
		}
		build := vfs.NewFileEntry
		key := rel
		if c.IsDir {
			build = vfs.NewDirEntry
			key += "/"
		}
		e, err := buildChild(build, w.parent, c, rel)
		if err != nil {
			if w.win.inRange(rel, c.IsDir) {
				invalid = append(invalid, invalidKey(key))
			}
			continue
		}
		items = append(items, walkItem{rel: rel, key: key, dir: c.IsDir, entry: e})
	}
	return items, invalid
}

// buildChild builds the Entry for c. The entry constructors accept any valid relative path, so the child's name is
// checked here: it must be a single path segment.
func buildChild(
	build func(vfs.Location, string, ...vfs.EntryOption) (vfs.Entry, error), parent vfs.Location, c *Child, rel string,
) (vfs.Entry, error) {
	if c.Name == "" || c.Name == "." || c.Name == ".." || strings.Contains(c.Name, "/") {
		return vfs.Entry{}, &fs.PathError{Op: "list", Path: rel, Err: fs.ErrInvalid}
	}
	return build(parent, rel, c.Options...)
}

// visit yields one entry and, for a directory that must be read, walks it. It returns false once the listing must end.
func (w *walker) visit(it *walkItem) bool {
	if w.win.wantsEntry(it.rel, it.dir) && !w.entry(it.entry) {
		return false
	}
	if !it.dir || !w.win.opens(it.rel) {
		return true
	}
	if w.descend != nil {
		keep, err := w.descend(w.ctx, it.entry)
		if err != nil {
			w.fail(vfs.Entry{}, listError(it.entry.Path(), err))
			return false
		}
		if !keep {
			return true
		}
	}
	return w.walk(it.rel, it.entry)
}
