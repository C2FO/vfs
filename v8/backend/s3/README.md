# VFS v8 AWS S3 backend

Package [`github.com/c2fo/vfs/v8/backend/s3`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces using the [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2). It is the v8 counterpart to the v7 [`backend/s3`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend/s3) package.

## Behavior (summary)

- **Scheme:** `s3` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3#Scheme)). URIs look like `s3://bucket/key`.
- **Authority:** the S3 bucket name; paths are absolute key prefixes with `/`.
- **Listing:** [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) with [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption)s; [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive) returns [`ErrNotSupported`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ErrNotSupported).
- **Metadata:** [`File.Stat`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta), [`Authority`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileIdentity), [`LastModified`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta), [`Size`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileMeta).
- **Deletes:** use [`Location.DeleteFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location); optional [`DeleteAllVersions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3#DeleteAllVersions) for versioned buckets. Legacy v7 [`delete.WithAllVersions`](https://pkg.go.dev/github.com/c2fo/vfs/options/delete#WithAllVersions) is still recognized in delete option lists.
- **New-file options:** [`FileContentType`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3#FileContentType) sets `Content-Type` on upload/copy when applicable.
- **Construction:** [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3#NewFileSystem) with [`WithOptions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3#WithOptions), [`WithClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3#WithClient), etc. There is no global v7-style `backend.Register` in v8.
- **Concrete file API:** `*s3.File` still exposes `CopyToFile` / `MoveToFile` / `CopyToLocation` / `MoveToLocation` for ergonomics (same pattern as other v8 blob backends).

## Usage

```go
import (
    "context"

    "github.com/c2fo/vfs/v8/backend/s3"
)

fs := s3.NewFileSystem(s3.WithOptions(s3.Options{Region: "us-east-1"}))
f, err := fs.NewFile("my-bucket", "/path/to/object.txt", s3.FileContentType("text/plain"))
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

With AWS credentials and a **dedicated test bucket**, run the shared [v8 testsuite](../testsuite) against this backend:

```bash
export VFS_S3_BUCKET=your-test-bucket
# optional: export VFS_S3_TEST_PATH=/vfs-integration-test/

go test -tags=vfsintegration ./backend/s3/... -run 'Conformance|IOConformance' -v
```

See [docs/conformance_tests.md](../../../docs/conformance_tests.md) for the general pattern. The v7 module also has `backend/s3/conformance_test.go` using the v7 testsuite; v8 uses `github.com/c2fo/vfs/v8/backend/testsuite`.

## See also

- Package [`doc.go`](doc.go) (overview, ACLs, authentication notes)
- [v8 module overview](../../README.md)
- [Backends index](../README.md)
- [pkg.go.dev](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/s3)
