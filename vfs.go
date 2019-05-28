package vfs

import (
	"fmt"
	"io"
	"regexp"
	"time"
)

// FileSystem represents a filesystem with any authentication accounted for.
type FileSystem interface {
	// NewFile initializes a File on the specified volume at path 'name'. On error, nil is returned for the file.
	//
	//   * path is expected to always be absolute and therefore must begin with a separator character and may not be an
	//   empty string.  As a path to a file, 'name' may not end with a trailing separator character.
	//   * The file may or may not already exist.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * On error, nil is returned for the file.
	//   * fileName param must be a an absolute path to a file and therefore may not start or end with a separator characters.
	//     This is not to be confused with vfs.Locations' NewFile() which requires a path relative the current location.
	//   * Note that not all filesystems will have a "volume" and will therefore be "":
	//       file:///path/to/file has a volume of "" and name /path/to/file
	//       whereas
	//       s3://mybucket/path/to/file has a volume of "mybucket and name /path/to/file
	//       results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	NewFile(volume string, name string) (File, error)

	// NewLocation initializes a Location on the specified volume with the given path. On error, nil is returned
	// for the location
	//
	//   * The path may or may not already exist.  Note that on keystore filesystems like S3 or GCS, paths never truly exist.
	//   * path is expected to always be absolute and therefore must begin and end with a separator character. This is not to
	//        be confused with vfs.Locations' NewLocation() which requires a path relative to the current location.
	//
	// See NewFile for note on volume.
	NewLocation(volume string, path string) (Location, error)

	// Name returns the name of the FileSystem ie: Amazon S3, os, Google Cloud Storage, etc...
	Name() string

	// Scheme, related to Name, is the uri scheme used by the FileSystem: s3, file, gs, etc...
	Scheme() string

	// Retry will return the retry function to be used by any file system.
	Retry() Retry
}

// Location represents a filesystem path which serves as a start point for directory-like functionality.  A location may
// or may not actually exist on the filesystem.
type Location interface {
	// String() returns the fully qualified URI for the Location.  IE, file://bucket/some/path/
	fmt.Stringer

	// List returns a slice of strings representing the base names of the files found at the Location. All implementations
	// are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location. If the user
	// cares about the distinction between an empty location and a non-existent one, Location.Exists() should be checked
	// first.
	List() ([]string, error)

	// ListByPrefix returns a slice of strings representing the base names of the files found in Location whose
	// filenames match the given prefix. An empty slice will be returned even for locations that don't exist.
	ListByPrefix(prefix string) ([]string, error)

	// ListByRegex returns a slice of strings representing the base names of the files found in Location that
	// matched the given regular expression. An empty slice will be returned even for locations that don't exist.
	ListByRegex(regex *regexp.Regexp) ([]string, error)

	// Returns the volume as string.  Some filesystems may not have a volume and will return "".  In URI parlance,
	// volume equates to authority.  For example s3://mybucket/path/to/file.txt, volume would return "mybucket".
	Volume() string

	//Path returns absolute path to the Location with leading and trailing slashes, ie /some/path/to/
	Path() string

	// Exists returns boolean if the location exists on the file system. Also returns an error if any.
	Exists() (bool, error)

	// NewLocation is an initializer for a new Location relative to the existing one.
	//
	// * "relativePath" parameter may use dot (. and ..) paths and may not begin with a separator character but must
	// end with a separator character.
	//
	// For location:
	//     file:///some/path/to/
	// calling:
	//     NewLocation("../../")
	// would return a new vfs.Location representing:
	//     file:///some/
	NewLocation(relativePath string) (Location, error)

	// ChangeDir updates the existing Location's path to the provided relative path. For instance, for location:
	// file:///some/path/to/, calling ChangeDir("../../") update the location instance to file:///some/.
	//
	// relativePath may use dot (. and ..) paths and may not begin with a separator character but must end with
	// a separator character.
	//   ie., path/to/location, path/to/location/, ./path/to/location, and ./path/to/location/ are all effectively equal.
	ChangeDir(relativePath string) error

	//FileSystem returns the underlying vfs.FileSystem struct for Location.
	FileSystem() FileSystem

	// NewFile will instantiate a vfs.File instance at or relative to the current location's path.
	//
	//   * fileName param may use dot (. and ..) paths and may not begin or end with a separator character.
	//   * Resultant File path will be the shortest path name equivalent of combining the Location path and relative path, if any.
	//       ie, /tmp/dir1/ as location and fileName "newdir/./../newerdir/file.txt"
	//       results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	//   * The file may or may not already exist.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * In the case of an error, nil is returned for the file.
	NewFile(fileName string) (File, error)

	// DeleteFile deletes the file of the given name at the location. This is meant to be a short cut for
	// instantiating a new file and calling delete on that, with all the necessary error handling overhead.
	//
	// fileName may be a relative path to a file but, as a file, may not end with a separator charactier
	//   ie., path/to/file.txt, ../../other/path/to/file.text are acceptable but path/to/file.txt/ is not
	DeleteFile(fileName string) error

	// URI returns the fully qualified URI for the Location.  IE, s3://bucket/some/path/
	//
	// URI's for locations must always end with a separator character.
	URI() string
}

