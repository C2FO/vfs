# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Tests
- Use typed expectations consistently for added type safety.

## [contrib/vfsevents/v1.1.0] - 2025-10-07
### Security
- Updated to go 1.24.7
- Updated dependcies
### Fixed
- Fixed lint issues where t.Context() shoudl be used and pubsub v1 is deprecated, so updated.

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
