# VFS v8 Azure Blob Storage backend

Package [`github.com/c2fo/vfs/v8/backend/azure`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces for Azure Blob Storage. It is the v8 counterpart to the v7 [`backend/azure`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend/azure) package.

## Behavior (summary)

- **Scheme:** `az` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#Scheme)). URIs use the `az` scheme with container and blob paths as documented in the package.
- **Listing:** [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) with [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption)s where supported.
- **Deletes:** use [`Location.DeleteFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location) and backend-specific delete options as applicable.
- **Construction:** [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#NewFileSystem) with functional options. Default [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#NewFileSystem) reads account settings from environment variables when options are unset (see package documentation). There is no global v7-style `backend.Register` in v8.

## Construction

- [`WithOptions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#WithOptions) — [`Options`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#Options) for account, endpoint, and client behavior.
- [`WithClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#WithClient) — inject a concrete Azure [`Client`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure#Client) (for example in tests).

## Usage

```go
import "github.com/c2fo/vfs/v8/backend/azure"

fs := azure.NewFileSystem(azure.WithOptions(azure.Options{
	AccountName: "myaccount",
	AccountKey:    "…",
}))
f, err := fs.NewFile("mycontainer", "/path/to/blob.txt")
if err != nil {
	return err
}
defer f.Close()
```

## See also

- [Backends index](../README.md)
- [v8 module overview](../../README.md)
- [pkg.go.dev](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/azure)
