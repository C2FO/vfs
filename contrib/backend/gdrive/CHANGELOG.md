# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Initial commit of the Google Drive community-contributed backend (`gdrive://`), backed by a small Drive v3 REST client over `net/http` with `golang.org/x/oauth2` authentication (access token, token source, service account, Application Default Credentials, device code), `drive` or `drive.file` scope, shared drive selection via the location authority, and `WithBaseURL`/`WithHTTPClient` for proxies or test servers.
- Documented VFS semantics for Drive's ID-based model in `DESIGN.md`: path resolution with a folder-ID cache, `ErrAmbiguous` for duplicate names, a folder guard on file operations, permanent delete by default (`WithTrashOnDelete` to trash), `ErrNotDownloadable`/`WithExportMimeType` for Google-native files, multipart and resumable uploads.
- `WithRequestTimeout` (default 2 minutes; an idle timeout for downloads) and an opt-in `WithRequestLogger` callback (`RequestLog` per HTTP attempt, upload session IDs redacted, no chosen logging library); `ErrTimeout` and `ErrNotFolder` errors.
- Folder creation is serialized per client so parallel writes share new folders; resumable chunks recover via a session status query instead of being resent; a miss under a cached folder verifies that folder instead of flushing the cache; copy/move send their Drive write once; shared-drive deletes fall back to trash when Drive answers a permanent delete with 404 (what a content manager receives) or 403 insufficientFilePermissions; writes over Google-native files are refused.
- Unit tests using mockery mocks of `types.Client`/`types.UploadSession`, the shared conformance and IO suites run against an in-process fake Drive on every `go test`, WireMock testcontainers tests of the wire protocol (`vfsintegration` tag), and credentialed real-drive tests.
