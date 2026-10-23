//go:build vfsintegration

package sqlfs_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/testsuite"
	"github.com/c2fo/vfs/v7/options"
)

// pgDSN is the connection string of the shared PostgreSQL container started by TestMain.
var pgDSN string

// pgPrefixSeq hands out a unique table prefix per test so tests share one container without interfering.
var pgPrefixSeq atomic.Int64

func TestMain(m *testing.M) {
	os.Exit(runWithPostgres(m))
}

func runWithPostgres(m *testing.M) int {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("sqlfs"),
		tcpostgres.WithUsername("sqlfs"),
		tcpostgres.WithPassword("sqlfs"),
		tcpostgres.BasicWaitStrategies(),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		return 1
	}
	defer func() { _ = testcontainers.TerminateContainer(ctr) }()

	pgDSN, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres connection string: %v\n", err)
		return 1
	}
	return m.Run()
}

// newPostgresFS returns a filesystem on the shared container using a private table prefix, with the schema created.
func newPostgresFS(t testing.TB, opts ...options.NewFileSystemOption[sqlfs.FileSystem]) (*sqlfs.FileSystem, *sql.DB) {
	t.Helper()
	fsys, db, _ := newPostgresFSWithPrefix(t, opts...)
	return fsys, db
}

// newPostgresFSWithPrefix is like newPostgresFS but also returns the table prefix.
func newPostgresFSWithPrefix(t testing.TB, opts ...options.NewFileSystemOption[sqlfs.FileSystem],
) (*sqlfs.FileSystem, *sql.DB, string) {
	t.Helper()
	db, err := sql.Open("pgx", pgDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	prefix := fmt.Sprintf("t%d_", pgPrefixSeq.Add(1))
	all := append([]options.NewFileSystemOption[sqlfs.FileSystem]{
		sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.PostgreSQL()), sqlfs.WithTablePrefix(prefix),
	}, opts...)
	fsys, err := sqlfs.NewFileSystem(all...)
	require.NoError(t, err)
	require.NoError(t, fsys.EnsureSchema(t.Context()))
	require.NoError(t, fsys.ValidateSchema(t.Context()))
	return fsys, db, prefix
}

func TestPostgresConformance(t *testing.T) {
	fsys, _ := newPostgresFS(t)
	loc, err := fsys.NewLocation(testNS, "/conformance/")
	require.NoError(t, err)
	testsuite.RunConformanceTests(t, loc)
}

func TestPostgresConformanceSmallChunks(t *testing.T) {
	fsys, _ := newPostgresFS(t, sqlfsWithChunkSize(4096))
	loc, err := fsys.NewLocation(testNS, "/conformance/")
	require.NoError(t, err)
	testsuite.RunConformanceTests(t, loc)
}

func TestPostgresIOConformance(t *testing.T) {
	fsys, _ := newPostgresFS(t)
	loc, err := fsys.NewLocation(testNS, "/io/")
	require.NoError(t, err)
	testsuite.RunIOTests(t, loc)
}

func TestPostgresEnsureSchemaConcurrent(t *testing.T) {
	_, db := newPostgresFS(t)
	prefix := fmt.Sprintf("c%d_", pgPrefixSeq.Add(1))

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			fsys, err := sqlfs.NewFileSystem(
				sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.PostgreSQL()), sqlfs.WithTablePrefix(prefix))
			if err != nil {
				errs[i] = err
				return
			}
			errs[i] = fsys.EnsureSchema(context.Background())
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}

