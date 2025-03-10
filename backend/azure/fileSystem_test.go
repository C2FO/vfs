package azure

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
)

type FileSystemTestSuite struct {
	suite.Suite
}

func (s *FileSystemTestSuite) TestVFSFileSystemImplementor() {
	fs := FileSystem{}
	s.Implements((*vfs.FileSystem)(nil), &fs, "Does not implement the vfs.FileSystem interface")
}

func (s *FileSystemTestSuite) TestNewFile() {
	fs := NewFileSystem()
	file, err := fs.NewFile("", "")
	s.EqualError(err, "non-empty strings for container and path are required", "volume and path are required")
	s.Nil(file)

	fs = NewFileSystem()
	file, err = fs.NewFile("temp", "")
	s.EqualError(err, "non-empty strings for container and path are required", "volume and path are required")
	s.Nil(file)

	fs = NewFileSystem()
	file, err = fs.NewFile("", "/blah/blah.txt")
	s.EqualError(err, "non-empty strings for container and path are required", "volume and path are required")
	s.Nil(file)

	fs = NewFileSystem()
	file, err = fs.NewFile("temp", "blah/blah.txt")
	s.EqualError(err, "absolute file path is invalid - must include leading slash and may not include trailing slash",
		"the path is invalid so we expect an error")
	s.Nil(file, "Since an error was returned we expect a nil file to be returned")

	fs = NewFileSystem()
	file, err = fs.NewFile("temp", "/foo/bar/test.txt")
	s.NoError(err, "The file path and volume are valid so we expect no errors")
	s.NotNil(file, "No error was returned so we expect to get a non-nil file struct")
	s.Equal("az://temp/foo/bar/test.txt", file.String())
}

func (s *FileSystemTestSuite) TestNewFile_NilReceiver() {
	var fs *FileSystem
	file, err := fs.NewFile("temp", "/foo/bar/test.txt")
	s.EqualError(err, "azure.FileSystem receiver pointer must be non-nil", "the receiver pointer is nil so we would receive an error")
	s.Nil(file, "Since there was an error we expect a nil file to be returned")
}

func (s *FileSystemTestSuite) TestNewLocation() {
	fs := NewFileSystem()
	loc, err := fs.NewLocation("", "")
	s.Error(err, "volume and path are required")
	s.Nil(loc, "volume and path are required")

	fs = NewFileSystem()
	loc, err = fs.NewLocation("", "/foo/bar/")
	s.Error(err, "volume and path are required")
	s.Nil(loc, "volume and path are required")

	fs = NewFileSystem()
	loc, err = fs.NewLocation("temp", "")
	s.Error(err, "volume and path are required")
	s.Nil(loc, "volume and path are required")

	fs = NewFileSystem()
	loc, err = fs.NewLocation("temp", "foo/bar/")
	s.EqualError(err, "absolute location path is invalid - must include leading and trailing slashes",
		"The path does not start with a slash and therefore not an absolute path so we expect an error")
	s.Nil(loc, "Since an error was returned the location is nil")

	fs = NewFileSystem()
	loc, err = fs.NewLocation("temp", "/foo/bar")
	s.EqualError(err, "absolute location path is invalid - must include leading and trailing slashes",
		"The path does not end with a slash and therefore not an absolute path so we expect an error")
	s.Nil(loc, "Since an error was returned the location is nil")

	fs = NewFileSystem()
	loc, err = fs.NewLocation("temp", "/foo/bar/")
	s.NoError(err, "the path is valid so we expect no error")
	s.NotNil(loc, "Since there was no error we expect a non-nil location")
	s.Equal("az://temp/foo/bar/", loc.String())

	fs = NewFileSystem()
	loc, err = fs.NewLocation("temp", "/path/../to/")
	s.NoError(err, "the path is valid so we expect no error")
	s.NotNil(loc, "Since there was no error we expect a non-nil location")
	s.Equal("az://temp/to/", loc.String())

	fs = NewFileSystem()
	loc, err = fs.NewLocation("temp", "/path/./to/")
	s.NoError(err, "the path is valid so we expect no error")
	s.NotNil(loc, "Since there was no error we expect a non-nil location")
	s.Equal("az://temp/path/to/", loc.String())
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
	s.Equal("az", fs.Scheme())
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

func TestAzureFileSystem(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}
