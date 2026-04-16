# VFS v8 in-memory backend

Package [`github.com/c2fo/vfs/v8/backend/mem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/mem) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces in process memory. It is intended for **tests** and **ephemeral** storage, not production durability.

## Behavior (summary)

- **Scheme:** `mem` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/mem#Scheme)).
- A [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/mem#File) is not reported by [`Exists`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta) until it is written, [`Touch`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta) is called, or similar—aligned with the v7 memory backend.
- **[`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List)** yields base names under that location; [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) returns [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported).
- **No** global registration: construct a [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/mem#FileSystem) with [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/mem#NewFileSystem) (there is no v8 `vfssimple` yet).

## Usage

```go
import (
    "context"

    "github.com/c2fo/vfs/v8/backend/mem"
)

fs := mem.NewFileSystem()
f, err := fs.NewFile("my-volume", "/tmp/example.txt")
if err != nil {
    return err
}
if err := f.Touch(); err != nil {
    return err
}
defer f.Close()

loc, err := fs.NewLocation("my-volume", "/tmp/")
if err != nil {
    return err
}

for ent, err := range loc.List(context.Background()) {
    if err != nil {
        return err
    }
    _ = ent.Name
}
```

## Conformance

Run `go test -tags=vfsintegration ./...` in this module to execute the shared [v8 backend testsuite](../testsuite) (`RunConformanceTests` and `RunIOTests`) against mem.

## See also

- [v8 module overview](../../README.md)
- [Backends index](../README.md)
- [pkg.go.dev documentation](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/mem)
