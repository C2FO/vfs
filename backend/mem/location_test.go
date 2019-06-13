package mem

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v4"
)

/**********************************
 ************TESTS*****************
 **********************************/

type memLocationTest struct {
	suite.Suite
	testFile   vfs.File
	fileSystem *FileSystem
}

func (s *memLocationTest) SetupSuite() {

	//just clearing out any files that file_test may have produced
	if len(fileList) != 0 {
		systemMap = make(map[string]*File)
		fileList = make([]*File, 0)
	}
}

func (s *memLocationTest) TearDownSuite() {
	teardownTestFiles()
}

func (s *memLocationTest) SetupTest() {
	fs := &FileSystem{}

	//	"/home/test_files/subdir/" is the location of s's testFile
	file, err := fs.NewFile("", "/home/test_files/subdir/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file
	s.fileSystem = fs
	WriteZeroBytes(s.testFile)

}

//TestFSName tests out whether or not the location knows what filesystem it is on
func (s *memLocationTest) TestFSName() {

	assert.ObjectsAreEqual(s.testFile.Location().FileSystem(), "mem")
	assert.ObjectsAreEqual(s.testFile.Location().FileSystem().Name(), "In-Memory Filesystem")
	assert.ObjectsAreEqual(s.testFile.Location().FileSystem().Scheme(), "mem")
}

//TestList tests that a location is capable of finding all files that exist there
func (s *memLocationTest) TestList() {
	expected := []string{"test.txt"}
	actual, _ := s.testFile.Location().List()
	s.Equal(expected, actual)
}

/*
TestList_NonExistentDirectory is a test copied over from OS
that creates locations and ensures that they do not exist
by showing that no files live on those directories
*/
func (s *memLocationTest) TestList_NonExistentDirectory() {
	location, err := s.testFile.Location().NewLocation("not/a/directory/")
	s.Nil(err, "error isn't expected")

	exists, err := location.Exists()
	s.Nil(err, "error isn't expected")
	s.False(exists, "location should return false for Exists")

	contents, err := location.List()
	s.Nil(err, "error isn't expected")
	s.Equal(0, len(contents), "List should return empty slice for non-existent directory")

	prefixContents, err := location.ListByPrefix("anything")
	s.Nil(err, "error isn't expected")
	s.Equal(0, len(prefixContents), "ListByPrefix should return empty slice for non-existent directory")

	regex, _ := regexp.Compile("[-]+")
	regexContents, err := location.ListByRegex(regex)
	s.Nil(err, "error isn't expected")
	s.Equal(0, len(regexContents), "ListByRegex should return empty slice for non-existent directory")
}

//TestListByPrefix creates some files and provides a prefix. Succeeds on correct string slice returned
func (s *memLocationTest) TestListByPrefix() {

	_, _ = s.fileSystem.NewFile("", "foo.txt")
	_, _ = s.fileSystem.NewFile("", "/home/test_files/subdir/file1.txt")
	_, _ = s.fileSystem.NewFile("", "/home/test_files/subdir/file2.txt")
	_, _ = s.fileSystem.NewFile("", "/home/directories/test/mat.txt")
	_, _ = s.fileSystem.NewFile("", "/test/files/car.txt")
	nameSlice, _ := s.testFile.Location().ListByPrefix("f")
	expectedSlice := []string{"file1.txt", "file2.txt"}
	assert.ObjectsAreEqual(expectedSlice, nameSlice)
	emptySlice, _ := s.testFile.Location().ListByPrefix("m")
	assert.ObjectsAreEqual(make([]string, 0), emptySlice) //no files should be found with this prefix at this location
}

//TestListByRegex provides a simple regular expression and ensures that the correct fileNames matched that regEx
func (s *memLocationTest) TestListByRegex() {

	newFile, _ := s.fileSystem.NewFile("", "/test_files/test.txt")
	WriteZeroBytes(newFile)
	expected := []string{"test.txt"}
	regex, _ := regexp.Compile("[est]+")
	actual, _ := newFile.Location().ListByRegex(regex)
	s.Equal(expected, actual)
	cerr := newFile.Location().ChangeDir("../")
	assert.NoError(s.T(), cerr, "Unexpected error changing directories")
	regex2, _ := regexp.Compile("[test.txt]")
	actual2, _ := newFile.Location().ListByRegex(regex2)
	s.Equal(expected, actual2)

}

//TestExists ensures that a real location exists, and one that was simply created does not
func (s *memLocationTest) TestExists() {
	otherFile, _ := s.fileSystem.NewFile("", "foo/foo.txt")
	s.True(s.testFile.Location().Exists())
	s.False(otherFile.Location().Exists())
}

//TestNewLocation ensures that we can create new locations, even with relative dot paths
func (s *memLocationTest) TestNewLocation() {

	otherFile, _ := s.fileSystem.NewFile("", "/foo/foo.txt")
	fileLocation := otherFile.Location()
	subDir, _ := fileLocation.NewLocation("other/")
	s.Equal("/foo/other/", subDir.Path())

	relDir, _ := subDir.NewLocation("../../bar/")
	s.Equal("/bar/", relDir.Path(), "relative dot path works")
}

//TestNewFile tests that location can create a file at its current path
func (s *memLocationTest) TestNewFile() {
	loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	s.NoError(err)

	newfile, _ := loc.NewFile("../../bam/this.txt")
	s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")
}

//TestChangeDir tests that we can change the directory on a location but that it doesn't change the file's location
func (s *memLocationTest) TestChangeDir() {

	newFile, _ := s.fileSystem.NewFile("", "/dir/to/change/change.txt")
	WriteZeroBytes(newFile)
	loc := newFile.Location()
	cerr := loc.ChangeDir("extraDir")
	assert.NoError(s.T(), cerr, "Unexpected error while changing directory")
	exists, _ := loc.Exists()
	s.False(exists)
	s.False(newFile.Location().Path() == loc.Path())
}

//TestVolume makes sure that the mem-fs returns the empty string for its volume
func (s *memLocationTest) TestVolume() {
	volume := s.testFile.Location().Volume()

	// For Unix, this returns an empty string. For windows, it would be something like 'C:'
	s.Equal("", volume)
}

//TestPath makes sure that locations return the correct paths, along with leading and trailing slashes
func (s *memLocationTest) TestPath() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	WriteZeroBytes(file)
	location := file.Location()
	s.Equal("/some/file/", location.Path())
	derr := file.Delete()
	assert.NoError(s.T(), derr, "Delete failed unexpectedly")
}

