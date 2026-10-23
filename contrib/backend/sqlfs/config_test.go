package sqlfs_test

import (
	"database/sql"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7/options"
)

type ConfigTestSuite struct {
	suite.Suite
	db *sql.DB
}

func TestConfigTestSuite(t *testing.T) { suite.Run(t, new(ConfigTestSuite)) }

func (s *ConfigTestSuite) SetupTest() {
	db, err := sql.Open("sqlite", ":memory:")
	s.Require().NoError(err)
	s.db = db
}

func (s *ConfigTestSuite) TearDownTest() { s.Require().NoError(s.db.Close()) }

func (s *ConfigTestSuite) TestNewFileSystem() {
	type opt = options.NewFileSystemOption[sqlfs.FileSystem]
	tests := []struct {
		name          string
		opts          func() []opt
		expectedError string
	}{
		{
			name: "database and dialect only",
			opts: func() []opt { return []opt{sqlfs.WithDB(s.db), sqlfs.WithDialect(sqlfs.SQLite())} },
		},
		{
			name: "all options",
			opts: func() []opt {
				return []opt{
					sqlfs.WithDB(s.db), sqlfs.WithDialect(sqlfs.PostgreSQL()), sqlfs.WithTablePrefix("app_1"),
					sqlfs.WithChunkSize(sqlfs.MinChunkSize), sqlfs.WithMaxFileSize(10), sqlfs.WithOperationTimeout(0),
					sqlfs.WithStagingRetention(time.Minute), sqlfs.WithRevisionRetention(time.Minute),
				}
			},
		},
		{
			name:          "missing database",
			opts:          func() []opt { return []opt{sqlfs.WithDialect(sqlfs.SQLite())} },
			expectedError: "database is required",
		},
		{
			name:          "nil database",
			opts:          func() []opt { return []opt{sqlfs.WithDB(nil), sqlfs.WithDialect(sqlfs.SQLite())} },
			expectedError: "database is required",
		},
		{
			name:          "missing dialect",
			opts:          func() []opt { return []opt{sqlfs.WithDB(s.db)} },
			expectedError: "dialect is required",
		},
		{
			name:          "chunk size zero",
			opts:          func() []opt { return s.base(sqlfs.WithChunkSize(0)) },
			expectedError: "chunk size",
		},
		{
			name:          "chunk size too small",
			opts:          func() []opt { return s.base(sqlfs.WithChunkSize(sqlfs.MinChunkSize - 1)) },
			expectedError: "chunk size",
		},
		{
			name:          "chunk size too large",
			opts:          func() []opt { return s.base(sqlfs.WithChunkSize(sqlfs.MaxChunkSize + 1)) },
			expectedError: "chunk size",
		},
		{
			name:          "unsafe table prefix",
			opts:          func() []opt { return s.base(sqlfs.WithTablePrefix(`x"; DROP TABLE y; --`)) },
			expectedError: "table prefix",
		},
		{
			name:          "empty table prefix",
			opts:          func() []opt { return s.base(sqlfs.WithTablePrefix("")) },
			expectedError: "table prefix",
		},
		{
			name:          "negative max file size",
			opts:          func() []opt { return s.base(sqlfs.WithMaxFileSize(-1)) },
			expectedError: "maximum file size",
		},
		{
			name:          "negative timeout",
			opts:          func() []opt { return s.base(sqlfs.WithOperationTimeout(-time.Second)) },
			expectedError: "durations",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := sqlfs.NewFileSystem(tt.opts()...)
			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Require().ErrorIs(err, sqlfs.ErrInvalidConfig)
				s.Contains(err.Error(), tt.expectedError)
				s.Nil(got)
				return
			}
			s.Require().NoError(err)
			s.Equal("sqlfs", got.Scheme())
			s.Equal("SQL FS", got.Name())
		})
	}
}

func (s *ConfigTestSuite) base(extra ...options.NewFileSystemOption[sqlfs.FileSystem]) []options.NewFileSystemOption[sqlfs.FileSystem] {
	return append([]options.NewFileSystemOption[sqlfs.FileSystem]{sqlfs.WithDB(s.db), sqlfs.WithDialect(sqlfs.SQLite())}, extra...)
}

func (s *ConfigTestSuite) TestConstructionDoesNoIO() {
	// The database file cannot be created here, so any I/O attempt would fail.
	db, err := sql.Open("sqlite", "file:/nonexistent-dir-for-sqlfs/x.db")
	s.Require().NoError(err)
	defer func() { _ = db.Close() }()

	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.SQLite()))
	s.Require().NoError(err)

	f, err := fsys.NewFile("documents", "/missing/file.txt")
	s.Require().NoError(err, "handles are lazy")
	s.Equal("sqlfs://documents/missing/file.txt", f.URI())

	_, err = f.Exists()
	s.Require().Error(err, "first operation needing I/O reports the failure")
	s.Error(fsys.ValidateSchema(s.T().Context()))
}

func (s *ConfigTestSuite) TestErrorSentinels() {
	s.Require().ErrorIs(sqlfs.ErrNotFound, fs.ErrNotExist)
	s.Require().ErrorIs(sqlfs.ErrTimeout, errors.Unwrap(sqlfs.ErrTimeout))
	s.Require().ErrorIs(sqlfs.ErrTimeout, contextDeadline())
}
