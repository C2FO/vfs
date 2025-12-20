package azure

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/azure/mocks"
	"github.com/c2fo/vfs/v7/utils"
)

type LocationTestSuite struct {
	suite.Suite
}

func (s *LocationTestSuite) TestVFSLocationImplementor() {
	l := Location{}
	s.Implements((*vfs.Location)(nil), &l, "Does not implement the vfs.Location interface")
}

func (s *LocationTestSuite) TestString() {
	fs := NewFileSystem()
	l, _ := fs.NewLocation("test-container", "/")
	s.Equal("az://test-container/", l.String())

	l, err := l.NewLocation("foo/bar/baz/")
	s.Require().NoError(err, "Should change directories successfully")
	s.Equal("az://test-container/foo/bar/baz/", l.String())

	l, _ = fs.NewLocation("temp", "/foo/bar/baz/")
	s.Equal("az://temp/foo/bar/baz/", l.String())
}

func (s *LocationTestSuite) TestList() {
	client := mocks.NewClient(s.T())
	client.EXPECT().List(mock.Anything).Return([]string{"file1.txt", "file2.txt"}, nil)
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	listing, err := l.List()
	s.Require().NoError(err)
	s.Len(listing, 2)
}

func (s *LocationTestSuite) TestListByPrefix() {
	client := mocks.NewClient(s.T())
	client.EXPECT().List(mock.Anything).Return([]string{"file1.txt", "file2.txt", "foo.txt"}, nil)
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	listing, err := l.ListByPrefix("file")
	s.Require().NoError(err)
	s.Len(listing, 2)
	s.Equal("file1.txt", listing[0])
	s.Equal("file2.txt", listing[1])
}

func (s *LocationTestSuite) TestListByRegex() {
	client := mocks.NewClient(s.T())
	client.EXPECT().List(mock.Anything).Return([]string{"file1.txt", "file2.txt", "foo.txt"}, nil)
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	regex := regexp.MustCompile("file")
	listing, err := l.ListByRegex(regex)
	s.Require().NoError(err)
	s.Len(listing, 2, "expect the 2 files with the substring 'file' to be returned")
	s.Equal("file1.txt", listing[0])
	s.Equal("file2.txt", listing[1])
}

//nolint:staticcheck // deprecated method test
func (s *LocationTestSuite) TestVolume() {
	fs := NewFileSystem()
	l, err := fs.NewLocation("test-container", "/")
	s.Require().NoError(err)
	s.Equal("test-container", l.Volume())

	l, err = fs.NewLocation("another-container", "/")
	s.Require().NoError(err)
	s.Equal("another-container", l.Volume())
}

func (s *LocationTestSuite) TestPath() {
	l := Location{path: "/foo/bar/"}
	s.Equal("/foo/bar/", l.Path())

	l = Location{path: "/foo/bar/baz/"}
	s.Equal("/foo/bar/baz/", l.Path())

	l = Location{path: "/foo"}
	s.Equal("/foo/", l.Path())

	l = Location{path: "foo/"}
	s.Equal("/foo/", l.Path())

	l = Location{path: "foo"}
	s.Equal("/foo/", l.Path())

	l = Location{}
	s.Equal("/", l.Path())
}

func (s *LocationTestSuite) TestExists() {
	client := mocks.NewClient(s.T())
	client.EXPECT().Properties("test-container", "").Return(&BlobProperties{}, nil)
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	exists, err := l.Exists()
	s.Require().NoError(err)
	s.True(exists)
}

func (s *LocationTestSuite) TestExists_NonExistentFile() {
	client := mocks.NewClient(s.T())
	client.EXPECT().Properties("test-container", "").Return(nil, errors.New("no such file"))
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	exists, err := l.Exists()
	s.Require().NoError(err)
	s.False(exists)
}

func (s *LocationTestSuite) TestNewLocation() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	nl, err := l.NewLocation("")
	s.Require().Error(err,
		"An empty relative path does not end with a slash and therefore is not a valid relative path so this should return an error")
	s.Nil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")

	nl, err = l.NewLocation("path/to/")
	s.Require().NoError(err, "The new path was a valid relative path so there should be no error")
	s.NotNil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")
	s.NotNil(nl.(*Location).fileSystem, "file system must be non-nil")
	s.Equal("/some/folder/path/to/", nl.Path())

	nl, err = l.NewLocation("path/../to/")
	s.Require().NoError(err, "The new path was a valid relative path so there should be no error")
	s.NotNil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")
	s.NotNil(nl.(*Location).fileSystem, "file system must be non-nil")
	s.Equal("/some/folder/to/", nl.Path())

	nl, err = l.NewLocation("/test-container/")
	s.Require().Error(err, "The new path begins with a slash and therefore is not a valid relative path so this should return an error")
	s.Nil(nl, "There was an error creating the new location so a nil pointer should be returned")

	nl, err = l.NewLocation("test-container")
	s.Require().Error(err, "The new path does not end with a slash and therefore is not a valid relative path so this should return an error")
	s.Nil(nl, "There was an error creating the new location so a nil pointer should be returned")
}

func (s *LocationTestSuite) TestNewLocation_NilReceiver() {
	var l *Location
	nl, err := l.NewLocation("test-container/")
	s.Require().ErrorIs(err, errLocationRequired, "The receiver for NewLocation must be non-nil so we expect an error")
	s.Nil(nl, "An error was returned so we expect a nil location to be returned")
}