func TestPostgresBinaryAndMetacharacters(t *testing.T) {
	fsys, _ := newPostgresFS(t)

	all := make([]byte, 256*4)
	for i := range all {
		all[i] = byte(i)
	}
	put(t, fsys, "/binary.bin", all)
	require.True(t, bytes.Equal(all, get(t, fsys, "/binary.bin")))

	// Names containing SQL metacharacters and non-ASCII text must round-trip as data.
	names := []string{"/it's.txt", `/a"b.txt`, "/100%_done.txt", "/semi;colon.txt", "/日本語.txt", "/emoji-🙂.txt", "/a\\b.txt"}
	for _, name := range names {
		put(t, fsys, name, []byte(name))
	}
	for _, name := range names {
		require.Equal(t, name, string(get(t, fsys, name)))
	}

	// LIKE wildcards in a prefix are literal characters, not patterns.
	loc, err := fsys.NewLocation(testNS, "/")
	require.NoError(t, err)
	got, err := loc.ListByPrefix("100%")
	require.NoError(t, err)
	require.Equal(t, []string{"100%_done.txt"}, got)
	got, err = loc.ListByPrefix("100_")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestPostgresNamespaceIsolation(t *testing.T) {
	fsys, _ := newPostgresFS(t)
	for _, ns := range []string{"alpha", "alpha2", "beta"} {
		f, err := fsys.NewFile(ns, "/same.txt")
		require.NoError(t, err)
		_, err = f.Write([]byte(ns))
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}
	for _, ns := range []string{"alpha", "alpha2", "beta"} {
		f, err := fsys.NewFile(ns, "/same.txt")
		require.NoError(t, err)
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		require.Equal(t, ns, string(b))
		require.NoError(t, f.Close())
	}

	loc, err := fsys.NewLocation("alpha", "/")
	require.NoError(t, err)
	names, err := loc.List()
	require.NoError(t, err)
	require.Equal(t, []string{"same.txt"}, names)
}

func TestPostgresConcurrentWritersExactlyOneWins(t *testing.T) {
	fsys, _ := newPostgresFS(t)
	put(t, fsys, "/race.txt", []byte("start"))

	const writers = 8
	files := make([]vfs.File, writers)
	for i := range files {
		f, err := fsys.NewFile(testNS, "/race.txt")
		require.NoError(t, err)
		_, err = f.Write([]byte{byte('a' + i)})
		require.NoError(t, err)
		files[i] = f
	}

	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range files {
		wg.Go(func() { errs[i] = files[i].Close() })
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		require.ErrorIs(t, err, sqlfs.ErrConflict)
	}
	require.Equal(t, 1, wins, "exactly one concurrent writer must win")
	require.Len(t, get(t, fsys, "/race.txt"), 1)
}

func TestPostgresStableReadDuringReplacement(t *testing.T) {
	fsys, _ := newPostgresFS(t, sqlfsWithChunkSize(4096))
	original := pattern(5 * 4096)
	put(t, fsys, "/stable.bin", original)

	reader, err := fsys.NewFile(testNS, "/stable.bin")
	require.NoError(t, err)
	head := make([]byte, 4096+10) //nolint:prealloc // sized for the first read
	_, err = io.ReadFull(reader, head)
	require.NoError(t, err)

	put(t, fsys, "/stable.bin", bytes.Repeat([]byte{0xFF}, 5*4096))

	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.True(t, bytes.Equal(original, append(head, rest...)), "reader must keep seeing the revision it started on")
}

func TestPostgresCleanup(t *testing.T) {
	fsys, db, prefix := newPostgresFSWithPrefix(t,
		sqlfs.WithRevisionRetention(time.Millisecond), sqlfs.WithStagingRetention(time.Millisecond))
	put(t, fsys, "/gc.bin", pattern(10000))
	put(t, fsys, "/gc.bin", pattern(20000))

	// A writer that never closes leaves a staged revision behind.
	crashed, err := fsys.NewFile(testNS, "/crashed.bin")
	require.NoError(t, err)
	_, err = crashed.Write(pattern(10000))
	require.NoError(t, err)

	time.Sleep(20 * time.Millisecond)
	stats, err := fsys.Cleanup(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, stats.SupersededRemoved)
	require.Equal(t, 1, stats.StagedRemoved)

	var revisions int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+prefix+"revisions").Scan(&revisions))
	require.Equal(t, 1, revisions, "only the published revision of gc.bin remains")
	require.True(t, bytes.Equal(pattern(20000), get(t, fsys, "/gc.bin")))
}

func TestPostgresAcceptedTablePrefixes(t *testing.T) {
	_, db := newPostgresFS(t)
	// PostgreSQL folds unquoted identifiers to lower case, so mixed-case prefixes only work if migrations quote them.
	for _, prefix := range []string{"1_pg_", "PgApp_", "UPPER"} {
		fsys, err := sqlfs.NewFileSystem(sqlfs.WithDB(db), sqlfs.WithDialect(sqlfs.PostgreSQL()), sqlfs.WithTablePrefix(prefix))
		require.NoError(t, err)
		require.NoError(t, fsys.EnsureSchema(t.Context()))
		require.NoError(t, fsys.ValidateSchema(t.Context()))
		put(t, fsys, "/p.txt", []byte(prefix))
		require.Equal(t, prefix, string(get(t, fsys, "/p.txt")))
	}
}

func TestPostgresMoveAndDeleteGuardGenerations(t *testing.T) {
	fsys, _ := newPostgresFS(t)
	put(t, fsys, "/a.txt", []byte("a"))

	const movers = 6
	errs := make([]error, movers)
	var wg sync.WaitGroup
	for i := range movers {
		wg.Go(func() {
			src, err := fsys.NewFile(testNS, "/a.txt")
			if err != nil {
				errs[i] = err
				return
			}
			dst, err := fsys.NewFile(testNS, fmt.Sprintf("/moved-%d.txt", i))
			if err != nil {
				errs[i] = err
				return
			}
			errs[i] = src.MoveToFile(dst)
		})
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded, "only one move may consume the source path")

	loc, err := fsys.NewLocation(testNS, "/")
	require.NoError(t, err)
	names, err := loc.List()
	require.NoError(t, err)
	require.Len(t, names, 1, "the moved file must exist exactly once")
}
