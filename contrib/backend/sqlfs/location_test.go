package sqlfs_test

import (
	"database/sql"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/sqlfs"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/vfssimple"
)

type LocationTestSuite struct {
	suite.Suite
	fs *sqlfs.FileSystem
	db *sql.DB
}

func TestLocationTestSuite(t *testing.T) { suite.Run(t, new(LocationTestSuite)) }

func (s *LocationTestSuite) SetupTest() { s.fs, s.db = newSQLiteEnv(s.T()) }

func (s *LocationTestSuite) TestURIs() {
	tests := []struct {
		name        string
		authority   string
		path        string
		isFile      bool
		expectedURI string
	}{
		{name: "file", authority: "documents", path: "/reports/annual.pdf", isFile: true, expectedURI: "sqlfs://documents/reports/annual.pdf"},
		{name: "location", authority: "documents", path: "/reports/", expectedURI: "sqlfs://documents/reports/"},
		{name: "root", authority: "documents", path: "/", expectedURI: "sqlfs://documents/"},
		{name: "file with spaces", authority: "docs", path: "/a b/c d.txt", isFile: true, expectedURI: "sqlfs://docs/a b/c d.txt"},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			if tt.isFile {
				f, err := s.fs.NewFile(tt.authority, tt.path)
				s.Require().NoError(err)
				s.Equal(tt.expectedURI, f.URI())
				return
			}
			l, err := s.fs.NewLocation(tt.authority, tt.path)
			s.Require().NoError(err)
			s.Equal(tt.expectedURI, l.URI())
		})
	}
}

func (s *LocationTestSuite) TestPathValidation() {
	loc, err := s.fs.NewLocation(testNS, "/base/")
	s.Require().NoError(err)

	tests := []struct {
		name          string
		call          func() error
		expectedError bool
	}{
		{name: "fs NewFile relative path", call: func() error { return newFileErr(s.fs, "path/to/file.txt") }, expectedError: true},
		{name: "fs NewFile empty path", call: func() error { return newFileErr(s.fs, "") }, expectedError: true},
		{name: "fs NewFile directory path", call: func() error { return newFileErr(s.fs, "/dir/") }, expectedError: true},
		{name: "fs NewLocation relative path", call: func() error { return newLocErr(s.fs, "rel/") }, expectedError: true},
		{name: "fs NewLocation without trailing slash", call: func() error { return newLocErr(s.fs, "/reports") }, expectedError: true},
		{name: "fs NewLocation empty path", call: func() error { return newLocErr(s.fs, "") }, expectedError: true},
		{name: "loc NewLocation empty", call: func() error { _, err := loc.NewLocation(""); return err }, expectedError: true},
		{name: "loc NewLocation absolute", call: func() error { _, err := loc.NewLocation("/home/"); return err }, expectedError: true},
		{name: "loc NewLocation file path", call: func() error { _, err := loc.NewLocation("file.txt"); return err }, expectedError: true},
		{name: "loc NewFile empty", call: func() error { _, err := loc.NewFile(""); return err }, expectedError: true},
		{name: "loc NewFile absolute", call: func() error { _, err := loc.NewFile("/abs.txt"); return err }, expectedError: true},
		{name: "loc NewLocation relative", call: func() error { _, err := loc.NewLocation("a/b/"); return err }},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.call()
			if tt.expectedError {
				s.Error(err)
				return
			}
			s.NoError(err)
		})
	}
}

func (s *LocationTestSuite) TestDotSegmentsAndChangeDir() {
	loc, err := s.fs.NewLocation(testNS, "/base/")
	s.Require().NoError(err)

	nl, err := loc.NewLocation("l1/./l2/../l3/")
	s.Require().NoError(err)
	s.Equal("/base/l1/l3/", nl.Path())

	s.Require().NoError(nl.ChangeDir("../up/")) //nolint:staticcheck // ChangeDir is part of the interface and must work
	s.Equal("/base/l1/up/", nl.Path())
	s.Equal(testNS, nl.Authority().String())
	s.Equal(testNS, nl.Volume()) //nolint:staticcheck // Volume is part of the interface and must work
	s.Same(s.fs, nl.FileSystem())
	s.Equal(nl.URI(), nl.(*sqlfs.Location).String())
	s.Require().Error(nl.ChangeDir("")) //nolint:staticcheck // see above
}

func (s *LocationTestSuite) TestNonASCIINames() {
	const p = "/résumé 2026 (final).txt"
	put(s.T(), s.fs, p, []byte("héllo"))
	s.Equal("héllo", string(get(s.T(), s.fs, p)))

	f, err := s.fs.NewFile(testNS, p)
	s.Require().NoError(err)
	s.Equal("résumé 2026 (final).txt", f.Name())
	s.Equal(p, f.Path())
}

