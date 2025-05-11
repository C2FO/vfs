package os

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/utils"
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

	// new tests for location update
	s.Run("new file with relative path updates location", func() {
		newFile, err := loc.NewFile("../newfile.txt")
		s.NoError(err)
		s.Equal("/foo/bar/newfile.txt", newFile.Path(), "NewFile with relative path should update location correctly")
		s.Equal("/foo/bar/", newFile.Location().Path(), "NewFile with relative path should update location correctly")
	})

	s.Run("new file with relative path to root", func() {
		newFile, err := loc.NewFile("../../../../newrootfile.txt")
		s.NoError(err)
		s.Equal("/newrootfile.txt", newFile.Path(), "NewFile with relative path to root should update location correctly")
		s.Equal("/", newFile.Location().Path(), "NewFile with relative path to root should update location correctly")
	})
}

//nolint:staticcheck // deprecated method test
func (s *osLocationTest) TestChangeDir() {
	otherFile, _ := s.tmploc.NewFile("foo/foo.txt")
	fileLocation := otherFile.Location()
	cwd := fileLocation.Path()
	err := fileLocation.ChangeDir("other/")
	s.NoError(err, "change dir error not expected")
	s.Equal(fileLocation.Path(), utils.EnsureTrailingSlash(path.Join(cwd, "other")))
}

//nolint:staticcheck // deprecated method test
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

// TestOpen tests the Open method in the Location implementation
func (s *osLocationTest) TestOpen() {
	// Create a temp file with content
	tempFileName := "open_test_file.txt"
	testContent := "hello world test content"

	// Create file for testing within test directory structure
	file, err := s.tmploc.NewFile("test_files/" + tempFileName)
	s.NoError(err, "No error expected creating test file")

	_, err = file.Write([]byte(testContent))
	s.NoError(err, "Write should not error")
	s.NoError(file.Close(), "Close should not error")

	// Get the file's location
	loc := file.Location()

	// Test Opening the file
	opened, err := loc.Open(tempFileName)
	s.NoError(err, "Opening an existing file should not return an error")
	s.NotNil(opened, "Opened file should not be nil")

	// Read the content to verify
	data := make([]byte, len(testContent))
	n, err := opened.Read(data)
	s.NoError(err, "Reading from opened file should not error")
	s.Equal(len(testContent), n, "Should read all content")
	s.Equal(testContent, string(data), "Content should match")
	
	// Close the opened file to avoid "file in use" errors on Windows
	s.NoError(opened.Close(), "Closing opened file should not error")

	// Test opening non-existent file
	_, err = loc.Open("non-existent-file.txt")
	s.Error(err, "Opening a non-existent file should return an error")
	var pathErr *fs.PathError
	s.ErrorAs(err, &pathErr, "Error should be a fs.PathError")
	s.ErrorIs(pathErr.Err, fs.ErrNotExist, "Underlying error should be fs.ErrNotExist")

	// Test opening with path traversal attempts (should be rejected)
	_, err = loc.Open("../outside.txt")
	s.Error(err, "Opening a file with path traversal should return an error")
	s.ErrorAs(err, &pathErr, "Error should be a fs.PathError")
	s.ErrorIs(pathErr.Err, fs.ErrInvalid, "Underlying error should be fs.ErrInvalid")

	_, err = loc.Open("./file.txt")
	s.Error(err, "Opening path with ./ should return an error")

	_, err = loc.Open(".")
	s.Error(err, "Opening . should return an error")

	_, err = loc.Open("..")
	s.Error(err, "Opening .. should return an error")

	// Clean up
	s.NoError(file.Delete(), "Delete should not error")
}

func TestOSLocation(t *testing.T) {
	suite.Run(t, new(osLocationTest))
}
