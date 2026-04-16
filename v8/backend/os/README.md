# VFS v8 OS (local filesystem) backend

Package [`github.com/c2fo/vfs/v8/backend/os`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/os) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces using the host OS file APIs. It is the v8 counterpart to the v7 [`backend/os`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend/os) package.

## Behavior (summary)

- **Scheme:** `file` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/os#Scheme)).
- **Paths:** Same normalization as v7 (`normalizeOSPath`, drive letters on Windows, forward slashes internally).
- **Listing:** [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) with [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption)s; [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) returns [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported).
- **Transfers:** v7-style `CopyToLocation` / `MoveToFile` are not on the v8 [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interface; use [`vfs.Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy) or [`FileTransfers`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers) where applicable.
- **Options:** [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/os#NewFileSystem) accepts v7-shaped [`WithTempDir`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/os#WithTempDir) (re-uses [`github.com/c2fo/vfs/v7/options`](https://pkg.go.dev/github.com/c2fo/vfs/v7/options) `NewFileSystemOption`).

## Usage

```go
import (
    "context"

    vfsos "github.com/c2fo/vfs/v8/backend/os"
)

fs := vfsos.NewFileSystem()
loc, err := fs.NewLocation("", "/tmp/myproject/")
if err != nil {
    return err
}
f, err := loc.NewFile("notes.txt")
if err != nil {
    return err
}
defer f.Close()

for ent, err := range loc.List(context.Background()) {
    if err != nil {
        return err
    }
    _ = ent.Name
}
```

## Conformance

```bash
go test -tags=vfsintegration ./...
```

Optional: `VFS_OS_TEST_PATH` selects a base directory for integration tests (see [`conformance_test.go`](conformance_test.go)).

## See also

- [v8 module overview](../../README.md)
- [Backends index](../README.md)