func (s *LocationTestSuite) TestListing() {
	for _, p := range []string{
		"/a/file1.txt", "/a/file2.txt", "/a/other.txt", "/a/File3.txt", "/a/a_b.txt", "/a/axb.txt", "/a/sub/nested.txt", "/b/x.txt",
	} {
		put(s.T(), s.fs, p, []byte("x"))
	}
	loc, err := s.fs.NewLocation(testNS, "/a/")
	s.Require().NoError(err)

	tests := []struct {
		name     string
		list     func() ([]string, error)
		expected []string
	}{
		{name: "list excludes nested and other dirs", list: loc.List,
			expected: []string{"File3.txt", "a_b.txt", "axb.txt", "file1.txt", "file2.txt", "other.txt"}},
		{name: "prefix filter", list: func() ([]string, error) { return loc.ListByPrefix("file") },
			expected: []string{"file1.txt", "file2.txt"}},
		{name: "prefix is case sensitive", list: func() ([]string, error) { return loc.ListByPrefix("File") },
			expected: []string{"File3.txt"}},
		{name: "prefix with wildcard character", list: func() ([]string, error) { return loc.ListByPrefix("a_") },
			expected: []string{"a_b.txt"}},
		{name: "prefix with relative directory", list: func() ([]string, error) { return loc.ListByPrefix("sub/nes") },
			expected: []string{"nested.txt"}},
		{name: "regex filter", list: func() ([]string, error) { return loc.ListByRegex(regexp.MustCompile(`^file\d\.txt$`)) },
			expected: []string{"file1.txt", "file2.txt"}},
		{name: "regex without matches", list: func() ([]string, error) { return loc.ListByRegex(regexp.MustCompile(`Z`)) },
			expected: []string{}},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := tt.list()
			s.Require().NoError(err)
			s.Equal(tt.expected, got)
		})
	}
}

func (s *LocationTestSuite) TestStagedFileHiddenFromListing() {
	f, err := s.fs.NewFile(testNS, "/staged/pending.txt")
	s.Require().NoError(err)
	_, err = f.Write([]byte("not yet"))
	s.Require().NoError(err)

	loc, err := s.fs.NewLocation(testNS, "/staged/")
	s.Require().NoError(err)
	names, err := loc.List()
	s.Require().NoError(err)
	s.Empty(names)

	s.Require().NoError(f.Close())
	names, err = loc.List()
	s.Require().NoError(err)
	s.Equal([]string{"pending.txt"}, names)
}

func (s *LocationTestSuite) TestNamespaceIsolation() {
	for ns, content := range map[string]string{"a": "from a", "b": "from b", "": "from default"} {
		f, err := s.fs.NewFile(ns, "/x.txt")
		s.Require().NoError(err)
		_, err = f.Write([]byte(content))
		s.Require().NoError(err)
		s.Require().NoError(f.Close())
	}
	for ns, content := range map[string]string{"a": "from a", "b": "from b", "": "from default"} {
		s.Run("namespace "+ns, func() {
			f, err := s.fs.NewFile(ns, "/x.txt")
			s.Require().NoError(err)
			b := make([]byte, 64)
			n, _ := f.Read(b)
			s.Equal(content, string(b[:n]))
			s.Require().NoError(f.Close())

			loc, err := s.fs.NewLocation(ns, "/")
			s.Require().NoError(err)
			names, err := loc.List()
			s.Require().NoError(err)
			s.Equal([]string{"x.txt"}, names)
		})
	}

	// deleting in one namespace leaves the others alone
	f, err := s.fs.NewFile("a", "/x.txt")
	s.Require().NoError(err)
	s.Require().NoError(f.Delete())
	other, err := s.fs.NewFile("b", "/x.txt")
	s.Require().NoError(err)
	ok, err := other.Exists()
	s.Require().NoError(err)
	s.True(ok)
}

func (s *LocationTestSuite) TestTablePrefixIsolation() {
	other, err := sqlfs.NewFileSystem(sqlfs.WithDB(s.db), sqlfs.WithDialect(sqlfs.SQLite()), sqlfs.WithTablePrefix("other_"))
	s.Require().NoError(err)
	s.Require().NoError(other.EnsureSchema(s.T().Context()))

	put(s.T(), s.fs, "/iso.txt", []byte("default prefix"))

	f, err := other.NewFile(testNS, "/iso.txt")
	s.Require().NoError(err)
	ok, err := f.Exists()
	s.Require().NoError(err)
	s.False(ok)
}

func (s *LocationTestSuite) TestExists() {
	loc, err := s.fs.NewLocation(testNS, "/never/used/")
	s.Require().NoError(err)
	ok, err := loc.Exists()
	s.Require().NoError(err)
	s.True(ok, "virtual directories always exist when the schema is valid")

	bare := newSQLiteBare(s.T())
	bareLoc, err := bare.NewLocation(testNS, "/")
	s.Require().NoError(err)
	ok, err = bareLoc.Exists()
	s.Require().ErrorIs(err, sqlfs.ErrInvalidSchema)
	s.False(ok)
}

func (s *LocationTestSuite) TestDeleteFileThroughLocation() {
	put(s.T(), s.fs, "/del/a.txt", []byte("x"))
	loc, err := s.fs.NewLocation(testNS, "/del/")
	s.Require().NoError(err)
	s.Require().NoError(loc.DeleteFile("a.txt"))

	names, err := loc.List()
	s.Require().NoError(err)
	s.Empty(names)
	s.Require().Error(loc.DeleteFile(""))
}

func (s *LocationTestSuite) TestRegistration() {
	const uri = "sqlfs://documents/reg/a.txt"
	s.Require().NotContains(backend.RegisteredBackends(), sqlfs.Scheme, "importing the package must not register a backend")
	_, err := vfssimple.NewFile(uri)
	s.Require().Error(err)

	backend.Register(sqlfs.Scheme, s.fs)
	s.T().Cleanup(func() { backend.Unregister(sqlfs.Scheme) })

	f, err := vfssimple.NewFile(uri)
	s.Require().NoError(err)
	_, err = f.Write([]byte("via vfssimple"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.Equal("via vfssimple", string(get(s.T(), s.fs, "/reg/a.txt")))
}

func newFileErr(fsys *sqlfs.FileSystem, p string) error {
	_, err := fsys.NewFile(testNS, p)
	return err
}

func newLocErr(fsys *sqlfs.FileSystem, p string) error {
	_, err := fsys.NewLocation(testNS, p)
	return err
}
