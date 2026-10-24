package sqlfs_test

import (
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
)

type SchemaTestSuite struct {
	suite.Suite
	dsn string
	db  *sql.DB
	fs  *sqlfs.FileSystem
}

func TestSchemaTestSuite(t *testing.T) { suite.Run(t, new(SchemaTestSuite)) }

func (s *SchemaTestSuite) SetupTest() {
	s.dsn = sqliteDSN(s.T())
	s.db = s.open()
	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(s.db), sqlfs.WithDialect(sqlfs.SQLite()))
	s.Require().NoError(err)
	s.fs = fsys
}

func (s *SchemaTestSuite) open() *sql.DB {
	db, err := sql.Open("sqlite", s.dsn)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = db.Close() })
	return db
}

func (s *SchemaTestSuite) version() int {
	var v int
	s.Require().NoError(s.db.QueryRow("SELECT MAX(version) FROM vfs_schema").Scan(&v))
	return v
}

func (s *SchemaTestSuite) TestValidateMissingSchemaDoesNotCreate() {
	err := s.fs.ValidateSchema(s.T().Context())
	s.Require().ErrorIs(err, sqlfs.ErrInvalidSchema)

	var n int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master").Scan(&n))
	s.Zero(n, "ValidateSchema must not modify the database")
}

func (s *SchemaTestSuite) TestForeignKeysRequired() {
	// SQLite leaves foreign keys off unless the DSN enables them.
	db, err := sql.Open("sqlite", strings.Replace(s.dsn, "&_pragma=foreign_keys(1)", "", 1))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = db.Close() })
	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.SQLite()))
	s.Require().NoError(err)

	err = fsys.EnsureSchema(s.T().Context())
	s.Require().ErrorIs(err, sqlfs.ErrInvalidConfig)
	s.Contains(err.Error(), "foreign key")

	s.Require().NoError(s.fs.EnsureSchema(s.T().Context()), "a correctly configured connection creates the schema")
	err = fsys.ValidateSchema(s.T().Context())
	s.Require().ErrorIs(err, sqlfs.ErrInvalidConfig)
	s.Contains(err.Error(), "foreign key")
}

func (s *SchemaTestSuite) TestValidateReportsNonSchemaErrorsAsThemselves() {
	db := s.open()
	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.SQLite()))
	s.Require().NoError(err)
	s.Require().NoError(db.Close())

	err = fsys.ValidateSchema(s.T().Context())
	s.Require().Error(err)
	s.NotErrorIs(err, sqlfs.ErrInvalidSchema, "a closed database is not a missing schema")
}

func (s *SchemaTestSuite) TestEnsureFreshAndRepeated() {
	ctx := s.T().Context()
	s.Require().NoError(s.fs.EnsureSchema(ctx))
	s.Require().NoError(s.fs.ValidateSchema(ctx))
	s.Equal(1, s.version())

	s.Require().NoError(s.fs.EnsureSchema(ctx), "idempotent")
	var rows int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM vfs_schema").Scan(&rows))
	s.Equal(1, rows, "second run must not re-apply the migration")
}

func (s *SchemaTestSuite) TestEnsureConcurrent() {
	ctx := s.T().Context()
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range errs {
		fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(s.open()), sqlfs.WithDialect(sqlfs.SQLite()))
		s.Require().NoError(err)
		wg.Go(func() { errs[i] = fsys.EnsureSchema(ctx) })
	}
	wg.Wait()
	for _, err := range errs {
		s.Require().NoError(err)
	}
	var rows int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM vfs_schema").Scan(&rows))
	s.Equal(1, rows, "schema created exactly once")
}

func (s *SchemaTestSuite) TestNewerSchemaVersionRejected() {
	ctx := s.T().Context()
	s.Require().NoError(s.fs.EnsureSchema(ctx))
	_, err := s.db.Exec("INSERT INTO vfs_schema (version) VALUES (99)")
	s.Require().NoError(err)

	for name, call := range map[string]func() error{
		"validate": func() error { return s.fs.ValidateSchema(ctx) },
		"ensure":   func() error { return s.fs.EnsureSchema(ctx) },
	} {
		s.Run(name, func() {
			err := call()
			s.Require().ErrorIs(err, sqlfs.ErrInvalidSchema)
			s.Contains(err.Error(), "upgrade")
		})
	}
}

func (s *SchemaTestSuite) TestCustomPrefix() {
	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(s.db), sqlfs.WithDialect(sqlfs.SQLite()), sqlfs.WithTablePrefix("app_"))
	s.Require().NoError(err)
	s.Require().NoError(fsys.EnsureSchema(s.T().Context()))

	var n int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM app_schema").Scan(&n))
	s.Require().ErrorIs(s.fs.ValidateSchema(s.T().Context()), sqlfs.ErrInvalidSchema, "default prefix tables were not created")
}

// TestExternalMigrationParity applies the published migration SQL directly, as an external migration tool would.
func (s *SchemaTestSuite) TestExternalMigrationParity() {
	for _, m := range sqlfs.SQLite().Migrations() {
		_, err := s.db.Exec(m.SQL)
		s.Require().NoError(err)
	}
	s.Require().NoError(s.fs.ValidateSchema(s.T().Context()))

	f, err := s.fs.NewFile("documents", "/a.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("hello"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *SchemaTestSuite) TestMigrationFilesUseDefaultPrefix() {
	for _, d := range []sqlfs.Dialect{sqlfs.SQLite(), sqlfs.PostgreSQL()} {
		for _, m := range d.Migrations() {
			s.Contains(m.SQL, "vfs_files", "%s migration %d must use the literal vfs_ prefix", d.Name(), m.Version)
		}
	}
}
