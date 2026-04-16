# VFS v8 Google Cloud Storage backend

Package [`github.com/c2fo/vfs/v8/backend/gs`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces using [`cloud.google.com/go/storage`](https://pkg.go.dev/cloud.google.com/go/storage). It is the v8 counterpart to the v7 [`backend/gs`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend/gs) package.

## Behavior (summary)

- **Scheme:** `gs` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#Scheme)). URIs look like `gs://bucket/object/key`.
- **Authority:** the GCS bucket name; paths are absolute object name prefixes with `/`.
- **Listing:** [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) with [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption)s; [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) returns [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported).
- **Metadata:** [`File.Stat`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta), [`Authority`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileIdentity), [`LastModified`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta), [`Size`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta).
- **Deletes:** use [`Location.DeleteFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location); optional [`DeleteAllVersions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#DeleteAllVersions) when object versioning is enabled. Legacy v7 [`delete.WithAllVersions`](https://pkg.go.dev/github.com/c2fo/vfs/options/delete#WithAllVersions) is still recognized in delete option lists.
- **New-file options:** [`FileContentType`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#FileContentType) sets object content type on writers and copies when applicable.
- **Construction:** [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#NewFileSystem) with [`WithOptions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#WithOptions), [`WithClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#WithClient), [`WithContext`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#WithContext), [`WithRetryer`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs#WithRetryer). There is no global v7-style `backend.Register` in v8.
- **Concrete file API:** `*gs.File` exposes `CopyToFile` / `MoveToFile` / `CopyToLocation` / `MoveToLocation` for ergonomics.

## Usage

```go
import (
    "context"

    "github.com/c2fo/vfs/v8/backend/gs"
)

fs := gs.NewFileSystem(gs.WithOptions(gs.Options{}))
f, err := fs.NewFile("my-bucket", "/path/to/object.txt", gs.FileContentType("text/plain"))
if err != nil {
    return err
}
defer f.Close()

loc, err := fs.NewLocation("my-bucket", "/path/to/")
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

## Integration / conformance

With Application Default Credentials and a **dedicated test bucket**, run the shared [v8 testsuite](../testsuite):

```bash
export VFS_GS_BUCKET=your-test-bucket
# optional: export VFS_GS_TEST_PATH=/vfs-integration-test/

go test -tags=vfsintegration ./backend/gs/... -run 'Conformance|IOConformance|PersistentFolder' -v
```

See [docs/conformance_tests.md](../../../docs/conformance_tests.md). The v7 `backend/gs` package has a parallel conformance test using the v7 testsuite.

## See also

- Package [`doc.go`](doc.go) (authentication and client notes)
- [v8 module overview](../../README.md)
- [Backends index](../README.md)
- [pkg.go.dev](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/gs)