// File represents a file on a filesystem.  A File may or may not actually exist on the filesystem.
type File interface {
	io.Closer
	io.Reader
	io.Seeker
	io.Writer
	fmt.Stringer

	// Exists returns boolean if the file exists on the file system.  Also returns an error if any.
	Exists() (bool, error)

	// Location returns the vfs.Location for the File.
	Location() Location

	// CopyToLocation will copy the current file to the provided location. If the file already exists at the location,
	// the contents will be overwritten with the current file's contents. In the case of an error, nil is returned
	// for the file.
	CopyToLocation(location Location) (File, error)

	// CopyToFile will copy the current file to the provided file instance. If the file already exists,
	// the contents will be overwritten with the current file's contents. In the case of an error, nil is returned
	// for the file.
	CopyToFile(File) error

	// MoveToLocation will move the current file to the provided location.
	//
	//   * If the file already exists at the location, the contents will be overwritten with the current file's contents.
	//   * If the location does not exist, an attempt will be made to create it.
	//   * Upon success, a vfs.File, representing the file at the new location, will be returned.
	//   * In the case of an error, nil is returned for the file.
	//   * When moving within the same Scheme, native move/rename should be used where possible
	//
	MoveToLocation(location Location) (File, error)

	// MoveToFile will move the current file to the provided file instance. If a file with the current file's name already exists,
	// the contents will be overwritten with the current file's contents. The current instance of the file will be removed.
	MoveToFile(File) error

	// Delete unlinks the File on the filesystem.
	Delete() error

	// LastModified returns the timestamp the file was last modified (as *time.Time).
	LastModified() (*time.Time, error)

	// Size returns the size of the file in bytes.
	Size() (uint64, error)

	// Path returns absolute path (with leading slash) including filename, ie /some/path/to/file.txt
	//
	// If the directory portation of a file is desired, call
	//   someFile.Location().Path()
	Path() string

	// Name returns the base name of the file path.  For file:///some/path/to/file.txt, it would return file.txt
	Name() string

	// URI returns the fully qualified URI for the File.  IE, s3://bucket/some/path/to/file.txt
	URI() string
}

// Options are structs that contain various options specific to the filesystem
type Options interface{}

// Retry is a function that can be used to wrap any operation into a definable retry operation. The wrapped argument
// is called by the underlying VFS implementation.
//
// Ex:
// var retrier Retry = func(wrapped func() error) error {
//    var ret error
//    for i := 0; i < 5; i++ {
//       if err := wrapped(); err != nil { ret = err; continue }
//    }
//    return ret
// }
type Retry func(wrapped func() error) error

// DefaultRetryer returns a no-op retryer which simply calls the wrapped command without looping.
func DefaultRetryer() Retry {
	return func(c func() error) error { return c() }
}
