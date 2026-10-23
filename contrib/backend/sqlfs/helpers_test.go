package sqlfs_test

import (
	"database/sql"
	"io"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7/options"
)

// sqliteDSN returns a DSN for a fresh SQLite database file with the pragmas recommended in the README.
func sqliteDSN(t testing.TB) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "sqlfs.db") +
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"
}

// newSQLiteFS returns a filesystem backed by a new SQLite database with the schema created.
func newSQLiteFS(t testing.TB, opts ...options.NewFileSystemOption[sqlfs.FileSystem]) *sqlfs.FileSystem {
	t.Helper()
	fsys, _ := newSQLiteEnv(t, opts...)
	return fsys
}

// newSQLiteEnv is like newSQLiteFS but also returns the database so tests can inspect rows.
func newSQLiteEnv(t testing.TB, opts ...options.NewFileSystemOption[sqlfs.FileSystem]) (*sqlfs.FileSystem, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	all := append([]options.NewFileSystemOption[sqlfs.FileSystem]{
		sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.SQLite()),
	}, opts...)
	fsys, err := sqlfs.NewFileSystem(all...)
	require.NoError(t, err)
	require.NoError(t, fsys.EnsureSchema(t.Context()))
	return fsys, db
}

func sqlfsWithChunkSize(n int) options.NewFileSystemOption[sqlfs.FileSystem] {
	return sqlfs.WithChunkSize(n)
}

const testNS = "documents"

// put writes content to path in the test namespace and closes the file.
func put(t testing.TB, fsys *sqlfs.FileSystem, p string, content []byte) {
	t.Helper()
	f, err := fsys.NewFile(testNS, p)
	require.NoError(t, err)
	_, err = f.Write(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// get reads the whole file at path in the test namespace.
func get(t testing.TB, fsys *sqlfs.FileSystem, p string) []byte {
	t.Helper()
	f, err := fsys.NewFile(testNS, p)
	require.NoError(t, err)
	b, err := io.ReadAll(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return b
}

// pattern returns n deterministic non-repeating-per-chunk bytes.
func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// newSQLiteBare returns a filesystem whose database has no schema.
func newSQLiteBare(t testing.TB) *sqlfs.FileSystem {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.SQLite()))
	require.NoError(t, err)
	return fsys
}

type syncWaitGroup = sync.WaitGroup
