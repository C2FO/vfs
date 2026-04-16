package gs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

var (
	errLocationRequired = errors.New("non-nil gs.Location pointer is required")
	errPathRequired     = errors.New("non-empty string for path is required")
)

// Location implements [vfs.Location] for Google Cloud Storage.
type Location struct {
	fileSystem   *FileSystem
	prefix       string
	bucketHandle BucketHandleWrapper
	authority    authority.Authority
}

// List lists objects under this location using the GCS Objects API (see [vfs.ListOption]).
func (l *Location) List(ctx context.Context, opts ...vfs.ListOption) iter.Seq2[vfs.Entry, error] {
	cfg := vfs.ApplyListOptions(vfs.ListConfig{}, opts...)

	return func(yield func(vfs.Entry, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(vfs.Entry{}, err)
			return
		}
		if cfg.Recursive {
			yield(vfs.Entry{}, vfs.ErrNotSupported)
			return
		}

		names, err := l.listNames(ctx, cfg)
		if err != nil {
			yield(vfs.Entry{}, err)
			return
		}

		for _, name := range names {
			if err := ctx.Err(); err != nil {
				yield(vfs.Entry{}, err)
				return
			}
			if cfg.Matcher != nil && !cfg.Matcher(name) {
				continue
			}
			if !yield(vfs.Entry{Kind: vfs.EntryBlob, Name: name}, nil) {
				return
			}
		}
	}
}

func (l *Location) listNames(ctx context.Context, cfg vfs.ListConfig) ([]string, error) {
	filenamePrefix := cfg.Prefix
	prefix := utils.RemoveLeadingSlash(path.Join(l.prefix, filenamePrefix))
	if filenamePrefix == "" || strings.HasSuffix(filenamePrefix, "/") {
		prefix = utils.EnsureTrailingSlash(prefix)
	}
	if prefix == "/" {
		prefix = ""
	}
	d := path.Dir(prefix)
	q := &storage.Query{
		Delimiter: "/",
		Prefix:    prefix,
		Versions:  false,
	}

	handle, err := l.getBucketHandle()
	if err != nil {
		return nil, err
	}
	var fileNames []string

	it := handle.WrappedObjects(l.fileSystem.ctx, q)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		objAttrs, err := it.Next()
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}
			return nil, err
		}
		if objAttrs.Prefix == "" && objAttrs.Name != d && !strings.HasSuffix(objAttrs.Name, "/") {
			fn := strings.TrimPrefix(objAttrs.Name, utils.EnsureTrailingSlash(d))
			fileNames = append(fileNames, fn)
		}
	}

	return fileNames, nil
}

// Authority returns the bucket the location is contained in.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the prefix the location references in GCS calls.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.prefix))
}

// Exists returns whether the bucket exists and is accessible.
func (l *Location) Exists() (bool, error) {
	_, err := l.getBucketAttrs()
	if err != nil {
		if errors.Is(err, storage.ErrBucketNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NewLocation returns a new location by resolving relativePath against this location.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if relativePath == "" {
		return nil, errPathRequired
	}

	if err := utils.ValidateRelativeLocationPath(relativePath); err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: l.fileSystem,
		prefix:     path.Join(l.prefix, relativePath),
		authority:  l.Authority(),
	}, nil
}

// NewFile uses the properties of the calling location to generate a [vfs.File].
func (l *Location) NewFile(relFilePath string, opts ...vfsopt.NewFileOption) (vfs.File, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if relFilePath == "" {
		return nil, errPathRequired
	}

	if err := utils.ValidateRelativeFilePath(relFilePath); err != nil {
		return nil, err
	}

	newLocation, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(relFilePath)))
	if err != nil {
		return nil, err
	}

	return &File{
		location: newLocation.(*Location),
		key:      utils.EnsureLeadingSlash(path.Join(l.prefix, relFilePath)),
		opts:     opts,
	}, nil
}

// DeleteFile removes the object at fileName.
func (l *Location) DeleteFile(fileName string, opts ...vfsopt.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}
	gf, ok := file.(*File)
	if !ok {
		return fmt.Errorf("gs: unexpected file implementation %T", file)
	}
	return gf.deleteObject(opts...)
}

// FileSystem returns the underlying [vfs.FileSystem].
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return fmt.Sprintf("%s://%s%s", l.FileSystem().Scheme(), l.Authority().String(), l.Path())
}

// String implement fmt.Stringer, returning the location's URI as the default string.
func (l *Location) String() string {
	return l.URI()
}

// getBucketHandle returns cached Bucket handle for the location.
func (l *Location) getBucketHandle() (BucketHandleWrapper, error) {
	if l.bucketHandle != nil {
		return l.bucketHandle, nil
	}

	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	handler := &RetryBucketHandler{Retry: l.fileSystem.retryer, handler: client.Bucket(l.Authority().String())}
	l.bucketHandle = handler
	return l.bucketHandle, nil
}

// getBucketAttrs returns the bucket's attributes.
func (l *Location) getBucketAttrs() (*storage.BucketAttrs, error) {
	handle, err := l.getBucketHandle()
	if err != nil {
		return nil, err
	}

	return handle.Attrs(l.fileSystem.ctx)
}

var _ vfs.Location = (*Location)(nil)
