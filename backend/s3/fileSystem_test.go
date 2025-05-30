package s3

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
)

type fileSystemTestSuite struct {
	suite.Suite
}

var (
	s3fs *FileSystem
)

type mockClient struct {
	*s3.Client
}

func (ts *fileSystemTestSuite) SetupTest() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}
	client := mockClient{s3.NewFromConfig(cfg)}
	s3fs = &FileSystem{client: client}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	// test with options
	newFS := NewFileSystem(WithOptions(Options{Region: "us-east-1"}))
	ts.NotNil(newFS, "Should return a new fileSystem for s3")
	ts.Equal("us-east-1", newFS.options.Region, "Should set region to us-east-1")

	// test with client
	newFS = NewFileSystem(WithClient(s3cliMock))
	ts.NotNil(newFS, "Should return a new fileSystem for s3")
	ts.Equal(s3cliMock, newFS.client, "Should set client to s3cliMock")
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

	s3fs = &FileSystem{}
	client, err = s3fs.Client()
	ts.NoError(err, "no error")
	ts.NotNil(client, "client was set")
	ts.NotNil(s3fs.client, "client was set")
}

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
