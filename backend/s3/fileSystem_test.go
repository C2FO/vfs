package s3

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/backend/s3/mocks"
	"github.com/c2fo/vfs/v7/utils"
)

type fileSystemTestSuite struct {
	suite.Suite
}

var s3fs *FileSystem

func (ts *fileSystemTestSuite) SetupTest() {
	s3fs = &FileSystem{client: mocks.NewClient(ts.T())}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	// test with options
	newFS := NewFileSystem(WithOptions(Options{Region: "us-east-1"}))
	ts.NotNil(newFS, "Should return a new fileSystem for s3")
	ts.Equal("us-east-1", newFS.options.Region, "Should set region to us-east-1")

	// test with client
	// A fresh mock is used here rather than the package-level s3cliMock var (owned by
	// fileTestSuite in file_test.go): that var is only populated by that suite's SetupTest,
	// and test file execution order isn't guaranteed to run it first.
	mockClient := mocks.NewClient(ts.T())
	newFS = NewFileSystem(WithClient(mockClient))
	ts.NotNil(newFS, "Should return a new fileSystem for s3")
	ts.Equal(mockClient, newFS.client, "Should set client to the mock client")
}

func (ts *fileSystemTestSuite) TestNewFile() {
	filePath := "/path/to/file.txt"
	file, err := s3fs.NewFile("bucketName", filePath)
	ts.Require().NoError(err, "No errors returned by NewFile(%s)", filePath)
	ts.NotNil(file, "fs.NewFile(%s) should assign all but first name component to key", filePath)
}

func (ts *fileSystemTestSuite) TestNewFile_Error() {
	// test nil pointer
	var nils3fs *FileSystem
	_, err := nils3fs.NewFile("", "/path/to/file.txt")
	ts.Require().ErrorIs(err, errFileSystemRequired, "errors returned by NewFile")

	// test validation error
	file, err := s3fs.NewFile("bucketName", "relative/path/to/file.txt")
	ts.Require().ErrorIs(err, utils.ErrBadAbsFilePath, "errors returned by NewFile")
	ts.Nil(file, "NewFile shouldn't return a file")

	filePath := ""
	file, err = s3fs.NewFile("", filePath)
	ts.Require().Error(err, "NewFile(%s)", filePath)
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)
}

func (ts *fileSystemTestSuite) TestNewLocation() {
	locPath := "/path/to/"
	loc, err := s3fs.NewLocation("bucketName", locPath)
	ts.Require().NoError(err, "No errors returned by NewLocation(%s)", locPath)
	ts.NotNil(loc, "fs.NewLocation(%s) should assign all but first name component to key", locPath)
}

