package gs

import (
	"github.com/c2fo/goutils/errorstack"
	"path"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/c2fo/vfs/v3"
	"github.com/c2fo/vfs/v3/utils"
)

// Location implements vfs.Location for gs fs.
type Location struct {
	fileSystem   *FileSystem
	prefix       string
	bucket       string
	bucketHandle *storage.BucketHandle
}

// String returns the full URI of the location.
func (l *Location) String() string {
	return l.URI()
}

// List returns a list of file name strings for the current location.
func (l *Location) List() ([]string, error) {
	return l.ListByPrefix("")
}

//ListByPrefix returns a slice of file base names and any error, if any
//prefix means filename prefix and therefore should not have slash
//List functions return only files
//List functions return only basenames
func (l *Location) ListByPrefix(filenamePrefix string) ([]string, error) {
	q := &storage.Query{
		Delimiter: "/",
		Prefix:    l.prefix + filenamePrefix,
		Versions:  false,
	}

	handle, err := l.getBucketHandle()
	if err != nil {
		return nil, err
	}
	var fileNames []string

	if err := l.fileSystem.Retry()(func() error {

		it := handle.Objects(l.fileSystem.ctx, q)
		for {
			objAttrs, err := it.Next()
			if err != nil {
				if err == iterator.Done {
					break
				}
				return err
			}
			//only include objects, not "directories"
			if objAttrs.Prefix == "" && objAttrs.Name != l.prefix {
				fileNames = append(fileNames, strings.TrimPrefix(objAttrs.Name, l.prefix))
			}
		}

		return nil
	}); err != nil {
		return nil, err
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
	if strings.Index(l.prefix, "/") == 0 {
		return utils.EnsureTrailingSlash(l.prefix)
	}
	return "/" + utils.EnsureTrailingSlash(l.prefix)
}

// Exists returns whether the location exists or not. In the case of an error, false is returned.
func (l *Location) Exists() (bool, error) {
	_, err := l.getBucketAttrs()
	if err != nil {
		if err.Error() == doesNotExistError {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NewLocation creates a new location instance relative to the current location's path.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	newLocation := *l
	err := newLocation.ChangeDir(relativePath)
	if err != nil {
		return nil, err
	}
	return &newLocation, nil
}

// ChangeDir changes the current location's path to the new, relative path.
func (l *Location) ChangeDir(relativePath string) error {
	newPrefix := path.Join(l.prefix, relativePath)
	l.prefix = utils.EnsureTrailingSlash(utils.CleanPrefix(newPrefix))
	return nil
}

// FileSystem returns the GCS file system instance.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile returns a new file instance at the given path, relative to the current location.
func (l *Location) NewFile(filePath string) (vfs.File, error) {
	return newFile(l.fileSystem, l.bucket, path.Join(l.prefix, filePath))
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
func (l *Location) getBucketHandle() (*storage.BucketHandle, error) {
	if l.bucketHandle != nil {
		return l.bucketHandle, nil
	}

	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	l.bucketHandle = client.Bucket(l.bucket)
	return l.bucketHandle, nil
}

// getObjectAttrs returns the file's attributes
func (l *Location) getBucketAttrs() (*storage.BucketAttrs, error) {
	var attrs *storage.BucketAttrs
	if err := l.fileSystem.Retry()(func() error {
		handle, err := l.getBucketHandle()
		if err != nil {
			return err
		}

		attrs, err = handle.Attrs(l.fileSystem.ctx)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return attrs, nil
}
