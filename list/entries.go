package list

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"path"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
)

// Entries lists the contents of loc as a stream of entries in ascending byte order of [OrderKey], fetching from the
// backend as the caller iterates. The location itself is never yielded, and a location that does not exist yields
// nothing. Calling Entries performs no I/O; the listing starts when the returned sequence is ranged over.
//
//	for e, err := range list.Entries(ctx, loc, list.Recursive(), list.FilesOnly()) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(e.RelPath())
//	}
//
// Options are validated first. An invalid option or combination is yielded as one error that matches
// [ErrInvalidOption], and no request is made. Each option may be given at most once, so a caller that adds default
// options of its own must not repeat them.
//
// If loc implements vfs.EntryLister the backend lists natively and every option is applied again to what it yields, so
// a backend that ignores an option as a hint is still correct. A callback given to [DirFilter] or [DirFilterContext]
// may therefore be called more than once for the same directory, once by the backend and once here, and should be
// cheap and free of side effects.
//
// Otherwise Entries falls back to the location's List and ListByPrefix methods. That path yields files only, in byte
// order, without size or modification time (see vfs.Entry.HasMetadata), and it reports [errors.ErrUnsupported] for
// options that need directories: [Recursive], [MaxDepth] and [DirsOnly], and a [Glob] containing "/". [DirFilter] and
// [DirFilterContext] only matter to recursive listings, so they have no effect there. A location that does not exist
// yields nothing; on this path that relies on the location's List returning an error that matches fs.ErrNotExist.
// Because those methods accept no context, cancellation on the fallback path takes effect only between yielded
// entries.
//
// Errors are *fs.PathError values with Op "list". An error yielded with a zero vfs.Entry ends the listing, except one
// that matches fs.ErrInvalid, which reports a key that cannot be an entry and is followed by the rest of the listing.
// An error yielded with a directory's entry reports that directory could not be read; the listing continues with
// the next entry outside it. Errors are never hidden by filters, and do not count toward [Limit].
func Entries(ctx context.Context, loc vfs.Location, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, listError(loc.Path(), err))
			return
		}

		var src iter.Seq2[vfs.Entry, error]
		if lister, ok := loc.(vfs.EntryLister); ok {
			src = lister.ListEntries(ctx, opts...)
		} else {
			src = fallback(ctx, loc, q)
		}
		if src == nil {
			return
		}
		enforcer{ctx: ctx, loc: loc, q: q}.run(src, yield)
	}
}

// enforcer applies every option of a Query to a stream of entries, so a backend that ignored a hint is still correct.
type enforcer struct {
	ctx context.Context //nolint:containedctx // scoped to one listing, never stored beyond the iterator's run
	loc vfs.Location
	q   Query
}

// run copies src to yield through the enforcement stack: depth, pruning, cursor, prefix, glob, regexp, kind and custom
// filter, then limit. Pruning comes before the cursor so that a pruned directory that sorts before it still hides its
// subtree. Errors bypass every stage.
func (n enforcer) run(src iter.Seq2[vfs.Entry, error], yield func(vfs.Entry, error) bool) {
	pruned := ""
	count := 0
	for e, err := range src {
		if err != nil {
			if !yield(e, err) || fatal(e, err) {
				return
			}
			continue
		}

		keep, err := n.admit(e, &pruned)
		if err != nil {
			yield(vfs.Entry{}, err)
			return
		}
		if !keep {
			continue
		}
		if cerr := n.ctx.Err(); cerr != nil {
			yield(vfs.Entry{}, listError(n.loc.Path(), cerr))
			return
		}
		if !yield(e, nil) {
			return
		}
		count++
		if n.q.Limit > 0 && count >= n.q.Limit {
			return
		}
	}
}

// fatal reports whether an error yielded with e ends the listing: it carries no entry and is not an unrepresentable key.
func fatal(e vfs.Entry, err error) bool {
	return e.RelPath() == "" && !errors.Is(err, fs.ErrInvalid)
}

// admit reports whether e passes every option. pruned holds the key of the directory whose subtree is being skipped.
// A callback error is returned as a fatal listing error naming e.
func (n enforcer) admit(e vfs.Entry, pruned *string) (bool, error) {
	rel := e.RelPath()
	if depth := n.q.Depth(); depth > 0 && segments(rel) > depth {
		return false, nil
	}
	if *pruned != "" {
		if strings.HasPrefix(rel, *pruned) {
			return false, nil
		}
		*pruned = ""
	}
	// Pruning comes before the cursor so that a directory sorting before the cursor still prunes the entries after it.
	if err := n.prune(e, pruned); err != nil {
		return false, err
	}
	if n.q.StartAfter != "" && OrderKey(e) <= n.q.StartAfter {
		return false, nil
	}
	return n.matches(e)
}

// prune asks Descend whether the subtree of directory e is to be listed, and records it in pruned when not.
func (n enforcer) prune(e vfs.Entry, pruned *string) error {
	if !e.IsDir() || !n.q.Recursive || n.q.Descend == nil {
		return nil
	}
	if depth := n.q.Depth(); depth > 0 && segments(e.RelPath()) >= depth {
		return nil
	}
	// A directory that can hold no entry inside the prefix is never listed, so it is not asked about.
	dirKey := e.RelPath() + "/"
	if !strings.HasPrefix(dirKey, n.q.Prefix) && !strings.HasPrefix(n.q.Prefix, dirKey) {
		return nil
	}
	descend, err := n.q.Descend(n.ctx, e)
	if err != nil {
		return callbackError(e, err)
	}
	if !descend {
		*pruned = e.RelPath() + "/"
	}
	return nil
}

// matches applies the options that decide whether an entry is yielded, without affecting descent.
func (n enforcer) matches(e vfs.Entry) (bool, error) {
	rel := e.RelPath()
	if !strings.HasPrefix(rel, n.q.Prefix) {
		return false, nil
	}
	if n.q.Glob != "" {
		ok, err := path.Match(n.q.Glob, rel)
		if err != nil {
			return false, listError(e.Path(), err)
		}
		if !ok {
			return false, nil
		}
	}
	if n.q.Match != nil && !n.q.Match.MatchString(rel) {
		return false, nil
	}
	if (n.q.Kind == KindFiles && e.IsDir()) || (n.q.Kind == KindDirs && !e.IsDir()) {
		return false, nil
	}
	if n.q.Filter == nil {
		return true, nil
	}
	ok, err := n.q.Filter(n.ctx, e)
	if err != nil {
		return false, callbackError(e, err)
	}
	return ok, nil
}

// callbackError is the fatal error for a filter callback that failed on e.
func callbackError(e vfs.Entry, err error) error {
	return &fs.PathError{Op: "list", Path: e.Path(), Err: err}
}
