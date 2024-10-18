# vfssimple

---

Package vfssimple provides a basic and easy to use set of functions to any
supported backend file system by using full URI's:

* Local OS:             file:///some/path/to/file.txt
* Amazon S3:            s3://mybucket/path/to/file.txt
* Google Cloud Storage: gs://mybucket/path/to/file.txt

### Usage

Just import vfssimple.

```go
	package main

	import (
		"fmt"

		"github.com/c2fo/vfs/v6/vfssimple"
	)

	func main() {
		myLocalDir, err := vfssimple.NewLocation("file:///tmp/")
		if err != nil {
			panic(err)
		}

		myS3File, err := vfssimple.NewFile("s3://mybucket/some/path/to/key.txt")
		if err != nil {
			panic(err)
		}

		localFile, err := myS3File.MoveToLocation(myLocalDir)
		if err != nil {
			panic(err)
		}

		fmt.Printf("moved %s to %s\n", myS3File, localFile)
	}
```

### Authentication and Options

vfssimple is largely an example of how to initialize a set of backend file systems.  It only provides a default
initialization of the individual file systems.  See backend docs for specific authentication info for each backend but
generally speaking, most backends can use Environment variables to set credentials or client options.

File systems can only use one set of options. If you would like to configure more than one file system of the same type/schema with separate credentials,
you can register and map file system options to locations or individual objects. The vfssimple library will automatically try to
resolve the provided URI in NewFile() or NewLocation() to the registered file system.

```go
	package main

	import(
		"fmt"

		"github.com/c2fo/vfs/v6/backend"
		"github.com/c2fo/vfs/v6/backend/s3"
		"github.com/c2fo/vfs/v6/vfssimple"
	)

	func main() {
		bucketAuth := s3.NewFileSystem().WithOptions(s3.Options{
			AccessKeyID:     "key1",
			SecretAccessKey: "secret1",
			Region:          "us-west-2",
		})

		fileAuth := s3.NewFileSystem().WithOptions(s3.Options{
			AccessKeyID:     "key2",
			SecretAccessKey: "secret2",
			Region:          "us-west-2",
		})

		backend.Register("s3://bucket1/", bucketAuth)
		backend.Register("s3://bucket2/file.txt", fileAuth)

		secureFile, _ := vfssimple.NewFile("s3://bucket2/file.txt")
		publicLocation, _ := vfssimple.NewLocation("s3://bucket1/")

		secureFile.CopyToLocation(publicLocation)

		fmt.Printf("copied %s to %s\n", secureFile, publicLocation)
	}
```

### Registered Backend Resolution

Every backend type automatically registers itself as an available backend filesystem for vfssimple based on its scheme.  In this way,
vfssimple is able to determine which backend to use for any related URI.  As mentioned above, you can register your own initialized
filesystem as well.

vfssimple resolves backends by doing a prefix match of the URI to the registered backend names, choosing the longest(most specific) matching
backend filesystem.

For instance, given registered backends with the names:

```
	's3'                         - registered by default
	's3://somebucket/'           - perhaps this was registered using AWS access key id x
	's3://somebucket/path/'      - and this was registered using AWS access key id y
	's3://somebucket/path/a.txt' - and this was registered using AWS access key id z
	's3://some'                  - another contrived registered fs for bucket
```

See the expected registered bucket name for each:

```
	's3://somebucket/path/a.txt' - URI: 's3://somebucket/path/a.txt'         (most specific match)
	's3://somebucket/path/a.txt' - URI: 's3://somebucket/path/a.txt.tar.gz'  (prefix still matches)
	's3://somebucket/path/'      - URI: 's3://somebucket/path/otherfile.txt' (file only matches path-level registered fs)
	's3"//somebucket/path/'      - URI: 's3://somebucket/path/'              (exact path-level match)
	's3://somebucket/'           - URI: 's3://somebucket/test/file.txt'      (bucket-level match only)
	's3://somebucket/'           - URI: 's3://somebucket/test/'              (still bucket-level match only)
	's3://somebucket/'           - URI: 's3://somebucket/'                   (exact bucket-level match)
	's3://some'                  - URI: 's3://some-other-bucket/'            (bucket-level match)
	's3'                         - URI: 's3://other/'                        (scheme-level match, only)
	's3'                         - URI: 's3://other/file.txt'                (scheme-level match, only)
	's3'                         - URI: 's3://other/path/to/nowhere/'        (scheme-level match, only)
```

### Retry Option

This option allows you to specify a custom retry method which backend implementations can choose to utilize
when calling remote file systems. This adds some flexibility in how a retry on file operations should be handled.

```go
    package main

    import(
        "time"

        "github.com/c2fo/vfs/v6/backend"
        "github.com/c2fo/vfs/v6/backend/gs"
    )

    ...

    func InitializeWithRetry() error {
        bucketAuth := gs.NewFileSystem().WithOptions(gs.Options{
            Retry: func(wrapper func() error) error {
                for i := 0; i < 5; i++ {
                    if err := wrapper(); err != nil {
                        time.Sleep(1 * time.Second)
                        continue
                    }
                }
            },
        })
    }
```

## Functions

#### func  NewFile

```go
func NewFile(uri string) (vfs.File, error)
```
NewFile is a convenience function that allows for instantiating a file based on
a uri string. Any backend file system is supported, though some may require prior
configuration. See the docs for specific requirements of each.

#### func  NewLocation

```go
func NewLocation(uri string) (vfs.Location, error)
```
NewLocation is a convenience function that allows for instantiating a location
based on a uri string.Any backend file system is supported, though some may
require prior configuration. See the docs for specific requirements of each
