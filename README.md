# vfs

---

[![GitHub tag](https://img.shields.io/github/tag/c2fo/vfs.svg?style=flat)](https://github.com/c2fo/vfs/releases)
[![Build Status](https://travis-ci.org/c2fo/vfs.svg?branch=master)](https://travis-ci.org/c2fo/vfs)
[![GoDoc](https://godoc.org/github.com/c2fo/vfs?status.png)](http://godoc.org/github.com/c2fo/vfs)
[![codecov](https://codecov.io/gh/c2fo/vfs/branch/master/graph/badge.svg)](https://codecov.io/gh/c2fo/vfs)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](License.md)
[![Go Report Card](https://goreportcard.com/badge/github.com/c2fo/vfs)](https://goreportcard.com/report/github.com/c2fo/vfs)

Package vfs provides a pluggable, extensible, and opinionated set of filesystem
functionality for Go across a number of filesystem types such as os, S3, and GCS.

### Philosophy

When building our platform, initially we wrote a library that was something to
the effect of

      if config.DISK == "S3" {
    	  // do some s3 filesystem operation
      } else if config.DISK == "mock" {
          // fake something
      } else {
          // do some native os.xxx operation
      }

Not only was ugly but because the behaviors of each "filesystem" were
different and we had to constantly alter the file locations and pass a bucket
string (even if the fs didn't know what a bucket was).

We found a handful of third-party libraries that were interesting but none of
them had everything we needed/wanted. Of particular inspiration was
https://github.com/spf13/afero in its composition of the super-powerful stdlib
[io.*](https://godoc.org/io) interfaces. Unfortunately, it didn't support Google Cloud Storage and there
was still a lot of passing around of strings and structs. Few, if any, of the
vfs-like libraries provided interfaces to easily and confidently create new
filesystem backends.

###### What we needed/wanted was the following(and more):

* self-contained set of structs that could be passed around like a file/dir handle
* the struct would represent an existing or nonexistant file/dir
* provide common (and only common) functionality across all filesystem so that after initialization, we don't care
      what the underlying filesystem is and can therefore write our code agnosticly/portably
* use [io.*](https://godoc.org/io) interfaces such as [io.Reader](https://godoc.org/io#Reader) and [io.Writer](https://godoc.org/io#Writer) without needing to call a separate function
* extensibility to easily add other needed filesystems like Microsoft Azure Cloud File Storage or SFTP
* prefer native atomic functions when possible (ie S3 to S3 moving would use the native move api call rather than
      copy-delete)
* a uniform way of addressing files regardless of filesystem.  This is why we use complete URI's in [vfssimple](docs/vfssimple.md)
* [fmt.Stringer](https://godoc.org/fmt#Stringer) interface so that the file struct passed to a log message (or other [Stringer](https://godoc.org/fmt#Stringer) use) would show the URI
* mockable filesystem
* pluggability so that third-party implemenations of our interfaces could be used


### Install

Go install:

    go get -u github.com/c2fo/vfs/...

Glide installation:

    glide install github.com/c2fo/vfs


### Usage

We provde [vfssimple](docs/vfssimple.md) as basic way of initializing filesystem backends (see each
implemnations's docs about authentiation). [vfssimple](docs/vfssimple.md) pulls in every c2fo/vfs
backend. If you need to reduce the backend requirements (and app memory
footprint) or add a third party backend, you'll need to implement your own
"factory". See [backend](docs/backend.md) doc for more info.

You can then use those file systems to initialize locations which you'll be
referencing frequently, or initialize files directly

    osFile, err := vfssimple.NewFile("file:///path/to/file.txt")
    s3File, err := vfssimple.NewFile("s3://bucket/prefix/file.txt")

    osLocation, err := vfssimple.NewLocation("file:///tmp")
    s3Location, err := vfssimple.NewLocation("s3://bucket")

    osTmpFile, err := osLocation.NewFile("anotherFile.txt") // file at /tmp/anotherFile.txt

With a number of files and locations between s3 and the local file system you
can perform a number of actions without any consideration for the system's api
or implementation details.

    osFileExists, err := osFile.Exists() // true, nil
    s3FileExists, err := s3File.Exists() // false, nil
    err = osFile.CopyToFile(s3File) // nil
    s3FileExists, err = s3File.Exists() // true, nil

    movedOsFile, err := osFile.MoveToLocation(osLocation)
    osFileExists, err = osFile.Exists() // false, nil (move actions delete the original file)
    movedOsFileExists, err := movedOsFile.Exists() // true, nil

    s3FileUri := s3File.URI() // s3://bucket/prefix/file.txt
    s3FileName := s3File.Name() // file.txt
    s3FilePath := s3File.Path() // /prefix/file.txt

### Third-party Backends

  * none so far

Feel free to send a pull request if you want to add your backend to the list.

### See also:
* [vfscp](docs/vfscp.md)
* [vfssimple](docs/vfssimple.md)
* [backend](docs/backend.md)
  * [os backend](docs/os.md)
  * [gs backend](docs/gs.md)
  * [s3 backend](docs/s3.md)
* [utils](docs/utils.md)


### Ideas

Things to add:

* Add SFTP backend
* Add Azure storage backend
* Add in-memory backend
* Provide better List() functionality with more abstracted filtering and paging (iterator?) Return File structs vs URIs?
* Add better/any context.Context() support for deadline and cancellation

### Contrubutors

Brought to you by the Enterprise Pipeline team at C2FO:

John Judd - john.judd@c2fo.com

Jason Coble - [@jasonkcoble](https://twitter.com/jasonkcoble) - jason@c2fo.com

Chris Roush – chris.roush@c2fo.com

https://github.com/c2fo/

### Contributing

1. Fork it (<https://github.com/c2fo/vfs/fork>)
1. Create your feature branch (`git checkout -b feature/fooBar`)
1. Commit your changes (`git commit -am 'Add some fooBar'`)
1. Push to the branch (`git push origin feature/fooBar`)
1. Create a new Pull Request


### License

Distributed under the MIT license. See `http://github.com/c2fo/vfs/License.md
for more information.

## Interfaces

#### type File

```go
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
```

File represents a file on a filesystem. A File may or may not actually exist on
the filesystem.

#### type FileSystem

```go
type FileSystem interface {
	// NewFile initializes a File on the specified volume at path 'name'. On error, nil is returned
	// for the file.  
	// 
	// Note that not all filesystems will have a "volume":
	// file:///path/to/file has a volume of "" and name path/to/file
	// whereas
	// s3://mybucket/path/to file has a volume of mybucket
	NewFile(volume string, name string) (File, error)

	// NewLocation initializes a Location on the specified volume with the given path. On error, nil is returned
	// for the location.
	//
	// See NewFile for note on volume.
	NewLocation(volume string, path string) (Location, error)

	// Name returns the name of the FileSystem ie: Amazon S3, os, Google Cloud Storage, etc...
	Name() string

	// Scheme, related to Name, is the uri scheme used by the FileSystem: s3, file, gs, etc...
	Scheme() string
}
```

FileSystem represents a filesystem with any authentication accounted for.

#### type Location

```go
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
```

Location represents a filesystem path which serves as a start point for
directory-like functionality. A location may or may not actually exist on the
filesystem.

#### type Options

```go
type Options interface{}
```

Options are structs that contain various options specific to the filesystem