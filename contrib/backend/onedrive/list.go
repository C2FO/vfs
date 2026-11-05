package onedrive

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"net/http"
	"path"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

var _ vfs.EntryLister = (*Location)(nil)

// errNotFolder is returned when a listing reaches an item that exists but is a file.
var errNotFolder = errors.New("onedrive: not a folder")

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// A folder is read in full, following Graph's paging, when the listing reaches it; a recursive listing reads one
// folder at a time, depth first, so memory use is one folder per level of depth and a listing that stops early stops
// its requests. The size and modification time of every file come from the listing, the same values File.Size and
// File.LastModified report, so reading them makes no further request, and Entry.Sys returns the listed DriveItem.
//
// A location that does not exist lists nothing. A location that is a file is an error. A folder that cannot be read is
// reported with its entry and the listing continues; one Graph refuses access to matches fs.ErrPermission.
//
// A 401 from Graph, such as a token that expires mid-listing, is yielded once and ends the listing, and so does
// cancellation of ctx, which is checked before every folder is read.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return list.Native(l, opts, func(q list.Query, yield func(vfs.Entry, error) bool) {
		list.WalkDirs(ctx, l, q, l.readDir, l.fileSystem.Retry())(endOnAuthFailure(yield))
	})
}

// endOnAuthFailure wraps yield so that a 401 from Graph, which would fail every remaining request the same way, is
// yielded once, without an entry, and ends the listing.
func endOnAuthFailure(yield func(vfs.Entry, error) bool) func(vfs.Entry, error) bool {
	return func(e vfs.Entry, err error) bool {
		if apiErr, ok := errors.AsType[*apiError](err); ok && apiErr.StatusCode == http.StatusUnauthorized {
			yield(vfs.Entry{}, err)
			return false
		}
		return yield(e, err)
	}
}

// readDir reads every child of one folder of the location.
func (l *Location) readDir(ctx context.Context, relDir string) ([]list.Child, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	dir := path.Join(l.oneDrivePath(), relDir)
	items, err := client.ListChildren(ctx, dir)
	if err != nil {
		return nil, wrapListError(ctx, client, dir, err)
	}

	children := make([]list.Child, 0, len(items))
	for i := range items {
		item := items[i]
		if item.IsFolder {
			children = append(children, list.Child{Name: item.Name, IsDir: true})
			continue
		}
		children = append(children, list.Child{Name: item.Name, Options: []vfs.EntryOption{
			vfs.WithEntryMetadata(uint64(max(item.Size, 0)), item.LastModified),
			vfs.WithEntrySys(&item),
		}})
	}
	return children, nil
}

// wrapListError makes a missing folder match fs.ErrNotExist, which the engine reads as "nothing to list" or, for a
// folder that vanished mid-listing, as a partial failure.
//
// Graph answers 404 both for a path that does not exist and, for the children of a file, possibly for a file, so a 404
// is checked against the item itself: a file is the location existing but not being a folder, which stays an error, and
// so does a check that fails for any other reason.
func wrapListError(ctx context.Context, client Client, dir string, err error) error {
	if errors.Is(err, ErrNotFound) {
		item, gerr := client.GetItem(ctx, dir)
		switch {
		case gerr == nil && !item.IsFolder:
			return fmt.Errorf("%w: %s: %w", errNotFolder, dir, err)
		case gerr != nil && !errors.Is(gerr, ErrNotFound):
			// the folder's state is unknown, so this is not "missing"; an expired token, for one, must end the listing
			return wrapListError(ctx, client, dir, fmt.Errorf("checking %s after it was not found: %w", dir, gerr))
		}
		return fmt.Errorf("%w: %w", iofs.ErrNotExist, err)
	}
	if isPermissionDenied(err) {
		return fmt.Errorf("%w: %w", iofs.ErrPermission, err)
	}
	return err
}

// isPermissionDenied reports whether Graph refused access to a folder. Throttling answers 429 and is retried by the
// client, so it is not matched.
func isPermissionDenied(err error) bool {
	apiErr, ok := errors.AsType[*apiError](err)
	return ok && apiErr.StatusCode == http.StatusForbidden && apiErr.Code == "accessDenied"
}
