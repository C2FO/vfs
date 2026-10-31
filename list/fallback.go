package list

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"path"
	"slices"
	"strings"

	"github.com/c2fo/vfs/v7"
)

// fallback lists a location that has no native listing, using its List and ListByPrefix methods. It yields files only.
// Names arrive as strings, so no size or modification time is known and no per-file request is made; an entry's Info
// asks for them when called.
func fallback(ctx context.Context, loc vfs.Location, q Query) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		if reason := unsupportedWithoutNativeListing(q); reason != "" {
			yield(vfs.Entry{}, &fs.PathError{
				Op:   "list",
				Path: loc.Path(),
				Err:  fmt.Errorf("%w: %s needs a backend with native listing", errors.ErrUnsupported, reason),
			})
			return
		}
		if err := ctx.Err(); err != nil {
			yield(vfs.Entry{}, listError(loc.Path(), err))
			return
		}

		dir, names, err := fallbackNames(loc, q.Prefix)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				yield(vfs.Entry{}, listError(loc.Path(), err))
			}
			return
		}
		slices.Sort(names)
		for _, name := range names {
			rel := dir + name
			e, err := vfs.NewFileEntry(loc, rel)
			if err != nil {
				if !yield(vfs.Entry{}, invalidKey(rel)) {
					return
				}
				continue
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

// unsupportedWithoutNativeListing names the first option in q that needs directories, or returns "".
func unsupportedWithoutNativeListing(q Query) string {
	switch {
	case strings.Contains(q.Glob, "/"):
		return `a Glob containing "/"`
	case q.MaxDepth > 0:
		return "MaxDepth"
	case q.Recursive:
		return "Recursive"
	case q.Kind == KindDirs:
		return "DirsOnly"
	}
	return ""
}

// fallbackNames returns the file names a plain listing of loc finds for prefix, and the directory, relative to loc,
// they are in. A prefix containing "/" names a sub-location, as ListByPrefix does for such a prefix today.
func fallbackNames(loc vfs.Location, prefix string) (dir string, names []string, err error) {
	dir, base := path.Split(prefix)
	target := loc
	if dir != "" {
		if target, err = loc.NewLocation(dir); err != nil {
			return "", nil, fmt.Errorf("resolving %q: %w", dir, err)
		}
	}
	if base == "" {
		names, err = target.List() //nolint:staticcheck // the fallback exists to serve backends that only have the legacy methods
	} else {
		names, err = target.ListByPrefix(base) //nolint:staticcheck // the fallback exists to serve backends that only have the legacy methods
	}
	return dir, names, err
}
