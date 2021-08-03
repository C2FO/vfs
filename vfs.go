package vfs

import (
	"fmt"
	"io"
	"regexp"
	"time"
)

// FileSystem represents a file system with any authentication accounted for.
type FileSystem interface {
	// NewFile initializes a File on the specified volume at path 'absFilePath'.
	//
	//   * Accepts volume and an absolute file path.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * On error, nil is returned for the file.
	//   * Note that not all file systems will have a "volume" and will therefore be "":
	//       file:///path/to/file has a volume of "" and name /path/to/file
	//     whereas
	//       s3://mybucket/path/to/file has a volume of "mybucket and name /path/to/file
	//     results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	//   * The file may or may not already exist.
	NewFile(volume string, absFilePath string) (File, error)

	// NewLocation initializes a Location on the specified volume with the given path.
	//
	//   * Accepts volume and an absolute location path.
	//   * The file may or may not already exist. Note that on key-store file systems like S3 or GCS, paths never truly exist.
	//   * On error, nil is returned for the location.
	//
	// See NewFile for note on volume.
	NewLocation(volume string, absLocPath string) (Location, error)

	// Name returns the name of the FileSystem ie: Amazon S3, os, Google Cloud Storage, etc.
	Name() string

	// Scheme returns the uri scheme used by the FileSystem: s3, file, gs, etc.
	Scheme() string

	// Retry will return the retry function to be used by any file system.
	Retry() Retry
}

// Location represents a file system path which serves as a start point for directory-like functionality.  A location may
// or may not actually exist on the file system.
type Location interface {
	// String returns the fully qualified absolute URI for the Location.  IE, file://bucket/some/path/
	fmt.Stringer

	// List returns a slice of strings representing the base names of the files found at the Location.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	List() ([]string, error)

	// ListByPrefix returns a slice of strings representing the base names of the files found in Location whose filenames
	// match the given prefix.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * "relative" prefixes are allowed, ie, listByPrefix from "/some/path/" with prefix "to/somepattern" is the same as
	//     location "/some/path/to/" with prefix of "somepattern"
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	ListByPrefix(prefix string) ([]string, error)

	// ListByRegex returns a slice of strings representing the base names of the files found in Location that matched the
	// given regular expression.
	//
	//   * All implementations are expected to return ([]string{}, nil) in the case of a non-existent directory/prefix/location.
	//   * If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
	//     be checked first.
	ListByRegex(regex *regexp.Regexp) ([]string, error)

	// Volume returns the volume as string. In URI parlance, volume equates to authority.
	// For example s3://mybucket/path/to/file.txt, volume would return "mybucket".
	//
	// Note: Some file systems may not have a volume and will return "".
	Volume() string

	// Path returns absolute location path, ie /some/path/to/.  An absolute path must be resolved to it's shortest path:
	// see path.Clean
	Path() string

	// Exists returns boolean if the location exists on the file system. Returns an error if any.
	Exists() (bool, error)

	// NewLocation is an initializer for a new Location relative to the existing one.
	//
	// Given location:
	//     loc := fs.NewLocation(:s3://mybucket/some/path/to/")
	// calling:
	//     newLoc := loc.NewLocation("../../")
	// would return a new vfs.Location representing:
	//     s3://mybucket/some/
	//
	//   * Accepts a relative location path.
	NewLocation(relLocPath string) (Location, error)

	// ChangeDir updates the existing Location's path to the provided relative location path.
	//
	// Given location:
	// 	   loc := fs.NewLocation("file:///some/path/to/")
	// calling:
	//     loc.ChangeDir("../../")
	// would update the current location instance to
	// file:///some/.
	//
	//   * ChangeDir accepts a relative location path.
	ChangeDir(relLocPath string) error

	// FileSystem returns the underlying vfs.FileSystem struct for Location.
	FileSystem() FileSystem

	// NewFile will instantiate a vfs.File instance at or relative to the current location's path.
	//
	//   * Accepts a relative file path.
	//   * In the case of an error, nil is returned for the file.
	//   * Resultant File path will be the shortest path name equivalent of combining the Location path and relative path, if any.
	//       ie, /tmp/dir1/ as location and relFilePath "newdir/./../newerdir/file.txt"
	//       results in /tmp/dir1/newerdir/file.txt for the final vfs.File path.
	//   * Upon success, a vfs.File, representing the file's new path (location path + file relative path), will be returned.
	//   * The file may or may not already exist.
	NewFile(relFilePath string) (File, error)

	// DeleteFile deletes the file of the given name at the location.
	//
	// This is meant to be a short cut for instantiating a new file and calling delete on that, with all the necessary
	// error handling overhead.
	//
	// * Accepts relative file path.
	DeleteFile(relFilePath string) error

	// URI returns the fully qualified absolute URI for the Location.  IE, s3://bucket/some/path/
	//
	// URI's for locations must always end with a slash.
	URI() string
}

