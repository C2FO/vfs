package azure

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/azure/types"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

var errNoBlobLister = errors.New("the azure client does not implement types.BlobLister")

// listPageSize is the largest page a listing asks Azure for, so a page costs one request.
const listPageSize = 1000

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Blobs are read one page of up to 1,000 at a time, and a page is requested only when the listing needs more, so a
// listing that stops early stops its requests. The size and modification time of every file come from the listing
// itself, so reading them makes no further request, and Entry.Sys returns the listed *container.BlobItem. A
// non-recursive listing is a hierarchy listing that rolls blobs up at "/", and a recursive listing is a flat listing.
// A directory entry exists wherever a blob name has a "/" below the listed location, whether or not a placeholder blob
// exists.
//
// Azure cannot start a listing after a given name, so a listing that starts after a cursor, such as one that resumes
// with list.After, reads the blobs before the cursor and drops them. The cost of such a listing grows with the
// cursor's position. Listing relies on Azure returning blobs in ascending byte order; a listing that does not ends with
// an error instead of wrong results.
//
// The Client must also implement types.BlobLister, as the default client does; otherwise the listing fails with an error
// matching errors.ErrUnsupported. A 403 response is reported as an error matching fs.ErrPermission, and a container
// that does not exist is an error, not an empty listing.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := list.ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		client, err := l.fileSystem.Client()
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		lister, ok := client.(types.BlobLister)
		if !ok {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: fmt.Errorf("%w: %w", errors.ErrUnsupported, errNoBlobLister)})
			return
		}
		list.FlatKeys(ctx, l, q, l.pages(lister), nil)(yield)
	}
}

// keyPrefix returns the blob name prefix of the location: "" for the container root, otherwise the path with no leading
// slash and one trailing slash.
func (l *Location) keyPrefix() string {
	prefix := utils.RemoveLeadingSlash(l.Path())
	if prefix == "" {
		return ""
	}
	return utils.EnsureTrailingSlash(prefix)
}

// pages returns the page function for FlatKeys. It makes one pager when first called and takes each page from it:
// Azure resumes with an opaque continuation marker the pager holds, so FlatKeys's StartAfter is not sent. FlatKeys drops
// the keys up to its cursor itself.
func (l *Location) pages(lister types.BlobLister) list.PageFunc {
	root := l.keyPrefix()
	var pager types.BlobPager

	return func(ctx context.Context, req list.PageRequest) (list.Page, error) {
		if pager == nil {
			p, err := lister.BlobPager(ctx, l.Authority().String(), types.BlobListRequest{
				Prefix: root + req.Prefix, Delimited: req.Delimited, PageSize: listPageSize,
			})
			if err != nil {
				return list.Page{}, classifyListError(err)
			}
			pager = p
		}

		result, err := pager.NextPage(ctx)
		if err != nil {
			return list.Page{}, classifyListError(err)
		}

		page := list.Page{Done: result.Done, Objects: make([]list.Object, 0, len(result.Blobs))}
		for _, blob := range result.Blobs {
			page.Objects = append(page.Objects, list.Object{
				Key: strings.TrimPrefix(blob.Name, root),
				Options: []vfs.EntryOption{
					vfs.WithEntryMetadata(uint64(max(blob.Size, 0)), blob.LastModified),
					vfs.WithEntrySys(blob.Native),
				},
			})
		}
		for _, prefix := range result.Prefixes {
			page.Prefixes = append(page.Prefixes, strings.TrimPrefix(prefix, root))
		}
		return page, nil
	}
}

// classifyListError makes a 403 response match fs.ErrPermission and leaves other errors as they are.
func classifyListError(err error) error {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%w: %w", iofs.ErrPermission, err)
	}
	return err
}
