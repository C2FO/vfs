# Google Drive Backend for VFS

A VFS backend for Google Drive (`gdrive://`) using the Drive v3 REST API directly over `net/http`,
with `golang.org/x/oauth2` for authentication. It does not depend on the Google API client libraries.

## Installation

```bash
go get github.com/c2fo/vfs/contrib/backend/gdrive
```

## Usage

```go
import (
    "github.com/c2fo/vfs/contrib/backend/gdrive"
)

func main() {
    fs := gdrive.NewFileSystem(gdrive.WithAccessToken("ya29...."))

    location, err := fs.NewLocation("", "/Reports/2026/")
    if err != nil {
        log.Fatal(err)
    }

    file, err := location.NewFile("summary.txt")
    if err != nil {
        log.Fatal(err)
    }
    if _, err := file.Write([]byte("hello")); err != nil {
        log.Fatal(err)
    }
    if err := file.Close(); err != nil { // upload happens on Close
        log.Fatal(err)
    }
}
```

Through `vfssimple` (import the package for its registration side effect and set `VFS_GDRIVE_ACCESS_TOKEN`):

```go
f, err := vfssimple.NewFile("gdrive:///Reports/2026/summary.txt")
```

### URIs and drives

| URI | Meaning |
|-----|---------|
| `gdrive:///path/to/file.txt` | `path/to/file.txt` in the user's My Drive (or the drive set with `WithDriveID`) |
| `gdrive://0ABcDeFg/path/to/file.txt` | the same path in the shared drive with ID `0ABcDeFg` |

## Authentication

| Option | Use |
|--------|-----|
| `WithAccessToken(token)` | a pre-obtained OAuth2 token; never refreshed |
| `WithTokenSource(ts)` | any `oauth2.TokenSource` (handles refresh) |
| `WithServiceAccountJSON(key)` | a service account key; share the target folder or shared drive with the account's email address |
| `WithDefaultCredentials()` | Google Application Default Credentials |
| `WithDeviceCode(id, secret, cb)` | OAuth2 device flow; the OAuth client must be of the "TVs and Limited Input devices" type |

With no credential option, `VFS_GDRIVE_ACCESS_TOKEN` is used when set.

### Scopes

`WithScope` selects the OAuth2 scope. The default, `gdrive.ScopeDrive` (`.../auth/drive`), sees all
files the account can access. `gdrive.ScopeDriveFile` (`.../auth/drive.file`) only exposes files the
application created: this is safer, but **files that already exist in Drive cannot be addressed by
path** and appear to be missing.

## Options

| Option | Purpose |
|--------|---------|
| `WithDriveID(id)` | default shared drive for locations with no authority |
| `WithChunkSize(n)` | resumable upload chunk size (default 8 MiB, rounded down to a 256 KiB multiple) |
| `WithTempDir(dir)` | directory for temp files used while reading/writing |
| `WithRetryCount(n)` | retries for transient errors (default 3) |
| `WithRequestTimeout(d)` | per-request timeout (default 2m, `0` disables); an idle timeout for downloads, and an upload chunk must be sent within it |
| `WithRequestLogger(fn)` | call `fn` once per HTTP attempt with a `RequestLog` (method, redacted URL, attempt, status, duration, error; never credentials). Map it onto whatever logger you use |
| `WithTrashOnDelete()` | move deleted files to the trash instead of deleting permanently |
| `WithExportMimeType(m)` | export Google-native files (Docs, Sheets) as `m` when read |
| `WithBaseURL(u)` / `WithHTTPClient(c)` | point at a proxy or test server / customize transport |
| `WithClient(c)` | supply a custom `Client` (testing) |

```go
gdrive.WithRequestLogger(func(_ context.Context, rec gdrive.RequestLog) {
    log.Printf("%s %s -> %d (%s) %v", rec.Method, rec.URL, rec.Status, rec.Duration, rec.Err)
})
```

## How Drive maps onto VFS

Drive is not a path-addressed store, so a few behaviors differ from object stores. The complete set of
decisions is in [DESIGN.md](./DESIGN.md); the ones that matter in practice:

- **Duplicate names.** Drive allows two files named `a.txt` in one folder; a VFS path cannot say which.
  Operations on such a path (`Exists`, `Size`, `Read`, `Delete`, copy, move, and writes) fail with
  `gdrive.ErrAmbiguous`. Listing shows both entries. Rename or remove one in Drive to resolve it.
- **Folders are guarded.** `Delete`, copy and move on a path that is a folder fail with
  `gdrive.ErrIsFolder`; Drive would otherwise delete or move the folder and everything in it.
  `File.Exists` is false for a folder.
- **Missing parent folders are created** on write, copy fallback and move fallback.
- **Writes replace in place.** Writing an existing file updates it (same file ID, sharing preserved).
  Copy and move onto an existing file *replace* it: the destination path then holds a different file ID,
  so the old destination's sharing, comments and revision history do not carry over. The old file is only
  removed after the new one exists; if that removal fails the error says so, and both files remain.
- **A path through a file** (`report.txt/child`) does not exist; writing there fails with `gdrive.ErrNotFolder`.
- **Delete is permanent** by default, so `Exists` is false afterwards. Use `WithTrashOnDelete` to keep
  files recoverable; trashed files are invisible to the backend either way. In a shared drive only
  organizers may delete permanently. Drive answers that refusal with `404 notFound`, and the file is
  trashed instead. Drive purges trashed files after 30 days.
- **Google-native files** (Docs, Sheets, Slides, shortcuts) have no bytes. Reading one returns
  `gdrive.ErrNotDownloadable` unless `WithExportMimeType` is set. They are listed, and can be moved or deleted,
  but writing, copying or moving bytes over one fails with `gdrive.ErrNotDownloadable` rather than destroying it.
- **Uploads happen on `Close`.** Reads and writes go through a local temp file. Files up to 5 MiB go in
  one multipart request, larger files use a resumable upload. A failed chunk is never blindly resent:
  the session is asked how much it received and only the rest is sent (up to `WithRetryCount` times).
- **`Touch`** creates an empty file, or sets the modification time of an existing one without touching content.
- **Rate limits.** `429`, `503` and Drive's rate-limit `403` responses are retried with backoff
  (honoring `Retry-After`) for idempotent requests only. A create, a new-file upload, and a copy are
  not retried: Drive can apply the POST and still return one of those statuses, and a second POST
  would leave two items with the same name.
- **Concurrent writers.** Folder creation is serialized within one `FileSystem`, so parallel writes into
  a new folder share it. Drive itself has no locking and no atomic create-if-absent by name, so two
  *processes* creating the same new path at once can still produce duplicates, which then surface as
  `ErrAmbiguous`.

Folder IDs are cached for a minute. A miss under a cached folder costs one extra request to check the
folder still exists; if it is gone the cache is dropped and the path re-resolved. Other changes to folder
structure by other clients (such as a rename) may take up to the TTL to be noticed.

## Testing

```bash
# Unit tests, plus the shared conformance + IO suites against an in-process fake Drive
go test ./...

# Wire-protocol tests against a WireMock container (needs Docker)
go test -tags=vfsintegration -run TestWireMockTestSuite ./...

# Real Drive (see the header of realdrive_test.go for every variable)
VFS_GDRIVE_ACCESS_TOKEN=... go test -v -tags=vfsintegration -run 'TestConformance|TestRealDrive' ./...
```

The conformance suite expects its base location (`VFS_GDRIVE_TEST_PATH`, default `/vfs-test/`) to exist;
the real-drive test creates it. Real-drive tests create and delete files under that path and a unique
`vfs-realtest-*` folder.
