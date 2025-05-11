package gs

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Location implements vfs.Location for gs fs.
type Location struct {
	fileSystem   *FileSystem
	prefix       string
	bucketHandle BucketHandleWrapper
	authority    authority.Authority
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
	prefix := utils.RemoveLeadingSlash(path.Join(l.prefix, filenamePrefix))
	// add trailing slash to location prefix when file query prefix is empty:
	//     NewLocation("/some/path/").ListByPrefix("")
	// OR when it ended with a slash (for directory level searches):
	//     NewLocation("/some/path/").ListByPrefix("dir1/dir2/")
	// obviously we don't want to add a trailing slash if we're looking for a file prefix:
	//     NewLocation("/some/path/").ListByPrefix("dir1/MyFilePrefix")
	if filenamePrefix == "" || filenamePrefix[len(filenamePrefix)-1:] == "/" {
		prefix = utils.EnsureTrailingSlash(prefix)
	}
	// remove location prefix altogether if this is the root
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
//
// Deprecated: Use Authority instead.
//
//	authStr := loc.Authority().String()
func (l *Location) Volume() string {
	return l.Authority().String()
}

// Authority returns the Authority for the Location.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the path of the file at the current location, starting with a leading '/'
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.prefix))
}

// Exists returns whether the location exists or not. In the case of an error, false is returned.
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

// NewLocation creates a new location instance relative to the current location's path.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	if l == nil {
		return nil, errors.New("non-nil gs.Location pointer is required")
	}

	if relativePath == "" {
		return nil, errors.New("non-empty string relativePath is required")
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

// ChangeDir changes the current location's path to the new, relative path.
//
// Deprecated: Use NewLocation instead:
//
//	loc, err := loc.NewLocation("../../")
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

	newLoc, err := l.NewLocation(relativePath)
	if err != nil {
		return err
	}
	*l = *newLoc.(*Location)

	return nil
}

// FileSystem returns the GCS file system instance.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile returns a new file instance at the given path, relative to the current location.
func (l *Location) NewFile(relFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if l == nil {
		return nil, errors.New("non-nil gs.Location pointer is required")
	}

	if relFilePath == "" {
		return nil, errors.New("non-empty string filePath is required")
	}

	err := utils.ValidateRelativeFilePath(relFilePath)
	if err != nil {
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

// DeleteFile deletes the file at the given path, relative to the current location.
func (l *Location) DeleteFile(fileName string, opts ...options.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}

	return file.Delete(opts...)
}

// URI returns a URI string for the GCS location.
func (l *Location) URI() string {
	return utils.GetLocationURI(l)
}

// Open opens the named file at this location.
// This implements the fs.FS interface from io/fs.
func (l *Location) Open(name string) (fs.File, error) {
	// fs.FS expects paths with no leading slash
	name = strings.TrimPrefix(name, "/")

	// For io/fs compliance, we need to validate that it doesn't contain "." or ".." elements
	if name == "." || name == ".." || strings.Contains(name, "/.") || strings.Contains(name, "./") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	// Create a standard vfs file using NewFile
	vfsFile, err := l.NewFile(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	// Check if the file exists, as fs.FS.Open requires the file to exist
	exists, err := vfsFile.Exists()
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if !exists {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return vfsFile, nil
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
	handler := &RetryBucketHandler{Retry: l.fileSystem.retryer, handler: client.Bucket(l.Authority().String())}
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
