package sqlfs

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
)

// SQLite returns the dialect for SQLite databases. Open the database with foreign keys enabled, a busy timeout, and
// immediate transactions for best behavior under concurrency, for example (modernc.org/sqlite):
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
func (sqliteDialect) LockSchema(context.Context, Execer) error { return nil }

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
			if code == 2067 || code == 1555 { // UNIQUE, PRIMARYKEY
				return KindUniqueViolation
			}
		case 14: // SQLITE_CANTOPEN
			return KindUnavailable
		}
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed"), strings.Contains(msg, "PRIMARY KEY constraint failed"):
		return KindUniqueViolation
	case strings.Contains(msg, "database is locked"), strings.Contains(msg, "SQLITE_BUSY"):
		return KindRetryable
	}
	return KindOther
}
