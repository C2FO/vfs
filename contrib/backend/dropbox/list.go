package dropbox

import (
	"context"
	"fmt"
	iofs "io/fs"
	"iter"
	"strings"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

var _ vfs.EntryLister = (*Location)(nil)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// A folder is read with one list_folder request, followed by list_folder/continue requests while Dropbox reports more
// entries, when the listing reaches it; a recursive listing reads one folder at a time, depth first, so memory use is
// one folder per level of depth and a listing that stops early stops its requests. Dropbox's own recursive mode is not
// used because it does not guarantee an order. The size and server modification time of every file come from the
// listing, the same time File.LastModified reports, so reading them makes no further request, and Entry.Sys returns
// the listed *files.FileMetadata.
//
// Entry names are the display names Dropbox reports. Dropbox paths are case-insensitive, so a location written with
// different letter case than the folder lists the same entries, with the folder's own spelling.
//
// A folder that cannot be read is yielded with its entry and an error, and the listing continues; a folder with
// restricted content matches fs.ErrPermission, and a location that does not exist lists nothing. A location that is a
// file is an error. Dropbox limits the rate of requests, and the SDK does not retry them, so a very large recursive
// listing can see rate limit errors; each is reported for the folder it hit.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return list.Native(l, opts, func(q list.Query, yield func(vfs.Entry, error) bool) {
		list.WalkDirs(ctx, l, q, l.readDir, l.fileSystem.Retry())(yield)
	})
}

// readDir reads every page of one folder of the location.
func (l *Location) readDir(_ context.Context, relDir string) ([]list.Child, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	// Dropbox takes a folder path without a trailing slash, and "" for the root.
	dir := strings.TrimSuffix(l.Path()+relDir, "/")
	result, err := client.ListFolder(&files.ListFolderArg{Path: dir})
	if err != nil {
		return nil, wrapListError(err)
	}

	var children []list.Child
	for {
		children = appendChildren(children, result.Entries)
		if !result.HasMore {
			return children, nil
		}
		if result, err = client.ListFolderContinue(&files.ListFolderContinueArg{Cursor: result.Cursor}); err != nil {
			return nil, wrapListError(err)
		}
	}
}

// appendChildren adds the files and folders among entries to children; deleted entries have no place in a listing.
func appendChildren(children []list.Child, entries []files.IsMetadata) []list.Child {
	for _, entry := range entries {
		switch m := entry.(type) {
		case *files.FolderMetadata:
			children = append(children, list.Child{Name: m.Name, IsDir: true})
		case *files.FileMetadata:
			children = append(children, list.Child{Name: m.Name, Options: []vfs.EntryOption{
				vfs.WithEntryMetadata(m.Size, time.Time(m.ServerModified)),
				vfs.WithEntrySys(m),
			}})
		}
	}
	return children
}

// wrapListError makes a missing folder match fs.ErrNotExist, which the engine reads as "nothing to list" or, for a
// folder that vanished mid-listing, as a partial failure, and a folder whose content Dropbox restricts match
// fs.ErrPermission.
func wrapListError(err error) error {
	switch {
	case isNotFoundError(err):
		return fmt.Errorf("%w: %w", iofs.ErrNotExist, err)
	case strings.Contains(err.Error(), "restricted_content"):
		return fmt.Errorf("%w: %w", iofs.ErrPermission, err)
	}
	return err
}
