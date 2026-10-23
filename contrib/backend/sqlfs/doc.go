// Package sqlfs provides a vfs backend that stores files in a relational database through database/sql.
//
// Files are stored as fixed-size chunks under immutable revisions. A write fills a staged revision and Close
// publishes it atomically, so readers never observe partial content. Database drivers and the *sql.DB are owned by
// the caller; the package imports no driver. Database-specific behavior is isolated behind the Dialect interface,
// with PostgreSQL and SQLite provided.
//
// Unlike other backends, sqlfs does not register itself with the backend registry on import. Construct a FileSystem
// with NewFileSystem and register it with backend.Register(sqlfs.Scheme, fs).
package sqlfs
