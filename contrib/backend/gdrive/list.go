package gdrive

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

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// A folder is read in full, with one files.list request per page of up to 1000 children, when the listing reaches it;
// a recursive listing reads one folder at a time, depth first, so memory use is one folder per level of depth and a
// listing that stops early stops its requests. The size and modification time of every file come from the listing, the
// same values File.Size and File.LastModified report, so reading them makes no further request, and Entry.Sys returns
// the listed Item. A Google-native file, such as a Google Doc, is listed as a file with size 0.
//
// A location that does not exist lists nothing. A location that is a file is an error, as is a folder name that
// matches more than one folder, which a path cannot choose between; in a recursive listing such a folder is yielded
// with its entry and an error and the listing continues. A folder that cannot be read is reported the same way, and
// one Drive refuses access to matches fs.ErrPermission. An entry whose name contains a slash cannot be part of a path
// and is reported as an error matching fs.ErrInvalid. Drive allows several files in one folder to share a name; each
// is listed.
//
// A 401 from Drive, such as a token that expires mid-listing, is yielded once and ends the listing, and so does
// cancellation of ctx, which is checked before every folder is read.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return list.Native(l, opts, func(q list.Query, yield func(vfs.Entry, error) bool) {
		list.WalkDirs(ctx, l, q, l.readDir, l.fileSystem.Retry())(endOnAuthFailure(yield))
	})
}

// endOnAuthFailure wraps yield so that a 401 from Drive, which would fail every remaining request the same way, is
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
	client, err := l.client()
	if err != nil {
		return nil, err
	}

	dir := path.Join(l.drivePath(), relDir)
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
			vfs.WithEntryMetadata(uint64(max(item.Size, 0)), item.ModTime),
			vfs.WithEntrySys(&item),
		}})
	}
	return children, nil
}

// wrapListError makes a missing folder match fs.ErrNotExist, which the engine reads as "nothing to list" or, for a
// folder that vanished mid-listing, as a partial failure.
//
// ErrNotFolder means a segment of the path is a file. When that file is the location itself the location exists but
// is not a folder, which stays an error. When it is an ancestor nothing can exist at the path, so it is missing, as
// Exists and List treat it.
func wrapListError(ctx context.Context, client Client, dir string, err error) error {
	if errors.Is(err, ErrNotFolder) {
		if _, gerr := client.GetItem(ctx, dir); isMissing(gerr) {
			return fmt.Errorf("%w: %w", iofs.ErrNotExist, err)
		}
		return err
	}
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %w", iofs.ErrNotExist, err)
	}
	if isPermissionDenied(err) {
		return fmt.Errorf("%w: %w", iofs.ErrPermission, err)
	}
	return err
}

// isPermissionDenied reports whether Drive refused access to a folder. Rate limits also answer 403, but with their own
// reasons, so they are not matched.
func isPermissionDenied(err error) bool {
	apiErr, ok := errors.AsType[*apiError](err)
	if !ok || apiErr.StatusCode != http.StatusForbidden {
		return false
	}
	switch apiErr.Reason {
	case "insufficientFilePermissions", "forbidden", "appNotAuthorizedToFile":
		return true
	}
	return false
}