// File represents a file on a file system.  A File may or may not actually exist on the file system.
type File interface {
	io.Closer
	io.Reader
	io.Seeker
	io.Writer
	fmt.Stringer

	// Exists returns boolean if the file exists on the file system.  Returns an error, if any.
	Exists() (bool, error)

	// Location returns the vfs.Location for the File.
	Location() Location

	// CopyToLocation will copy the current file to the provided location.
	//
	//   * Upon success, a vfs.File, representing the file at the new location, will be returned.
	//   * In the case of an error, nil is returned for the file.
	//   * CopyToLocation should use native functions when possible within the same scheme.
	//   * If the file already exists at the location, the contents will be overwritten with the current file's contents.
	//   * Unless Seek position is at 0,0 a vfs.CopyToNotPossible will be returned
	//   * CopyToLocation will Close both the source and target Files which therefore can't be appended to without first
	//     calling Seek() to move the cursor to the end of the file.
	CopyToLocation(location Location) (File, error)

	// CopyToFile will copy the current file to the provided file instance.
	//
	//   * In the case of an error, nil is returned for the file.
	//   * CopyToLocation should use native functions when possible within the same scheme.
	//   * If the file already exists, the contents will be overwritten with the current file's contents.
	//   * Unless Seek position is at 0,0 a vfs.CopyToNotPossible will be returned
	//   * CopyToFile will Close both the source and target Files which therefore can't be appended to without first
	//     calling Seek() to move the cursor to the end of the file.
	CopyToFile(file File) error

	// MoveToLocation will move the current file to the provided location.
	//
	//   * If the file already exists at the location, the contents will be overwritten with the current file's contents.
	//   * If the location does not exist, an attempt will be made to create it.
	//   * Upon success, a vfs.File, representing the file at the new location, will be returned.
	//   * In the case of an error, nil is returned for the file.
	//   * When moving within the same Scheme, native move/rename should be used where possible.
	//   * If the file already exists, the contents will be overwritten with the current file's contents.
	//   * Unless Seek position is at 0,0 a vfs.CopyToNotPossible will be returned
	//   * MoveToLocation will Close both the source and target Files which therefore can't be appended to without first
	//     calling Seek() to move the cursor to the end of the file.
	MoveToLocation(location Location) (File, error)

	// MoveToFile will move the current file to the provided file instance.
	//
	//   * If the file already exists, the contents will be overwritten with the current file's contents.
	//   * Unless Seek position is at 0,0 a vfs.CopyToNotPossible will be returned
	//   * The current instance of the file will be removed.
	//   * MoveToFile will Close both the source and target Files which therefore can't be appended to without first
	//     calling Seek() to move the cursor to the end of the file.
	MoveToFile(file File) error

	// Delete unlinks the File on the file system.
	Delete() error

	// LastModified returns the timestamp the file was last modified (as *time.Time).
	LastModified() (*time.Time, error)

	// Size returns the size of the file in bytes.
	Size() (uint64, error)

	// Path returns absolute path, including filename, ie /some/path/to/file.txt
	//
	// If the directory portion of a file is desired, call
	//   someFile.Location().Path()
	Path() string

	// Name returns the base name of the file path.
	//
	// For file:///some/path/to/file.txt, it would return file.txt
	Name() string

	// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
	// Returns error if unable to touch File.
	Touch() error

	// URI returns the fully qualified absolute URI for the File.  IE, s3://bucket/some/path/to/file.txt
	URI() string
}

// Options are structs that contain various options specific to the file system
type Options interface{}

// Retry is a function that can be used to wrap any operation into a definable retry operation. The wrapped argument
// is called by the underlying VFS implementation.
//
// Ex:
//   var retrier Retry = func(wrapped func() error) error {
//     var ret error
//     for i := 0; i < 5; i++ {
//        if err := wrapped(); err != nil { ret = err; continue }
//     }
//     return ret
//   }
type Retry func(wrapped func() error) error

// DefaultRetryer returns a no-op retryer which simply calls the wrapped command without looping.
func DefaultRetryer() Retry {
	return func(c func() error) error { return c() }
}
