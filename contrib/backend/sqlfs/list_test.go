package sqlfs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntriesTestSuite checks the native listing on SQLite. The Postgres integration tests repeat the ordering checks
// where the database's default collation is not byte order.
type ListEntriesTestSuite struct {
	suite.Suite
	fs *sqlfs.FileSystem
}

func TestListEntriesTestSuite(t *testing.T) { suite.Run(t, new(ListEntriesTestSuite)) }

func (s *ListEntriesTestSuite) SetupTest() {
	s.fs = newSQLiteFS(s.T())
}

func (s *ListEntriesTestSuite) location(p string) vfs.Location {
	loc, err := s.fs.NewLocation(testNS, p)
	s.Require().NoError(err)
	return loc
}

func (s *ListEntriesTestSuite) keys(loc vfs.Location, opts ...options.ListOption) []string {
	entries, err := list.Collect(loc.(vfs.EntryLister).ListEntries(s.T().Context(), opts...))
	s.Require().NoError(err)
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, list.OrderKey(e))
	}
	return keys
}

func (s *ListEntriesTestSuite) TestImplementsEntryLister() {
	s.Implements((*vfs.EntryLister)(nil), s.location("/"))
}

func (s *ListEntriesTestSuite) TestByteOrder() {
	sqlfs.SetListPageSize(s.fs, 2)
	listingOrderFixture(s.T(), s.fs)
	s.Equal(listingOrderRecursive, s.keys(s.location("/docs/"), list.Recursive()))
	s.Equal(listingOrderTopLevel, s.keys(s.location("/docs/")))
}

func (s *ListEntriesTestSuite) TestLocationBoundaries() {
	for _, p := range []string{"/docs/a.txt", "/docs-x/b.txt", "/docs2/c.txt", "/docsa.txt", "/doc/d.txt"} {
		put(s.T(), s.fs, p, []byte("x"))
	}
	other, err := s.fs.NewFile("other", "/docs/z.txt")
	s.Require().NoError(err)
	_, err = other.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().NoError(other.Close())

	s.Equal([]string{"a.txt"}, s.keys(s.location("/docs/"), list.Recursive()),
		"neither sibling paths sharing the prefix nor another namespace are listed")
}

func (s *ListEntriesTestSuite) TestSQLMetacharactersAreLiteral() {
	for _, p := range []string{"/m%/a.txt", "/m_/b.txt", "/mx/c.txt", "/m%x/d.txt"} {
		put(s.T(), s.fs, p, []byte("x"))
	}
	s.Equal([]string{"a.txt"}, s.keys(s.location("/m%/"), list.Recursive()))
	s.Equal([]string{"b.txt"}, s.keys(s.location("/m_/"), list.Recursive()))
	s.Equal([]string{"m%/a.txt"}, s.keys(s.location("/"), list.Recursive(), list.Prefix("m%/")))
}

func (s *ListEntriesTestSuite) TestEntriesCarryTheListedMetadata() {
	put(s.T(), s.fs, "/meta/a.txt", []byte("hello"))
	f, err := s.fs.NewFile(testNS, "/meta/a.txt")
	s.Require().NoError(err)
	modTime, err := f.LastModified()
	s.Require().NoError(err)

	entries, err := list.Collect(list.Entries(s.T().Context(), s.location("/meta/")))
	s.Require().NoError(err)
	s.Require().Len(entries, 1)
	info, err := entries[0].Info()
	s.Require().NoError(err)
	s.Equal(int64(5), info.Size())
	s.True(modTime.Equal(info.ModTime()), "listed %v, file reports %v", info.ModTime(), modTime)
}

func (s *ListEntriesTestSuite) TestEmptyLocationListsNothing() {
	s.Empty(s.keys(s.location("/nothing/here/"), list.Recursive()))
}

// listingOrderFixture writes files whose byte order differs from both (dir, name) order and from case-insensitive or
// locale-aware collations.
func listingOrderFixture(t *testing.T, fsys *sqlfs.FileSystem) {
	t.Helper()
	for _, p := range []string{
		"/docs/a/z.txt", "/docs/a/b/c.txt", "/docs/a-b.txt", "/docs/a.txt",
		"/docs/B.txt", "/docs/b.txt", "/docs/é.txt", "/docs/e.txt", "/docs/_u.txt",
	} {
		put(t, fsys, p, []byte(time.Now().String()))
	}
}

var (
	listingOrderRecursive = []string{
		"B.txt", "_u.txt", "a-b.txt", "a.txt", "a/", "a/b/", "a/b/c.txt", "a/z.txt", "b.txt", "e.txt", "é.txt",
	}
	listingOrderTopLevel = []string{"B.txt", "_u.txt", "a-b.txt", "a.txt", "a/", "b.txt", "e.txt", "é.txt"}
)
