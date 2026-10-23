package sqlfs

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
)

// SQLite returns the dialect for SQLite databases. Open the database with foreign keys enabled (required: EnsureSchema
// and ValidateSchema fail with ErrInvalidConfig otherwise), a busy timeout, and immediate transactions for best
// behavior under concurrency, for example (modernc.org/sqlite):
//
//	file:files.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate
func SQLite() Dialect { return sqliteDialect{} }

type sqliteDialect struct{}

func (sqliteDialect) Name() string                    { return "sqlite" }
func (sqliteDialect) Rebind(q string) string          { return q }
func (sqliteDialect) QuoteIdentifier(n string) string { return `"` + n + `"` }
func (sqliteDialect) Migrations() []Migration {
	return []Migration{mustMigration("sqlite", 1, "0001_init.sql")}
}

func (sqliteDialect) UpsertChunk(t string) string {
	return "INSERT INTO " + t + " (revision_id, chunk_index, data) VALUES (?, ?, ?) " +
		"ON CONFLICT (revision_id, chunk_index) DO UPDATE SET data = excluded.data"
}

// LockSchema is a no-op: SQLite serializes writers, and schema initialization begins with a write.
func (sqliteDialect) LockSchema(context.Context, Execer, string) error { return nil }

// ValidateSession requires foreign key enforcement, which SQLite leaves off by default. The setting is per
// connection; checking one connection is enough when every connection is opened from the same DSN.
func (sqliteDialect) ValidateSession(ctx context.Context, q Execer) error {
	var on int
	if err := q.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil {
		return err
	}
	if on != 1 {
		return fmt.Errorf("%w: SQLite foreign key enforcement is off; enable it in the DSN, for example "+
			"_pragma=foreign_keys(1) (modernc.org/sqlite) or _foreign_keys=1 (mattn/go-sqlite3)", ErrInvalidConfig)
	}
	return nil
}

func (sqliteDialect) Classify(err error) ErrorKind {
	if err == nil {
		return KindOther
	}
	if errors.Is(err, driver.ErrBadConn) {
		return KindUnavailable
	}
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		code := coded.Code()
		switch code & 0xff {
		case 5, 6: // SQLITE_BUSY, SQLITE_LOCKED
			return KindRetryable
		case 19: // SQLITE_CONSTRAINT
			switch code {
			case 2067, 1555: // UNIQUE, PRIMARYKEY
				return KindUniqueViolation
			case 787: // FOREIGNKEY
				return KindForeignKeyViolation
			}
		case 14: // SQLITE_CANTOPEN
			return KindUnavailable
		}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed"), strings.Contains(msg, "PRIMARY KEY constraint failed"):
		return KindUniqueViolation
	case strings.Contains(msg, "FOREIGN KEY constraint failed"):
		return KindForeignKeyViolation
	case strings.Contains(msg, "no such table"):
		return KindUndefinedTable
	case strings.Contains(msg, "database is locked"), strings.Contains(msg, "SQLITE_BUSY"):
		return KindRetryable
	}
	return KindOther
}
