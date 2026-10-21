package onedrive

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/onedrive/mocks"
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

func (s *LocationTestSuite) location(p string) *Location {
	loc, err := s.fs.NewLocation("", p)
	s.Require().NoError(err)
	return loc.(*Location)
}

var listing = []DriveItem{
	{Name: "a.txt"},
	{Name: "b.csv"},
	{Name: "sub", IsFolder: true},
	{Name: "abc.txt"},
}

func (s *LocationTestSuite) TestPath() {
	loc, err := s.fs.NewLocation("", "/path/to/folder/")
	s.Require().NoError(err)
	s.Equal("/path/to/folder/", loc.Path())
}

func (s *LocationTestSuite) TestURI() {
	loc, err := s.fs.NewLocation("", "/path/to/folder/")
	s.Require().NoError(err)
	s.Equal("onedrive:///path/to/folder/", loc.URI())
}

func (s *LocationTestSuite) TestString() {
	loc, err := s.fs.NewLocation("", "/path/to/folder/")
	s.Require().NoError(err)
	s.Equal("onedrive:///path/to/folder/", loc.String())
}

func (s *LocationTestSuite) TestFileSystem() {
	loc, err := s.fs.NewLocation("", "/path/to/folder/")
	s.Require().NoError(err)
	s.Equal(s.fs, loc.FileSystem())
}

func (s *LocationTestSuite) TestNewLocation() {
	loc, err := s.fs.NewLocation("", "/path/to/")
	s.Require().NoError(err)

	s.Run("valid relative path", func() {
		newLoc, err := loc.NewLocation("subfolder/")
		s.Require().NoError(err)
		s.Equal("/path/to/subfolder/", newLoc.Path())
	})

	s.Run("parent path", func() {
		newLoc, err := loc.NewLocation("../other/")
		s.Require().NoError(err)
		s.Equal("/path/other/", newLoc.Path())
	})

	s.Run("empty path", func() {
		_, err := loc.NewLocation("")
		s.Require().Error(err)
		s.Equal(errPathRequired, err)
	})

	s.Run("nil location", func() {
		var nilLoc *Location
		_, err := nilLoc.NewLocation("subfolder/")
		s.Require().Error(err)
		s.Equal(errLocationRequired, err)
	})
}

func (s *LocationTestSuite) TestNewFile() {
	loc, err := s.fs.NewLocation("", "/path/to/")
	s.Require().NoError(err)

	s.Run("valid file name", func() {
		file, err := loc.NewFile("file.txt")
		s.Require().NoError(err)
		s.Equal("file.txt", file.Name())
		s.Equal("/path/to/file.txt", file.Path())
	})

	s.Run("file in subdirectory", func() {
		file, err := loc.NewFile("subdir/file.txt")
		s.Require().NoError(err)
		s.Equal("file.txt", file.Name())
		s.Equal("/path/to/subdir/file.txt", file.Path())
	})

	s.Run("empty file name", func() {
		_, err := loc.NewFile("")
		s.Require().Error(err)
		s.Equal(errPathRequired, err)
	})

	s.Run("nil location", func() {
		var nilLoc *Location
		_, err := nilLoc.NewFile("file.txt")
		s.Require().Error(err)
		s.Equal(errLocationRequired, err)
	})
}

func (s *LocationTestSuite) TestChangeDir() {
	loc, err := s.fs.NewLocation("", "/path/to/")
	s.Require().NoError(err)

	odLoc := loc.(*Location)

	s.Run("valid relative path", func() {
		err := odLoc.ChangeDir("subfolder/")
		s.Require().NoError(err)
		s.Equal("/path/to/subfolder/", odLoc.Path())
	})

	s.Run("empty path", func() {
		err := odLoc.ChangeDir("")
		s.Require().Error(err)
		s.Equal(errPathRequired, err)
	})

	s.Run("nil location", func() {
		var nilLoc *Location
		err := nilLoc.ChangeDir("subfolder/")
		s.Require().Error(err)
		s.Equal(errLocationRequired, err)
	})
}

