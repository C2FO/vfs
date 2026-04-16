# VFS v8 backends

Storage implementations that satisfy [`github.com/c2fo/vfs/v8`](https://pkg.go.dev/github.com/c2fo/vfs/v8) interfaces live under **`backend/`**, mirroring the layout of the v7 module (`backend/os`, `backend/s3`, …).

| Package | Import path | Description |
|---------|-------------|-------------|
| [mem](mem/README.md) | `github.com/c2fo/vfs/v8/backend/mem` | In-memory `FileSystem` for tests and ephemeral use |
| [os](os/README.md) | `github.com/c2fo/vfs/v8/backend/os` | Local filesystem (`file` scheme), v8 [`Lister`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister) listing |
| [s3](s3/README.md) | `github.com/c2fo/vfs/v8/backend/s3` | AWS S3 (`s3://`), v8 listing and object metadata |
| [gs](gs/README.md) | `github.com/c2fo/vfs/v8/backend/gs` | Google Cloud Storage (`gs://`), v8 listing and object metadata |
| [azure](azure/README.md) | `github.com/c2fo/vfs/v8/backend/azure` | Azure Blob Storage (`az` scheme), v8 listing and blob operations |
| [ftp](ftp/README.md) | `github.com/c2fo/vfs/v8/backend/ftp` | FTP (`ftp://`), v8 listing and file operations; [`WithClientGetter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithClientGetter) / [`WithDataConnGetter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/ftp#WithDataConnGetter) for tests and custom wiring |
| [sftp](sftp/README.md) | `github.com/c2fo/vfs/v8/backend/sftp` | SFTP (`sftp://`), SSH-based listing and file operations; [`WithClientGetter`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/sftp#WithClientGetter) for tests and custom dial logic |
| [testsuite](testsuite/README.md) | `github.com/c2fo/vfs/v8/backend/testsuite` | Shared v8 conformance helpers ([`RunConformanceTests`](testsuite/conformance.go), [`RunIOTests`](testsuite/io_conformance.go)); use from backend integration tests behind `vfsintegration` |

See the [v8 module README](../README.md) for core interfaces and other packages (`options`, `v7adapter`).
