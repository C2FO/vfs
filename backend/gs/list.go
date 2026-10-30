package gs

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"net/http"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

// listPageSize is how many objects one FlatKeys page reads from the object iterator. It matches the page size of the
// GCS objects.list call, so a page costs about one request.
const listPageSize = 1000

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Objects are read from one GCS object iterator, which fetches a page of up to 1,000 objects only when the listing
// needs more, so a listing that stops early stops its requests. The size and modification time of every file come from
// the listing itself, so reading them makes no further request, and Entry.Sys returns the listed *storage.ObjectAttrs.
// A non-recursive listing asks GCS to roll objects up at "/", and a recursive listing does not. GCS has no directories
// in flat buckets, so a directory entry exists wherever an object name has a "/" below the listed location, whether or
// not a folder placeholder object exists.
//
// A 403 response is reported as an error matching fs.ErrPermission. A bucket that does not exist is an error, not an
// empty listing, so a misspelled bucket is not mistaken for an empty one.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := list.ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		handle, err := l.getBucketHandle()
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		list.FlatKeys(ctx, l, q, l.pages(handle), nil)(yield)
	}
}

// keyPrefix returns the object name prefix of the location: "" for the bucket root, otherwise the path with no leading
// slash and one trailing slash.
func (l *Location) keyPrefix() string {
	prefix := utils.RemoveLeadingSlash(l.prefix)
	if prefix == "" {
		return ""
	}
	return utils.EnsureTrailingSlash(prefix)
}

// pages returns the page function for FlatKeys. It makes one object iterator when first called and takes each page from
// it, rather than starting a new listing for each page: FlatKeys resumes immediately after the previous page, which is
// where the iterator already is, and the iterator rolls up directories itself.
func (l *Location) pages(handle BucketHandleWrapper) list.PageFunc {
	root := l.keyPrefix()
	var objects ObjectIteratorWrapper

	return func(ctx context.Context, req list.PageRequest) (list.Page, error) {
		if objects == nil {
			query := &storage.Query{Prefix: root + req.Prefix}
			if req.Delimited {
				query.Delimiter = "/"
			}
			if req.StartAfter != "" {
				// StartOffset includes the object it names; FlatKeys drops keys up to the cursor itself.
				query.StartOffset = root + req.StartAfter
			}
			objects = handle.WrappedObjects(ctx, query)
		}

		var page list.Page
		for range listPageSize {
			attrs, err := objects.Next()
			if errors.Is(err, iterator.Done) {
				page.Done = true
				return page, nil
			}
			if err != nil {
				return list.Page{}, classifyListError(err)
			}
			if attrs.Prefix != "" {
				page.Prefixes = append(page.Prefixes, strings.TrimPrefix(attrs.Prefix, root))
				continue
			}
			page.Objects = append(page.Objects, list.Object{
				Key: strings.TrimPrefix(attrs.Name, root),
				Options: []vfs.EntryOption{
					vfs.WithEntryMetadata(uint64(max(attrs.Size, 0)), attrs.Updated),
					vfs.WithEntrySys(attrs),
				},
			})
		}
		return page, nil
	}
}

// classifyListError makes a 403 response match fs.ErrPermission and leaves other errors as they are.
func classifyListError(err error) error {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusForbidden {
		return fmt.Errorf("%w: %w", iofs.ErrPermission, err)
	}
	return err
}
