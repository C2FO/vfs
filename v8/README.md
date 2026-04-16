# VFS v8

The **v8** module (`github.com/c2fo/vfs/v8`) defines the next generation of vfs APIs: **composable interfaces**, **iterator-based listing** with **functional options**, **`context.Context`** on transfer helpers, and light **`io/fs`** interoperability. It is versioned and released as its own Go module inside this repository (alongside v7).

**Documentation:** [pkg.go.dev/github.com/c2fo/vfs/v8](https://pkg.go.dev/github.com/c2fo/vfs/v8)

## Status

| Area | State |
|------|--------|
| Core interfaces (`vfs` package) | Defined and stable enough to implement against |
| Option markers ([`options`](options/README.md)) | Defined for `NewFile`, delete, and future file-system options |
| v7 interoperability ([`v7adapter`](v7adapter/README.md)) | Wraps v7 `File`, `Location`, and `FileSystem` for incremental migration |
| Backends ([`backend/`](backend/README.md)) | [`mem`](backend/mem/README.md), [`os`](backend/os/README.md), [`s3`](backend/s3/README.md), [`gs`](backend/gs/README.md), [`azure`](backend/azure/README.md), [`ftp`](backend/ftp/README.md), [`sftp`](backend/sftp/README.md); shared [`testsuite`](backend/testsuite/README.md) for integration tests |

Use **v7** (`github.com/c2fo/vfs/v7`) for production storage operations today unless you are experimenting with or implementing v8.

## Installation

```bash
go get github.com/c2fo/vfs/v8@latest
```

When working **inside this repository**, the v8 `go.mod` uses a `replace` directive for the local v7 module; consumers outside the repo resolve v7 from the module proxy as usual.

## Quick examples

```go
import (
	"context"

	vfs "github.com/c2fo/vfs/v8"
)
```

**List entries at a location** (see [`Lister.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List)):

```go
ctx := context.Background()
for ent, err := range loc.List(ctx, vfs.WithPrefix("incoming/")) {
	if err != nil {
		return err
	}
	_ = ent.Name
}
```

**Delete by name on a location** (replaces many uses of v7 `File.Delete`):

```go
if err := loc.DeleteFile("done.csv"); err != nil {
	return err
}
```

**Stream bytes with cancellation** ([`vfs.Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy)):

```go
n, err := vfs.Copy(ctx, dstFile, srcFile)
_ = n
```

Use the in-memory backend for unit tests: `mem.NewFileSystem()` then `NewLocation` / `NewFile` as in [§9.1](MIGRATION.md#91-construct-a-file-system-location-and-file) of the migration guide.

Full **v7 → v8** tables, adapter usage, and more examples: **[MIGRATION.md](MIGRATION.md)** (especially **§9 Concrete examples**).

## Why v8?

v7’s [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v7#File) interface bundles streaming I/O, seek, copy/move, and metadata into one large surface. That complicates backends whose APIs are not POSIX-shaped (for example object stores with prefix listing and ranged GET/PUT).

v8 splits concerns:

- **Identity** — URIs, paths, and authority ([`FileIdentity`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileIdentity), [`LocationIdentity`](https://pkg.go.dev/github.com/c2fo/vfs/v8#LocationIdentity)).
- **Streaming** — [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) requires [`io.Reader`](https://pkg.go.dev/io#Reader), [`io.Writer`](https://pkg.go.dev/io#Writer), and [`io.Closer`](https://pkg.go.dev/io#Closer). **Seek** and ranged I/O are **optional** via type assertions to [`io.Seeker`](https://pkg.go.dev/io#Seeker), [`FileReaderAt`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileReaderAt), and [`FileWriterAt`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileWriterAt).
- **Metadata** — [`FileMeta`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta) (`Exists`, [`Stat`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File.Stat) as [`fs.FileInfo`](https://pkg.go.dev/io/fs#FileInfo), size, modification time, [`Touch`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File.Touch)).
- **Listing** — One API: [`Lister.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) yields [`Entry`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Entry) values through [`iter.Seq2`](https://pkg.go.dev/iter#Seq2); use [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption) helpers such as [`WithPrefix`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithPrefix), [`WithRegexp`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRegexp), [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive), and [`WithPageSize`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithPageSize).
- **Transfers** — [`Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy) streams with context; copy/move/rename semantics are documented for implementations ([`FileTransfers`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers)). Native server-side operations stay in backends.
- **Capabilities** — Optional [`CapabilityProvider`](https://pkg.go.dev/github.com/c2fo/vfs/v8#CapabilityProvider) for feature flags ([`Capabilities`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Capabilities)).

## Package layout

| Path | Purpose |
|------|---------|
| [`github.com/c2fo/vfs/v8`](https://pkg.go.dev/github.com/c2fo/vfs/v8) | Core interfaces, [`Entry`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Entry), listing options, [`Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy), errors |
| [`github.com/c2fo/vfs/v8/options`](options/README.md) | Marker interfaces for typed options (`NewFile`, delete, …) |
| [`github.com/c2fo/vfs/v8/backend/mem`](backend/mem/README.md) | In-memory [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem) for tests and ephemeral use ([`backend`](backend/README.md) index) |
| [`github.com/c2fo/vfs/v8/backend/os`](backend/os/README.md) | Local filesystem (`file` scheme) |
| [`github.com/c2fo/vfs/v8/backend/s3`](backend/s3/README.md) | AWS S3 (`s3://`) |
| [`github.com/c2fo/vfs/v8/backend/gs`](backend/gs/README.md) | Google Cloud Storage (`gs://`) |
| [`github.com/c2fo/vfs/v8/backend/azure`](backend/azure/README.md) | Azure Blob Storage (`az` scheme) |
| [`github.com/c2fo/vfs/v8/backend/ftp`](backend/ftp/README.md) | FTP (`ftp://`) |
| [`github.com/c2fo/vfs/v8/backend/sftp`](backend/sftp/README.md) | SFTP (`sftp://`) |
| [`github.com/c2fo/vfs/v8/backend/testsuite`](backend/testsuite/README.md) | Shared conformance tests (`vfsintegration`) |
| [`github.com/c2fo/vfs/v8/v7adapter`](v7adapter/README.md) | Adapters from v7 types to v8 interfaces |

## Core types (at a glance)

- **[`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem)** — `NewFile`, `NewLocation`, `Name`, `Scheme`.
- **[`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location)** — Directory- or prefix-like scope; embeds [`Lister`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister); `NewLocation`, `NewFile`, `DeleteFile`, `Exists`, `FileSystem`.
- **[`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File)** — Stream + identity + metadata (see above).
- **[`Entry`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Entry)** — One listed name with [`EntryKind`](https://pkg.go.dev/github.com/c2fo/vfs/v8#EntryKind) (file, location, blob, unknown) and optional resolved handles.
- **[`Entry.DirEntry`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Entry.DirEntry)** — Optional [`fs.DirEntry`](https://pkg.go.dev/io/fs#DirEntry) view for tooling; not a full read/write [`fs.FS`](https://pkg.go.dev/io/fs#FS).

## Listing

Listing is **lazy**: consumers range over [`Lister.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) and stop early; backends keep pagination tokens internal. [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) is unsupported in the v7 adapter and yields [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported) until a backend implements it.

## POSIX and cloud semantics

Paths are absolute and normalized per backend. **URIs** remain the portable interchange (same spirit as v7). Where POSIX and object-store semantics differ, backends document behavior. Use [`errors.Is`](https://pkg.go.dev/errors#Is) with [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported) when a capability is missing.

## Relationship to v7

- v8 **reuses** [`github.com/c2fo/vfs/v7/utils/authority`](https://pkg.go.dev/github.com/c2fo/vfs/v7/utils/authority) for authority values on identities.
- v7 **option types** live in [`github.com/c2fo/vfs/v7/options`](https://pkg.go.dev/github.com/c2fo/vfs/v7/options); v8 defines parallel markers in [`v8/options`](options/README.md). They are not the same Go types—use the v8 package when implementing v8 interfaces.
- **[`v7adapter`](v7adapter/README.md)** wraps existing v7 implementations so you can call v8-shaped APIs without rewriting backends immediately.

## Further reading

- **[Migrating from v7](MIGRATION.md)** — registration, listing, deletes, copy/move, and [`v7adapter`](v7adapter/README.md)
- [Backends (`backend/`)](backend/README.md)
- [Option markers (`options`)](options/README.md)
- [v7 adapter (`v7adapter`)](v7adapter/README.md)
- Package overview in [doc.go](doc.go) (also rendered on pkg.go.dev)
