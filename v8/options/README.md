# VFS v8 options

Package [`github.com/c2fo/vfs/v8/options`](https://pkg.go.dev/github.com/c2fo/vfs/v8/options) defines **marker interfaces** for typed configuration passed into v8 APIs. Backends implement **concrete option types** that satisfy these interfaces (same pattern as v7’s [`github.com/c2fo/vfs/v7/options`](https://pkg.go.dev/github.com/c2fo/vfs/v7/options)).

## Interfaces

| Interface | Used by | Implementations |
|-----------|---------|-----------------|
| [`NewFileOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8/options#NewFileOption) | [`FileSystem.NewFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#FileSystem.NewFile), [`Location.NewFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location.NewFile) | Backend-specific structs (content type, encryption, …) |
| [`DeleteOption`](https://pkg.go.dev/github.com/c2fo/vfs/v8/options#DeleteOption) | [`File`](https://pkg.go.dev/github.com/c2fo/vfs/v8#File) delete operations and [`Location.DeleteFile`](https://pkg.go.dev/github.com/c2fo/vfs/v8#Location.DeleteFile) | e.g. versioned delete, MFA delete |

Each interface requires a **name** method (`NewFileOptionName`, `DeleteOptionName`) so callers and logs can identify options without reflection.

## Relationship to v7 options

v7 and v8 option types live in **different packages** and are **not interchangeable** at compile time:

- v7: `github.com/c2fo/vfs/v7/options`
- v8: `github.com/c2fo/vfs/v8/options`

When using **[`v7adapter`](../v7adapter/README.md)**, v8 `NewFile` / `DeleteFile` variadic options are **not yet forwarded** to v7; the adapter calls the underlying v7 methods without extra v7 options. For backend-specific v7 options, continue to use v7 constructors and types, or unwrap with [`AsV7File` / `AsV7Location`](../v7adapter/README.md#unwrapping).

## Listing options

Directory listing uses **functional options in the root `vfs` package**, not this subpackage:

- [`WithPrefix`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithPrefix), [`WithRegexp`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRegexp), [`WithNameMatcher`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithNameMatcher), [`WithRecursive`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithRecursive), [`WithPageSize`](https://pkg.go.dev/github.com/c2fo/vfs/v8#WithPageSize)

See the [v8 module README](../README.md#listing).

## See also

- [v8 module overview](../README.md)
- [pkg.go.dev documentation](https://pkg.go.dev/github.com/c2fo/vfs/v8/options)
