package azure

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
)

type LocationTestSuite struct {
	suite.Suite
}

func (s *LocationTestSuite) TestVFSLocationImplementor() {
	l := Location{}
	s.Implements((*vfs.Location)(nil), &l, "Does not implement the vfs.Location interface")
}

func (s *LocationTestSuite) TestString() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("test-container", "/")
	s.Equal("https://test-account.blob.core.windows.net/test-container/", l.String())

	err := l.ChangeDir("foo/bar/baz/")
	s.NoError(err, "Should change directories successfully")
	s.Equal("https://test-account.blob.core.windows.net/test-container/foo/bar/baz/", l.String())

	l, _ = fs.NewLocation("temp", "/foo/bar/baz/")
	s.Equal("https://test-account.blob.core.windows.net/temp/foo/bar/baz/", l.String())
}

func (s *LocationTestSuite) TestList() {
	client := MockAzureClient{ExpectedResult: []string{"file1.txt", "file2.txt"}}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	listing, err := l.List()
	s.NoError(err)
	s.Len(listing, 2)
}

func (s *LocationTestSuite) TestListByPrefix() {
	client := MockAzureClient{ExpectedResult: []string{"file1.txt", "file2.txt", "foo.txt"}}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	listing, err := l.ListByPrefix("file")
	s.NoError(err)
	s.Len(listing, 2)
	s.Equal("file1.txt", listing[0])
	s.Equal("file2.txt", listing[1])
}

func (s *LocationTestSuite) TestListByRegex() {
	client := MockAzureClient{ExpectedResult: []string{"file1.txt", "file2.txt", "foo.txt"}}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	regex := regexp.MustCompile("file")
	listing, err := l.ListByRegex(regex)
	s.NoError(err)
	s.Len(listing, 2, "expect the 2 files with the substring 'file' to be returned")
	s.Equal("file1.txt", listing[0])
	s.Equal("file2.txt", listing[1])
}

func (s *LocationTestSuite) TestVolume() {
	l := Location{container: "test-container"}
	s.Equal("test-container", l.Volume())

	l = Location{container: "another-container"}
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
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	exists, err := l.Exists()
	s.NoError(err)
	s.True(exists)
}

func (s *LocationTestSuite) TestExists_NonExistentFile() {
	client := MockAzureClient{PropertiesError: errors.New("no such file")}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	exists, err := l.Exists()
	s.NoError(err)
	s.False(exists)
}

func (s *LocationTestSuite) TestNewLocation() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	nl, err := l.NewLocation("")
	s.Error(err, "An empty relative path does not end with a slash and therefore is not a valid relative path so this should return an error")
	s.Nil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")

	nl, err = l.NewLocation("path/to/")
	s.NoError(err, "The new path was a valid relative path so there should be no error")
	s.NotNil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")
	s.NotNil(nl.(*Location).fileSystem, "file system must be non-nil")
	s.Equal("/some/folder/path/to/", nl.Path())

	nl, err = l.NewLocation("path/../to/")
	s.NoError(err, "The new path was a valid relative path so there should be no error")
	s.NotNil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")
	s.NotNil(nl.(*Location).fileSystem, "file system must be non-nil")
	s.Equal("/some/folder/to/", nl.Path())

	nl, err = l.NewLocation("/test-container/")
	s.Error(err, "The new path begins with a slash and therefore is not a valid relative path so this should return an error")
	s.Nil(nl, "There was an error creating the new location so a nil pointer should be returned")

	nl, err = l.NewLocation("test-container")
	s.Error(err, "The new path does not end with a slash and therefore is not a valid relative path so this should return an error")
	s.Nil(nl, "There was an error creating the new location so a nil pointer should be returned")
}

func (s *LocationTestSuite) TestNewLocation_NilReceiver() {
	var l *Location
	nl, err := l.NewLocation("test-container/")
	s.Error(err, "The receiver for NewLocation must be non-nil so we expect an error")
	s.Equal("azure.Location receiver pointer must be non-nil", err.Error())
	s.Nil(nl, "An error was returned so we expect a nil location to be returned")
}

