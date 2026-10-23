# sqlfs

A [vfs](https://github.com/c2fo/vfs) backend that stores files in a relational database through `database/sql`.
PostgreSQL and SQLite are supported through built-in dialects. The package imports **no database driver**: you open the
`*sql.DB` with the driver you prefer and hand it to the backend.

```
go get github.com/c2fo/vfs/contrib/backend/sqlfs
```

URIs look like `sqlfs://<namespace>/<path>`. The authority is a **namespace**, so one database can hold several
independent file trees.

## Quick start

```go
import (
    "context"
    "database/sql"

    _ "github.com/jackc/pgx/v5/stdlib"

    "github.com/c2fo/vfs/contrib/backend/sqlfs"
    "github.com/c2fo/vfs/v7/backend"
)

db, err := sql.Open("pgx", "postgres://user:pass@host:5432/app")
if err != nil { /* ... */ }

fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.PostgreSQL()))
if err != nil { /* ... */ }

if err := fsys.EnsureSchema(ctx); err != nil { /* ... */ }

// sqlfs never registers itself. Register your configured instance to use it with vfssimple.
backend.Register(sqlfs.Scheme, fsys)

f, _ := fsys.NewFile("documents", "/reports/q1.txt")
_, _ = f.Write([]byte("hello"))
err = f.Close() // publishes atomically
```

`NewFileSystem` performs no I/O. Nothing touches the database until you call a method that needs it.

### SQLite

Use a pure-Go driver such as `modernc.org/sqlite` (name `sqlite`) and set these pragmas in the DSN:

```
file:/path/files.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate
```

* `busy_timeout` avoids immediate `SQLITE_BUSY` failures between writers.
* `foreign_keys(1)` enforces the chunk to revision relationship.
* `_txlock=immediate` takes the write lock when a transaction begins, so write transactions do not fail midway.

Only the pragmas matter; any SQLite driver works with `sqlfs.SQLite()`.

## Schema lifecycle

| Method | Purpose |
|--------|---------|
| `EnsureSchema(ctx)` | Creates or upgrades the tables. Safe to call concurrently from several processes (PostgreSQL takes an advisory lock). |
| `ValidateSchema(ctx)` | Read-only check that the schema exists and is the version this build understands. Use it at startup when the runtime user cannot run DDL. |

Run `EnsureSchema` from a migration job or a privileged startup step. The runtime account only needs
`SELECT`, `INSERT`, `UPDATE` and `DELETE` on the four tables (`<prefix>schema`, `<prefix>revisions`, `<prefix>files`,
`<prefix>chunks`). Migrations are embedded in the package under `migrations/<dialect>/` if you prefer to apply them
with your own tool; replace each literal `vfs_` identifier with your prefix. `EnsureSchema` double-quotes every table
and index name, so if your prefix has uppercase letters or starts with a digit, quote the names the same way.

## Options

| Option | Default | Notes |
|--------|---------|-------|
| `WithDB(db)` | required | Owned by the caller; sqlfs never closes it. |
| `WithDialect(d)` | required | `sqlfs.PostgreSQL()` or `sqlfs.SQLite()`. Implement `Dialect` for others. |
| `WithTablePrefix(p)` | `vfs_` | 1-32 characters of `A-Za-z0-9_`. |
| `WithChunkSize(n)` | 1 MiB | 4 KiB to 64 MiB. Existing content keeps the size it was written with. |
| `WithMaxFileSize(n)` | unlimited | Writes beyond the limit fail. |
| `WithOperationTimeout(d)` | 30s | Bounds each database round trip; exceeding it returns `ErrTimeout`. |
| `WithStagingRetention(d)` | 24h | Idle time before `Cleanup` removes an abandoned write. |
| `WithRevisionRetention(d)` | 1h | How long replaced or deleted content stays readable for in-progress readers. |

## Semantics

* **Atomic publish.** Writes accumulate in a staged revision and become visible only on `Close`. A crashed writer
  never exposes partial content. `Abort` discards a write explicitly.
* **Stable reads.** A reader is pinned to the revision it opened, so replacing the file does not corrupt a read in
  progress. If the pinned content is reclaimed by `Cleanup` before the reader finishes, reads fail with `ErrConflict`.
* **Optimistic concurrency.** Each file has a generation counter. If another writer replaced the file after yours
  started, `Close` returns `ErrConflict` (`ErrAlreadyExists` when two creators race). Re-read and retry:

  ```go
  for range 3 {
      if err = update(); !errors.Is(err, sqlfs.ErrConflict) { break }
  }
  ```

* **Write mode follows the OS backend.** Writing after `Read`/`Seek` on an existing file overlays the written range,
  copying the untouched chunks inside the database. Otherwise the write replaces the content.
* **Sparse writes.** Seeking past the end and writing leaves a hole that reads as zeros; only touched chunks are stored.
* **Server-side copy and move.** Within one `FileSystem`, copy is an in-database chunk copy and move is a metadata
  update; no content crosses the wire. Copies to or from other backends stream.

## Cleanup

Replaced content, deleted files and abandoned writes are not removed inline. Call `Cleanup` on a schedule (a cron job
or a ticker in one process is enough; it is safe to run concurrently):

```go
stats, err := fsys.Cleanup(ctx)
```

sqlfs starts no background goroutines. Cleanup works in small batches and honors context cancellation, so it can be
interrupted and resumed.

## Errors

`ErrNotFound` (wraps `fs.ErrNotExist`), `ErrAlreadyExists`, `ErrConflict`, `ErrInvalidConfig`, `ErrInvalidSchema`,
`ErrUnavailable`, `ErrTimeout` (wraps `context.DeadlineExceeded`) and `ErrCorrupt`. Use `errors.Is`.
Transient database errors (deadlocks, serialization failures, lock timeouts) are retried up to three times with
jittered backoff before surfacing.

## Limitations

* Directories are implicit; there is no way to create an empty directory.
* `ListByPrefix` and `ListByRegex` filter in Go after listing the directory, so very large directories cost a scan.
* Modification times and retention windows use the clock of the process that performs the operation, so keep
  clocks of processes sharing a database reasonably in sync.
* Content lives in ordinary tables; very large datasets may be better served by an object store.

## Testing

```
go test ./...                                  # SQLite, no external services
go test -tags=vfsintegration ./...             # adds PostgreSQL via testcontainers (needs Docker)
go test -run xxx -bench . ./...                # benchmarks
```
