package sqlfs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	_ "modernc.org/sqlite"
)

type codedError struct{ code int }

func (e codedError) Error() string { return fmt.Sprintf("sqlite error %d", e.code) }
func (e codedError) Code() int     { return e.code }

// recordingQuerier wraps a real database to record statements and to make commits fail. It decorates rather than
// mocks because the tests need real transactions and rows underneath the injected fault.
type recordingQuerier struct {
	querier
	mu         sync.Mutex
	queries    []string
	failCommit error
	// lostCommit, if set, commits the transaction and then reports this error, as when a connection drops after
	// the database committed.
	lostCommit error
	// execDelay, if set, delays every statement executed in a transaction.
	execDelay time.Duration
	// beforeBegin, if set, runs before each transaction starts.
	beforeBegin func()
}

func (r *recordingQuerier) record(q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, q)
}

func (r *recordingQuerier) seenContaining(sub string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, q := range r.queries {
		if strings.Contains(q, sub) {
			n++
		}
	}
	return n
}

func (r *recordingQuerier) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = nil
}

func (r *recordingQuerier) seen(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, q := range r.queries {
		if strings.HasPrefix(strings.TrimSpace(q), prefix) {
			n++
		}
	}
	return n
}

func (r *recordingQuerier) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	r.record(q)
	return r.querier.ExecContext(ctx, q, args...)
}

func (r *recordingQuerier) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	r.record(q)
	return r.querier.QueryContext(ctx, q, args...)
}

func (r *recordingQuerier) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	r.record(q)
	return r.querier.QueryRowContext(ctx, q, args...)
}

func (r *recordingQuerier) BeginTx(ctx context.Context, o *sql.TxOptions) (txn, error) {
	if r.beforeBegin != nil {
		r.beforeBegin()
	}
	tx, err := r.querier.BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	return &recordingTxn{txn: tx, r: r}, nil
}

type recordingTxn struct {
	txn
	r *recordingQuerier
}

func (t *recordingTxn) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	t.r.record(q)
	if t.r.execDelay > 0 {
		select {
		case <-time.After(t.r.execDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return t.txn.ExecContext(ctx, q, args...)
}

func (t *recordingTxn) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	t.r.record(q)
	return t.txn.QueryContext(ctx, q, args...)
}

func (t *recordingTxn) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	t.r.record(q)
	return t.txn.QueryRowContext(ctx, q, args...)
}

func (t *recordingTxn) Commit() error {
	if t.r.failCommit != nil {
		_ = t.Rollback()
		return t.r.failCommit
	}
	if t.r.lostCommit != nil {
		if err := t.txn.Commit(); err != nil {
			return err
		}
		return t.r.lostCommit
	}
	return t.txn.Commit()
}

const faultChunk = MinChunkSize

type FaultTestSuite struct {
	suite.Suite
	db    *sql.DB
	plain *FileSystem
	rec   *recordingQuerier
	fs    *FileSystem
}

func TestFaultTestSuite(t *testing.T) { suite.Run(t, new(FaultTestSuite)) }

func (s *FaultTestSuite) SetupTest() {
	dsn := "file:" + filepath.Join(s.T().TempDir(), "fault.db") +
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = db.Close() })
	s.db = db

	s.plain, err = NewFileSystem(WithDB(db), WithDialect(SQLite()), WithChunkSize(faultChunk))
	s.Require().NoError(err)
	s.Require().NoError(s.plain.EnsureSchema(s.T().Context()))

	s.rec = &recordingQuerier{querier: dbQuerier{db}}
	s.fs, err = NewFileSystem(withQuerier(s.rec), WithDialect(SQLite()), WithChunkSize(faultChunk))
	s.Require().NoError(err)
}

func (s *FaultTestSuite) put(fsys *FileSystem, p string, content []byte) {
	f, err := fsys.NewFile("documents", p)
	s.Require().NoError(err)
	_, err = f.Write(content)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *FaultTestSuite) get(fsys *FileSystem, p string) []byte {
	f, err := fsys.NewFile("documents", p)
	s.Require().NoError(err)
	b, err := io.ReadAll(f)
	s.Require().NoError(err)
	return b
}

