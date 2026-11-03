# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [[contrib/backend/sqlfs/v0.3.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.3.0)] - 2026-11-03
### Added
- `Location` implements `vfs.EntryLister`, so `list.Entries` lists natively: each query returns up to 1,000 files in byte order of their path, only as entries are consumed, carries each file's size and modification time, and skips directories the listing does not descend into ([#389](https://github.com/C2FO/vfs/issues/389)).
- Schema version 2 adds the index that listing uses. Run `EnsureSchema` once after upgrading: until then `ValidateSchema` rejects the database with `ErrInvalidSchema`, although reads and writes keep working.
- `ByteOrderer`, an optional `Dialect` interface for databases whose default collation is not byte order. The PostgreSQL dialect implements it with the `"C"` collation.

## [[contrib/backend/sqlfs/v0.2.4](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.4)] - 2026-11-03
### Fixed
- Updated core VFS dependency to v7.33.1, which adds the `list.Entries` listing API.

## [[contrib/backend/sqlfs/v0.2.3](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.3)] - 2026-10-28
### Fixed
- Test only: `TestTimeoutBoundsEachStatementNotTheTransaction` no longer fails intermittently on slow CI runners; its timeout now leaves more headroom around the injected per-statement delay.

## [[contrib/backend/sqlfs/v0.2.2](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.2)] - 2026-10-27
### Documentation
- Package documentation now mentions the explicit `EnsureSchema` and `Cleanup` calls.

## [[contrib/backend/sqlfs/v0.2.1](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.1)] - 2026-10-26
### Documentation
- Documented the `CleanupStats` returned by `Cleanup`.

## [[contrib/backend/sqlfs/v0.2.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.2.0)] - 2026-10-26
### Security
- Updated indirect test dependencies `github.com/klauspost/compress` to v1.18.7, `github.com/moby/go-archive` to v0.3.0, and `github.com/moby/sys/user` to v0.4.1 to resolve Dependabot advisories.

## [[contrib/backend/sqlfs/v0.1.0](https://github.com/C2FO/vfs/releases/tag/contrib%2Fbackend%2Fsqlfs%2Fv0.1.0)] - 2026-10-24
### Added
- Initial SQL FS backend (`sqlfs://`) with PostgreSQL and SQLite dialects.