func (s *LocationTestSuite) TestChangeDir() {
	l := Location{}
	err := l.ChangeDir("test-container/")
	s.NoError(err)
	s.Equal("/test-container/", l.Path())

	err = l.ChangeDir("path/../to/./new/dir/")
	s.NoError(err)
	s.Equal("/test-container/to/new/dir/", l.Path())

	l = Location{}
	err = l.ChangeDir("/test-container/")
	s.Error(err, "The path begins with a slash and therefore is not a relative path so this should return an error")
	s.Equal("relative location path is invalid - may not include leading slash but must include trailing slash", err.Error())

	l = Location{}
	err = l.ChangeDir("test-container")
	s.Error(err, "The path does not end with a slash and therefore is not a relative path so this should return an error")
	s.Equal("relative location path is invalid - may not include leading slash but must include trailing slash", err.Error())

	l = Location{}
	err = l.ChangeDir("")
	s.Error(err, "An empty relative path does not end with a slash and therefore is not a valid relative path so this should return an error")
	s.Equal("relative location path is invalid - may not include leading slash but must include trailing slash", err.Error())
}

func (s *LocationTestSuite) TestChangeDir_NilReceiver() {
	var l *Location
	s.Nil(l)
	err := l.ChangeDir("")
	s.Error(err)
	s.Equal("azure.Location receiver pointer must be non-nil", err.Error())
}

func (s *LocationTestSuite) TestFileSystem() {
	fs := NewFileSystem()
	l := Location{fileSystem: fs}
	s.NotNil(l.FileSystem())
}

func (s *LocationTestSuite) TestNewFile() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	l, _ := fs.NewLocation("test-container", "/folder/")

	f, err := l.NewFile("")
	s.Error(err, "Empty string is not a valid relative file path so we expect an error")
	s.Equal("relative file path is invalid - may not include leading or trailing slashes", err.Error())
	s.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("/foo/bar.txt")
	s.Error(err, "The file path begins with a slash therefore it is not a valid relative file path so we expect an error")
	s.Equal("relative file path is invalid - may not include leading or trailing slashes", err.Error())
	s.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("foo/bar/")
	s.Error(err, "The file path ends with a slash therefore it is not a valid relative file path so we expect an error")
	s.Equal("relative file path is invalid - may not include leading or trailing slashes", err.Error())
	s.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("foo/bar.txt")
	s.NoError(err, "The file path is valid so we expect no error to be returned")
	s.NotNil(f, "The call to NewFile did not return an error so we expect a non-nil pointer to a file struct")
	s.Equal("/folder/foo/bar.txt", f.Path())
	s.Equal("https://test-container.blob.core.windows.net/test-container/folder/foo/bar.txt", f.URI())
}

func (s *LocationTestSuite) TestNewFile_NilReceiver() {
	var l *Location
	f, err := l.NewFile("foo/bar.txt")
	s.Error(err, "Can't create a new file from a nil location so we expect an error")
	s.Equal("azure.Location receiver pointer must be non-nil", err.Error())
	s.Nil(f, "the call to NewFile returned an error so we expect a nil pointer")
}

func (s *LocationTestSuite) TestDeleteFile() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	s.NoError(l.DeleteFile("clever_file.txt"), "the file exists so we do not expect an error")
}

func (s *LocationTestSuite) TestDeleteFile_DoesNotExist() {
	client := MockAzureClient{ExpectedError: errors.New("no such file")}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	s.Error(l.DeleteFile("nosuchfile.txt"), "the file does not exist so we expect an error")
}

func (s *LocationTestSuite) TestURI() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("test-container", "/")
	s.Equal("https://test-account.blob.core.windows.net/test-container/", l.URI())

	err := l.ChangeDir("foo/bar/baz/")
	s.NoError(err, "Should change directories successfully")
	s.Equal("https://test-account.blob.core.windows.net/test-container/foo/bar/baz/", l.URI())

	vfsLoc, err := fs.NewLocation("temp", "/foo/bar/baz/")
	s.NoError(err, "Path is valid so we expect no errors")
	s.Equal("https://test-account.blob.core.windows.net/temp/foo/bar/baz/", vfsLoc.URI())
}

func (s *LocationTestSuite) TestContainerURL() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	s.Equal("https://test-account.blob.core.windows.net/test-container/", l.(*Location).ContainerURL())
}

func TestAzureLocation(t *testing.T) {
	suite.Run(t, new(LocationTestSuite))
}
