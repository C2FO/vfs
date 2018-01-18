# vfs - Virtual File System
> Go library to generalize commands and behavior when interacting with various file systems.

The vfs library includes interfaces which allow you to interact with files and locations on various file systems in a generic way. Currently supported file systems:
* Local fs (Windows, OS X, Linux)
* Amazon S3
* GCS

These interfaces are composed of standard Go library interfaces, allowing for simple file manipulation within, and between the supported file systems.

At C2FO we have created a factory system that is integrated with our app configuration that allows for simply initializing the various locations we tend to do file work in. You can build your own similar system directly on top of the various file  system implementations and the provided generic interfaces, or you can use the simple interface included in the vfs package.
The usage examples below will detail this simple interface. We will eventually be providing a version of our factory as an  example of how this library can be used in a more complex project.

A couple notes on configuration for this interface (vfssimple.NewFile and vfssimple.NewLocation):
* Before calling either function you must initialize any file systems you expect to be using.
* Local: The local file system requires no configuration. Simply call vfssimple.InitializeLocalFileSystem so the internals are prepared to expect "file:///" URIs.
* S3: The vfssimple.InitializeS3FileSystem() method requires authentication parameters for the user, see godoc for this function.
* GCS: In addition to calling vfssimple.InitializeGSFileSystem, you are expected to have authenticated with GCS using the Google Cloud Shell for the user running the app. We will be looking into more flexible forms of authentication (similar to the S3 library) in the future, but this was an ideal use case for us to start with, and therefore, all that is currently provided.

## Installation

OS X, Linux, and Windows:

```sh
glide install github.com/c2fo/vfs
```

## Usage example

```go
import "github.com/c2fo/vfs/vfssimple"

// The following functions tell vfssimple we expect to handle a particular file system in subsequent calls to
// vfssimple.NewFile() and vfssimple.NewLocation
// Local files, ie: "file:///"
vfssimple.InitializeLocalFileSystem()

// Google Cloud Storage, ie: "gs://"
vfs.InitializeGSFileSystem()

// Amazon S3, ie: "s3://"
vfssimple.InitializeS3FileSystem(accessKeyId, secreteAccessKey, token)

// alternative to above for S3, if you've already initialized a client of interface s3iface.S3API
vfssimple.SetS3Client(client)
```

You can then use those file systems to initialize locations which you'll be referencing frequently, or initialize files directly

```go
osFile, err := vfssimple.NewFile("file:///path/to/file.txt")
s3File, err := vfssimple.NewFile("s3://bucket/prefix/file.txt")

osLocation, err := vfssimple.NewLocation("file:///tmp")
s3Location, err := vfssimple.NewLocation("s3://bucket")

osTmpFile, err := osLocation.NewFile("anotherFile.txt") // file at /tmp/anotherFile.txt
```

With a number of files and locations between s3 and the local file system you can perform a number of actions without any consideration for the system's api or implementation details.

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

// vfs.File and vfs.Location implement fmt.Stringer, returning x.URI()
fmt.Sprintf("Working on file: %s", s3File) // "Working on file: s3://bucket/prefix/file.txt"
```

## Development setup

Fork the project and clone it locally, then in the cloned directory...

```sh
glide install
go test $(glide novendor)
```

## Release History

* 0.1.0
    * The first release
    * Support for local file system, s3, and gcs
    * Initial README.md
* 1.0.0
    * Apply last of bugfixes from old repo
* 1.0.1
    * Enable server-side encryption

## Meta

Brought to you by the Enterprise Pipeline team at C2FO:

John Judd - john.judd@c2fo.com

Jason Coble - [@jasonkcoble](https://twitter.com/jasonkcoble) - jason@c2fo.com

Chris Roush – chris.roush@c2fo.com

Distributed under the MIT license. See ``LICENSE`` for more information.

[https://github.com/c2fo/](https://github.com/c2fo/)

## Contributing

1. Fork it (<https://github.com/c2fo/vfs/fork>)
2. Create your feature branch (`git checkout -b feature/fooBar`)
3. Commit your changes (`git commit -am 'Add some fooBar'`)
4. Push to the branch (`git push origin feature/fooBar`)
5. Create a new Pull Request