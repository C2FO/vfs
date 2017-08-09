// Package vfs provides a platform-independent interface to generalized set of filesystem
// functionality across a number of filesystem types such as os, S3, and GCS.
package vfs

import (
	"fmt"
	"io"
	"regexp"
	"time"
)

// FileSystem represents a filesystem with any authentication accounted for.
type FileSystem interface {
	// NewFile initializes a File on the specified volume at path 'name'. On error, nil is returned
	// for the file.
	NewFile(volume string, name string) (File, error)

	// NewLocation initializes a Location on the specified volume with the given path. On error, nil is returned
	// for the location.
	NewLocation(volume string, path string) (Location, error)

	// Name returns the name of the FileSystem ie: s3, disk, gcs, etc...
	Name() string

	// Scheme, related to Name, is the uri scheme used by the FileSystem: s3, file, gs, etc...
	Scheme() string
}

// Location represents a filesystem path which serves as a start point for directory-like functionality.  A location may
// or may not actually exist on the filesystem.
type Location interface {
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

	// Exists returns boolean if the file exists on the file system. Also returns an error if any.
	Exists() (bool, error)

	// NewLocation is an initializer for a new Location relative to the existing one. For instance, for location:
	// file://some/path/to/, calling NewLocation("../../") would return a new vfs.Location representing file://some/.
	// The new location instance should be on the same file system volume as the location it originated from.
	NewLocation(relativePath string) (Location, error)

	// ChangeDir updates the existing Location's path to the provided relative path. For instance, for location:
	// file://some/path/to/, calling ChangeDir("../../") update the location instance to file://some/.
	ChangeDir(relativePath string) error

	//FileSystem returns the underlying vfs.FileSystem struct for Location.
	FileSystem() FileSystem

	// NewFile will instantiate a vfs.File instance at the current location's path. In the case of an error,
	// nil will be returned.
	NewFile(fileName string) (File, error)

	// DeleteFile deletes the file of the given name at the location. This is meant to be a short cut for
	// instantiating a new file and calling delete on that, with all the necessary error handling overhead.
	DeleteFile(fileName string) error

	// URI returns the fully qualified URI for the Location.  IE, file://bucket/some/path/
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

	// MoveToLocation will move the current file to the provided location. If the file already exists at the location,
	// the contents will be overwritten with the current file's contents. In the case of an error, nil is returned
	// for the file.
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
	Path() string

	// Name returns the base name of the file path.  For file:///some/path/to/file.txt, it would return file.txt
	Name() string

	// URI returns the fully qualified URI for the File.  IE, s3://bucket/some/path/to/file.txt
	URI() string
}
