package s3

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Each request to S3 is one ListObjectsV2 call of up to 1,000 keys, made only when the caller asks for more entries, so
// a listing that stops early stops its requests. The size and modification time of every file come from the listing
// itself, so reading them makes no further request, and Entry.Sys returns the listed types.Object. S3 can list a
// modification time with different precision than HeadObject reports, so a listed time can differ from
// File.LastModified by less than a second. A non-recursive
// listing asks S3 to roll keys up at "/", and a recursive listing does not. S3 has no directories, so a directory
// entry exists wherever a key has a "/" below the listed location, whether or not a folder marker object exists.
//
// Listing relies on S3 returning keys in ascending byte order, as general purpose buckets do. A listing that returns
// keys out of order, as S3 directory buckets (S3 Express One Zone) do, ends with an error instead of wrong results.
//
// An access-denied response is reported as an error matching iofs.ErrPermission. A bucket that does not exist is an
// error, not an empty listing, so a misspelled bucket is not mistaken for an empty one.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := list.ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		client, err := l.fileSystem.backendClient()
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		list.FlatKeys(ctx, l, q, l.pages(client), nil)(yield)
	}
}

// keyPrefix returns the key prefix of the location: "" for the bucket root, otherwise the path with no leading slash and
// one trailing slash.
func (l *Location) keyPrefix() string {
	prefix := utils.RemoveLeadingSlash(l.prefix)
	if prefix == "" {
		return ""
	}
	return utils.EnsureTrailingSlash(prefix)
}

// pages returns the page function for FlatKeys. Every request resumes with StartAfter rather than a continuation token,
// because FlatKeys resumes after a rolled-up directory at a key that no continuation token represents.
func (l *Location) pages(client backendClient) list.PageFunc {
	root := l.keyPrefix()
	bucket := l.Authority().String()

	return func(ctx context.Context, req list.PageRequest) (list.Page, error) {
		input := &s3.ListObjectsV2Input{Bucket: aws.String(bucket)}
		if prefix := root + req.Prefix; prefix != "" {
			input.Prefix = aws.String(prefix)
		}
		if req.Delimited {
			input.Delimiter = aws.String("/")
		}
		if req.StartAfter != "" {
			input.StartAfter = aws.String(root + req.StartAfter)
		}

		out, err := client.ListObjectsV2(ctx, input)
		if err != nil {
			return list.Page{}, classifyListError(err)
		}
		if out == nil {
			out = &s3.ListObjectsV2Output{}
		}
		return pageFromOutput(out, root), nil
	}
}

// pageFromOutput converts one response into a Page of keys relative to the location.
func pageFromOutput(out *s3.ListObjectsV2Output, root string) list.Page {
	page := list.Page{
		Objects: make([]list.Object, 0, len(out.Contents)),
		Done:    !aws.ToBool(out.IsTruncated),
	}
	for i := range out.Contents {
		object := out.Contents[i]
		if object.Key == nil {
			continue
		}
		page.Objects = append(page.Objects, list.Object{
			Key:     strings.TrimPrefix(*object.Key, root),
			Options: objectOptions(object),
		})
	}
	for _, prefix := range out.CommonPrefixes {
		if prefix.Prefix != nil {
			page.Prefixes = append(page.Prefixes, strings.TrimPrefix(*prefix.Prefix, root))
		}
	}
	return page
}

// objectOptions returns the listed metadata of an object as entry options.
func objectOptions(object types.Object) []vfs.EntryOption {
	return []vfs.EntryOption{
		vfs.WithEntryMetadata(uint64(max(aws.ToInt64(object.Size), 0)), aws.ToTime(object.LastModified)),
		vfs.WithEntrySys(object),
	}
}

// apiError is the part of an AWS API error that classifyListError reads.
type apiError interface {
	ErrorCode() string
}

// classifyListError makes an access-denied response match iofs.ErrPermission and leaves other errors as they are.
func classifyListError(err error) error {
	var api apiError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "AccessDenied", "AllAccessDisabled":
			return fmt.Errorf("%w: %w", iofs.ErrPermission, err)
		}
	}
	return err
}