//nolint:staticcheck // deprecated method test
func (s *LocationTestSuite) TestChangeDir() {
	l, err := NewFileSystem().NewLocation("test-container", "/")
	s.Require().NoError(err)

	err = l.ChangeDir("some-dir/")
	s.Require().NoError(err)
	s.Equal("/some-dir/", l.Path())

	err = l.ChangeDir("path/../to/./new/dir/")
	s.Require().NoError(err)
	s.Equal("/some-dir/to/new/dir/", l.Path())

	l, err = NewFileSystem().NewLocation("test-container", "/")
	s.Require().NoError(err)

	err = l.ChangeDir("/test-dir/")
	s.Require().ErrorIs(err, utils.ErrBadRelLocationPath,
		"The path begins with a slash and therefore is not a relative path so this should return an error")

	l, err = NewFileSystem().NewLocation("test-container", "/")
	s.Require().NoError(err)

	err = l.ChangeDir("test-dir")
	s.Require().ErrorIs(err, utils.ErrBadRelLocationPath,
		"The path does not end with a slash and therefore is not a relative path so this should return an error")

	l, err = NewFileSystem().NewLocation("test-container", "/")
	s.Require().NoError(err)

	err = l.ChangeDir("")
	s.Require().ErrorIs(err, utils.ErrBadRelLocationPath,
		"An empty relative path does not end with a slash and therefore is not a valid relative path so this should return an error")
}

func (s *LocationTestSuite) TestChangeDir_NilReceiver() {
	var l *Location
	s.Nil(l)
	err := l.ChangeDir("")
	s.Require().ErrorIs(err, errLocationRequired)
}

func (s *LocationTestSuite) TestFileSystem() {
	fs := NewFileSystem()
	l := Location{fileSystem: fs}
	s.NotNil(l.FileSystem())
}

func (s *LocationTestSuite) TestNewFile() {
	fs := NewFileSystem()
	l, _ := fs.NewLocation("test-container", "/folder/")

	f, err := l.NewFile("")
	s.Require().ErrorIs(err, utils.ErrBadRelFilePath,
		"Empty string is not a valid relative file path so we expect an error")
	s.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("/foo/bar.txt")
	s.Require().ErrorIs(err, utils.ErrBadRelFilePath,
		"The file path begins with a slash therefore it is not a valid relative file path so we expect an error")
	s.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("foo/bar/")
	s.Require().ErrorIs(err, utils.ErrBadRelFilePath,
		"The file path ends with a slash therefore it is not a valid relative file path so we expect an error")
	s.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("foo/bar.txt")
	s.Require().NoError(err, "The file path is valid so we expect no error to be returned")
	s.NotNil(f, "The call to NewFile did not return an error so we expect a non-nil pointer to a file struct")
	s.Equal("/folder/foo/bar.txt", f.Path())
	s.Equal("az://test-container/folder/foo/bar.txt", f.URI())

	// new tests for location update
	s.Run("new file with relative path updates location", func() {
		newFile, err := l.NewFile("../newfile.txt")
		s.Require().NoError(err)
		s.Equal("/newfile.txt", newFile.Path(), "NewFile with relative path should update location correctly")
		s.Equal("/", newFile.Location().Path(), "NewFile with relative path should update location correctly")
	})

	s.Run("new file with relative path to root", func() {
		newFile, err := l.NewFile("../../../../newrootfile.txt")
		s.Require().NoError(err)
		s.Equal("/newrootfile.txt", newFile.Path(), "NewFile with relative path to root should update location correctly")
		s.Equal("/", newFile.Location().Path(), "NewFile with relative path to root should update location correctly")
	})
}

func (s *LocationTestSuite) TestNewFile_NilReceiver() {
	var l *Location
	f, err := l.NewFile("foo/bar.txt")
	s.Require().ErrorIs(err, errLocationRequired, "Can't create a new file from a nil location so we expect an error")
	s.Nil(f, "the call to NewFile returned an error so we expect a nil pointer")
}

func (s *LocationTestSuite) TestDeleteFile() {
	client := mocks.NewClient(s.T())
	client.EXPECT().Delete(mock.Anything).Return(nil)
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	s.Require().NoError(l.DeleteFile("clever_file.txt"), "the file exists so we do not expect an error")
}

func (s *LocationTestSuite) TestDeleteFile_DoesNotExist() {
	client := mocks.NewClient(s.T())
	client.EXPECT().Delete(mock.Anything).Return(errors.New("no such file"))
	fs := NewFileSystem(WithClient(client))
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	s.Require().Error(l.DeleteFile("nosuchfile.txt"), "the file does not exist so we expect an error")
}

func (s *LocationTestSuite) TestURI() {
	fs := NewFileSystem()
	l, _ := fs.NewLocation("test-container", "/")
	s.Equal("az://test-container/", l.URI())

	l, err := l.NewLocation("foo/bar/baz/")
	s.Require().NoError(err, "Should change directories successfully")
	s.Equal("az://test-container/foo/bar/baz/", l.URI())

	vfsLoc, err := fs.NewLocation("temp", "/foo/bar/baz/")
	s.Require().NoError(err, "Path is valid so we expect no errors")
	s.Equal("az://temp/foo/bar/baz/", vfsLoc.URI())
}

func TestAzureLocation(t *testing.T) {
	suite.Run(t, new(LocationTestSuite))
}
