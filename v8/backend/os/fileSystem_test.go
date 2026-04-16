package os

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
)

type osFileSystemTestSuite struct {
	suite.Suite
}

func (s *osFileSystemTestSuite) TestName() {
	fs := &FileSystem{}
	s.Equal("os", fs.Name())
}

func (s *osFileSystemTestSuite) TestScheme() {
	fs := &FileSystem{}
	s.Equal("file", fs.Scheme())
}

func (s *osFileSystemTestSuite) TestNewFile() {
	fs := &FileSystem{}

	file, err := fs.NewFile("", "invalid/file")
	s.Require().ErrorIs(err, utils.ErrBadAbsFilePath)
	s.Nil(file)

	file, err = fs.NewFile("", "/valid/file")
	s.Require().NoError(err)
	s.IsType((*File)(nil), file)
}

func (s *osFileSystemTestSuite) TestNewLocation() {
	fs := &FileSystem{}

	loc, err := fs.NewLocation("", "/invalid/location")
	s.Require().ErrorIs(err, utils.ErrBadAbsLocationPath)
	s.Nil(loc)

	loc, err = fs.NewLocation("", "invalid/location/")
	s.Require().ErrorIs(err, utils.ErrBadAbsLocationPath)
	s.Nil(loc)

	loc, err = fs.NewLocation("", "/valid/location/")
	s.Require().NoError(err)
	s.IsType((*Location)(nil), loc)
}

func (s *osFileSystemTestSuite) TestNewFileSystem() {
	fs := NewFileSystem()
	s.IsType((*FileSystem)(nil), fs)
}

func (s *osFileSystemTestSuite) TestWithTempDirOption() {
	opt := WithTempDir{TempDir: "/custom/temp"}
	s.Equal("WithTempDir", opt.NewFileSystemOptionName())

	fs := NewFileSystem(opt)
	s.Equal("/custom/temp", fs.tempDir)
}

func TestOSFileSystem(t *testing.T) {
	suite.Run(t, new(osFileSystemTestSuite))
}
