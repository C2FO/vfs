package sqlfs

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListQueriesTestSuite checks the queries a listing makes, on a real SQLite database.
type ListQueriesTestSuite struct {
	suite.Suite
	db   *sql.DB
	rec  *recordingQuerier
	fsys *FileSystem
}

func TestListQueriesTestSuite(t *testing.T) { suite.Run(t, new(ListQueriesTestSuite)) }

func (s *ListQueriesTestSuite) SetupTest() {
	dsn := "file:" + filepath.Join(s.T().TempDir(), "list.db") +
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = db.Close() })
	s.db = db

	s.rec = &recordingQuerier{querier: dbQuerier{db}}
	s.fsys, err = NewFileSystem(withQuerier(s.rec), WithDialect(SQLite()))
	s.Require().NoError(err)
	s.Require().NoError(s.fsys.EnsureSchema(s.T().Context()))
}

func (s *ListQueriesTestSuite) write(paths ...string) {
	for _, p := range paths {
		f, err := s.fsys.NewFile("ns", p)
		s.Require().NoError(err)
		_, err = f.Write([]byte("x"))
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
	s.rec.reset()
}

func (s *ListQueriesTestSuite) listKeys(recursive bool) []string {
	loc, err := s.fsys.NewLocation("ns", "/d/")
	s.Require().NoError(err)
	var opts []options.ListOption
	if recursive {
		opts = append(opts, list.Recursive())
	}
	entries, err := list.Collect(list.Entries(s.T().Context(), loc, opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func (s *ListQueriesTestSuite) queries() int { return s.rec.seen(s.fsys.qs.listKeys) }

func (s *ListQueriesTestSuite) TestPagesResumeAfterTheLastKey() {
	s.fsys.listPageSize = 2
	s.write("/d/a", "/d/b", "/d/c", "/d/d", "/d/e")

	s.Equal([]string{"a", "b", "c", "d", "e"}, s.listKeys(true))
	s.Equal(3, s.queries(), "two full pages and a short one")
}

func (s *ListQueriesTestSuite) TestAFullLastPageNeedsOneMoreQuery() {
	s.fsys.listPageSize = 2
	s.write("/d/a", "/d/b")

	s.Equal([]string{"a", "b"}, s.listKeys(true))
	s.Equal(2, s.queries())
}

func (s *ListQueriesTestSuite) TestKeysPastTheLocationEndThePage() {
	s.write("/d/a", "/e/b", "/e/c")

	s.Equal([]string{"a"}, s.listKeys(true))
	s.Equal(1, s.queries(), "the first key past the location ends the listing")
}

func (s *ListQueriesTestSuite) TestNonRecursiveListingSkipsEachSubdirectory() {
	paths := make([]string, 0, 13)
	for _, dir := range []string{"a", "b"} {
		for _, f := range []string{"1", "2", "3", "4", "5", "6"} {
			paths = append(paths, "/d/"+dir+"/"+f)
		}
	}
	s.write(append(paths, "/d/top")...)
	s.fsys.listPageSize = 3

	s.Equal([]string{"a/", "b/", "top"}, s.listKeys(false))
	s.Equal(3, s.queries(), "one query for each subdirectory and one after the last; no files inside are paged through")
}

func (s *ListQueriesTestSuite) TestQueriesUseTheKeyIndex() {
	rows, err := s.db.QueryContext(s.T().Context(), "EXPLAIN QUERY PLAN "+s.fsys.qs.listKeys, "ns", "/d/", "/d/a", 10)
	s.Require().NoError(err)
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		s.Require().NoError(rows.Scan(&id, &parent, &unused, &detail))
		plan = append(plan, detail)
	}
	s.Require().NoError(rows.Err())
	joined := strings.Join(plan, "\n")
	s.Contains(joined, "vfs_files_key_idx")
	s.NotContains(joined, "TEMP B-TREE", "rows come in index order, not sorted per query")
}

func (s *ListQueriesTestSuite) TestOrderDoesNotDependOnTheIndex() {
	_, err := s.db.Exec("DROP INDEX vfs_files_key_idx")
	s.Require().NoError(err)
	s.write("/d/a/z", "/d/a/b/c")
	s.Equal([]string{"a/", "a/b/", "a/b/c", "a/z"}, s.listKeys(true), "the index is for speed; order is from the query")
}

func (s *TransientTestSuite) TestListQueryFailureEndsTheListing() {
	boom := errors.New("connection refused")
	mq := newMockQuerier(s.T())
	mq.EXPECT().QueryContext(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, boom).Once()

	loc, err := s.newFS(mq).NewLocation("documents", "/")
	s.Require().NoError(err)
	var errs []error
	var got []vfs.Entry
	for e, err := range loc.(vfs.EntryLister).ListEntries(s.T().Context(), list.Recursive()) {
		got = append(got, e)
		errs = append(errs, err)
	}
	s.Require().Len(errs, 1)
	s.Require().ErrorIs(errs[0], boom)
	var pe *fs.PathError
	s.Require().ErrorAs(errs[0], &pe)
	s.Equal("list", pe.Op)
	s.Equal(vfs.Entry{}.RelPath(), got[0].RelPath())
}

func (s *TransientTestSuite) TestListQueryIsBoundedByTheOperationTimeout() {
	mq := newMockQuerier(s.T())
	mq.EXPECT().QueryContext(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, _ string, _ ...any) (*sql.Rows, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}).Once()

	loc, err := s.newFS(mq).NewLocation("documents", "/")
	s.Require().NoError(err)
	_, err = list.Collect(loc.(vfs.EntryLister).ListEntries(context.Background()))
	s.Require().ErrorIs(err, ErrTimeout)
}
