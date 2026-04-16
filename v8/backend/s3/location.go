package s3

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

var (
	errLocationRequired = errors.New("non-nil s3.Location pointer is required")
	errPathRequired     = errors.New("non-empty string for path is required")
)

// Location implements [vfs.Location] for S3.
type Location struct {
	fileSystem *FileSystem
	prefix     string
	authority  authority.Authority
}

// List lists objects under this location using the S3 ListObjects API (see [vfs.ListOption]).
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
	var keys []string
	var err error
	if cfg.Prefix != "" {
		searchPrefix := utils.RemoveLeadingSlash(path.Join(l.prefix, cfg.Prefix))
		d := path.Dir(searchPrefix)
		listObjectsInput := l.getListObjectsInput()
		listObjectsInput.Prefix = aws.String(searchPrefix)
		keys, err = l.fullLocationList(ctx, listObjectsInput, d)
	} else {
		prefix := utils.RemoveLeadingSlash(l.prefix)
		listObjectsInput := l.getListObjectsInput()
		listObjectsInput.Prefix = aws.String(utils.EnsureTrailingSlash(prefix))
		keys, err = l.fullLocationList(ctx, listObjectsInput, prefix)
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

// Authority returns the bucket the location is contained in.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the prefix the location references in most s3 calls.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.prefix))
}

// Exists returns true if the bucket exists, and the user in the underlying s3.fileSystem.Client() has the appropriate
// permissions. Will receive false without an error if the bucket simply doesn't exist. Otherwise could receive
// false and any errors passed back from the API.
func (l *Location) Exists() (bool, error) {
	headBucketInput := &s3.HeadBucketInput{Bucket: aws.String(l.Authority().String())}
	client, err := l.fileSystem.Client()
	if err != nil {
		return false, err
	}
	_, err = client.HeadBucket(context.Background(), headBucketInput)
	if err != nil {
		var terr *types.NotFound
		if errors.As(err, &terr) {
			return false, nil
		}
		return false, err
	}

	return true, err
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

// NewFile uses the properties of the calling location to generate a [vfs.File]. The filePath
// argument is expected to be a relative path to the location's current path.
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

	newFile := &File{
		location: newLocation.(*Location),
		key:      utils.RemoveLeadingSlash(path.Join(l.prefix, relFilePath)),
		opts:     opts,
	}
	return newFile, nil
}

// DeleteFile removes the object at fileName.
func (l *Location) DeleteFile(fileName string, opts ...vfsopt.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}
	sf, ok := file.(*File)
	if !ok {
		return fmt.Errorf("s3: unexpected file implementation %T", file)
	}
	return sf.deleteObject(opts...)
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

func (l *Location) fullLocationList(ctx context.Context, input *s3.ListObjectsInput, prefix string) ([]string, error) {
	var keys []string
	client, err := l.fileSystem.Client()
	if err != nil {
		return keys, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		listObjectsOutput, err := client.ListObjects(ctx, input)
		if err != nil {
			return nil, err
		}
		newKeys := getNamesFromObjectSlice(listObjectsOutput.Contents, utils.EnsureTrailingSlash(utils.RemoveLeadingSlash(prefix)))
		keys = append(keys, newKeys...)

		if *listObjectsOutput.IsTruncated {
			input.Marker = listObjectsOutput.NextMarker
		} else {
			break
		}
	}

	return keys, nil
}

func (l *Location) getListObjectsInput() *s3.ListObjectsInput {
	return &s3.ListObjectsInput{
		Bucket:    aws.String(l.Authority().String()),
		Delimiter: aws.String("/"),
	}
}

func getNamesFromObjectSlice(objects []types.Object, locationPrefix string) []string {
	var keys []string
	for _, object := range objects {
		if *object.Key != locationPrefix {
			keys = append(keys, strings.TrimPrefix(*object.Key, locationPrefix))
		}
	}
	return keys
}

var _ vfs.Location = (*Location)(nil)
