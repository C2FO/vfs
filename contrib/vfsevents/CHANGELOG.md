# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security
- Update github.com/go-jose/go-jose/v4 to v4.1.4

### Security
- Update go.opentelemetry.io/otel/sdk to v1.43.0

## [contrib/vfsevents/v1.1.4] - 2026-03-25
### Security
- Update deps

## [contrib/vfsevents/v1.1.3] - 2026-03-06
### Security
- Updated to Go 1.25.7
- Updated core VFS dependency to v7.15.0
- Updated dependencies

## [contrib/vfsevents/v1.1.2] - 2025-12-19
### Fixed
- Use the correct assertion format overload.
- Use context from `testing.T` introduced in Go 1.24.
- Use typed expectations consistently for added type safety.
- Use walrus assignment where possible.

## [contrib/vfsevents/v1.1.1] - 2025-11-13
### Fixed
- Updated logic to correctly evaluate the S3 event name to determine the event type

## [contrib/vfsevents/v1.1.0] - 2025-10-07
### Security
- Updated to go 1.24.7
- Updated dependcies
### Fixed
- Fixed lint issues where t.Context() should be used and pubsub v1 is deprecated, so updated.

## [contrib/vfsevents/v1.0.2] - 2025-09-16
### Fixed
- Use more appropriate unit test assertion patterns where possible.
- Minor whitespace cleanup.
- General spelling and grammar corrections.
- Regenerate mockery types with the latest version.
- More reliable mock expectation assertions in unit tests.
- Use the shared `utils.Ptr` func everywhere.
- Enable testifylint in golangci-lint and fix associated issues.
- Enable 9 other various golangci-lint linters and fix associated issues.

## [contrib/vfsevents/v1.0.1] - 2025-08-05
### Fixed
- Fix gcsevents tests the break in ci by using mocks.

## [contrib/vfsevents/v1.0.0] - 2025-07-27
### Added
- Initial commit of vfsevents and implements #269.
