package sqlfs

import (
	"database/sql"
	"time"

	"github.com/c2fo/vfs/v7/options"
)

const (
	// DefaultTablePrefix is prepended to every table name unless WithTablePrefix is used.
	DefaultTablePrefix = "vfs_"
	// DefaultChunkSize is the default size of a stored content chunk.
	DefaultChunkSize = 1 << 20
	// MinChunkSize and MaxChunkSize bound WithChunkSize.
	MinChunkSize = 4 << 10
	MaxChunkSize = 64 << 20
	// DefaultOperationTimeout bounds each database statement.
	DefaultOperationTimeout = 30 * time.Second
	// DefaultStagingRetention is how long an unfinished write may go without activity before Cleanup removes it.
	DefaultStagingRetention = 24 * time.Hour
	// DefaultRevisionRetention is how long replaced or deleted content is kept for in-progress readers.
	DefaultRevisionRetention = time.Hour
)

type fsOption struct {
	name  string
	apply func(*FileSystem)
}

func (o fsOption) Apply(fs *FileSystem)            { o.apply(fs) }
func (o fsOption) NewFileSystemOptionName() string { return o.name }

func newOption(name string, apply func(*FileSystem)) options.NewFileSystemOption[FileSystem] {
	return fsOption{name: name, apply: apply}
}

// WithDB sets the database connection pool. It is required. The caller owns the pool and must close it.
func WithDB(db *sql.DB) options.NewFileSystemOption[FileSystem] {
	return newOption("db", func(fs *FileSystem) {
		if db == nil {
			fs.q = nil
			return
		}
		fs.q = dbQuerier{db}
	})
}

// WithDialect sets the database dialect. It is required.
func WithDialect(d Dialect) options.NewFileSystemOption[FileSystem] {
	return newOption("dialect", func(fs *FileSystem) { fs.dialect = d })
}

// WithTablePrefix sets the table name prefix (1 to 32 characters of A-Z, a-z, 0-9, or underscore). Default "vfs_".
func WithTablePrefix(prefix string) options.NewFileSystemOption[FileSystem] {
	return newOption("tablePrefix", func(fs *FileSystem) { fs.prefix = prefix })
}

// WithChunkSize sets the size in bytes of content chunks for newly written content, between MinChunkSize and
// MaxChunkSize. Existing content keeps the chunk size it was written with. Default 1 MiB.
func WithChunkSize(size int) options.NewFileSystemOption[FileSystem] {
	return newOption("chunkSize", func(fs *FileSystem) { fs.chunkSize = size })
}

// WithMaxFileSize limits the size of a file. Zero, the default, means no limit.
func WithMaxFileSize(size int64) options.NewFileSystemOption[FileSystem] {
	return newOption("maxFileSize", func(fs *FileSystem) { fs.maxFileSize = size })
}

// WithOperationTimeout bounds each database statement, including each statement inside a transaction, and the wait
// to begin a transaction. Bulk chunk copies and deletes are split into statements of bounded size so that large files
// do not exceed it. Zero disables the timeout. Default 30 seconds.
func WithOperationTimeout(d time.Duration) options.NewFileSystemOption[FileSystem] {
	return newOption("operationTimeout", func(fs *FileSystem) { fs.opTimeout = d })
}

// WithStagingRetention sets how long an unfinished write may be idle before Cleanup removes it. It must be positive.
// Default 24 hours.
func WithStagingRetention(d time.Duration) options.NewFileSystemOption[FileSystem] {
	return newOption("stagingRetention", func(fs *FileSystem) { fs.stagingRetention = d })
}

// WithRevisionRetention sets how long replaced or deleted content is kept so that in-progress readers can finish. It
// must be positive. Default one hour.
func WithRevisionRetention(d time.Duration) options.NewFileSystemOption[FileSystem] {
	return newOption("revisionRetention", func(fs *FileSystem) { fs.revisionRetention = d })
}

// withQuerier injects the database access layer. It is used by tests to inject faults.
func withQuerier(q querier) options.NewFileSystemOption[FileSystem] {
	return newOption("querier", func(fs *FileSystem) { fs.q = q })
}