func (s *FaultTestSuite) TestPublishFailureKeepsPreviousContent() {
	s.put(s.plain, "/p.txt", []byte("original"))

	f, err := s.fs.NewFile("documents", "/p.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("replacement"))
	s.Require().NoError(err)

	s.rec.failCommit = errors.New("simulated commit failure")
	err = f.Close()
	s.Require().Error(err)
	s.Contains(err.Error(), "simulated commit failure")

	s.Equal("original", string(s.get(s.plain, "/p.txt")), "readers still see the previous content")

	// the handle kept its staged writes, so the caller can retry once the database recovers
	s.rec.failCommit = nil
	s.Require().NoError(f.Close())
	s.Equal("replacement", string(s.get(s.plain, "/p.txt")))
}

func (s *FaultTestSuite) TestFailedNewFilePublishLeavesNoFile() {
	f, err := s.fs.NewFile("documents", "/new.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("x"))
	s.Require().NoError(err)

	s.rec.failCommit = errors.New("simulated commit failure")
	s.Require().Error(f.Close())
	s.rec.failCommit = nil

	other, err := s.plain.NewFile("documents", "/new.txt")
	s.Require().NoError(err)
	ok, err := other.Exists()
	s.Require().NoError(err)
	s.False(ok)
	s.Require().NoError(f.(*File).Abort())
}

func (s *FaultTestSuite) TestErrorsNeverIncludeContent() {
	f, err := s.fs.NewFile("documents", "/secret.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("top secret payload"))
	s.Require().NoError(err)

	s.rec.failCommit = errors.New("driver failure")
	err = f.Close()
	s.Require().Error(err)
	s.NotContains(err.Error(), "top secret payload")
	s.Contains(err.Error(), "driver failure", "the driver error stays in the chain")
	s.rec.failCommit = nil
	s.Require().NoError(f.(*File).Abort())
}

func (s *FaultTestSuite) TestSameInstanceCopyAndMoveNeverReadChunks() {
	content := bytes.Repeat([]byte("0123456789"), 5*faultChunk/10)
	s.put(s.fs, "/a.bin", content)

	a, err := s.fs.NewFile("documents", "/a.bin")
	s.Require().NoError(err)
	b, err := s.fs.NewFile("documents", "/b.bin")
	s.Require().NoError(err)
	c, err := s.fs.NewFile("documents", "/c.bin")
	s.Require().NoError(err)

	s.rec.mu.Lock()
	s.rec.queries = nil
	s.rec.mu.Unlock()

	s.Require().NoError(a.CopyToFile(b))
	s.Require().NoError(b.MoveToFile(c))

	s.Zero(s.rec.seen("SELECT data"), "no file content may be read into the application")
	s.Positive(s.rec.seen("INSERT INTO"), "chunks are copied inside the database")
	s.True(bytes.Equal(content, s.get(s.plain, "/c.bin")))
}

func (s *FaultTestSuite) TestFailedMoveAndCopyChangeNothing() {
	s.put(s.plain, "/src.txt", []byte("source"))
	s.put(s.plain, "/dst.txt", []byte("target"))

	for name, op := range map[string]func(src, dst *File) error{
		"move": func(src, dst *File) error { return src.MoveToFile(dst) },
		"copy": func(src, dst *File) error { return src.CopyToFile(dst) },
	} {
		s.Run(name, func() {
			src, err := s.fs.NewFile("documents", "/src.txt")
			s.Require().NoError(err)
			dst, err := s.fs.NewFile("documents", "/dst.txt")
			s.Require().NoError(err)

			s.rec.failCommit = errors.New("simulated commit failure")
			s.Require().Error(op(src.(*File), dst.(*File)))
			s.rec.failCommit = nil

			s.Equal("source", string(s.get(s.plain, "/src.txt")))
			s.Equal("target", string(s.get(s.plain, "/dst.txt")))
		})
	}
}

func (s *FaultTestSuite) TestBoundedBuffers() {
	const chunks = 64
	w, err := s.fs.NewFile("documents", "/bounded.bin")
	s.Require().NoError(err)
	wf := w.(*File)
	data := bytes.Repeat([]byte{7}, 1000)
	for written := 0; written < chunks*faultChunk; written += len(data) {
		_, err = wf.Write(data)
		s.Require().NoError(err)
		s.Require().LessOrEqual(len(wf.wbuf.data), faultChunk, "write buffer holds at most one chunk")
	}
	s.Require().NoError(wf.Close())

	r, err := s.fs.NewFile("documents", "/bounded.bin")
	s.Require().NoError(err)
	rf := r.(*File)
	buf := make([]byte, 700)
	total := 0
	for {
		n, err := rf.Read(buf)
		total += n
		s.Require().LessOrEqual(len(rf.rbuf.data), faultChunk, "read buffer holds at most one chunk")
		if errors.Is(err, io.EOF) {
			break
		}
		s.Require().NoError(err)
	}
	s.GreaterOrEqual(total, chunks*faultChunk)
}

type TransientTestSuite struct {
	suite.Suite
	db *sql.DB
}

func TestTransientTestSuite(t *testing.T) { suite.Run(t, new(TransientTestSuite)) }

func (s *TransientTestSuite) SetupTest() {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(s.T().TempDir(), "t.db")+"?_txlock=immediate")
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = db.Close() })
	s.db = db
}

