package azure

import (
	"github.com/c2fo/vfs/v5"
	"github.com/stretchr/testify/suite"
	"testing"
)

type FileTestSuite struct {
	suite.Suite
}

func (suite *FileTestSuite) TestVFSFileImplementor() {
	f := File{}
	suite.Implements((*vfs.File)(nil), &f, "Does not implement the vfs.File interface")
}

func (suite *FileTestSuite) TestClose() {
	f := File{}
	suite.NoError(f.Close())
}

func (suite *FileTestSuite) TestRead() {
	f := File{}
	n, err := f.Read([]byte{})
	suite.NoError(err)
	suite.Equal(0, n)
}

func (suite *FileTestSuite) TestSeek() {
	f := File{}
	newOffset, err := f.Seek(0, 0)
	suite.NoError(err)
	suite.Equal(int64(0), newOffset)
}

func (suite *FileTestSuite) TestWrite() {
	f := File{}
	n, err := f.Write([]byte{})
	suite.NoError(err)
	suite.Equal(0, n)
}

func (suite *FileTestSuite) TestString() {
	f := File{}
	suite.Equal("", f.String())
}

func (suite *FileTestSuite) TestExists() {
	f := File{}
	exists, err := f.Exists()
	suite.NoError(err)
	suite.False(exists)
}

func (suite *FileTestSuite) TestLocation() {
	f := File{}
	suite.NotNil(f.Location())
}

func (suite *FileTestSuite) TestCopyToLocation() {
	f := File{}
	copiedFile, err := f.CopyToLocation(&Location{})
	suite.NoError(err)
	suite.NotNil(copiedFile)
}

func (suite *FileTestSuite) TestCopyToFile() {
	f := File{}
	err := f.CopyToFile(&File{})
	suite.NoError(err)
}

func (suite *FileTestSuite) TestMoveToLocation() {
	f := File{}
	movedFile, err := f.MoveToLocation(&Location{})
	suite.NoError(err)
	suite.NotNil(movedFile)
}

func (suite *FileTestSuite) TestMoveToFile() {
	f := File{}
	err := f.MoveToFile(&File{})
	suite.NoError(err)
}

func (suite *FileTestSuite) TestDelete() {
	f := File{}
	suite.NoError(f.Delete())
}

func (suite *FileTestSuite) TestLastModified() {
	f := File{}
	t, err := f.LastModified()
	suite.NoError(err)
	suite.NotNil(t)
}

func (suite *FileTestSuite) TestSize() {
	f := File{}
	size, err := f.Size()
	suite.NoError(err)
	suite.Equal(uint64(0), size)
}

func (suite *FileTestSuite) TestPath() {
	f := File{}
	suite.Equal("", f.Path())
}

func (suite *FileTestSuite) TestName() {
	f := File{}
	suite.Equal("", f.Name())
}

func (suite *FileTestSuite) TestTouch() {
	f := File{}
	suite.NoError(f.Touch())
}

func (suite *FileTestSuite) TestURI() {
	f := File{}
	suite.Equal("", f.URI())
}

func TestAzureFile(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
