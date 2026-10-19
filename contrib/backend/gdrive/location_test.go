package gdrive

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/gdrive/mocks"
	"github.com/c2fo/vfs/contrib/backend/gdrive/types"
)

type LocationTestSuite struct {
	suite.Suite
	client *mocks.Client
	fs     *FileSystem
}

func TestLocationTestSuite(t *testing.T) {
	suite.Run(t, new(LocationTestSuite))
}

func (s *LocationTestSuite) SetupTest() {
	s.client = mocks.NewClient(s.T())
	s.fs = NewFileSystem(WithClient(s.client))
}

func (s *LocationTestSuite) loc(p string) *Location {
	l, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return l.(*Location)
}

var errBoom = errors.New("boom")

func (s *LocationTestSuite) TestList() {
	items := []types.Item{
		{Name: "a.txt"}, {Name: "sub", IsFolder: true}, {Name: "b.log"}, {Name: "ab.txt"},
	}

	tests := []struct {
		name        string
		call        func(l *Location) ([]string, error)
		listResult  []types.Item
		listErr     error
		expected    []string
		expectedErr string
	}{
		{name: "list skips folders", call: (*Location).List, listResult: items, expected: []string{"a.txt", "b.log", "ab.txt"}},
		{name: "missing folder lists empty", call: (*Location).List, listErr: ErrNotFound, expected: []string{}},
		{name: "list error", call: (*Location).List, listErr: errBoom, expectedErr: "boom"},
		{name: "prefix", call: func(l *Location) ([]string, error) { return l.ListByPrefix("a") }, listResult: items,
			expected: []string{"a.txt", "ab.txt"}},
		{name: "prefix error", call: func(l *Location) ([]string, error) { return l.ListByPrefix("a") }, listErr: errBoom,
			expectedErr: "boom"},
		{name: "regex", call: func(l *Location) ([]string, error) { return l.ListByRegex(regexp.MustCompile(`\.log$`)) },
			listResult: items, expected: []string{"b.log"}},
		{name: "regex error", call: func(l *Location) ([]string, error) { return l.ListByRegex(regexp.MustCompile(`x`)) },
			listErr: errBoom, expectedErr: "boom"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().ListChildren(mock.Anything, "dir").Return(tt.listResult, tt.listErr).Once()

			got, err := tt.call(s.loc("/dir/"))
			if tt.expectedErr != "" {
				s.Require().ErrorContains(err, tt.expectedErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, got)
		})
	}
}

func (s *LocationTestSuite) TestListByPrefixRelativePath() {
	s.client.EXPECT().ListChildren(mock.Anything, "dir/sub").
		Return([]types.Item{{Name: "pre1.txt"}, {Name: "other.txt"}}, nil).Once()

	got, err := s.loc("/dir/").ListByPrefix("sub/pre")
	s.Require().NoError(err)
	s.Equal([]string{"pre1.txt"}, got)
}

func (s *LocationTestSuite) TestExists() {
	tests := []struct {
		name        string
		path        string
		item        *types.Item
		err         error
		expected    bool
		expectedErr bool
		skipCall    bool
	}{
		{name: "root always exists", path: "/", skipCall: true, expected: true},
		{name: "folder", path: "/dir/", item: &types.Item{IsFolder: true}, expected: true},
		{name: "file at path", path: "/dir/", item: &types.Item{}, expected: false},
		{name: "missing", path: "/dir/", err: ErrNotFound, expected: false},
		{name: "error", path: "/dir/", err: errBoom, expectedErr: true},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			if !tt.skipCall {
				s.client.EXPECT().GetItem(mock.Anything, "dir").Return(tt.item, tt.err).Once()
			}
			got, err := s.loc(tt.path).Exists()
			if tt.expectedErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, got)
		})
	}
}

func (s *LocationTestSuite) TestNavigationAndIdentity() {
	l := s.loc("/a/")

	child, err := l.NewLocation("b/c/")
	s.Require().NoError(err)
	s.Equal("/a/b/c/", child.Path())
	s.Equal("gdrive:///a/b/c/", child.URI())
	s.Equal(child.URI(), child.(*Location).String())
	s.Equal(s.fs, child.FileSystem())
	s.Empty(child.Authority().String())
	s.Empty(child.(*Location).Volume())

	_, err = l.NewLocation("")
	s.Require().ErrorIs(err, errPathRequired)
	_, err = l.NewLocation("/abs/")
	s.Require().Error(err)

	f, err := l.NewFile("x/y.txt")
	s.Require().NoError(err)
	s.Equal("/a/x/y.txt", f.Path())
	s.Equal("/a/x/", f.Location().Path())

	_, err = l.NewFile("")
	s.Require().ErrorIs(err, errPathRequired)
	_, err = l.NewFile("/abs.txt")
	s.Require().Error(err)

	s.Require().NoError(l.ChangeDir("z/"))
	s.Equal("/a/z/", l.Path())
	s.Require().ErrorIs(l.ChangeDir(""), errPathRequired)
	s.Require().Error(l.ChangeDir("/abs/"))

	var nilLoc *Location
	_, err = nilLoc.NewLocation("a/")
	s.Require().ErrorIs(err, errLocationRequired)
	_, err = nilLoc.NewFile("a")
	s.Require().ErrorIs(err, errLocationRequired)
	s.Require().ErrorIs(nilLoc.ChangeDir("a/"), errLocationRequired)
}

func (s *LocationTestSuite) TestSharedDriveAuthority() {
	l, err := s.fs.NewLocation("0ABdrive", "/a/")
	s.Require().NoError(err)
	s.Equal("0ABdrive", l.Authority().String())
	s.Equal("gdrive://0ABdrive/a/", l.URI())

	f, err := l.NewFile("f.txt")
	s.Require().NoError(err)
	s.Equal("gdrive://0ABdrive/a/f.txt", f.URI())
}

func (s *LocationTestSuite) TestDeleteFile() {
	s.client.EXPECT().Delete(mock.Anything, "a/f.txt").Return(nil).Once()
	s.Require().NoError(s.loc("/a/").DeleteFile("f.txt"))

	s.client.EXPECT().Delete(mock.Anything, "a/g.txt").Return(errBoom).Once()
	s.Require().Error(s.loc("/a/").DeleteFile("g.txt"))

	s.Require().Error(s.loc("/a/").DeleteFile(""))
}

func (s *LocationTestSuite) TestClientErrorPropagates() {
	fs := NewFileSystem() // no credentials
	s.T().Setenv("VFS_GDRIVE_ACCESS_TOKEN", "")
	l, err := fs.NewLocation("", "/a/")
	s.Require().NoError(err)

	_, err = l.List()
	s.Require().ErrorIs(err, errCredentialsRequired)
	_, err = l.Exists()
	s.Require().ErrorIs(err, errCredentialsRequired)
}
