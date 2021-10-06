package s3

import (
	"errors"
	"path"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

// Location implements the vfs.Location interface specific to S3 fs.
type Location struct {
	fileSystem *FileSystem
	prefix     string
	bucket     string
}

// List calls the s3 API to list all objects in the location's bucket, with a prefix automatically
// set to the location's path. This will make a call to the s3 API for every 1000 keys to return.
// If you have many thousands of keys at the given location, this could become quite expensive.
func (l *Location) List() ([]string, error) {
	prefix := utils.RemoveLeadingSlash(l.prefix)
	listObjectsInput := l.getListObjectsInput().SetPrefix(utils.EnsureTrailingSlash(prefix))
	return l.fullLocationList(listObjectsInput, prefix)
}

// ListByPrefix calls the s3 API with the location's prefix modified relatively by the prefix arg passed to the
// function. The resource considerations of List() apply to this function as well.
func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	searchPrefix := utils.RemoveLeadingSlash(path.Join(l.prefix, prefix))
	d := path.Dir(searchPrefix)
	listObjectsInput := l.getListObjectsInput().SetPrefix(searchPrefix)
	return l.fullLocationList(listObjectsInput, d)
}

// ListByRegex retrieves the keys of all the files at the location's current path, then filters out all those
// that don't match the given regex. The resource considerations of List() apply here as well.
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

// Volume returns the bucket the location is contained in.
func (l *Location) Volume() string {
	return l.bucket
}

// Path returns the prefix the location references in most s3 calls.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.prefix))
}

// Exists returns true if the bucket exists, and the user in the underlying s3.fileSystem.Client() has the appropriate
// permissions. Will receive false without an error if the bucket simply doesn't exist. Otherwise could receive
// false and any errors passed back from the API.
func (l *Location) Exists() (bool, error) {
	headBucketInput := new(s3.HeadBucketInput).SetBucket(l.bucket)
	client, err := l.fileSystem.Client()
	if err != nil {
		return false, err
	}
	_, err = client.HeadBucket(headBucketInput)
	if err != nil {
		if err.(awserr.Error).Code() == s3.ErrCodeNoSuchBucket {
			return false, nil
		}
		return false, err
	}

	return true, err
}

// NewLocation makes a copy of the underlying Location, then modifies its path by calling ChangeDir with the
// relativePath argument, returning the resulting location. The only possible errors come from the call to
// ChangeDir, which, for the s3 implementation doesn't ever result in an error.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	if l == nil {
		return nil, errors.New("non-nil s3.Location pointer is required")
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

// ChangeDir takes a relative path, and modifies the underlying Location's path. The caller is modified by this
// so the only return is any error. For this implementation there are no errors.
func (l *Location) ChangeDir(relativePath string) error {
	if l == nil {
		return errors.New("non-nil s3.Location pointer is required")
	}
	if relativePath == "" {
		return errors.New("non-empty string relativePath is required")
	}
	err := utils.ValidateRelativeLocationPath(relativePath)
	if err != nil {
		return err
	}
	l.prefix = utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(path.Join(l.prefix, relativePath)))
	return nil
}

// NewFile uses the properties of the calling location to generate a vfs.File (backed by an s3.File). The filePath
// argument is expected to be a relative path to the location's current path.
func (l *Location) NewFile(filePath string) (vfs.File, error) {
	if l == nil {
		return nil, errors.New("non-nil s3.Location pointer is required")
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

// DeleteFile removes the file at fileName path.
func (l *Location) DeleteFile(fileName string) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}

	return file.Delete()
}

// FileSystem returns a vfs.FileSystem interface of the location's underlying file system.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return utils.GetLocationURI(l)
}

// String implement fmt.Stringer, returning the location's URI as the default string.
func (l *Location) String() string {
	return l.URI()
}

/*
	Private helpers
*/

func (l *Location) fullLocationList(input *s3.ListObjectsInput, prefix string) ([]string, error) {
	var keys []string
	client, err := l.fileSystem.Client()
	if err != nil {
		return keys, err
	}
	for {
		listObjectsOutput, err := client.ListObjects(input)
		if err != nil {
			return []string{}, err
		}
		newKeys := getNamesFromObjectSlice(listObjectsOutput.Contents, utils.EnsureTrailingSlash(utils.RemoveLeadingSlash(prefix)))
		keys = append(keys, newKeys...)

		// if s3 response "IsTruncated" we need to call List again with
		// an updated Marker (s3 version of paging)
		if *listObjectsOutput.IsTruncated {
			input.SetMarker(*listObjectsOutput.NextMarker)
		} else {
			break
		}
	}

	return keys, nil
}

func (l *Location) getListObjectsInput() *s3.ListObjectsInput {
	return new(s3.ListObjectsInput).SetBucket(l.bucket).SetDelimiter("/")
}

func getNamesFromObjectSlice(objects []*s3.Object, locationPrefix string) []string {
	var keys []string
	for _, object := range objects {
		if *object.Key != locationPrefix {
			keys = append(keys, strings.TrimPrefix(*object.Key, locationPrefix))
		}
	}
	return keys
}
