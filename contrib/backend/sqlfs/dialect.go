package sqlfs

//go:generate mockery

import (
	"context"
	"database/sql"
)

// ErrorKind classifies a database error for the storage engine.
type ErrorKind int

const (
	// KindOther is any error without special handling.
	KindOther ErrorKind = iota
	// KindUniqueViolation is a unique or primary key constraint violation.
	KindUniqueViolation
	// KindRetryable is a transient failure (serialization failure, deadlock, busy database) where running the
	// whole transaction again can succeed.
	KindRetryable
	// KindUnavailable means the database could not be reached.
	KindUnavailable
	// KindForeignKeyViolation is a foreign key constraint violation, such as writing a chunk for a revision that
	// Cleanup already removed.
	KindForeignKeyViolation
	// KindUndefinedTable means a referenced table does not exist, which the engine reports as a missing schema.
	KindUndefinedTable
)

// Migration is one versioned schema step. SQL holds one or more statements separated by a line that ends in a
// semicolon, written with the literal table prefix "vfs_", which EnsureSchema replaces with the configured prefix.
type Migration struct {
	Version int
	SQL     string
}

// Execer is the subset of database/sql used by dialect hooks. Both *sql.DB and *sql.Tx satisfy it.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Dialect isolates every database-specific behavior the storage engine needs. The engine writes all SQL with "?"
// placeholders and standard constructs; a dialect adapts those to its database. Implement this interface to add
// support for another database without changing the filesystem code.
type Dialect interface {
	// Name identifies the dialect in diagnostics.
	Name() string
	// Rebind converts "?" placeholders to the database's native form.
	Rebind(query string) string
	// QuoteIdentifier quotes a table or column name. Names have already been validated as [A-Za-z0-9_]+.
	QuoteIdentifier(name string) string
	// Migrations returns the ordered schema migrations.
	Migrations() []Migration
	// UpsertChunk returns an insert-or-replace statement for the chunks table taking (revision_id, chunk_index,
	// data) as "?" parameters.
	UpsertChunk(chunksTable string) string
	// LockSchema serializes concurrent initialization of the schema with the given table prefix for the life of
	// tx. It may be a no-op when the database serializes writers on its own.
	LockSchema(ctx context.Context, tx Execer, prefix string) error
	// Classify reports how the engine should treat err. Report KindUndefinedTable for a missing table so that
	// ValidateSchema can return ErrInvalidSchema, and KindForeignKeyViolation so that writes to reclaimed content
	// fail with ErrConflict.
	Classify(err error) ErrorKind
}

// SessionValidator may be implemented by a Dialect to verify connection settings the engine depends on. EnsureSchema
// and ValidateSchema call it and fail with ErrInvalidConfig when it returns an error.
type SessionValidator interface {
	ValidateSession(ctx context.Context, q Execer) error
}

// txn is a database transaction.
type txn interface {
	Execer
	Commit() error
	Rollback() error
}

// querier is the database access used by the storage engine. It exists so tests can inject failures.
type querier interface {
	Execer
	BeginTx(ctx context.Context, opts *sql.TxOptions) (txn, error)
}

type dbQuerier struct{ *sql.DB }

func (d dbQuerier) BeginTx(ctx context.Context, opts *sql.TxOptions) (txn, error) {
	tx, err := d.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