func (s *TransientTestSuite) newFS(mq *mockQuerier) *FileSystem {
	fsys, err := NewFileSystem(withQuerier(mq), WithDialect(SQLite()), WithOperationTimeout(50*time.Millisecond))
	s.Require().NoError(err)
	return fsys
}

func (s *TransientTestSuite) realBegin() func(context.Context, *sql.TxOptions) (txn, error) {
	return func(ctx context.Context, o *sql.TxOptions) (txn, error) { return dbQuerier{s.db}.BeginTx(ctx, o) }
}

func (s *TransientTestSuite) TestRetryTransientErrors() {
	tests := []struct {
		name          string
		failures      int
		err           error
		expectedBegin int
		expectedError bool
	}{
		{name: "succeeds after two busy errors", failures: 2, err: codedError{5}, expectedBegin: 3},
		{name: "gives up after the attempt limit", failures: 5, err: codedError{5}, expectedBegin: maxTxAttempts, expectedError: true},
		{name: "non-transient error is not retried", failures: 5, err: errors.New("boom"), expectedBegin: 1, expectedError: true},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.SetupTest()
			for _, m := range SQLite().Migrations() {
				_, err := s.db.Exec(m.SQL)
				s.Require().NoError(err)
			}

			mq := newMockQuerier(s.T())
			calls := 0
			mq.EXPECT().BeginTx(mock.Anything, mock.Anything).RunAndReturn(
				func(ctx context.Context, o *sql.TxOptions) (txn, error) {
					calls++
					if calls <= tt.failures {
						return nil, tt.err
					}
					return s.realBegin()(ctx, o)
				}).Maybe()

			f, err := s.newFS(mq).NewFile("documents", "/t.txt")
			s.Require().NoError(err)
			err = f.Touch()
			if tt.expectedError {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
			s.Equal(tt.expectedBegin, calls)
		})
	}
}

func (s *TransientTestSuite) TestOperationTimeout() {
	mq := newMockQuerier(s.T())
	mq.EXPECT().QueryContext(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ string, _ ...any) (*sql.Rows, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})

	loc, err := s.newFS(mq).NewLocation("documents", "/")
	s.Require().NoError(err)
	_, err = loc.List()
	s.Require().ErrorIs(err, ErrTimeout)
	s.Require().ErrorIs(err, context.DeadlineExceeded)
}

func (s *TransientTestSuite) TestUnavailableDatabase() {
	mq := newMockQuerier(s.T())
	mq.EXPECT().BeginTx(mock.Anything, mock.Anything).Return(nil, codedError{14})

	f, err := s.newFS(mq).NewFile("documents", "/t.txt")
	s.Require().NoError(err)
	err = f.Touch()
	s.Require().ErrorIs(err, ErrUnavailable)
}

func (s *TransientTestSuite) TestRenderMigrationPrefix() {
	m := Migration{Version: 1, SQL: "CREATE TABLE vfs_a (x INT);\nINSERT INTO vfs_a VALUES (1);\n"}
	stmts := renderMigration(m, "App_", SQLite().QuoteIdentifier)
	s.Equal([]string{`CREATE TABLE "App_a" (x INT)`, `INSERT INTO "App_a" VALUES (1)`}, stmts)
}

