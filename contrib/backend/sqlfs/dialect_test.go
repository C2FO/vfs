package sqlfs_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7/backend/testsuite"
)

func contextDeadline() error { return context.DeadlineExceeded }

type codedError struct{ code int }

func (e codedError) Error() string { return fmt.Sprintf("sqlite error %d", e.code) }
func (e codedError) Code() int     { return e.code }

type stateError struct{ state string }

func (e stateError) Error() string    { return "pg error " + e.state }
func (e stateError) SQLState() string { return e.state }

type DialectTestSuite struct{ suite.Suite }

func TestDialectTestSuite(t *testing.T) { suite.Run(t, new(DialectTestSuite)) }

func (s *DialectTestSuite) TestSQLiteClassify() {
	tests := []struct {
		name     string
		err      error
		expected sqlfs.ErrorKind
	}{
		{name: "nil error", err: nil, expected: sqlfs.KindOther},
		{name: "busy code", err: codedError{5}, expected: sqlfs.KindRetryable},
		{name: "locked code", err: codedError{6}, expected: sqlfs.KindRetryable},
		{name: "unique constraint code", err: codedError{2067}, expected: sqlfs.KindUniqueViolation},
		{name: "primary key constraint code", err: codedError{1555}, expected: sqlfs.KindUniqueViolation},
		{name: "foreign key constraint code", err: codedError{787}, expected: sqlfs.KindForeignKeyViolation},
		{name: "other constraint code", err: codedError{275}, expected: sqlfs.KindOther},
		{name: "foreign key message", err: errors.New("FOREIGN KEY constraint failed"), expected: sqlfs.KindForeignKeyViolation},
		{name: "missing table message", err: errors.New("SQL logic error: no such table: vfs_schema (1)"), expected: sqlfs.KindUndefinedTable},
		{name: "cannot open code", err: codedError{14}, expected: sqlfs.KindUnavailable},
		{name: "bad connection", err: driver.ErrBadConn, expected: sqlfs.KindUnavailable},
		{name: "unique message", err: errors.New("UNIQUE constraint failed: vfs_files.name"), expected: sqlfs.KindUniqueViolation},
		{name: "locked message", err: errors.New("database is locked"), expected: sqlfs.KindRetryable},
		{name: "wrapped code", err: fmt.Errorf("wrapped: %w", codedError{5}), expected: sqlfs.KindRetryable},
		{name: "unrelated", err: errors.New("boom"), expected: sqlfs.KindOther},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() { s.Equal(tt.expected, sqlfs.SQLite().Classify(tt.err)) })
	}
}

func (s *DialectTestSuite) TestPostgreSQLClassify() {
	tests := []struct {
		name     string
		err      error
		expected sqlfs.ErrorKind
	}{
		{name: "nil error", err: nil, expected: sqlfs.KindOther},
		{name: "unique violation", err: stateError{"23505"}, expected: sqlfs.KindUniqueViolation},
		{name: "serialization failure", err: stateError{"40001"}, expected: sqlfs.KindRetryable},
		{name: "deadlock", err: stateError{"40P01"}, expected: sqlfs.KindRetryable},
		{name: "lock timeout", err: stateError{"55P03"}, expected: sqlfs.KindRetryable},
		{name: "connection exception", err: stateError{"08006"}, expected: sqlfs.KindUnavailable},
		{name: "admin shutdown", err: stateError{"57P01"}, expected: sqlfs.KindUnavailable},
		{name: "foreign key violation", err: stateError{"23503"}, expected: sqlfs.KindForeignKeyViolation},
		{name: "undefined table", err: stateError{"42P01"}, expected: sqlfs.KindUndefinedTable},
		{name: "other state", err: stateError{"42601"}, expected: sqlfs.KindOther},
		{name: "bad connection", err: driver.ErrBadConn, expected: sqlfs.KindUnavailable},
		{name: "wrapped state", err: fmt.Errorf("wrapped: %w", stateError{"23505"}), expected: sqlfs.KindUniqueViolation},
		{name: "unrelated", err: errors.New("boom"), expected: sqlfs.KindOther},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() { s.Equal(tt.expected, sqlfs.PostgreSQL().Classify(tt.err)) })
	}
}

func (s *DialectTestSuite) TestRebindAndQuote() {
	pg, lite := sqlfs.PostgreSQL(), sqlfs.SQLite()
	s.Equal("SELECT $1, $2, $3", pg.Rebind("SELECT ?, ?, ?"))
	s.Equal("SELECT ?, ?", lite.Rebind("SELECT ?, ?"))
	s.Equal(`"vfs_files"`, pg.QuoteIdentifier("vfs_files"))
	s.Equal(`"vfs_files"`, lite.QuoteIdentifier("vfs_files"))
	s.Contains(pg.UpsertChunk("t"), "ON CONFLICT")
	s.Contains(lite.UpsertChunk("t"), "ON CONFLICT")
}

func (s *DialectTestSuite) TestMigrations() {
	for _, d := range []sqlfs.Dialect{sqlfs.SQLite(), sqlfs.PostgreSQL()} {
		s.Run(d.Name(), func() {
			ms := d.Migrations()
			s.Require().NotEmpty(ms)
			s.Equal(1, ms[0].Version)
			s.Contains(ms[0].SQL, "vfs_chunks")
		})
	}
	s.Contains(sqlfs.PostgreSQL().Migrations()[0].SQL, "BYTEA")
	s.Contains(sqlfs.SQLite().Migrations()[0].SQL, "BLOB")
}

// customDialect shows a third party can supply a dialect from its own package without touching sqlfs.
type customDialect struct{ sqlfs.Dialect }

func (customDialect) Name() string { return "custom" }

func (s *DialectTestSuite) TestCustomDialect() {
	fsys := newSQLiteFS(s.T(), sqlfs.WithDialect(customDialect{sqlfs.SQLite()}))
	loc, err := fsys.NewLocation("documents", "/custom/")
	s.Require().NoError(err)
	testsuite.RunConformanceTests(s.T(), loc)
}