func (s *LocationTestSuite) TestList() {
	tests := []struct {
		name          string
		path          string
		clientPath    string
		items         []DriveItem
		err           error
		expected      []string
		expectedError string
	}{
		{name: "files only, folders excluded", path: "/dir/", clientPath: "dir", items: listing, expected: []string{"a.txt", "b.csv", "abc.txt"}},
		{name: "root", path: "/", clientPath: "", items: []DriveItem{{Name: "r.txt"}}, expected: []string{"r.txt"}},
		{name: "empty folder", path: "/dir/", clientPath: "dir", items: nil, expected: []string{}},
		{name: "missing folder lists empty", path: "/nope/", clientPath: "nope", err: &apiError{StatusCode: 404}, expected: []string{}},
		{name: "other error", path: "/dir/", clientPath: "dir", err: errors.New("boom"), expectedError: "listing /dir/: boom"},
	}

	for i := range tests {
		tt := &tests[i]
		s.Run(tt.name, func() {
			s.client.EXPECT().ListChildren(mock.Anything, tt.clientPath).Return(tt.items, tt.err).Once()

			files, err := s.location(tt.path).List()
			if tt.expectedError != "" {
				s.Require().ErrorContains(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, files)
		})
	}
}

func (s *LocationTestSuite) TestListByPrefix() {
	tests := []struct {
		name       string
		prefix     string
		clientPath string
		expected   []string
	}{
		{name: "simple prefix", prefix: "a", clientPath: "dir", expected: []string{"a.txt", "abc.txt"}},
		{name: "no match", prefix: "z", clientPath: "dir", expected: []string{}},
		{name: "relative path prefix", prefix: "sub/a", clientPath: "dir/sub", expected: []string{"a.txt", "abc.txt"}},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().ListChildren(mock.Anything, tt.clientPath).Return(listing, nil).Once()

			files, err := s.location("/dir/").ListByPrefix(tt.prefix)
			s.Require().NoError(err)
			s.Equal(tt.expected, files)
		})
	}

	s.Run("list error", func() {
		s.client.EXPECT().ListChildren(mock.Anything, "dir").Return(nil, errors.New("boom")).Once()

		_, err := s.location("/dir/").ListByPrefix("a")
		s.Require().ErrorContains(err, "boom")
	})
}

func (s *LocationTestSuite) TestListByRegex() {
	s.client.EXPECT().ListChildren(mock.Anything, "dir").Return(listing, nil).Once()

	files, err := s.location("/dir/").ListByRegex(regexp.MustCompile(`\.txt$`))
	s.Require().NoError(err)
	s.Equal([]string{"a.txt", "abc.txt"}, files)

	s.client.EXPECT().ListChildren(mock.Anything, "dir").Return(nil, errors.New("boom")).Once()
	_, err = s.location("/dir/").ListByRegex(regexp.MustCompile(`.`))
	s.Require().ErrorContains(err, "boom")
}

func (s *LocationTestSuite) TestExists() {
	tests := []struct {
		name          string
		item          *DriveItem
		err           error
		expected      bool
		expectedError string
	}{
		{name: "folder exists", item: &DriveItem{IsFolder: true}, expected: true},
		{name: "file at path is not a location", item: &DriveItem{}, expected: false},
		{name: "missing", err: &apiError{StatusCode: 404}, expected: false},
		{name: "other error", err: errors.New("boom"), expectedError: "boom"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().GetItem(mock.Anything, "a/b").Return(tt.item, tt.err).Once()

			exists, err := s.location("/a/b/").Exists()
			if tt.expectedError != "" {
				s.Require().ErrorContains(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, exists)
		})
	}

	s.Run("root always exists", func() {
		exists, err := s.location("/").Exists()
		s.Require().NoError(err)
		s.True(exists)
	})
}

func (s *LocationTestSuite) TestDeleteFile() {
	s.client.EXPECT().Delete(mock.Anything, "dir/x.txt").Return(nil)

	s.Require().NoError(s.location("/dir/").DeleteFile("x.txt"))
	s.Require().ErrorIs(s.location("/dir/").DeleteFile(""), errPathRequired)
}

func (s *LocationTestSuite) TestWithoutCredentials() {
	s.T().Setenv("VFS_ONEDRIVE_ACCESS_TOKEN", "")
	s.T().Setenv("VFS_ONEDRIVE_TENANT_ID", "")
	loc, err := NewFileSystem().NewLocation("", "/dir/")
	s.Require().NoError(err)

	_, err = loc.List()
	s.Require().ErrorIs(err, errCredentialsRequired)
	_, err = loc.Exists()
	s.Require().ErrorIs(err, errCredentialsRequired)
}

func (s *LocationTestSuite) TestAuthority() {
	s.Empty(s.location("/dir/").Authority().String())
}