func (s *TransientTestSuite) TestRenderMigrationLineEndings() {
	expected := []string{`CREATE TABLE "vfs_a" (x INT)`, `INSERT INTO "vfs_a" VALUES (1)`}
	for name, sqlText := range map[string]string{
		"crlf":                "CREATE TABLE vfs_a (x INT);\r\nINSERT INTO vfs_a VALUES (1);\r\n",
		"trailing blanks":     "CREATE TABLE vfs_a (x INT);  \nINSERT INTO vfs_a VALUES (1);\t\n",
		"no final newline":    "CREATE TABLE vfs_a (x INT);\nINSERT INTO vfs_a VALUES (1);",
		"embedded migrations": "",
	} {
		s.Run(name, func() {
			if sqlText == "" {
				for _, d := range []Dialect{SQLite(), PostgreSQL()} {
					crlf := strings.ReplaceAll(d.Migrations()[0].SQL, "\n", "\r\n")
					s.Equal(renderMigration(d.Migrations()[0], "vfs_", d.QuoteIdentifier),
						renderMigration(Migration{Version: 1, SQL: crlf}, "vfs_", d.QuoteIdentifier))
				}
				return
			}
			s.Equal(expected, renderMigration(Migration{Version: 1, SQL: sqlText}, "vfs_", SQLite().QuoteIdentifier))
		})
	}
}

func (s *TransientTestSuite) TestSchemaLockKeyDependsOnPrefix() {
	s.NotEqual(schemaLockKey("vfs_"), schemaLockKey("app_"))
}

func (s *FaultTestSuite) TestCleanupSkipsRevisionRenewedAfterSelection() {
	fsys, err := NewFileSystem(withQuerier(s.rec), WithDialect(SQLite()), WithChunkSize(faultChunk),
		WithStagingRetention(time.Nanosecond))
	s.Require().NoError(err)

	w, err := fsys.NewFile("documents", "/renewed.txt")
	s.Require().NoError(err)
	_, err = w.Write([]byte("in progress"))
	s.Require().NoError(err)
	s.Require().NoError(w.(*File).flush())
	time.Sleep(3 * time.Millisecond)

	// The writer renews its staged revision after Cleanup selected it but before Cleanup's transaction begins.
	s.rec.beforeBegin = func() {
		_, execErr := s.db.Exec("UPDATE vfs_revisions SET expires_at = ? WHERE state = 0", nowNanos()+int64(time.Hour))
		s.Require().NoError(execErr)
		s.rec.beforeBegin = nil
	}
	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Zero(stats.StagedRemoved, "a renewed writer must not be counted or removed")

	var chunks int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM vfs_chunks").Scan(&chunks))
	s.Equal(1, chunks, "the active writer's chunks must survive")
	s.Require().NoError(w.Close())
}

func (s *FaultTestSuite) TestDeleteVersionRefusesChangedSource() {
	s.put(s.plain, "/mv.txt", []byte("v1"))
	src, err := s.plain.NewFile("documents", "/mv.txt")
	s.Require().NoError(err)
	f := src.(*File)
	s.Require().NoError(f.pin())
	observed := f.tok.row

	s.put(s.plain, "/mv.txt", []byte("v2 written while the move was streaming"))

	err = f.deleteVersion(observed)
	s.Require().ErrorIs(err, ErrConflict)
	got, err := s.plain.NewFile("documents", "/mv.txt")
	s.Require().NoError(err)
	b, err := io.ReadAll(got)
	s.Require().NoError(err)
	s.Equal("v2 written while the move was streaming", string(b), "the newer content must not be deleted")
}

func (s *FaultTestSuite) TestInTxStopsRetryWaitOnCancel() {
	ctx, cancel := context.WithCancel(s.T().Context())
	attempts := 0
	cancel()
	err := s.plain.inTx(ctx, "test", func(context.Context, txn) error {
		attempts++
		return errRetry
	})
	s.Require().ErrorIs(err, context.Canceled)
	s.LessOrEqual(attempts, 1)
}

