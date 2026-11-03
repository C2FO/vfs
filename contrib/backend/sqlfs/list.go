package sqlfs

import (
	"context"
	"iter"
	"strings"
	"time"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// defaultListPageSize is how many files one listing query returns.
const defaultListPageSize = 1000

var _ vfs.EntryLister = (*Location)(nil)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Each query returns up to 1,000 files in byte order of their path, starting after the last key of the previous page,
// and runs only when the caller asks for more entries, so a listing that stops early stops its queries. The queries are
// served by the key index that schema version 2 adds; on a database EnsureSchema has not upgraded, each query sorts the
// files below the location instead. A listing that
// skips a directory, because it is non-recursive or because of MaxDepth, DirFilter or a Glob, resumes after it instead
// of reading the files inside it. The size and modification time of every file come from the query itself, so reading
// them makes no further query.
//
// Directories are virtual: a directory entry exists wherever a file's path has a "/" below the listed location, and a
// location without files lists nothing. Each query is bounded by the operation timeout; a failed query ends the
// listing with an error.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return list.Native(l, opts, func(q list.Query, yield func(vfs.Entry, error) bool) {
		list.FlatKeys(ctx, l, q, l.pages, nil)(yield)
	})
}

// pages is the page function for FlatKeys. It ignores PageRequest.Delimited, so a non-recursive listing receives the
// first file inside each subdirectory and FlatKeys resumes after that subdirectory with a new query.
func (l *Location) pages(ctx context.Context, req list.PageRequest) (list.Page, error) {
	fsys := l.fileSystem
	prefix := l.path + req.Prefix
	after := ""
	if req.StartAfter != "" {
		after = l.path + req.StartAfter
	}

	ctx, cancel := fsys.opCtx(ctx)
	defer cancel()
	rows, err := fsys.q.QueryContext(ctx, fsys.qs.listKeys, l.authority.String(), prefix, after, fsys.listPageSize)
	if err != nil {
		return list.Page{}, fsys.wrapErr("list "+l.URI(), err)
	}
	defer func() { _ = rows.Close() }()

	var page list.Page
	n := 0
	for rows.Next() {
		n++
		var dir, name string
		var size, modifiedAt int64
		if err := rows.Scan(&dir, &name, &size, &modifiedAt); err != nil {
			return list.Page{}, fsys.wrapErr("list "+l.URI(), err)
		}
		key := dir + name
		if !strings.HasPrefix(key, prefix) {
			page.Done = true
			break
		}
		page.Objects = append(page.Objects, list.Object{
			Key:     key[len(l.path):],
			Options: []vfs.EntryOption{vfs.WithEntryMetadata(uint64(max(size, 0)), time.Unix(0, modifiedAt))},
		})
	}
	if err := rows.Err(); err != nil {
		return list.Page{}, fsys.wrapErr("list "+l.URI(), err)
	}
	if n < fsys.listPageSize {
		page.Done = true
	}
	return page, nil
}
