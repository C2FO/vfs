# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [[contrib/backend/sqlfs/v0.2.1](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.1)] - 2026-10-26
### Documentation
- Documented the `CleanupStats` returned by `Cleanup`.

## [[contrib/backend/sqlfs/v0.2.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.0)] - 2026-10-26
### Security
- Updated indirect test dependencies `github.com/klauspost/compress` to v1.18.7, `github.com/moby/go-archive` to v0.3.0, and `github.com/moby/sys/user` to v0.4.1 to resolve Dependabot advisories.

## [[contrib/backend/sqlfs/v0.1.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.1.0)] - 2026-10-24
### Added
- Initial SQL FS backend (`sqlfs://`) with PostgreSQL and SQLite dialects.
