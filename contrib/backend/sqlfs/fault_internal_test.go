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
}

func (r *recordingQuerier) record(q string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, q)
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
	stmts := renderMigration(Migration{Version: 1, SQL: "CREATE TABLE vfs_a (x INT);\nINSERT INTO vfs_a VALUES (1);\n"}, "app_")
	s.Equal([]string{"CREATE TABLE app_a (x INT)", "INSERT INTO app_a VALUES (1)"}, stmts)
}