func (s *FaultTestSuite) TestLostCommitThenAbortKeepsPublishedContent() {
	s.put(s.plain, "/lost.txt", []byte("original"))
	f, err := s.fs.NewFile("documents", "/lost.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("replacement"))
	s.Require().NoError(err)

	s.rec.lostCommit = errors.New("connection reset after commit")
	s.Require().Error(f.Close(), "the caller cannot tell the commit succeeded")
	s.rec.lostCommit = nil

	s.Require().NoError(f.(*File).Abort())
	s.Equal("replacement", string(s.get(s.plain, "/lost.txt")), "Abort must not remove published content")
	var chunks int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM vfs_chunks c JOIN vfs_files f ON f.revision_id = c.revision_id").Scan(&chunks))
	s.Equal(1, chunks)
}

func (s *FaultTestSuite) TestLostCommitThenCloseRetrySucceeds() {
	f, err := s.fs.NewFile("documents", "/retry.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("content"))
	s.Require().NoError(err)

	s.rec.lostCommit = errors.New("timeout after commit")
	s.Require().Error(f.Close())
	s.rec.lostCommit = nil

	s.Require().NoError(f.Close(), "retrying Close after a lost commit reports success")
	s.Equal("content", string(s.get(s.plain, "/retry.txt")))
	var revisions int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM vfs_revisions").Scan(&revisions))
	s.Equal(1, revisions, "content was published exactly once")
}

func (s *FaultTestSuite) TestTimeoutBoundsEachStatementNotTheTransaction() {
	const timeout = 200 * time.Millisecond
	fsys, err := NewFileSystem(withQuerier(s.rec), WithDialect(SQLite()), WithChunkSize(faultChunk),
		WithOperationTimeout(timeout))
	s.Require().NoError(err)
	f, err := fsys.NewFile("documents", "/slow.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("v1"))
	s.Require().NoError(err)

	// Publishing a new file runs two statements in one transaction; together they exceed the timeout.
	s.rec.execDelay = timeout * 2 / 3
	s.Require().NoError(f.Close(), "a transaction of individually bounded statements is not cut short")
	s.rec.execDelay = 0
}

func (s *FaultTestSuite) TestSlowStatementInTransactionTimesOut() {
	fsys, err := NewFileSystem(withQuerier(s.rec), WithDialect(SQLite()), WithChunkSize(faultChunk),
		WithOperationTimeout(50*time.Millisecond))
	s.Require().NoError(err)
	f, err := fsys.NewFile("documents", "/stuck.txt")
	s.Require().NoError(err)

	s.rec.execDelay = time.Second
	err = f.Touch()
	s.rec.execDelay = 0
	s.Require().ErrorIs(err, ErrTimeout)
}

func (s *FaultTestSuite) TestBulkChunkStatementsAreBounded() {
	defer func(old int) { statementBytes = old }(statementBytes)
	statementBytes = 2 * faultChunk // two chunks per statement

	content := bytes.Repeat([]byte("x"), 5*faultChunk)
	s.put(s.fs, "/bulk.bin", content)
	src, err := s.fs.NewFile("documents", "/bulk.bin")
	s.Require().NoError(err)
	dst, err := s.fs.NewFile("documents", "/bulk-copy.bin")
	s.Require().NoError(err)

	s.rec.reset()
	s.Require().NoError(src.CopyToFile(dst))
	s.Equal(3, s.rec.seenContaining("SELECT ?, chunk_index, data"), "5 chunks copied in statements of at most 2")
	s.True(bytes.Equal(content, s.get(s.plain, "/bulk-copy.bin")))

	fsys, err := NewFileSystem(withQuerier(s.rec), WithDialect(SQLite()), WithChunkSize(faultChunk),
		WithRevisionRetention(time.Nanosecond))
	s.Require().NoError(err)
	gone, err := fsys.NewFile("documents", "/bulk.bin")
	s.Require().NoError(err)
	s.Require().NoError(gone.Delete())
	time.Sleep(3 * time.Millisecond)

	s.rec.reset()
	stats, err := fsys.Cleanup(s.T().Context())
	s.Require().NoError(err)
	s.Equal(1, stats.SupersededRemoved)
	s.Equal(3, s.rec.seenContaining("DELETE FROM \"vfs_chunks\""), "5 chunks deleted in statements of at most 2")
	var chunks int
	s.Require().NoError(s.db.QueryRow("SELECT COUNT(*) FROM vfs_chunks").Scan(&chunks))
	s.Equal(5, chunks, "only the copy's chunks remain")
}
