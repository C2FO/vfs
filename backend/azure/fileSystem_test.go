package azure

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
)

type FileSystemTestSuite struct {
	suite.Suite
}

func (s *FileSystemTestSuite) TestVFSFileSystemImplementor() {
	fs := FileSystem{}
	s.Implements((*vfs.FileSystem)(nil), &fs, "Does not implement the vfs.FileSystem interface")
}

func (s *FileSystemTestSuite) TestNewFile() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err := fs.NewFile("", "")
	s.EqualError(err, "non-empty strings for container and path are required", "volume and path are required")
	s.Nil(file)

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("temp", "")
	s.EqualError(err, "non-empty strings for container and path are required", "volume and path are required")
	s.Nil(file)

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("", "/blah/blah.txt")
	s.EqualError(err, "non-empty strings for container and path are required", "volume and path are required")
	s.Nil(file)

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("temp", "blah/blah.txt")
	s.EqualError(err, "absolute file path is invalid - must include leading slash and may not include trailing slash",
		"the path is invalid so we expect an error")
	s.Nil(file, "Since an error was returned we expect a nil file to be returned")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	file, err = fs.NewFile("temp", "/foo/bar/test.txt")
	s.NoError(err, "The file path and volume are valid so we expect no errors")
	s.NotNil(file, "No error was returned so we expect to get a non-nil file struct")
	s.Equal("https://test-container.blob.core.windows.net/temp/foo/bar/test.txt", file.String())
}

func (s *FileSystemTestSuite) TestNewFile_NilReceiver() {
	var fs *FileSystem
	file, err := fs.NewFile("temp", "/foo/bar/test.txt")
	s.EqualError(err, "azure.FileSystem receiver pointer must be non-nil", "the receiver pointer is nil so we would receive an error")
	s.Nil(file, "Since there was an error we expect a nil file to be returned")
}

func (s *FileSystemTestSuite) TestNewLocation() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err := fs.NewLocation("", "")
	s.Error(err, "volume and path are required")
	s.Nil(loc, "volume and path are required")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("", "/foo/bar/")
	s.Error(err, "volume and path are required")
	s.Nil(loc, "volume and path are required")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "")
	s.Error(err, "volume and path are required")
	s.Nil(loc, "volume and path are required")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "foo/bar/")
	s.EqualError(err, "absolute location path is invalid - must include leading and trailing slashes",
		"The path does not start with a slash and therefore not an absolute path so we expect an error")
	s.Nil(loc, "Since an error was returned the location is nil")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/foo/bar")
	s.EqualError(err, "absolute location path is invalid - must include leading and trailing slashes",
		"The path does not end with a slash and therefore not an absolute path so we expect an error")
	s.Nil(loc, "Since an error was returned the location is nil")

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/foo/bar/")
	s.NoError(err, "the path is valid so we expect no error")
	s.NotNil(loc, "Since there was no error we expect a non-nil location")
	s.Equal("https://test-container.blob.core.windows.net/temp/foo/bar/", loc.String())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/path/../to/")
	s.NoError(err, "the path is valid so we expect no error")
	s.NotNil(loc, "Since there was no error we expect a non-nil location")
	s.Equal("https://test-container.blob.core.windows.net/temp/to/", loc.String())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	loc, err = fs.NewLocation("temp", "/path/./to/")
	s.NoError(err, "the path is valid so we expect no error")
	s.NotNil(loc, "Since there was no error we expect a non-nil location")
	s.Equal("https://test-container.blob.core.windows.net/temp/path/to/", loc.String())
}

func (s *FileSystemTestSuite) TestNewLocation_NilReceiver() {
	var fs *FileSystem
	loc, err := fs.NewLocation("temp", "/foo/bar/")
	s.EqualError(err, "azure.FileSystem receiver pointer must be non-nil",
		"The receiver pointer on the function call is nill so we should get an error")
	s.Nil(loc, "The call returned an error so the location should be nil")
}

