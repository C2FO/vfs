package azure

import (
	"github.com/c2fo/vfs/v5"
	"github.com/stretchr/testify/suite"
	"testing"
)

type FileSystemTestSuite struct {
	suite.Suite
}

func (suite *FileSystemTestSuite) TestVFSFileSystemImplementor() {
	fs := FileSystem{}
	suite.Implements((*vfs.FileSystem)(nil), &fs, "Does not implement the vfs.FileSystem interface")
}

func (suite *FileSystemTestSuite) TestNewFile() {
	fs := FileSystem{}
	file, err := fs.NewFile("", "")
	suite.NoError(err)
	suite.NotNil(file)
}

func (suite *FileSystemTestSuite) TestNewLocation() {
	fs := FileSystem{}
	loc, err := fs.NewLocation("", "")
	suite.NoError(err)
	suite.NotNil(loc)
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
	fs := FileSystem{}
	suite.NotNil(fs.Retry())
}

func TestAzureFileSystem(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}
