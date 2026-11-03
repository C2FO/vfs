# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- `Location` implements `vfs.EntryLister`, so `list.Entries` lists natively: each folder is read with `list_folder` and `list_folder/continue`, only as the listing reaches it, and every file carries its size and server modification time from the listing, so reading them makes no further request. An authentication failure ends the listing after it is reported once, and cancelling the context stops further requests ([#389](https://github.com/C2FO/vfs/issues/389)).

### Fixed
- Uploading or downloading a file whose path contains non-ASCII characters no longer fails. The SDK puts the unescaped path in the `Dropbox-API-Arg` header, which Dropbox requires to be `\uXXXX`-escaped; the client now escapes it, using surrogate pairs above U+FFFF.

## [[contrib/backend/dropbox/v1.2.4](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fdropbox%2Fv1.2.4)] - 2026-11-03
### Fixed
- Updated core VFS dependency to v7.33.1, which adds the `list.Entries` listing API.

## [[contrib/backend/dropbox/v1.2.3](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fdropbox%2Fv1.2.3)] - 2026-11-02
### Fixed
- Use `min`, `maps.Copy`, and `any` where `modernize` suggested them ([#391](https://github.com/C2FO/vfs/issues/391)).

## [[contrib/backend/dropbox/v1.2.2](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fdropbox%2Fv1.2.2)] - 2026-10-26
### Documentation
- Fixed README examples, documented the `VFS_DROPBOX_ACCESS_TOKEN` fallback and `WithMaxSimpleUploadSize`, and corrected the testing instructions.

## [[contrib/backend/dropbox/v1.2.1](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fdropbox%2Fv1.2.1)] - 2026-10-26
### Documentation
- Automated test coverage (no production code change): `testsuite.RunConformanceTests` and `RunIOTests` now run in ordinary `go test` (no credentials, Docker, or build tag) against an in-process stateful fake of the Dropbox API, for both the simple `upload` path and the chunked `upload_session/*` path, including `list_folder/continue` pagination ([#349](https://github.com/C2FO/vfs/issues/349)).

## [[contrib/backend/dropbox/v1.2.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fdropbox%2Fv1.2.0)] - 2026-08-28
### Fixed
- Adapted to a breaking API change in `github.com/dropbox/dropbox-sdk-go-unofficial/v6` (v6.0.5 → v6.6.1), which changed `files.FileMetadata.ServerModified` and `files.CommitInfo.ClientModified` / `files.UploadArg.ClientModified` from `time.Time` to `dropbox.DBXTime` (a distinct type with the same underlying representation). `File.LastModified()` and `File.Touch()` now convert between `time.Time` and `dropbox.DBXTime` explicitly.

### Security
- Updated Go version to 1.26.7 per the Go version policy in AGENTS.md ([#352](https://github.com/C2FO/vfs/issues/352)).
- Updated core VFS dependency to v7.27.0.
- Updated dependencies via `go get -u -t ./...` + `go mod tidy`, including `github.com/dropbox/dropbox-sdk-go-unofficial/v6` v6.0.5 → v6.6.1.

## [[contrib/backend/dropbox/v1.1.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fdropbox%2Fv1.1.0)] - 2026-06-18
### Security
- Updated core VFS dependency to v7.19.0

## [contrib/backend/dropbox/v1.0.1] - 2026-03-06
### Security
- Updated to Go 1.25.7
- Updated core VFS dependency to v7.15.0
- Updated dependencies

## [contrib/backend/dropbox/v1.0.0] - 2026-01-30
### Added
- Initial commit of dropbox community-contributed backend.