func (ts *fileSystemTestSuite) TestNewLocation_Error() {
	// test nil pointer
	var nils3fs *FileSystem
	_, err := nils3fs.NewLocation("", "/path/to/")
	ts.Require().ErrorIs(err, errFileSystemRequired, "errors returned by NewLocation")

	// test validation error
	file, err := s3fs.NewLocation("bucketName", "relative/path/to/")
	ts.Require().ErrorIs(err, utils.ErrBadAbsLocationPath, "errors returned by NewLocation")
	ts.Nil(file, "NewFile shouldn't return a file")

	locPath := ""
	file, err = s3fs.NewLocation("", locPath)
	ts.Require().ErrorIs(err, errAuthorityAndNameRequired, "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)
}

func (ts *fileSystemTestSuite) TestName_Error() {
	ts.Equal(name, s3fs.Name(), "Name() is s3.name const")
}

func (ts *fileSystemTestSuite) TestWithOptions() {
	// ignore non-s3.Options
	fs := s3fs.WithOptions("just a string")
	ts.Equal(s3fs, fs, "no change for non-s3.Options")

	// with option
	fs = s3fs.WithOptions(Options{
		Region: "us-east-1",
	})
	ts.NotNil(fs.options, "fs.options is not nil")
}

func (ts *fileSystemTestSuite) TestClient() {
	// client already set
	client, err := s3fs.Client()
	ts.Require().NoError(err, "no error")
	ts.Equal(s3fs.client, client, "client was already set")

	s3fs = &FileSystem{}
	client, err = s3fs.Client()
	ts.Require().NoError(err, "no error")
	ts.NotNil(client, "client was set")
	ts.NotNil(s3fs.client, "client was set")
}

func (ts *fileSystemTestSuite) TestWithClientOption_nilClient() {
	fs := NewFileSystem(WithClient(nil))

	client, err := fs.Client()
	ts.Require().ErrorIs(err, errClientNotSupported, "a nil client is reported, not replaced by a default")
	ts.Nil(client)
}

func (ts *fileSystemTestSuite) TestWithClientOption_typedNilClient() {
	var typedNil *s3.Client // satisfies Client at compile time, but the pointer itself is nil

	fs := NewFileSystem(WithClient(typedNil))

	client, err := fs.Client()
	ts.Require().ErrorIs(err, errClientNotSupported, "a typed-nil client is reported, not treated as a valid client")
	ts.Nil(client)
}

func (ts *fileSystemTestSuite) TestWithClient_unsupportedClient() {
	fs := (&FileSystem{}).WithClient("just a string")

	client, err := fs.Client()
	ts.Require().ErrorIs(err, errClientNotSupported, "unsupported client is reported, not ignored")
	ts.Contains(err.Error(), "string", "error names the offending type")
	ts.Nil(client, "no client is returned")

	// a subsequent valid client clears the deferred error
	supported := mocks.NewClient(ts.T())
	client, err = fs.WithClient(supported).Client()
	ts.Require().NoError(err)
	ts.Equal(supported, client)
}

func (ts *fileSystemTestSuite) TestWithClient_typedNilClient() {
	var typedNil *s3.Client // the type assertion to Client succeeds; only the pointer is nil

	fs := (&FileSystem{}).WithClient(typedNil)

	client, err := fs.Client()
	ts.Require().ErrorIs(err, errClientNotSupported, "a typed-nil client is reported, not treated as a valid client")
	ts.Nil(client, "no client is returned")
}

func (ts *fileSystemTestSuite) TestWithOptions_clearsDeferredClientError() {
	ts.Run("chainable method", func() {
		fs := (&FileSystem{}).WithClient("just a string")
		_, err := fs.Client()
		ts.Require().Error(err, "sanity check: error is latched before WithOptions")

		fs = fs.WithOptions(Options{Region: "us-east-1"})
		_, err = fs.Client()
		ts.Require().NoError(err, "supplying Options should clear a previously deferred client error")
	})

	ts.Run("functional option", func() {
		fs := NewFileSystem(WithClient(nil))
		_, err := fs.Client()
		ts.Require().Error(err, "sanity check: error is latched before WithOptions")

		fs = NewFileSystem(WithClient(nil), WithOptions(Options{Region: "us-east-1"}))
		_, err = fs.Client()
		ts.Require().NoError(err, "supplying Options should clear a previously deferred client error")
	})
}

func (ts *fileSystemTestSuite) TestBackendClient() {
	ts.Run("client supporting ListObjectsV2 is used directly", func() {
		client := newSDKStyleClient(ts.T())
		fs := &FileSystem{client: client}

		backend, err := fs.backendClient()
		ts.Require().NoError(err)
		ts.Same(client, backend)
	})

	ts.Run("client without ListObjectsV2 is adapted", func() {
		client := mocks.NewClient(ts.T())
		fs := &FileSystem{client: client}

		backend, err := fs.backendClient()
		ts.Require().NoError(err)
		ts.IsType(&legacyClient{}, backend)

		// the public accessor still hands back the client as supplied
		public, err := fs.Client()
		ts.Require().NoError(err)
		ts.Same(client, public)
	})

	ts.Run("deferred client error is reported", func() {
		fs := (&FileSystem{}).WithClient(42)

		backend, err := fs.backendClient()
		ts.Require().ErrorIs(err, errClientNotSupported)
		ts.Nil(backend)
	})
}

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
