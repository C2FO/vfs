package os

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v8/backend/testsuite"
)

type osLocationTestSuite struct {
	suite.Suite
	testFile   *File
	fileSystem *FileSystem
	tmploc     *Location
}

func (s *osLocationTestSuite) SetupSuite() {
	s.fileSystem = NewFileSystem()
	dir := s.T().TempDir()
	dir = utils.EnsureTrailingSlash(dir)
	var err error
	loc, err := s.fileSystem.NewLocation("", dir)
	s.Require().NoError(err)
	s.tmploc = loc.(*Location)
	setupTestFiles(s.tmploc)
}

func (s *osLocationTestSuite) SetupTest() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.Require().NoError(err)
	s.testFile = file.(*File)
}

func (s *osLocationTestSuite) TearDownTest() {
	err := s.testFile.Close()
	s.Require().NoError(err)
}

func (s *osLocationTestSuite) TestList() {
	ctx := context.Background()
	names, err := testsuite.CollectList(ctx, s.testFile.Location())
	s.Require().NoError(err)
	s.Equal([]string{"empty.txt", "prefix-file.txt", "test.txt"}, names)
}

func (s *osLocationTestSuite) TestList_NonExistentDirectory() {
	location, err := s.testFile.Location().NewLocation("not/a/directory/")
	s.Require().NoError(err)

	exists, err := location.Exists()
	s.Require().NoError(err)
	s.False(exists)

	contents, err := testsuite.CollectList(context.Background(), location)
	s.Require().NoError(err)
	s.Empty(contents)
}

func (s *osLocationTestSuite) TestDeleteFile() {
	const expectedText = "file to delete"
	const fileName = "deleteme.txt"
	file, err := s.tmploc.NewFile(fileName)
	s.Require().NoError(err)

	_, err = file.Write([]byte(expectedText))
	s.Require().NoError(err)
	s.Require().NoError(file.Close())

	exists, err := file.Exists()
	s.Require().NoError(err)
	s.True(exists)

	s.Require().NoError(s.tmploc.DeleteFile(fileName))
	exists, err = file.Exists()
	s.Require().NoError(err)
	s.False(exists)
}

func (s *osLocationTestSuite) TestURI() {
	file, err := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Require().NoError(err)
	location := file.Location().(*Location)
	s.Equal("file:///some/file/", location.URI())
}

func (s *osLocationTestSuite) TestPath() {
	file, err := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Require().NoError(err)
	location := file.Location().(*Location)
	s.Equal("/some/file/", location.Path())

	rootLocation := &Location{fileSystem: s.fileSystem, name: "/"}
	s.Equal("/", rootLocation.Path())
}

func TestOSLocation(t *testing.T) {
	suite.Run(t, new(osLocationTestSuite))
}

func setupTestFiles(baseLoc *Location) {
	createDir(baseLoc, "test_files")
	writeStringFile(baseLoc, "test_files/empty.txt", "")
	writeStringFile(baseLoc, "test_files/prefix-file.txt", "hello, Dave")
	writeStringFile(baseLoc, "test_files/test.txt", "hello world")
	createDir(baseLoc, "test_files/subdir")
	writeStringFile(baseLoc, "test_files/subdir/test.txt", "hello world too")
}

func createDir(baseLoc *Location, dirname string) {
	dir := filepath.Join(osLocationPath(baseLoc), filepath.FromSlash(dirname))
	if err := os.Mkdir(dir, 0750); err != nil {
		panic(err)
	}
}

func writeStringFile(baseLoc *Location, filename, data string) {
	file := filepath.Join(osLocationPath(baseLoc), filepath.FromSlash(filename))
	f, err := os.Create(file) //nolint:gosec
	if err != nil {
		panic(err)
	}
	if _, err := f.WriteString(data); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
