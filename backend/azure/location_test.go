package azure

import (
	"errors"
	"regexp"
	"testing"

	"github.com/c2fo/vfs/v5"
	"github.com/stretchr/testify/suite"
)

type LocationTestSuite struct {
	suite.Suite
}

func (suite *LocationTestSuite) TestVFSLocationImplementor() {
	l := Location{}
	suite.Implements((*vfs.Location)(nil), &l, "Does not implement the vfs.Location interface")
}

func (suite *LocationTestSuite) TestString() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("test-container", "/")
	suite.Equal("https://test-account.blob.core.windows.net/test-container/", l.String())

	err := l.ChangeDir("foo/bar/baz/")
	suite.NoError(err, "Should change directories successfully")
	suite.Equal("https://test-account.blob.core.windows.net/test-container/foo/bar/baz/", l.String())

	l, _ = fs.NewLocation("temp", "/foo/bar/baz/")
	suite.Equal("https://test-account.blob.core.windows.net/temp/foo/bar/baz/", l.String())
}

func (suite *LocationTestSuite) TestList() {
	client := MockAzureClient{ExpectedResult: []string{"file1.txt", "file2.txt"}}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	listing, err := l.List()
	suite.NoError(err)
	suite.Len(listing, 2)
}

func (suite *LocationTestSuite) TestListByPrefix() {
	client := MockAzureClient{ExpectedResult: []string{"file1.txt", "file2.txt", "foo.txt"}}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	listing, err := l.ListByPrefix("file")
	suite.NoError(err)
	suite.Len(listing, 2)
	suite.Equal("file1.txt", listing[0])
	suite.Equal("file2.txt", listing[1])
}

func (suite *LocationTestSuite) TestListByRegex() {
	client := MockAzureClient{ExpectedResult: []string{"file1.txt", "file2.txt", "foo.txt"}}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	regex, _ := regexp.Compile("file")
	listing, err := l.ListByRegex(regex)
	suite.NoError(err)
	suite.Len(listing, 2, "expect the 2 files with the substring 'file' to be returned")
	suite.Equal("file1.txt", listing[0])
	suite.Equal("file2.txt", listing[1])
}

func (suite *LocationTestSuite) TestVolume() {
	l := Location{container: "test-container"}
	suite.Equal("test-container", l.Volume())

	l = Location{container: "another-container"}
	suite.Equal("another-container", l.Volume())
}

func (suite *LocationTestSuite) TestPath() {
	l := Location{path: "/foo/bar/"}
	suite.Equal("/foo/bar/", l.Path())

	l = Location{path: "/foo/bar/baz/"}
	suite.Equal("/foo/bar/baz/", l.Path())

	l = Location{path: "/foo"}
	suite.Equal("/foo/", l.Path())

	l = Location{path: "foo/"}
	suite.Equal("/foo/", l.Path())

	l = Location{path: "foo"}
	suite.Equal("/foo/", l.Path())

	l = Location{}
	suite.Equal("/", l.Path())
}

func (suite *LocationTestSuite) TestExists() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	exists, err := l.Exists()
	suite.NoError(err)
	suite.True(exists)
}

func (suite *LocationTestSuite) TestExists_NonExistentFile() {
	client := MockAzureClient{PropertiesError: errors.New("no such file")}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	exists, err := l.Exists()
	suite.NoError(err)
	suite.False(exists)
}

func (suite *LocationTestSuite) TestNewLocation() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	nl, err := l.NewLocation("")
	suite.Error(err, "An empty relative path does not end with a slash and therefore is not a valid relative path so this should return an error")
	suite.Nil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")

	nl, err = l.NewLocation("path/to/")
	suite.NoError(err, "The new path was a valid relative path so there should be no error")
	suite.NotNil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")
	suite.NotNil(nl.(*Location).fileSystem, "file system must be non-nil")
	suite.Equal("/some/folder/path/to/", nl.Path())

	nl, err = l.NewLocation("path/../to/")
	suite.NoError(err, "The new path was a valid relative path so there should be no error")
	suite.NotNil(nl, "There were no errors encountered when creating the new location so the returned interface type should t non-nil")
	suite.NotNil(nl.(*Location).fileSystem, "file system must be non-nil")
	suite.Equal("/some/folder/to/", nl.Path())

	nl, err = l.NewLocation("/test-container/")
	suite.Error(err, "The new path begins with a slash and therefore is not a valid relative path so this should return an error")
	suite.Nil(nl, "There was an error creating the new location so a nil pointer should be returned")

	nl, err = l.NewLocation("test-container")
	suite.Error(err, "The new path does not end with a slash and therefore is not a valid relative path so this should return an error")
	suite.Nil(nl, "There was an error creating the new location so a nil pointer should be returned")
}

func (suite *LocationTestSuite) TestNewLocation_NilReceiver() {
	var l *Location
	nl, err := l.NewLocation("test-container/")
	suite.Error(err, "The receiver for NewLocaiton must be non-nil so we expect an error")
	suite.Equal("azure.Location receiver pointer must be non-nil", err.Error())
	suite.Nil(nl, "An error was returned so we expect a nil location to be returned")
}

