# gdrive:// semantics design

Google Drive is not a path-addressed store. Files are addressed by opaque ID, folders are files with
mime type `application/vnd.google-apps.folder`, a folder may hold several children with the same
name, and some "files" have no bytes at all (Docs, Sheets, shortcuts). VFS assumes unique
path-addressed files and byte content. This document records the rule chosen for every mismatch.
Each rule is exercised by the shared `backend/testsuite` suites (`RunConformanceTests`,
`RunIOTests`) plus backend-specific tests.

## URI and authority

`gdrive://<authority>/path/to/file`. The authority is a drive ID: empty means the user's My Drive,
otherwise a shared drive ID. `WithDriveID` sets the default for locations created with an empty
authority. One client (and one folder-ID cache) is kept per drive.

## Path resolution

Paths are resolved segment by segment from the drive root (`root` for My Drive, the drive ID for a
shared drive) with

    files.list q="'<parentId>' in parents and name='<segment>' and trashed=false"

with `supportsAllDrives=true`, `includeItemsFromAllDrives=true` (and `corpora=drive&driveId=` for
shared drives). Names are escaped for the query language (`\` and `'`). Resolved **folder** IDs
are cached for one minute; a file is always re-resolved so external edits are never masked. When an
operation reports not-found while relying on a cached folder ID, the folder is checked
(`files.get fields=id,trashed`); only if it is deleted or trashed is the cache dropped and the
operation re-run once. That includes a missing child of a cached folder: the cached folder is
checked, and when it still exists the not-found is returned with the cache left intact. A plain miss
therefore costs one request. A segment that resolves to a file fails with `ErrNotFolder`, which
`Exists`/`List` treat as absent. A segment that matches more than one item, including a folder that
shares its name with a file, fails with `ErrAmbiguous`.

Folder creation is serialized per client (re-checking the cache and re-listing under the lock), so
parallel writers in one process share a new folder rather than each creating a same-named duplicate.

## Folders and locations

VFS locations are virtual. `Location.Exists` is true when the path resolves to a folder (the root
always exists). Writing a file creates every missing parent folder with an explicit
`files.create` (never relying on implicit creation). `Location.List` returns only non-folder
children. `File.Exists` is false when the path resolves to a folder. `Delete`, `CopyToFile`
and `MoveToFile` first verify the item is a file, so a folder path can never be deleted, copied or
moved by a file operation (Drive deletes folders recursively).

## Duplicate names

Drive permits `a.txt` twice in one folder. Rule: any read-style or destructive operation that
resolves to more than one match (`Exists`, `Size`, `LastModified`, `Read`, `Delete`, `Copy`, `Move`,
and intermediate folder resolution) fails with `ErrAmbiguous` naming the path. Writes replace the
single match in place (`files.update`), or create when none exists; a write to an ambiguous path
fails. `List` returns every name, so duplicates appear twice; callers see the ambiguity at the
first operation on that name.

## Delete vs trash

`files.delete` (permanent) by default so `Exists` is false afterwards and storage is released.
`WithTrashOnDelete` switches to `files.update {trashed:true}`. All queries filter `trashed=false`.
In a shared drive, members below organizer may trash but not delete. The live API answers
`files.delete` with `404 notFound` in that case, and the backend falls back to trashing. A `403
insufficientFilePermissions` is treated the same way.

## Non-byte items

Items whose mime type starts with `application/vnd.google-apps.` and is not a folder (Docs, Sheets,
Slides, shortcuts) cannot be downloaded. `Read` returns `ErrNotDownloadable` unless
`WithExportMimeType` is set, in which case `files.export` is used. `Size` reports 0 (Drive does not
supply a size). They are still listed, can be deleted, and can be moved. Writing bytes over one
(upload, copy or move onto its path) is refused with `ErrNotDownloadable`, since replacing it would
discard the document.

## Writes, Seek and the IO matrix

Like the OneDrive backend, reads and writes go through a local temp file: a first read or seek
downloads the current content; a first write with no prior read/seek starts empty (replacing the
file). `Close` uploads when anything was written. This provides all 18 Read/Write/Seek sequences in
`RunIOTests` without server-side partial updates.

Uploads up to 5 MiB use a single `uploadType=multipart` request. Larger files use
`uploadType=resumable` in chunks that are a multiple of 256 KiB (default 8 MiB); the session
answers `308` for intermediate chunks and `200/201` after the last. A `308` that does not advance
the received offset counts against the retry limit, and the chunk that reaches the file's total must
come back with a file resource. Chunk PUTs bypass the generic retry loop: after a transient failure
(5xx, 408, 429, rate-limit 403, transport error) the session is queried
(`Content-Range: bytes */total`) and only the missing bytes are sent, up to the retry count.
`429`, `503`, and rate-limit `403` responses are retried only for idempotent methods.

## Timeouts and logging

Every attempt runs under a request timeout (default 2 minutes). For JSON calls and uploads it covers
the whole exchange; for downloads it covers the wait for headers and then acts as an idle timeout
between body reads, so long downloads only fail when they stall. A timeout surfaces as `ErrTimeout`
and is retried like any transport error. The opt-in `RequestLogger` callback receives one
`RequestLog` per attempt (method, URL, attempt, status, duration, error). The caller maps that onto
its own logger. Resumable session `upload_id`s are redacted from logged URLs, from `*url.Error`
values, and from other error messages, and the `Authorization` header is never included.

## Touch

Missing file: create an empty file. Existing file: `files.update` with `modifiedTime=now`
(Drive supports this natively, so `SkipTouchTimestampTest` stays false).

## Copy and move

`files.copy` is synchronous (no operation to poll). Move is `files.update` with
`addParents/removeParents` (and `name` for renames). Both require the destination folder to exist;
when it does not, the file layer falls back to a streamed copy (and delete for move), which
creates parents. Both must overwrite an existing destination file: the new item is created first and
the previous destination item is then removed by ID, so a failure never loses the old destination
(the error says two files now share the name). The Drive write itself is sent once; it is repeated only
after a not-found caused by a stale cached destination folder, when nothing was written. A replaced
file that is already gone counts as removed.

Overwriting by copy or move gives the destination path a new file ID. Drive has no server-side
"copy content into an existing file", so preserving the old ID would mean downloading and re-uploading
the bytes; the backend keeps copy server-side instead, and the old file's sharing, comments and
revisions are not carried over. Copying or moving a file onto itself is a no-op.

## Consistency

Child listing by parent ID is strongly consistent for items written through the same account, which
is what path resolution uses; free-text search (not used) is the part of Drive that lags. Ambiguity
therefore only surfaces for real duplicates. Deleted cached folders are detected on first use (see
path resolution); other external changes, such as a renamed folder, are covered by the TTL.

## Authentication and scopes

OAuth2 only, via `golang.org/x/oauth2`: static access token, `oauth2.TokenSource`, service-account
JSON, Application Default Credentials, or the device authorization flow. The default scope is
`https://www.googleapis.com/auth/drive`. `WithScope` accepts `drive.file`; with it Drive only exposes
files the app created, so pre-existing paths resolve as not found. This is documented in the README.

## Names

Special characters (`%`, `#`, `&`, `'`, `+`, spaces, unicode) are sent as JSON metadata or as
escaped query literals, never embedded in URL paths, so they round-trip unchanged. A `/` cannot
appear in a VFS file name; a name consisting only of `.`/`..` is rejected by core path validation.

## Spike result

`conformance_test.go` runs `RunConformanceTests` and `RunIOTests` against an in-process fake Drive
(`fakedrive_test.go`) that implements Drive v3 semantics including duplicate names, folders as
files, multipart/resumable upload, copy, move via parents, trash, and export. It runs in every
`go test`. `realdrive_test.go` (tag `vfsintegration`, `VFS_GDRIVE_*` env) runs the same suites
against a real drive; see the README. No `ConformanceOptions` skips are required.