//TestURI ensures that URI's for locations come out in the correct format
func (s *memLocationTest) TestURI() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	WriteZeroBytes(file)
	location := file.Location()
	expected := "mem:///some/file/"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
	derr := file.Delete()
	assert.NoError(s.T(), derr, "Delete failed unexpectedly")
}

//TestStringer tests the implementation of io.Stringer
func (s *memLocationTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	WriteZeroBytes(file)
	location := file.Location()
	expected := "mem:///some/file/"
	s.Equal(expected, location.String(), "%s does not match %s", location.String(), expected)
	derr := file.Delete()
	assert.NoError(s.T(), derr, "Delete failed unexpectedly")
}

//TestDeleteFile makes files, writes to them, deletes them, all while asserting things like existence and errors
func (s *memLocationTest) TestDeleteFile() {

	newFile, err := s.fileSystem.NewFile("", "home/bar.txt")
	assert.NoError(s.T(), err, "Unexpected creation error")
	derr1 := newFile.Delete()
	assert.Error(s.T(), derr1, "Unexpected existence error")
	WriteZeroBytes(newFile)
	otherFile, _ := s.fileSystem.NewFile("", "foo.txt")
	derr2 := otherFile.Delete()
	assert.Error(s.T(), derr2, "Unexpected existence error")
	WriteZeroBytes(otherFile)
	existence, eerr := otherFile.Exists()
	s.True(existence)
	assert.NoError(s.T(), eerr, "Unexpected existence error")
	derr3 := otherFile.Location().DeleteFile(otherFile.Name())
	assert.NoError(s.T(), derr3, "Unexpected existence error")
	existence1, eerr1 := otherFile.Exists()
	s.False(existence1)
	assert.NoError(s.T(), eerr1, "Unexpected existence error")
	s.True(systemMap["/foo.txt"] == nil)

}

func TestMemLocation(t *testing.T) {
	suite.Run(t, new(memLocationTest))
}
