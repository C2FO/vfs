# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Fixed
- `TestS3WatcherTestSuite/TestEnhancedMetadata`: fixed a CI-observed flake where the test could finish (and its mock-expectation teardown could run) before the background `pollOnce` goroutine's `DeleteMessage` call happened, since the test only synchronized on the event handler firing rather than on `pollOnce` fully returning. Now uses the same completion-based synchronization as `TestNonVersionedBucketMetadata` (extracted into a shared `waitForPoll` helper), which also proactively avoids the un-stopped `time.After` timer issue raised in review of [#341](https://github.com/C2FO/vfs/pull/341).

## [[contrib/vfsevents/v1.2.1](https://github.com/C2FO/vfs/releases/tag/contrib%2Fvfsevents%2Fv1.2.1)] - 2026-07-16
### Fixed
- `TestS3WatcherTestSuite/TestNonVersionedBucketMetadata`: replaced a fixed `time.Sleep` with synchronization on the `pollOnce` goroutine's completion (via an error channel), so the test no longer races with the goroutine's write to the shared `receivedEvent` variable and no longer risks tearing down before the `DeleteMessage` mock expectation is satisfied. This was found while validating [#313](https://github.com/C2FO/vfs/issues/313); the originally reported `TestStart` race was already fixed in v1.1.5.

## [[contrib/vfsevents/v1.2.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fvfsevents%2Fv1.2.0)] - 2026-06-18
### Security
- Updated core VFS dependency to v7.19.0

##[contrib/vfsevents/v1.1.5] - 2026-06-06
### Fixed
- `TestS3WatcherTestSuite/TestStart`: add `Maybe()` mock expectations for `ReceiveMessage` to prevent a race-induced unexpected-call panic on Windows, where the poll goroutine can fire before `Stop()` cancels the context.

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