func (suite *LocationTestSuite) TestChangeDir() {
	l := Location{}
	err := l.ChangeDir("test-container/")
	suite.NoError(err)
	suite.Equal("/test-container/", l.Path())

	err = l.ChangeDir("path/../to/./new/dir/")
	suite.NoError(err)
	suite.Equal("/test-container/to/new/dir/", l.Path())

	l = Location{}
	err = l.ChangeDir("/test-container/")
	suite.Error(err, "The path begins with a slash and therefore is not a relative path so this should return an error")
	suite.Equal("relative location path is invalid - may not include leading slash but must include trailing slash", err.Error())

	l = Location{}
	err = l.ChangeDir("test-container")
	suite.Error(err, "The path does not end with a slash and therefore is not a relative path so this should return an error")
	suite.Equal("relative location path is invalid - may not include leading slash but must include trailing slash", err.Error())

	l = Location{}
	err = l.ChangeDir("")
	suite.Error(err, "An empty relative path does not end with a slash and therefore is not a valid relative path so this should return an error")
	suite.Equal("relative location path is invalid - may not include leading slash but must include trailing slash", err.Error())
}

func (suite *LocationTestSuite) TestChangeDir_NilReceiver() {
	var l *Location
	suite.Nil(l)
	err := l.ChangeDir("")
	suite.Error(err)
	suite.Equal("azure.Location receiver pointer must be non-nil", err.Error())
}

func (suite *LocationTestSuite) TestFileSystem() {
	fs := NewFileSystem()
	l := Location{fileSystem: fs}
	suite.NotNil(l.FileSystem())
}

func (suite *LocationTestSuite) TestNewFile() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	l, _ := fs.NewLocation("test-container", "/folder/")

	f, err := l.NewFile("")
	suite.Error(err, "Empty string is not a valid relative file path so we expect an error")
	suite.Equal("relative file path is invalid - may not include leading or trailing slashes", err.Error())
	suite.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("/foo/bar.txt")
	suite.Error(err, "The file path begins with a slash therefore it is not a valid relative file path so we expect an error")
	suite.Equal("relative file path is invalid - may not include leading or trailing slashes", err.Error())
	suite.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("foo/bar/")
	suite.Error(err, "The file path ends with a slash therefore it is not a valid relative file path so we expect an error")
	suite.Equal("relative file path is invalid - may not include leading or trailing slashes", err.Error())
	suite.Nil(f, "Since the call to NewFile resulted in an error we expect a nil pointer")

	f, err = l.NewFile("foo/bar.txt")
	suite.NoError(err, "The file path is valid so we expect no error to be returned")
	suite.NotNil(f, "The call to NewFile did not return an error so we expect a non-nil pointer to a file struct")
	suite.Equal("/folder/foo/bar.txt", f.Path())
	suite.Equal("https://test-container.blob.core.windows.net/test-container/folder/foo/bar.txt", f.URI())
}

func (suite *LocationTestSuite) TestNewFile_NilReceiver() {
	var l *Location
	f, err := l.NewFile("foo/bar.txt")
	suite.Error(err, "Can't create a new file from a nil location so we expect an error")
	suite.Equal("azure.Location receiver pointer must be non-nil", err.Error())
	suite.Nil(f, "the call to NewFile returned an error so we expect a nil pointer")
}

func (suite *LocationTestSuite) TestDeleteFile() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	suite.NoError(l.DeleteFile("clever_file.txt"), "the file exists so we do not expect an error")
}

func (suite *LocationTestSuite) TestDeleteFile_DoesNotExist() {
	client := MockAzureClient{ExpectedError: errors.New("no such file")}
	fs := NewFileSystem().WithClient(&client)
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	suite.Error(l.DeleteFile("nosuchfile.txt"), "the file does not exist so we expect an error")
}

func (suite *LocationTestSuite) TestURI() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("test-container", "/")
	suite.Equal("https://test-account.blob.core.windows.net/test-container/", l.URI())

	err := l.ChangeDir("foo/bar/baz/")
	suite.NoError(err, "Should change directories successfully")
	suite.Equal("https://test-account.blob.core.windows.net/test-container/foo/bar/baz/", l.URI())

	vfsLoc, err := fs.NewLocation("temp", "/foo/bar/baz/")
	suite.NoError(err, "Path is valid so we expect no errors")
	suite.Equal("https://test-account.blob.core.windows.net/temp/foo/bar/baz/", vfsLoc.URI())
}

func (suite *LocationTestSuite) TestContainerURL() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("test-container", "/some/folder/")
	suite.Equal("https://test-account.blob.core.windows.net/test-container/", l.(*Location).ContainerURL())
}

func TestAzureLocation(t *testing.T) {
	suite.Run(t, new(LocationTestSuite))
}
