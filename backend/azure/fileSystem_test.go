package azure

import (
	"errors"
	"net/url"
	"testing"

	"github.com/c2fo/vfs/v5"
	"github.com/stretchr/testify/suite"
)

type FileSystemTestSuite struct {
	suite.Suite
}

func (suite *FileSystemTestSuite) TestVFSFileSystemImplementor() {
	fs := FileSystem{}
	suite.Implements((*vfs.FileSystem)(nil), &fs, "Does not implement the vfs.FileSystem interface")
}

func (suite *FileSystemTestSuite) TestNewFile() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err := fs.NewFile("", "")
	suite.Error(err, "volume and path are required")
	suite.Equal("non-empty strings for container and path are required", err.Error())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("temp", "")
	suite.Error(err, "volume and path are required")
	suite.Equal("non-empty strings for container and path are required", err.Error())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("", "/blah/blah.txt")
	suite.Error(err, "volume and path are required")
	suite.Equal("non-empty strings for container and path are required", err.Error())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("temp", "blah/blah.txt")
	suite.Error(err, "the path is invalid so we expect an error")
	suite.Equal("absolute file path is invalid - must include leading slash and may not include trailing slash", err.Error())
	suite.Nil(file, "Since an error was returned we expect a nil file to be returned")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("temp", "/foo/bar/test.txt")
	suite.NoError(err, "The file path and volume are valid so we expect no errors")
	suite.NotNil(file, "No error was returned so we expect to get a non-nil file struct")
	suite.Equal("https://test-container.blob.core.windows.net/temp/foo/bar/test.txt", file.String())
}

func (suite *FileSystemTestSuite) TestNewFile_NilReceiver() {
	var fs *FileSystem
	file, err := fs.NewFile("temp", "/foo/bar/test.txt")
	suite.Error(err, "the receiver pointer is nil so we whould receive an error")
	suite.Equal("azure.FileSystem receiver pointer must be non-nil", err.Error())
	suite.Nil(file, "Since there was an error we expect a nil file to be returned")
}

func (suite *FileSystemTestSuite) TestNewLocation() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err := fs.NewLocation("", "")
	suite.Error(err, "volume and path are required")
	suite.Nil(loc, "volume and path are required")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("", "/foo/bar/")
	suite.Error(err, "volume and path are required")
	suite.Nil(loc, "volume and path are required")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "")
	suite.Error(err, "volume and path are required")
	suite.Nil(loc, "volume and path are required")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "foo/bar/")
	suite.Error(err, "The path does not start with a slash and therefore not an absolute path so we expect an error")
	suite.Equal("absolute location path is invalid - must include leading and trailing slashes", err.Error())
	suite.Nil(loc, "Since an error was returned the location is nil")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/foo/bar")
	suite.Error(err, "The path does not end with a slash and therefore not an absolute path so we expect an error")
	suite.Equal("absolute location path is invalid - must include leading and trailing slashes", err.Error())
	suite.Nil(loc, "Since an error was returned the location is nil")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/foo/bar/")
	suite.NoError(err, "the path is valid so we expect no error")
	suite.NotNil(loc, "Since there was no error we expect a non-nil location")
	suite.Equal("https://test-container.blob.core.windows.net/temp/foo/bar/", loc.String())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/path/../to/")
	suite.NoError(err, "the path is valid so we expect no error")
	suite.NotNil(loc, "Since there was no error we expect a non-nil location")
	suite.Equal("https://test-container.blob.core.windows.net/temp/to/", loc.String())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/path/./to/")
	suite.NoError(err, "the path is valid so we expect no error")
	suite.NotNil(loc, "Since there was no error we expect a non-nil location")
	suite.Equal("https://test-container.blob.core.windows.net/temp/path/to/", loc.String())
}

func (suite *FileSystemTestSuite) TestNewLocation_NilReceiver() {
	var fs *FileSystem
	loc, err := fs.NewLocation("temp", "/foo/bar/")
	suite.Error(err, "The receiver pointer on the function call is nill so we should get an error")
	suite.Equal("azure.FileSystem receiver pointer must be non-nil", err.Error())
	suite.Nil(loc, "The call returned an error so the location should be nil")
}

func (suite *FileSystemTestSuite) TestName() {
	fs := FileSystem{}
	suite.Equal("azure", fs.Name())
}

func (suite *FileSystemTestSuite) TestScheme() {
	fs := FileSystem{}
	suite.Equal("https", fs.Scheme())
}

func (suite *FileSystemTestSuite) TestRetry() {
	var doNothing = func() error {
		return nil
	}

	var errorRetry = func(wrapped func() error) error {
		return errors.New("I always error")
	}

	fs := FileSystem{}
	retryFn := fs.Retry()
	suite.NotNil(retryFn)
	err := retryFn(doNothing)
	suite.NoError(err, "The default retry function just calls the passed func")

	fs = *NewFileSystem().WithOptions(Options{RetryFunc: errorRetry})
	retryFn = fs.Retry()
	err = retryFn(doNothing)
	suite.Error(err, "This implementation should use the retry function from the options which always errors")
	suite.Equal("I always error", err.Error())
}

func (suite *FileSystemTestSuite) TestNewFileSystem() {
	fs := NewFileSystem()
	suite.NotNil(fs, "Should return a non-nil pointer to the new file system")
}

func (suite *FileSystemTestSuite) TestWithOptions() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "foo-account"})
	suite.NotNil(fs, "Should return the modified FileSystem struct")
	suite.Equal("foo-account", fs.options.AccountName)

	fs = NewFileSystem().WithOptions("Not Azure Options...")
	suite.Equal("", fs.options.AccountName)
}

func (suite *FileSystemTestSuite) TestClient() {
	fs := NewFileSystem().WithClient(&MockAzureClient{})
	suite.NotNil(fs.Client())

	fs = NewFileSystem()
	suite.NotNil(fs.Client())
}

func (suite *FileSystemTestSuite) TestParsePath() {
	uri := "https://my-account.blob.core.windows.net/my_container/foo/bar/baz/"
	u, _ := url.Parse(uri)
	volume, path, err := ParsePath(u.Path)
	suite.NoError(err, "Path is valid so we should not get an error")
	suite.Equal("my_container", volume)
	suite.Equal("/foo/bar/baz/", path)

	uri = "https://my-account.blob.core.windows.net/my_container/"
	u, _ = url.Parse(uri)
	volume, path, err = ParsePath(u.Path)
	suite.NoError(err, "Path is valid so we should not get an error")
	suite.Equal("my_container", volume)
	suite.Equal("/", path)

	uri = "https://my-account.blob.core.windows.net/"
	u, _ = url.Parse(uri)
	volume, path, err = ParsePath(u.Path)
	suite.Error(err, "a container is required so we should get an error")
	suite.Equal("", volume, "we got an error so volume should be empty")
	suite.Equal("", path, "we got an error so path should be empty")
}

func TestAzureFileSystem(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}
