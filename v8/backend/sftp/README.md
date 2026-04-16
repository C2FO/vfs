# VFS v8 SFTP backend

Package [`github.com/c2fo/vfs/v8/backend/sftp`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp) implements the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces over SFTP (SSH). It is the v8 counterpart to the v7 [`backend/sftp`](https://pkg.go.dev/github.com/c2fo/vfs/v7/backend/sftp) package.

## Behavior (summary)

- **Scheme:** `sftp` (see [`Scheme`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#Scheme)). URIs look like `sftp://user@host:22/path/`.
- **Listing:** [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) with [`ListOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8#ListOption)s.
- **Auto-disconnect:** optional idle disconnect via [`Options.AutoDisconnect`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#Options); see [`Options.ConnectTimeout`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#Options) for connection and SSH auth timeouts.
- **Construction:** [`NewFileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#NewFileSystem) with functional options. There is no global v7-style `backend.Register` in v8.

## Construction and dependency injection

Typical options:

- [`WithOptions`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#WithOptions) — SSH keys, passwords, known-hosts callbacks, timeouts, etc. ([`Options`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#Options)).
- [`WithClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#WithClient) — inject a concrete [`Client`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#Client) (for example in tests).
- [`WithConnectTimeout`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#WithConnectTimeout) — shorthand for setting [`Options.ConnectTimeout`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#Options).

For **custom client acquisition** (tests or custom dial logic), use:

- [`WithClientGetter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#WithClientGetter) — supply how the SFTP client and SSH connection are created (defaults to [`GetClient`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#GetClient)).

If unset, each [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#FileSystem) lazily uses the same default as production; there are no package-level replacement variables.

## Usage

```go
import (
	"golang.org/x/crypto/ssh"

	"github.com/c2fo/vfs/v8/backend/sftp"
)

fs := sftp.NewFileSystem(sftp.WithOptions(sftp.Options{
	Username:           "user",
	Password:           "pass",
	KnownHostsCallback: ssh.InsecureIgnoreHostKey(), // example only; use proper host keys in production
}))
loc, err := fs.NewLocation("user@example.com:22", "/remote/dir/")
if err != nil {
	return err
}
_ = loc
```

## See also

- [Backends index](../README.md)
- [v8 module overview](../../README.md)
- [pkg.go.dev](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp)
