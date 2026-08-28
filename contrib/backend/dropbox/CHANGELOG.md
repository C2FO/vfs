# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
