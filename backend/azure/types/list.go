package types

import (
	"context"
	"time"
)

// BlobLister is an optional interface a Client implements to support native listing, through Location.ListEntries.
// Client itself is unchanged so that existing implementations keep working; a Client that does not also implement
// BlobLister cannot be listed with list.Entries.
type BlobLister interface {
	// BlobPager returns a pager over the blobs of a container that match req. Creating a pager makes no request.
	BlobPager(ctx context.Context, containerName string, req BlobListRequest) (BlobPager, error)
}

// BlobListRequest describes the listing a BlobPager performs.
type BlobListRequest struct {
	// Prefix is the blob name prefix every returned blob starts with. It is empty for the whole container.
	Prefix string
	// Delimited reports whether to roll blobs up at the next "/" after Prefix and return the rolled-up virtual
	// directories as BlobPage.Prefixes, as a hierarchy listing does. When false every blob under Prefix is returned.
	Delimited bool
	// PageSize is the largest number of blobs and prefixes a page holds. Zero lets the service choose.
	PageSize int32
}

// BlobPager reads a blob listing one page at a time, each call making one request.
type BlobPager interface {
	// NextPage returns the next page of the listing. Blobs and prefixes are each in ascending byte order. The last
	// page has Done set; calling NextPage after that returns an empty, done page.
	NextPage(ctx context.Context) (BlobPage, error)
}

// BlobPage is one page of a blob listing.
type BlobPage struct {
	// Blobs are the blobs of the page.
	Blobs []BlobItem
	// Prefixes are the rolled-up virtual directories of a delimited listing, each ending in "/".
	Prefixes []string
	// Done reports that no page follows.
	Done bool
}

// BlobItem is one listed blob.
type BlobItem struct {
	// Name is the full blob name, including its virtual directory path.
	Name string
	// Size is the blob's size in bytes.
	Size int64
	// LastModified is when the blob was last modified.
	LastModified time.Time
	// Native is the service's own representation of the blob, available to callers through Entry.Sys.
	Native any
}
