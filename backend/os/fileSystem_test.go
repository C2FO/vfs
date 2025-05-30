package os

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type osFileSystemTest struct {
	suite.Suite
}

func (o *osFileSystemTest) TestRetry() {
	fs := &FileSystem{}
	o.IsType(vfs.DefaultRetryer(), fs.Retry()) //nolint:staticcheck // deprecated
}

func (o *osFileSystemTest) TestName() {
	fs := &FileSystem{}
	o.Equal("os", fs.Name())
}

func (o *osFileSystemTest) TestScheme() {
	fs := &FileSystem{}
	o.Equal("file", fs.Scheme())
}

func (o *osFileSystemTest) TestNewFile() {
	fs := &FileSystem{}

	// failure on validation
	file, err := fs.NewFile("", "invalid/file")
	o.EqualError(err, utils.ErrBadAbsFilePath, "error expected for invalid file")
	o.Nil(file, "file should be nil on err")

	// success
	file, err = fs.NewFile("", "/valid/file")
	o.NoError(err, "no error expected for valid file")
	o.IsType(&File{}, file)
}

func (o *osFileSystemTest) TestNewLocation() {
	fs := &FileSystem{}

	// failure on validation
	loc, err := fs.NewLocation("", "/invalid/location")
	o.EqualError(err, utils.ErrBadAbsLocationPath, "error expected for invalid file")
	o.Nil(loc, "file should be nil on err")

	loc, err = fs.NewLocation("", "invalid/location/")
	o.EqualError(err, utils.ErrBadAbsLocationPath, "error expected for invalid file")
	o.Nil(loc, "file should be nil on err")

	// success
	loc, err = fs.NewLocation("", "/valid/location/")
	o.NoError(err, "no error expected for valid file")
	o.IsType(&Location{}, loc)
}

func (o *osFileSystemTest) TestLocation() {
	fs := &FileSystem{}
	o.Equal("file", fs.Scheme())
}

func (o *osFileSystemTest) TestNewFileSystem() {
	fs := NewFileSystem()
	o.IsType(&FileSystem{}, fs)
}

func TestOSFileSystem(t *testing.T) {
	suite.Run(t, new(osFileSystemTest))
}
