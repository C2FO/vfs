# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [contrib/lockfile/v1.1.0] - 2025-10-07
### Security
- Update go form 1.23 to 1.24
- Update dependencies
### Fixed
- lockfile_test.go was using its own package namespace which required importing lockfile (and did so incorrectly reference v7 (which doesn't apply to contrib packages).

## [contrib/lockfile/v1.0.1] - 2025-09-16
### Fixed
- Use more appropriate unit test assertion patterns where possible.
- Minor whitespace cleanup.
- Enabled tagliatelle golangci-lint linter and fix associated issues.

## [contrib/lockfile/v1.0.0] - 2025-08-04
### Added
- Initial commit of lockfile.
