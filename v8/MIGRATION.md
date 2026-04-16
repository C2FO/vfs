# Migrating from v7 to v8

This guide helps you move call sites and tests from [`github.com/c2fo/vfs/v7`](https://pkg.go.dev/github.com/c2fo/vfs/v7) to [`github.com/c2fo/vfs/v8`](https://pkg.go.dev/github.com/c2fo/vfs/v8). It complements the [module README](README.md), [`v7adapter` package](v7adapter/README.md), and per-backend docs under [`backend/`](backend/README.md).

## Should you migrate yet?

- **v7** remains the primary choice for production storage flows unless you are adopting v8 deliberately (see [README § Status](README.md#status)).
- You can **depend on both modules** during migration: add a `require` for `github.com/c2fo/vfs/v8` alongside v7 and migrate packages incrementally.

## 1. Module and imports

```bash
go get github.com/c2fo/vfs/v8@latest
```

- Core types: `github.com/c2fo/vfs/v8` (often imported as `vfs`).
- Typed options for v8 APIs: `github.com/c2fo/vfs/v8/options` — these are **not** the same Go types as `github.com/c2fo/vfs/v7/options`. Use v8 markers when calling v8 `NewFile`, `DeleteFile`, and listing helpers.
- Authority strings and parsing still use **`github.com/c2fo/vfs/v7/utils/authority`** from both modules.

## 2. File system construction (no global registration)

| v7 | v8 |
|----|-----|
| Register backends with [`backend.Register`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend#Register) and use [`vfssimple`](https://pkg.go.dev/github.com/c2fo/vfs/v7/vfssimple) for URI-based helpers | Construct a **concrete** v8 backend with `NewFileSystem(...)` and functional options (e.g. [`s3.NewFileSystem`](backend/s3/README.md)). There is **no** v8 `backend.Register` and **no** v8 `vfssimple` yet (see [mem backend README](backend/mem/README.md)). |

Obtain `FileSystem`, `Location`, and `File` from that instance: `fs.NewLocation(authority, path)`, `fs.NewFile(authority, path, opts...)`, or `loc.NewFile(rel, opts...)`.

## 3. Listing: slices → iterator + options

v7 [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v7#Location) exposes three methods that return `([]string, error)`:

- `List()`
- `ListByPrefix(prefix string)`
- `ListByRegex(regex *regexp.Regexp)`

v8 [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location) embeds [`Lister`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister): a single [`List(ctx, opts...)`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) returning [`iter.Seq2`](https://pkg.go.dev/iter#Seq2)`[Entry, error]`.

| v7 call | v8 equivalent (conceptually) |
|---------|--------------------------------|
| `List()` | `List(ctx)` — no extra list options |
| `ListByPrefix(prefix)` | `List(ctx, vfs.WithPrefix(prefix))` |
| `ListByRegex(re)` | `List(ctx, vfs.WithRegexp(re))` |

Use [`WithPageSize`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithPageSize), [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive), etc. where the backend supports them. Each iteration yields an [`Entry`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Entry) (name and kind), not only a string.

**Context:** always pass a [`context.Context`](https://pkg.go.dev/context#Context) into `List` for cancellation.

## 4. Deletes

| v7 | v8 |
|----|-----|
| [`File.Delete`](https://pkg.go.dev/github.com/c2fo/vfs/v7#File.Delete)(`opts ...DeleteOption`) | Prefer [`Location.DeleteFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location.DeleteFile)(`rel, opts ...`) on the parent location. The v8 [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interface does **not** include `Delete`; backend-specific concrete types may still offer helpers, but the portable contract is delete-by-name on the location. |

Use v8 [`options`](options/README.md) for delete markers (and backend-specific types such as S3/GCS “all versions” where documented).

## 5. Copy and move

v7 [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v7#File) methods `CopyToLocation`, `CopyToFile`, `MoveToLocation`, `MoveToFile` take no context.

v8 separates streaming and identity:

- Prefer [`vfs.Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy) with `context` for streaming copies where appropriate.
- For method-style transfers, use [`FileTransfers`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers) (`CopyToLocation`, `CopyToFile`, `MoveToLocation`, `MoveToFile` with `ctx` as the first argument). Concrete v8 backends may also expose similar methods on `*File` for ergonomics; see each backend README.

## 6. `File` surface: seek and metadata

- v7 [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v7#File) embeds [`io.Seeker`](https://pkg.go.dev/io#Seeker).
- v8 [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) requires [`io.Reader`](https://pkg.go.dev/io#Reader), [`io.Writer`](https://pkg.go.dev/io#Writer), [`io.Closer`](https://pkg.go.dev/io#Closer). **Seek** and ranged I/O are **optional**: use type assertions to [`io.Seeker`](https://pkg.go.dev/io#Seeker), [`FileReaderAt`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileReaderAt), and [`FileWriterAt`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileWriterAt).
- Metadata: v8 [`FileMeta`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta) uses [`Stat() (fs.FileInfo, error)`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta.Stat) instead of ad hoc v7-style accessors on the interface.

## 7. Incremental migration with `v7adapter`

If your code still obtains **v7** `FileSystem` / `Location` / `File` from registered backends, wrap them:

| Wrap | Function |
|------|----------|
| v7 → v8 `FileSystem` | [`FileSystemFromV7`](v7adapter/README.md) |
| v7 → v8 `Location` | [`LocationFromV7`](v7adapter/README.md) |
| v7 → v8 `File` | [`FileFromV7`](v7adapter/README.md) |
| v7 → v8 `FileTransfers` | [`FileTransfersFromV7`](v7adapter/README.md) |

Unwrap with [`AsV7File`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#AsV7File), [`AsV7Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#AsV7Location), [`AsV7FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/v7adapter#AsV7FileSystem) when you need the original v7 value.

**Limitations** (see [v7adapter README](v7adapter/README.md#limitations)):

- v8 `NewFile` / `DeleteFile` option values are **not** forwarded as v7 options; adapter calls v7 without those extras.
- [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) is not implemented on top of v7 listing APIs.

## 8. Suggested migration order

1. Add the v8 module and compile a single package.
2. Replace listing call sites with `List` + `ctx` + options; collect `Entry` names as needed.
3. Replace `File.Delete` with `Location.DeleteFile` (or backend-specific helpers).
4. Thread `context` into copy/move helpers per [`FileTransfers`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileTransfers).
5. Where you cannot yet switch construction, **wrap** v7 values with `v7adapter` and migrate callers to v8 interfaces first; later swap to native v8 backends.

## 9. Concrete examples (v8)

The snippets below use the in-memory backend so they stay self-contained. Swap `mem.NewFileSystem()` for [`s3.NewFileSystem`](backend/s3/README.md), [`gs.NewFileSystem`](backend/gs/README.md), [`os`](backend/os/README.md), [`azure.NewFileSystem`](backend/azure/README.md), [`ftp.NewFileSystem`](backend/ftp/README.md), or [`sftp.NewFileSystem`](backend/sftp/README.md) with the right options for real storage.

### 9.1 Construct a file system, location, and file

```go
import (
	vfs "github.com/c2fo/vfs/v8"
	"github.com/c2fo/vfs/v8/backend/mem"
)

func writeMemFile() error {
	fs := mem.NewFileSystem()

	// Authority is the mem "volume"; path must be absolute.
	loc, err := fs.NewLocation("myvol", "/data/")
	if err != nil {
		return err
	}

	f, err := loc.NewFile("hello.txt")
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write([]byte("hello, v8")); err != nil {
		return err
	}
	_ = loc.URI() // mem://myvol/data/
	_ = f.String()
	return nil
}

// Or in one step from authority + absolute path:
func newFromPath(fs *mem.FileSystem) (vfs.File, error) {
	return fs.NewFile("myvol", "/data/hello.txt")
}
```

Use `fs.NewFile("myvol", "/data/hello.txt")` when you prefer authority + absolute file path in one call.

### 9.2 List everything at a location

```go
func listAll(ctx context.Context, loc vfs.Location) error {
	for ent, err := range loc.List(ctx) {
		if err != nil {
			return err
		}
		_ = ent.Name
		_ = ent.Kind // vfs.EntryKindFile, vfs.EntryKindLocation, …
	}
	return nil
}
```

### 9.3 List with prefix (replaces v7 `ListByPrefix`)

```go
func listLogs(ctx context.Context, loc vfs.Location) error {
	for ent, err := range loc.List(ctx, vfs.WithPrefix("logs/")) {
		if err != nil {
			return err
		}
		_ = ent.Name
	}
	return nil
}
```

### 9.4 List with regexp (replaces v7 `ListByRegex`)

```go
import "regexp"

func listTxt(ctx context.Context, loc vfs.Location) error {
	re := regexp.MustCompile(`\.txt$`)
	for ent, err := range loc.List(ctx, vfs.WithRegexp(re)) {
		if err != nil {
			return err
		}
		_ = ent.Name
	}
	return nil
}
```

### 9.5 Collect names into a slice

```go
func namesAt(ctx context.Context, loc vfs.Location) ([]string, error) {
	var out []string
	for ent, err := range loc.List(ctx) {
		if err != nil {
			return nil, err
		}
		out = append(out, ent.Name)
	}
	return out, nil
}
```

### 9.6 Delete a file by relative name (replaces v7 `File.Delete` for many cases)

```go
func removeReport(loc vfs.Location) error {
	return loc.DeleteFile("old-report.csv")
}
```

### 9.7 Streaming copy with [`vfs.Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy)

[`Copy`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Copy) is a small `io.Copy` wrapper that checks `ctx` before starting:

```go
func streamCopy(ctx context.Context, dst vfs.File, src vfs.File) (int64, error) {
	// Ensure implementations support Reader/Writer as needed.
	return vfs.Copy(ctx, dst, src)
}
```

Backend-specific `*File` types often also implement `CopyToFile` / `MoveToFile` without going through `vfs.Copy`; see the backend README.

### 9.8 Optional seek (v8 `File` does not require `io.Seeker`)

```go
import (
	"io"
	"io/fs"
)

func seekIfSupported(f vfs.File) error {
	if s, ok := f.(io.Seeker); ok {
		_, err := s.Seek(0, io.SeekStart)
		return err
	}
	return nil
}

func statModTime(f vfs.File) (fs.FileInfo, error) {
	return f.Stat()
}
```

### 9.9 Wrap existing v7 values for v8 callers

```go
import (
	"context"

	vfsv7 "github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v8/v7adapter"
)

func listV7Backend(ctx context.Context, fs vfsv7.FileSystem) error {
	locV7, err := fs.NewLocation("vol", "/data/")
	if err != nil {
		return err
	}
	loc := v7adapter.LocationFromV7(locV7)

	for ent, err := range loc.List(ctx) {
		if err != nil {
			return err
		}
		_ = ent.Name
	}
	return nil
}
```

### 9.10 Side-by-side: v7 listing vs v8 listing

**v7** (returns a slice):

```go
// names, err := locV7.List()
// names, err := locV7.ListByPrefix("pre")
// names, err := locV7.ListByRegex(re)
```

**v8** (iterator + options):

```go
// for ent, err := range loc.List(ctx) { … }
// for ent, err := range loc.List(ctx, vfs.WithPrefix("pre")) { … }
// for ent, err := range loc.List(ctx, vfs.WithRegexp(re)) { … }
```

## See also

- [v8 README](README.md) — design goals and package layout
- [v7adapter README](v7adapter/README.md) — wrappers and limitations
- [Backends index](backend/README.md) — v8 backend packages and schemes
- [Option markers](options/README.md) — v8 `NewFile` / delete options
