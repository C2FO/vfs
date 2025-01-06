package os

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type osLocationTest struct {
	suite.Suite
	testFile   vfs.File
	fileSystem *FileSystem
	tmploc     vfs.Location
}

func (s *osLocationTest) SetupSuite() {
	fs := &FileSystem{}
	dir, err := os.MkdirTemp("", "os_location_test")
	dir = utils.EnsureTrailingSlash(dir)
	s.NoError(err)
	s.tmploc, err = fs.NewLocation("", dir)
	s.NoError(err)
	setupTestFiles(s.tmploc)
}

func (s *osLocationTest) TearDownSuite() {
	teardownTestFiles(s.tmploc)
}

func (s *osLocationTest) SetupTest() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	if err != nil {
		s.Fail("No file was opened")
	}
	s.testFile = file
}

func (s *osLocationTest) TestList() {
	expected := []string{"empty.txt", "prefix-file.txt", "test.txt"}
	actual, _ := s.testFile.Location().List()
	s.Equal(expected, actual)
}

func (s *osLocationTest) TestList_NonExistentDirectory() {
	location, err := s.testFile.Location().NewLocation("not/a/directory/")
	s.NoError(err, "error isn't expected")

	exists, err := location.Exists()
	s.NoError(err, "error isn't expected")
	s.False(exists, "location should return false for Exists")

	contents, err := location.List()
	s.NoError(err, "error isn't expected")
	s.Empty(contents, "List should return empty slice for non-existent directory")

	prefixContents, err := location.ListByPrefix("anything")
	s.NoError(err, "error isn't expected")
	s.Empty(prefixContents, "ListByPrefix should return empty slice for non-existent directory")

	regex := regexp.MustCompile("-+")
	regexContents, err := location.ListByRegex(regex)
	s.NoError(err, "error isn't expected")
	s.Empty(regexContents, "ListByRegex should return empty slice for non-existent directory")
}

func (s *osLocationTest) TestListByPrefix() {
	expected := []string{"prefix-file.txt"}
	actual, _ := s.testFile.Location().ListByPrefix("prefix")
	s.Equal(expected, actual)
}

func (s *osLocationTest) TestListByRegex() {
	expected := []string{"prefix-file.txt"}
	regex := regexp.MustCompile("-+")
	actual, _ := s.testFile.Location().ListByRegex(regex)
	s.Equal(expected, actual)
}

func (s *osLocationTest) TestExists() {
	otherFile, _ := s.tmploc.NewFile("foo/foo.txt")
	s.True(s.testFile.Location().Exists())
	s.False(otherFile.Location().Exists())
}

func (s *osLocationTest) TestNewLocation() {
	otherFile, _ := s.fileSystem.NewFile("", "/foo/foo.txt")
	fileLocation := otherFile.Location()
	subDir, _ := fileLocation.NewLocation("other/")
	s.Equal("/foo/other/", subDir.Path())

	relDir, _ := subDir.NewLocation("../../bar/")
	s.Equal("/bar/", relDir.Path(), "relative dot path works")
}

func (s *osLocationTest) TestNewFile() {
	loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	s.NoError(err)

	newfile, _ := loc.NewFile("../../bam/this.txt")
	s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")
}

func (s *osLocationTest) TestChangeDir() {
	otherFile, _ := s.tmploc.NewFile("foo/foo.txt")
	fileLocation := otherFile.Location()
	cwd := fileLocation.Path()
	err := fileLocation.ChangeDir("other/")
	s.NoError(err, "change dir error not expected")
	s.Equal(fileLocation.Path(), utils.EnsureTrailingSlash(path.Join(cwd, "other")))
}

func (s *osLocationTest) TestVolume() {
	volume := s.testFile.Location().Volume()

	// For Unix, this returns an empty string. For windows, it would be something like 'C:'
	s.Equal(filepath.VolumeName(os.TempDir()), volume)
}

func (s *osLocationTest) TestPath() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	location := file.Location()
	s.Equal("/some/file/", location.Path())

	rootLocation := Location{fileSystem: s.fileSystem, name: "/"}
	s.Equal("/", rootLocation.Path())
}

func (s *osLocationTest) TestURI() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	location := file.Location()
	expected := "file:///some/file/"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
}

func (s *osLocationTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	location := file.Location()
	s.Equal("file:///some/file/", location.String())
}

func (s *osLocationTest) TestDeleteFile() {
	expectedText := "file to delete"
	fileName := "test.txt"
	file, err := s.tmploc.NewFile(fileName)
	s.NoError(err, "Creating file to test delete shouldn't fail")

	_, err = file.Write([]byte(expectedText))
	s.NoError(err, "Shouldn't fail to write text to file.")

	s.NoError(file.Close())

	exists, err := file.Exists()
	s.NoError(err, "Exists shouldn't throw error.")
	s.True(exists, "Exists should return true for test file.")

	s.NoError(s.tmploc.DeleteFile(fileName), "Deleting the file shouldn't throw an error.")
	exists, err = file.Exists()
	s.NoError(err, "Shouldn't throw error testing for exists after delete.")
	s.False(exists, "Exists should return false after deleting the file.")
}

func TestOSLocation(t *testing.T) {
	suite.Run(t, new(osLocationTest))
}
