# VFS v8 backend testsuite

Package [`github.com/c2fo/vfs/v8/backend/testsuite`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/testsuite) provides shared **conformance** helpers for implementations of the v8 [`FileSystem`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem), [`Location`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location), and [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) interfaces. Storage backends do not import this from production code; they call it from tests, usually behind the **`vfsintegration`** build tag.

## Entry points

- [`RunConformanceTests`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/testsuite#RunConformanceTests) — core filesystem, location, and file behavior.
- [`RunIOTests`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/testsuite#RunIOTests) — optional scripted read/write/seek when files support `io.Seeker`.
- [`CollectList`](https://pkg.go.dev/github.com/c2fo/vfs/v8/backend/testsuite#CollectList) — materialize names from [`Location.List`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Lister.List) for assertions.

## Running

From a backend package (example: in-memory backend):

```bash
go test -tags=vfsintegration ./...
```

See [docs/conformance_tests.md](../../../docs/conformance_tests.md) in the repository for the full convention.

## See also

- [Backends index](../README.md)
- [v8 module overview](../../README.md)