func (s *FileSystemTestSuite) TestName() {
	fs := FileSystem{}
	s.Equal("azure", fs.Name())
}

func (s *FileSystemTestSuite) TestScheme() {
	fs := FileSystem{}
	s.Equal("https", fs.Scheme())
}

func (s *FileSystemTestSuite) TestRetry() {
	var doNothing = func() error {
		return nil
	}

	var errorRetry = func(wrapped func() error) error {
		return errors.New("i always error")
	}

	fs := NewFileSystem()
	retryFn := fs.Retry()
	s.NotNil(retryFn)
	err := retryFn(doNothing)
	s.NoError(err, "The default retry function just calls the passed func")

	fs = NewFileSystem().WithOptions(Options{RetryFunc: errorRetry})
	retryFn = fs.Retry()
	err = retryFn(doNothing)
	s.EqualError(err, "i always error", "This implementation should use the retry function from the options which always errors")
}

func (s *FileSystemTestSuite) TestNewFileSystem() {
	fs := NewFileSystem()
	s.NotNil(fs, "Should return a non-nil pointer to the new file system")
}

func (s *FileSystemTestSuite) TestWithOptions() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "foo-account"})
	s.NotNil(fs, "Should return the modified FileSystem struct")
	s.Equal("foo-account", fs.options.AccountName)

	fs = NewFileSystem().WithOptions("Not Azure Options...")
	s.Empty(fs.options.AccountName)
}

func (s *FileSystemTestSuite) TestClient() {
	fs := NewFileSystem().WithClient(&MockAzureClient{})
	s.NotNil(fs.Client())

	fs = NewFileSystem()
	s.NotNil(fs.Client())
}

func (s *FileSystemTestSuite) TestParsePath() {
	uri := "https://my-account.blob.core.windows.net/my_container/foo/bar/baz/"
	u, _ := url.Parse(uri)
	volume, path, err := ParsePath(u.Path)
	s.NoError(err, "Path is valid so we should not get an error")
	s.Equal("my_container", volume)
	s.Equal("/foo/bar/baz/", path)

	uri = "https://my-account.blob.core.windows.net/my_container/"
	u, _ = url.Parse(uri)
	volume, path, err = ParsePath(u.Path)
	s.NoError(err, "Path is valid so we should not get an error")
	s.Equal("my_container", volume)
	s.Equal("/", path)

	uri = "https://my-account.blob.core.windows.net/"
	u, _ = url.Parse(uri)
	volume, path, err = ParsePath(u.Path)
	s.Error(err, "a container is required so we should get an error")
	s.Empty(volume, "we got an error so volume should be empty")
	s.Empty(path, "we got an error so path should be empty")

	uri = "https://my-account.blob.core.windows.net/my_container/foo/bar/baz.txt"
	u, _ = url.Parse(uri)
	volume, path, err = ParsePath(u.Path)
	s.NoError(err, "File Path is valid so we should not get an error")
	s.Equal("my_container", volume)
	s.Equal("/foo/bar/baz.txt", path)
}

func (s *FileSystemTestSuite) TestIsValidURI() {
	uri := "https://my-account.blob.core.windows.net/my_container/foo/bar/baz/"
	u, _ := url.Parse(uri)
	s.True(IsValidURI(u), "the uri should be recognized as an Azure uri")

	uri = "foo://my-account.blob.core.windows.net/my_container/foo/bar/baz/"
	u, _ = url.Parse(uri)
	s.False(IsValidURI(u), "the uri has an invalid scheme so it is not an Azure uri")

	uri = "https://yadda.yadda.yadda/my_container/foo/bar/baz/"
	u, _ = url.Parse(uri)
	s.False(IsValidURI(u), "the host does not match .*.blob.core.windows.net so it is not an Azure uri")

	uri = "foo://yadda.yadda.yadda/my_container/foo/bar/baz/"
	u, _ = url.Parse(uri)
	s.False(IsValidURI(u), "nothing ab out this uri is right...")
}

func TestAzureFileSystem(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}
