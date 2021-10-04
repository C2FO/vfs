package gs

import (
	"errors"
	"path"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

// Location implements vfs.Location for gs fs.
type Location struct {
	fileSystem   *FileSystem
	prefix       string
	bucket       string
	bucketHandle BucketHandleWrapper
}

// String returns the full URI of the location.
func (l *Location) String() string {
	return l.URI()
}

// List returns a list of file name strings for the current location.
func (l *Location) List() ([]string, error) {
	return l.ListByPrefix("")
}

// ListByPrefix returns a slice of file base names and any error, if any
// List functions return only file basenames
func (l *Location) ListByPrefix(filenamePrefix string) ([]string, error) {
	prefix := utils.RemoveLeadingSlash(utils.EnsureTrailingSlash(path.Join(l.prefix, filenamePrefix)))
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
		objAttrs, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}
		// only include objects, not "directories"
		if objAttrs.Prefix == "" && objAttrs.Name != d && !strings.HasSuffix(objAttrs.Name, "/") {
			fn := strings.TrimPrefix(objAttrs.Name, utils.EnsureTrailingSlash(d))
			fileNames = append(fileNames, fn)
		}
	}

	return fileNames, nil
}

// ListByRegex returns a list of file names at the location which match the provided regular expression.
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	keys, err := l.List()
	if err != nil {
		return []string{}, err
	}

	var filteredKeys []string
	for _, key := range keys {
		if regex.MatchString(key) {
			filteredKeys = append(filteredKeys, key)
		}
	}
	return filteredKeys, nil
}

// Volume returns the GCS bucket name.
func (l *Location) Volume() string {
	return l.bucket
}

// Path returns the path of the file at the current location, starting with a leading '/'
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.prefix))
}

// Exists returns whether the location exists or not. In the case of an error, false is returned.
func (l *Location) Exists() (bool, error) {
	_, err := l.getBucketAttrs()
	if err != nil {
		if err == storage.ErrBucketNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NewLocation creates a new location instance relative to the current location's path.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	if l == nil {
		return nil, errors.New("non-nil gs.Location pointer is required")
	}

	// make a copy of the original location first, then ChangeDir, leaving the original location as-is
	newLocation := &Location{}
	*newLocation = *l
	err := newLocation.ChangeDir(relativePath)
	if err != nil {
		return nil, err
	}
	return newLocation, nil
}

// ChangeDir changes the current location's path to the new, relative path.
func (l *Location) ChangeDir(relativePath string) error {
	if l == nil {
		return errors.New("non-nil gs.Location pointer is required")
	}
	if relativePath == "" {
		return errors.New("non-empty string relativePath is required")
	}
	err := utils.ValidateRelativeLocationPath(relativePath)
	if err != nil {
		return err
	}
	l.prefix = utils.EnsureTrailingSlash(utils.EnsureLeadingSlash(path.Join(l.prefix, relativePath)))
	return nil
}

// FileSystem returns the GCS file system instance.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile returns a new file instance at the given path, relative to the current location.
func (l *Location) NewFile(filePath string) (vfs.File, error) {
	if l == nil {
		return nil, errors.New("non-nil gs.Location pointer is required")
	}
	if filePath == "" {
		return nil, errors.New("non-empty string filePath is required")
	}
	err := utils.ValidateRelativeFilePath(filePath)
	if err != nil {
		return nil, err
	}
	newFile := &File{
		fileSystem: l.fileSystem,
		bucket:     l.bucket,
		key:        utils.EnsureLeadingSlash(path.Join(l.prefix, filePath)),
	}
	return newFile, nil
}

// DeleteFile deletes the file at the given path, relative to the current location.
func (l *Location) DeleteFile(fileName string) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}

	return file.Delete()
}

// URI returns a URI string for the GCS location.
func (l *Location) URI() string {
	return utils.GetLocationURI(l)
}

// getBucketHandle returns cached Bucket struct for file
func (l *Location) getBucketHandle() (BucketHandleWrapper, error) {
	if l.bucketHandle != nil {
		return l.bucketHandle, nil
	}

	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	handler := &RetryBucketHandler{Retry: l.fileSystem.Retry(), handler: client.Bucket(l.bucket)}
	l.bucketHandle = handler
	return l.bucketHandle, nil
}

// getObjectAttrs returns the file's attributes
func (l *Location) getBucketAttrs() (*storage.BucketAttrs, error) {
	handle, err := l.getBucketHandle()
	if err != nil {
		return nil, err
	}

	return handle.Attrs(l.fileSystem.ctx)
}
