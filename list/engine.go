package list

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/c2fo/vfs/v7"
)

// window is the part of a Query that both engines apply themselves: which entries lie inside the requested prefix,
// cursor and depth, and which directories therefore need to be read at all.
type window struct {
	prefix string
	after  string
	depth  int
}

func newWindow(q Query) window {
	return window{prefix: q.Prefix, after: q.StartAfter, depth: q.Depth()}
}

// descender returns the function that decides which directories are descended into: q.Descend for a recursive listing,
// and nil otherwise. A non-recursive listing with a nested prefix reads the directories on the way to the prefix, and
// those are not pruned.
func descender(q Query) EntryFunc {
	if !q.Recursive {
		return nil
	}
	return q.Descend
}

// segments returns the depth of the entry at rel.
func segments(rel string) int { return strings.Count(rel, "/") + 1 }

// inRange reports whether the entry at rel, a directory when dir is true, lies after the cursor and inside the prefix.
// It ignores depth, which a caller that knows the entry's real depth checks itself.
func (w window) inRange(rel string, dir bool) bool {
	key := rel
	if dir {
		key += "/"
	}
	if w.after != "" && key <= w.after {
		return false
	}
	return strings.HasPrefix(rel, w.prefix)
}

// wantsEntry reports whether the entry at rel, a directory when dir is true, is inside the window and so is yielded.
func (w window) wantsEntry(rel string, dir bool) bool {
	return w.inRange(rel, dir) && (w.depth == 0 || segments(rel) <= w.depth)
}

// opens reports whether the directory at rel must be read: it can contain entries after the cursor, it is an ancestor
// of the prefix or lies inside it, and its children are not deeper than the depth limit.
func (w window) opens(rel string) bool {
	dirKey := rel + "/"
	if w.after > dirKey && !strings.HasPrefix(w.after, dirKey) {
		return false
	}
	if !strings.HasPrefix(dirKey, w.prefix) && !strings.HasPrefix(w.prefix, dirKey) {
		return false
	}
	return w.depth == 0 || segments(rel) < w.depth
}

// listError wraps err as the *fs.PathError every listing error is, leaving an error that already is one alone.
func listError(path string, err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Op == "list" {
		return err
	}
	return &fs.PathError{Op: "list", Path: path, Err: err}
}

// RetryFunc wraps one backend request in the file system's retry policy. A vfs.Retry value, such as the one a file
// system's Retry method returns, is a RetryFunc. A nil RetryFunc runs each request once.
type RetryFunc = func(wrapped func() error) error

// attempt runs fn through retry, but makes retried attempts return ctx.Err() without calling fn once ctx is done.
// A RetryFunc takes no context, so a retrier would otherwise keep retrying a canceled listing.
func attempt(ctx context.Context, retry RetryFunc, fn func() error) error {
	run := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn()
	}
	if retry == nil {
		return run()
	}
	return retry(run)
}

// emitter yields to the caller, checking the context before each yield.
type emitter struct {
	ctx    context.Context //nolint:containedctx // scoped to one listing, never stored beyond the iterator's run
	parent vfs.Location
	yield  func(vfs.Entry, error) bool
}

// entry yields e, or yields the cancellation error and returns false if the context is done.
func (m emitter) entry(e vfs.Entry) bool {
	if m.canceled() {
		return false
	}
	return m.yield(e, nil)
}

// fail yields err with e, which is the zero Entry for a failure that ends the listing.
func (m emitter) fail(e vfs.Entry, err error) bool {
	return m.yield(e, err)
}

// canceled yields the context's error and reports true if the context is done.
func (m emitter) canceled() bool {
	err := m.ctx.Err()
	if err == nil {
		return false
	}
	m.yield(vfs.Entry{}, listError(m.parent.Path(), err))
	return true
}

// invalidKey is the non-fatal error for a key or name that cannot be an Entry.
func invalidKey(key string) error {
	return &fs.PathError{Op: "list", Path: key, Err: fs.ErrInvalid}
}
