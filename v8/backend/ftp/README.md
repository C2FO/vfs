# VFS v8 FTP backend

Package [`github.com/c2fo/vfs/v8/backend/ftp`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces over FTP. It is the v8 counterpart to the v7 [`backend/ftp`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend/ftp) package.

## Behavior (summary)

- **Scheme:** `ftp` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#Scheme)). URIs look like `ftp://user@host:21/path/`.
- **Listing:** [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) with [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption)s.
- **Deletes:** files are removed via internal helpers; use [`Location.DeleteFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location) where applicable.
- **Construction:** [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#NewFileSystem) with functional options. There is no global v7-style `backend.Register` in v8.

## Construction and dependency injection

Typical options:

- [`WithOptions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithOptions) — credentials, TLS, timeouts, and other [`Options`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#Options).
- [`WithClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithClient) — inject a concrete FTP [`types.Client`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp/types#Client) (for example in tests).
- [`WithDataConn`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithDataConn) — inject an existing data connection.

For **custom connection behavior** without replacing the whole client (for example in unit tests or layered auth), use:

- [`WithClientGetter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithClientGetter) — supply how the **control** connection is obtained (defaults to [`GetClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#GetClient)).
- [`WithDataConnGetter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithDataConnGetter) — supply how read/write/single-op **data** connections are obtained (defaults to the package implementation used for production).

If unset, each [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#FileSystem) lazily uses the same defaults as production; there are no package-level replacement variables.

## Usage

```go
import "github.com/c2fo/vfs/v8/backend/ftp"

fs := ftp.NewFileSystem(ftp.WithOptions(ftp.Options{
	Username: "user",
	Password: "pass",
}))
f, err := fs.NewFile("user@ftp.example.com:21", "/remote/path/file.txt")
if err != nil {
	return err
}
defer f.Close()
```

## See also

- [Backends index](../README.md)
- [v8 module overview](../../README.md)
- [pkg.go.dev](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp)
