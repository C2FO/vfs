package s3

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/utils"
)

type fileSystemTestSuite struct {
	suite.Suite
}

var (
	s3fs *FileSystem
)

type mockClient struct {
	*s3.S3
}

func (ts *fileSystemTestSuite) SetupTest() {
	sess := session.Must(session.NewSession())
	client := mockClient{s3.New(sess)}
	s3fs = &FileSystem{client: client}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	newFS := NewFileSystem().WithClient(s3apiMock)
	ts.NotNil(newFS, "Should return a new fileSystem for s3")
}

func (ts *fileSystemTestSuite) TestNewFile() {
	filePath := "/path/to/file.txt"
	file, err := s3fs.NewFile("bucketName", filePath)
	ts.NoError(err, "No errors returned by NewFile(%s)", filePath)
	ts.NotNil(file, "fs.NewFile(%s) should assign all but first name component to key", filePath)
}

func (ts *fileSystemTestSuite) TestNewFile_Error() {
	// test nil pointer
	var nils3fs *FileSystem
	_, err := nils3fs.NewFile("", "/path/to/file.txt")
	ts.EqualError(err, "non-nil s3.FileSystem pointer is required", "errors returned by NewFile")

	// test validation error
	file, err := s3fs.NewFile("bucketName", "relative/path/to/file.txt")
	ts.EqualError(err, utils.ErrBadAbsFilePath, "errors returned by NewFile")
	ts.Nil(file, "NewFile shouldn't return a file")

	filePath := ""
	file, err = s3fs.NewFile("", filePath)
	ts.Error(err, "NewFile(%s)", filePath)
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)
}

func (ts *fileSystemTestSuite) TestNewLocation() {
	locPath := "/path/to/"
	loc, err := s3fs.NewLocation("bucketName", locPath)
	ts.NoError(err, "No errors returned by NewLocation(%s)", locPath)
	ts.NotNil(loc, "fs.NewLocation(%s) should assign all but first name component to key", locPath)
}

func (ts *fileSystemTestSuite) TestNewLocation_Error() {
	// test nil pointer
	var nils3fs *FileSystem
	_, err := nils3fs.NewLocation("", "/path/to/")
	ts.EqualError(err, "non-nil s3.FileSystem pointer is required", "errors returned by NewLocation")

	// test validation error
	file, err := s3fs.NewLocation("bucketName", "relative/path/to/")
	ts.EqualError(err, utils.ErrBadAbsLocationPath, "errors returned by NewLocation")
	ts.Nil(file, "NewFile shouldn't return a file")

	locPath := ""
	file, err = s3fs.NewLocation("", locPath)
	ts.EqualError(err, "non-empty strings for bucket and key are required", "NewLocation(%s)", locPath)
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
	ts.NoError(err, "no error")
	ts.Equal(s3fs.client, client, "client was already set")

	// bad options
	badOpt := "not an s3.Options"
	s3fs.client = nil
	s3fs.options = badOpt
	_, err = s3fs.Client()
	ts.EqualError(err, "unable to create client, vfs.Options must be an s3.Options", "client was already set")

	s3fs = &FileSystem{}
	client, err = s3fs.Client()
	ts.NoError(err, "no error")
	ts.NotNil(client, "client was set")
	ts.NotNil(s3fs.client, "client was set")

}

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
