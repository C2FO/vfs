package s3

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/mocks"
)

type fileSystemTestSuite struct {
	suite.Suite
}

var (
	s3fs *FileSystem
)

func (ts *fileSystemTestSuite) SetupTest() {
	var err error
	s3apiMock = &mocks.S3API{}
	s3fs, err = NewFileSystem(s3apiMock)
	if err != nil {
		ts.Fail("Shouldn't return an error creating NewFileSystem.")
	}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	newFS, err := NewFileSystem(s3apiMock)
	ts.Nil(err, "s3.NewFileSystem() shouldn't return an error")
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

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
