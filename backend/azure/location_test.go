package azure

import (
	"github.com/c2fo/vfs/v5"
	"github.com/stretchr/testify/suite"
	"testing"
)

type LocationTestSuite struct {
	suite.Suite
}

func (suite *LocationTestSuite) TestVFSLocationImplementor() {
	l := Location{}
	suite.Implements((*vfs.Location)(nil), &l, "Does not implement the vfs.Location interface")
}

func (suite *LocationTestSuite) TestString() {
	l := Location{}
	suite.Equal("", l.String())
}

func (suite *LocationTestSuite) TestList() {
	l := Location{}
	listing, err := l.List()
	suite.NoError(err)
	suite.Len(listing, 0)
}

func (suite *LocationTestSuite) TestListByPrefix() {
	l := Location{}
	listing, err := l.ListByPrefix("")
	suite.NoError(err)
	suite.Len(listing, 0)
}

func (suite *LocationTestSuite) TestListByRegex() {
	l := Location{}
	listing, err := l.ListByRegex(nil)
	suite.NoError(err)
	suite.Len(listing, 0)
}

func (suite *LocationTestSuite) TestVolume() {
	l := Location{}
	suite.Equal("", l.Volume())
}

func (suite *LocationTestSuite) TestPath() {
	l := Location{}
	suite.Equal("", l.Path())
}

func (suite *LocationTestSuite) TestExists() {
	l := Location{}
	exists, err := l.Exists()
	suite.NoError(err)
	suite.False(exists)
}

func (suite *LocationTestSuite) TestNewLocation() {
	l := Location{}
	nl, err := l.NewLocation("")
	suite.NoError(err)
	suite.NotNil(nl)
}

func (suite *LocationTestSuite) TestChangeDir() {
	l := Location{}
	suite.NoError(l.ChangeDir(""))
}

func (suite *LocationTestSuite) TestFileSystem() {
	l := Location{}
	suite.NotNil(l.FileSystem())
}

func (suite *LocationTestSuite) TestNewFile() {
	l := Location{}
	f, err := l.NewFile("")
	suite.NoError(err)
	suite.NotNil(f)
}

func (suite *LocationTestSuite) TestDeleteFile() {
	l := Location{}
	suite.NoError(l.DeleteFile(""))
}

func (suite *LocationTestSuite) TestURI() {
	l := Location{}
	suite.Equal("", l.URI())
}

func TestAzureLocation(t *testing.T) {
	suite.Run(t, new(LocationTestSuite))
}
