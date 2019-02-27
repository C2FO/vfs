package s3

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/suite"
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
	ts.Nil(err, "No errors returned by NewFile(%s)", filePath)
	ts.NotNil(file, "fs.NewFile(%s) should assign all but first name component to key", filePath)
}

func (ts *fileSystemTestSuite) TestNewFile_Error() {
	filePath := ""
	file, err := s3fs.NewFile("", filePath)
	ts.Error(err, "NewFile(%s)", filePath)
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)
}

func (ts *fileSystemTestSuite) TestName_Error() {
	ts.Equal(name, s3fs.Name(), "Name() is s3.name const")
}

func (ts *fileSystemTestSuite) TestWithOptions() {
	//ignore non-s3.Options
	fs := s3fs.WithOptions("just a string")
	ts.Equal(s3fs, fs, "no change for non-s3.Options")

	// with option
	fs = s3fs.WithOptions(Options{
		Region: "us-east-1",
	})
	ts.NotNil(fs.options, "fs.options is not nil")
}

func (ts *fileSystemTestSuite) TestClient() {
	//client already set
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
