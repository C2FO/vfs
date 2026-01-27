<img src="logo.jpg" width="800">

[![GitHub tag](https://img.shields.io/github/tag/c2fo/vfs.svg?style=flat)](https://github.com/c2fo/vfs/releases)
[![Build Status](https://travis-ci.org/C2FO/vfs.svg?branch=master)](https://travis-ci.org/C2FO/vfs)
[![GoDoc](https://pkg.go.dev/badge/github.com/c2fo/vfs/v6?utm_source=godoc)](https://pkg.go.dev/github.com/c2fo/vfs/v7)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.md)
[![Go Report Card](https://goreportcard.com/badge/github.com/c2fo/vfs)](https://goreportcard.com/report/github.com/c2fo/vfs)
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)

## Overview

*vfs* provides a unified interface for working with different storage backends (local OS, Azure, S3, GCS, SFTP, etc.) in
a consistent way. Rather than handling each backend independently, you interact with a core set of interfaces that
abstract backend details. This allows you to:

* Seamlessly read, write, and manage files across multiple storages.
* Treat remote or cloud-based paths as if they were local (e.g. using standard `io.Reader`/`io.Writer` interfaces).
* Simplify environment-agnostic development by keeping most logic backend-agnostic.
* Plug in or mock storage functionality to facilitate testing and modular application design.

By focusing on *[FileSystem](https://pkg.go.dev/github.com/c2fo/vfs/v7#FileSystem)*,
*[Location](https://pkg.go.dev/github.com/c2fo/vfs/v7#Location)*, and
*[File](https://pkg.go.dev/github.com/c2fo/vfs/v7#File)* interfaces, you can build reusable flows for file operations without
needing to deeply understand each backend's specific APIs. Users can add or swap backends as needed, providing flexibility
for hybrid or evolving storage requirements.

## Installation
```bash
go get -u github.com/c2fo/vfs/v7
```

## Quick Start

```go
package main

import (
    "fmt"
    "strings"
    "io"
    "log"

    "github.com/c2fo/vfs/v7"
    "github.com/c2fo/vfs/v7/vfssimple"
)

func main() {
    // Create local OS file from a URI
    osFile, err := vfssimple.NewFile("file:///tmp/example.txt")
    if err != nil {
        log.Fatal(err)
    }
    defer osFile.Close()

    // Write to the file
    _, err = io.Copy(osFile, strings.NewReader("Hello from vfs!"))
    if err != nil {
        log.Fatal(err)
    }

    if err := osFile.Close(); err != nil {
        log.Fatal(err)
    }
    fmt.Println("File created and written:", osFile.URI())
}
```
This snippet shows the basic setup: an osFile is created from a URI and written to using standard libraries.

## Advanced Usage
* See the [GoDoc](https://pkg.go.dev/github.com/c2fo/vfs/v7) for deeper technical details, backend-specific options, and additional APIs.
* Specialized operations like copying between S3 locations or advanced configs (e.g. Azure credentials) are documented in each backend doc.

* [vfscp](docs/vfscp.md)
* [vfssimple](docs/vfssimple.md)
* [backend](docs/backend.md)
    * [os backend](docs/os.md)
    * [gs backend](docs/gs.md)
    * [s3 backend](docs/s3.md)
    * [in-memory backend](docs/mem.md)
    * [sftp backend](docs/sftp.md)
    * [ftp backend](docs/ftp.md)
    * [azure backend](docs/azure.md)
    * [conformance tests](docs/conformance_tests.md)
* [utils](docs/utils.md)

### Community Contributed Backends

These backends are maintained in `contrib/backend/` as separate Go modules:

* [Dropbox backend](contrib/backend/dropbox/README.md) - Dropbox cloud storage

See [contrib/backend/README.md](contrib/backend/README.md) for guidelines on contributing new backends.

## Additional Tools

*   [lockfile](contrib/lockfile/README.md): Provides distributed locking mechanisms using vfs backends.
*   [vfsevents](contrib/vfsevents/README.md): Production-ready file event monitoring with support for real-time notifications, retry logic, and multiple watcher types (VFS polling, S3 events, GCS events).

## FAQ

**Q**: Why am I seeing an empty file when using `io.Copy` on some backends if my source is empty?
**A**: An empty `Reader` often means the backend doesn't write a file until data is actually written. Use `utils.TouchCopy` if you need to ensure a zero-byte file is created.

**Q**: Will vfs v6 still be supported?
**A**: Yes and no. We will continue to provide security patches and bug fixes for v6, but new features and enhancements
will be added to v7.

**Q**: How long will v6 be supported?
**A**: We will support v6 until the next major release, v8, is released. Then v7 will be supported until v9 is released.

## Upgrading

### Upgrading from v6 to v7

Please review these changes and update your code accordingly to ensure compatibility with v7.

#### S3 Backend

The project now uses the `aws-sdk-go-v2` library instead of the deprecated, EOL `aws-sdk-go`. This update necessitated
these changes to the S3 backend:

- The S3 backend's `s3fs.Client()` function now returns an `s3.Client` which is a subset of AWS's sdk v2 functionality.
  This change may require updates to your code if you were relying on client functionality not directly required by the
  s3 vfs backend.
- The `Option.Retry` field is now an `aws.Retryer` instead of a `request.Retry`. Ensure that your Option logic is
  compatible with the new type.

#### Azure Backend

- Scheme for Azure has been updated from `https` to `az`. Update your code to use the new scheme.
- Authority for Azure has been updated from `blob.core.windows.net` to `my-container-name`, such that the full URI
  is `az://my-blob-name/path/to/file.txt` rather than
  `https://my-storage-account-name.core.windows.net/my-container-name/path/to/file.txt`.

#### GS Backend
- The `Options.Retry` field, with the now deprecated `vfs.Retry` type, has been moved to `gs.FileSystem` as the new `gs.Retyer`
  type.  It is now set via `gs.NewFileSystem` function using functional option `gs.WithRetryer`.  All previous `vfs.Retry`
  usage has been replaced with `gs.WithRetryer`.  Update your code to use the new `gs.WithRetryer` functional option.

#### All Backends
Some methods in the *Location* and *FileSystem* interfaces have been deprecated because they use terminology that doesn't apply to all backends.
They will be removed in a future release. Update your code to use the new methods.  See [#235](https://github.com/C2FO/vfs/issues/235).
- `location.Volume()` method which returns the authority as a string has been deprecated in favor of the `location.Authority()` method which
  returns an `authority.Authority` struct. Update your code to use the `Authority().String()` method instead of `Volume()`.
- `location.ChangeDir()` method ash been deprecated in favor of the existing `location.NewLocation()` method. Update your code to use the
  `NewLocation()` method instead of `ChangeDir()`.
- `vfs.Options` struct has been deprecated in favor of using backend-specific structs.
- `filesystem.Retry()` method has been deprecated in favor of using backend-specific functional options, such as `gs.WithRetryer`.

Additionally, we have added functional option interface, `FileSystemOption`, to allow for more flexible configuration
of backends. This interface allows for more complex configuration options to be passed to the via the `NewFileSystem` function.
This will replace backend-specific chainable functions that require casting the filesystem to the backend type first. See [#238](https://github.com/C2FO/vfs/issues/238).

### Upgrading from v5 to v6

With `v6`, sftp.Options struct changed to accept an array of Key Exchange algorithms rather than a string. To update,
change the syntax of the auth commands.

```
  "keyExchanges":"diffie-hellman-group-a256"
```

becomes

```
  "keyExchanges":["diffie-hellman-group-a256"]
```

## Contributing

1. Fork it (`git clone https://github.com/c2fo/vfs.git`)
2. Create your feature branch (`git checkout -b feature/fooBar`)
3. Commit your changes (`git commit -am "Add some fooBar"`)
4. Push to the branch (`git push origin feature/fooBar`)
5. Create a new Pull Request

## License
Distributed under the MIT License. See the [LICENSE.md](LICENSE.md) file for details.
