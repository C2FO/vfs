# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Security
- Updated indirect test dependencies `github.com/klauspost/compress` to v1.18.7, `github.com/moby/go-archive` to v0.3.0, and `github.com/moby/sys/user` to v0.4.1 to resolve Dependabot advisories.

## [[contrib/backend/gdrive/v0.1.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fgdrive%2Fv0.1.0)] - 2026-10-21
### Fixed
- POST requests (folder create, new-file upload, copy) are no longer retried on 429, 503, or a rate-limit 403, because Drive can apply the request and still return those statuses.
- A resumable upload stops when a 308 does not advance the received offset, and the chunk that reaches the file size fails if Drive does not return a file resource.
- Transport errors no longer include the resumable upload session ID. A folder that shares its name with a file is `ErrAmbiguous`. A missing child of a cached folder no longer flushes the folder cache.

### Added
- Initial commit of the Google Drive community-contributed backend (`gdrive://`), backed by a small Drive v3 REST client over `net/http` with `golang.org/x/oauth2` authentication (access token, token source, service account, Application Default Credentials, device code), `drive` or `drive.file` scope, shared drive selection via the location authority, and `WithBaseURL`/`WithHTTPClient` for proxies or test servers.
- Documented VFS semantics for Drive's ID-based model in `DESIGN.md`: path resolution with a folder-ID cache, `ErrAmbiguous` for duplicate names, a folder guard on file operations, permanent delete by default (`WithTrashOnDelete` to trash), `ErrNotDownloadable`/`WithExportMimeType` for Google-native files, multipart and resumable uploads.
- `WithRequestTimeout` (default 2 minutes; an idle timeout for downloads) and an opt-in `WithRequestLogger` callback (`RequestLog` per HTTP attempt, upload session IDs redacted, no chosen logging library); `ErrTimeout` and `ErrNotFolder` errors.
- Folder creation is serialized per client so parallel writes share new folders; resumable chunks recover via a session status query instead of being resent; a miss under a cached folder verifies that folder instead of flushing the cache; copy/move send their Drive write once; shared-drive deletes fall back to trash when Drive answers a permanent delete with 404 (what a content manager receives) or 403 insufficientFilePermissions; writes over Google-native files are refused.
- Unit tests using mockery mocks of `types.Client`/`types.UploadSession`, the shared conformance and IO suites run against an in-process fake Drive on every `go test`, WireMock testcontainers tests of the wire protocol (`vfsintegration` tag), and credentialed real-drive tests.
