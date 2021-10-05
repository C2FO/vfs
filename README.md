# vfs

---

[![GitHub tag](https://img.shields.io/github/tag/c2fo/vfs.svg?style=flat)](https://github.com/c2fo/vfs/releases)
[![Build Status](https://travis-ci.org/C2FO/vfs.svg?branch=master)](https://travis-ci.org/C2FO/vfs)
[![GoDoc](https://pkg.go.dev/badge/github.com/c2fo/vfs/v6?utm_source=godoc)](https://pkg.go.dev/github.com/c2fo/vfs/v6)
[![codecov](https://codecov.io/gh/c2fo/vfs/branch/master/graph/badge.svg)](https://codecov.io/gh/c2fo/vfs)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](License.md)
[![Go Report Card](https://goreportcard.com/badge/github.com/c2fo/vfs)](https://goreportcard.com/report/github.com/c2fo/vfs)
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)

Package vfs provides a pluggable, extensible, and opinionated set of file system
functionality for Go across a number of file system types such as os, Azure, S3, GCS
and SFTP.


### Philosophy

When building our platform, initially we wrote a library that was something to
the effect of

```go
      if config.DISK == "S3" {
    	  // do some s3 file system operation
      } else if config.DISK == "mock" {
          // fake something
      } else {
          // do some native os.xxx operation
      }
```

Not only was ugly but because the behaviors of each "file system" were 
different and we had to constantly alter the file locations and pass a bucket string (even
if the file system didn't know what a bucket was).

We found a handful of third-party libraries that were interesting but none of
them had everything we needed/wanted. Of particular inspiration was
https://github.com/spf13/afero in its composition of the super-powerful stdlib
[io.*](https://godoc.org/io) interfaces. Unfortunately, it didn't support Google Cloud Storage and there
was still a lot of passing around of strings and structs. Few, if any, of the
vfs-like libraries provided interfaces to easily and confidently create new
file system backends.

###### What we needed/wanted was the following(and more):

* self-contained set of structs that could be passed around like a file/dir handle
* the struct would represent an existing or nonexistent file/dir
* provide common (and only common) functionality across all file system so that after initialization, we don't care
      what the underlying file system is and can therefore write our code agnostically/portably
* use [io.*](https://godoc.org/io) interfaces such as [io.Reader](https://godoc.org/io#Reader) and [io.Writer](https://godoc.org/io#Writer) without needing to call a separate function
* extensibility to easily add other needed file systems like Microsoft Azure Cloud File Storage
* prefer native atomic functions when possible (ie S3 to S3 moving would use the native move api call rather than
      copy-delete)
* a uniform way of addressing files regardless of file system.  This is why we use complete URI's in vfssimple
* [fmt.Stringer](https://godoc.org/fmt#Stringer) interface so that the file struct passed to a log message (or other Stringer use) would show the URI
* mockable file system
* pluggability so that third-party implementations of our interfaces could be used


### Install

Pre 1.17:
```
go get -u github.com/c2fo/vfs/v6
```

Post 1.17:
```
go install -u github.com/c2fo/vfs/v6
```

### Upgrading

#### Upgrading from v5 to v6
With v6.0.0, sftp.Options struct changed to to accept an array of Key Exchange algorithms rather than a string. To update, change the syntax of the auth commands.
```
  "keyExchanges":"diffie-hellman-group-a256"
```
becomes
```
  "keyExchanges":["diffie-hellman-group-a256"]
```

### Usage

We provide [vfssimple](docs/vfssimple.md) as basic way of initializing file system backends (see each
implementations's docs about authentication). [vfssimple](docs/vfssimple.md) pulls in every c2fo/vfs
backend. If you need to reduce the backend requirements (and app memory
footprint) or add a third party backend, you'll need to implement your own
"factory". See [backend](docs/backend.md) doc for more info.

You can then use those file systems to initialize locations which you'll be
referencing frequently, or initialize files directly

```go
    osFile, err := vfssimple.NewFile("file:///path/to/file.txt")
    s3File, err := vfssimple.NewFile("s3://bucket/prefix/file.txt")

    osLocation, err := vfssimple.NewLocation("file:///tmp/")
    s3Location, err := vfssimple.NewLocation("s3://bucket/")

    osTmpFile, err := osLocation.NewFile("anotherFile.txt") // file at /tmp/anotherFile.txt
```

You can perform a number of actions without any consideration for the system's api or implementation details.

```go
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
```

File's [io.*](https://godoc.org/io) interfaces may be used directly:

```go
    reader := strings.NewReader("Clear is better than clever")
    gsFile, err := vfssimple.NewFile("gs://somebucket/path/to/file.txt")

    byteCount, err := io.Copy(gsFile, reader)
    err := gsFile.Close()
```

Note: [io.Copy()](https://godoc.org/io#Copy) doesn't strictly define what happens if a reader is empty.  This is complicated because io.Copy
will first delegate actual copying in the following:
  1. if the io.Reader also implements io.WriterTo, WriteTo() will do the copy
  2. if the io.Writer also implements io.ReaderFrom, ReadFrom() will do the copy
  3. finally, if neither 1 or 2, io.Copy will do it's own buffered copy

In case 3, and most implementations of cases 1 and 2, if reader is empty, Write() never gets called. What that means for
vfs is there is no way for us to ensure that an empty file does or doesn't get written on an io.Copy().  For instance 
OS always creates a file, regardless of calling Write() whereas S3 must Write() and Close().

As such, vfs cannot guarantee copy behavior except in our own CopyToFile, MoveToFile, CopyToLocation, and MoveToLocation
functions.  If you need to ensure a file gets copied/moved with io.Copy(), you must do so yourself OR use vfs's [utils.TouchCopy](docs/utils.md)


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
  * [in-memory backend](docs/mem.md)
  * [sftp backend](docs/sftp.md)
  * [azure backend](docs/azure.md)  
* [utils](docs/utils.md)

### Ideas

Things to add:
* Provide better List() functionality with more abstracted filtering and paging (iterator?) Return File structs vs URIs?
* Add better/any context.Context() support


### Contributors

Brought to you by the Enterprise Pipeline team at C2FO:

* John Judd - john.judd@c2fo.com

* Jason Coble - [@jasonkcoble](https://twitter.com/jasonkcoble) - jason@c2fo.com

* Chris Roush – chris.roush@c2fo.com

* Moe Zeid    - moe.zeid@c2fo.com

https://github.com/c2fo/

### Contributing

    1. Fork it (<https://github.com/c2fo/vfs/fork>)
    2. Create your feature branch (`git checkout -b feature/fooBar`)
    3. Commit your changes (`git commit -am 'Add some fooBar'`)
    4. Push to the branch (`git push origin feature/fooBar`)
    5. Create a new Pull Request


### License

Distributed under the MIT license. See `http://github.com/c2fo/vfs/License.md
for more information.


### Definitions

###### absolute path
- A path is said to be absolute if it provides the entire context
need to find a file, including the file system root. An absolute path must
begin with a slash and may include . and .. directories.

###### file path
- A file path ends with a filename and therefore may not end with a slash.  It may be relative or absolute.

###### location path
- A location/directory path must end with a slash.  It may be relative or absolute.

###### relative path
- A relative path is a way to locate a directory or file relative to
another directory. A relative path may not begin with a slash but may include .
and .. directories.

###### URI
- A Uniform Resource Identifier (URI) is a string of characters that
unambiguously identifies a particular resource. To guarantee uniformity, all
URIs follow a predefined set of syntax rules, but also maintain extensibility
through a separately defined hierarchical naming scheme (e.g. http://).

## Interfaces

#### type File

```go
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
	//   * CopyToLocation will Close both the source and target Files which therefore can't be appended to without first
	//     calling Seek() to move the cursor to the end of the file.
	CopyToLocation(location Location) (File, error)

	// CopyToFile will copy the current file to the provided file instance.
	//
	//   * In the case of an error, nil is returned for the file.
	//   * CopyToLocation should use native functions when possible within the same scheme.
	//   * If the file already exists, the contents will be overwritten with the current file's contents.
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
	//   * MoveToLocation will Close both the source and target Files which therefore can't be appended to without first
	//     calling Seek() to move the cursor to the end of the file.
	MoveToLocation(location Location) (File, error)

	// MoveToFile will move the current file to the provided file instance.
	//
	//   * If the file already exists, the contents will be overwritten with the current file's contents.
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
```

File represents a file on a file system. A File may or may not actually exist on
the file system.

#### type FileSystem

```go
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
```

FileSystem represents a file system with any authentication accounted for.

#### type Location

```go
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

	// Given location:
	// 	   loc := fs.NewLocation("file:///some/path/to/")
	// calling:
	//     loc.ChangeDir("../../")
	// would update the current location instance to
	// file:///some/.
	//
	//   * ChangeDir accepts a relative location path.
	ChangeDir(relLocPath string) error

	//FileSystem returns the underlying vfs.FileSystem struct for Location.
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
```

Location represents a file system path which serves as a start point for
directory-like functionality. A location may or may not actually exist on the
file system.

#### type Options

```go
type Options interface{}
```

Options are structs that contain various options specific to the file system

#### type Retry

```go
type Retry func(wrapped func() error) error
```

Retry is a function that can be used to wrap any operation into a definable
retry operation. The wrapped argument is called by the underlying VFS
implementation.

Ex:
```go
    var retrier Retry = func(wrapped func() error) error {
      var ret error
      for i := 0; i < 5; i++ {
         if err := wrapped(); err != nil { ret = err; continue }
      }
      return ret
    }
```
#### func  DefaultRetryer

```go
func DefaultRetryer() Retry
```
DefaultRetryer returns a no-op retryer which simply calls the wrapped command
without looping.
