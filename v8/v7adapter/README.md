# VFS v8 v7 adapter

Package [`github.com/c2fo/vfs/v8/v7adapter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter) wraps **v7** [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v7#File), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v7#Location), and [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v7#FileSystem) values so they satisfy **v8** interfaces. Use it to migrate call sites to v8 APIs while storage implementations still target v7.

## When to use

- You want **`Lister.List`** with `iter.Seq2` and **functional listing options** over an existing v7 backend.
- You want **v8-shaped** `Stat` / [`fs.FileInfo`](https://pkg.go.dev/io/fs#FileInfo) on top of v7 files.
- You are gradually moving code to **`context.Context`**-aware helpers such as [`FileTransfers`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers).

Avoid the adapter in hot paths if profiling shows wrapping cost matters; prefer native v8 backends once they exist.

## Constructors

| Function | Wraps |
|----------|--------|
| [`FileFromV7`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#FileFromV7) | `vfs.File` (v7) → [`vfs.File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) (v8) |
| [`LocationFromV7`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#LocationFromV7) | `vfs.Location` (v7) → [`vfs.Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location) (v8) |
| [`FileSystemFromV7`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#FileSystemFromV7) | `vfs.FileSystem` (v7) → [`vfs.FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem) (v8) |
| [`FileTransfersFromV7`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#FileTransfersFromV7) | v7 `File` → [`vfs.FileTransfers`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers) (copy/move with `context`) |

## Unwrapping

| Function | Purpose |
|----------|---------|
| [`AsV7File`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#AsV7File) | Recover v7 `File` from an adapter `File` |
| [`AsV7Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#AsV7Location) | Recover v7 `Location` from an adapter `Location` |
| [`AsV7FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#AsV7FileSystem) | Recover v7 `FileSystem` from an adapter `FileSystem` |

Transfers such as [`FileTransfers.CopyToLocation`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers.CopyToLocation) require the **destination** to be an adapter [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#Location) (or unwrap manually and call v7 APIs).

## Limitations

1. **v8 `NewFile` / `DeleteFile` options** — Variadic [`v8/options`](https://pkg.go.dev/github.com/c2fo/vfs/v8/options) values are **not translated** into v7 [`options`](https://pkg.go.dev/github.com/c2fo/vfs/v7/options). The adapter calls v7 `NewFile` / `DeleteFile` **without** extra v7 options. Use v7 entry points or unwrap when you need backend-specific v7 options.

2. **Recursive listing** — [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) is not implemented on top of v7’s separate list APIs; the adapter reports [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported).

3. **`Stat`** — Implemented by synthesizing [`fs.FileInfo`](https://pkg.go.dev/io/fs#FileInfo) from v7 metadata (name, size, mod time, mode `0644`). It is not a direct `os.Stat` equivalent on every backend.

## Example

```go
import (
    "context"

    vfsv7 "github.com/c2fo/vfs/v7"
    "github.com/c2fo/vfs/v8/v7adapter"
)

func example(fs vfsv7.FileSystem) error {
    locV7, err := fs.NewLocation("vol", "/data/")
    if err != nil {
        return err
    }

    loc := v7adapter.LocationFromV7(locV7)

    for ent, err := range loc.List(context.Background()) {
        if err != nil {
            return err
        }
        _ = ent.Name
    }
    return nil
}
```

## See also

- [v8 module overview](../README.md)
- [pkg.go.dev documentation](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter)
