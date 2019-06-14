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


}

func (s *memLocationTest) TearDownSuite() {
	//teardownTestFiles()
}

func (s *memLocationTest) SetupTest() {
	fs := &FileSystem{
		make(map[string]*File),
		make([]*File, 0),
	}

	file, err := fs.NewFile("", "/test_files/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file.(*File)
	s.fileSystem = fs
	//s.fileSystem.Initialize()
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

	_, nerr := s.fileSystem.NewFile("", "foo.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")

	_, nerr2 := s.fileSystem.NewFile("", "/home/test_files/subdir/file1.txt")
	assert.NoError(s.T(),nerr2,"Unexpected error creating a new file")

	_, nerr3:= s.fileSystem.NewFile("", "/home/test_files/subdir/file2.txt")
	assert.NoError(s.T(),nerr3,"Unexpected error creating a new file")

	_, nerr4:= s.fileSystem.NewFile("", "/home/directories/test/mat.txt")
	assert.NoError(s.T(),nerr4,"Unexpected error creating a new file")

	_, nerr5 := s.fileSystem.NewFile("", "/test/files/car.txt")
	assert.NoError(s.T(),nerr5,"Unexpected error creating a new file")

	nameSlice, _ := s.testFile.Location().ListByPrefix("f")
	expectedSlice := []string{"file1.txt", "file2.txt"}
	assert.ObjectsAreEqual(expectedSlice, nameSlice)
	emptySlice, _ := s.testFile.Location().ListByPrefix("m")
	assert.ObjectsAreEqual(make([]string, 0), emptySlice) //no files should be found with this prefix at this location
}

//TestListByRegex provides a simple regular expression and ensures that the correct fileNames matched that regEx
func (s *memLocationTest) TestListByRegex() {

	newFile, nerr := s.fileSystem.NewFile("", "/test_files/test.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")

	WriteZeroBytes(newFile)
	expected := []string{"test.txt"}
	regex, comperr := regexp.Compile("[est]+")
	assert.NoError(s.T(),comperr,"Unexpected error compiling regEx")
	actual, regerr := newFile.Location().ListByRegex(regex)
	assert.NoError(s.T(),regerr,"Unexpected error listing by regEx")

	s.Equal(expected, actual)
	cerr := newFile.Location().ChangeDir("../")
	assert.NoError(s.T(), cerr, "Unexpected error changing directories")
	regex2, regerr2 := regexp.Compile("[test.txt]")
	assert.NoError(s.T(),regerr2,"Unexpected error compiling regEx")
	actual2, regerr3 := newFile.Location().ListByRegex(regex2)
	assert.NoError(s.T(),regerr3,"Unexpected error listing by regEx")
	s.Equal(expected, actual2)

}

//TestExists ensures that a real location exists, and one that was simply created does not
func (s *memLocationTest) TestExists() {
	otherFile, nerr := s.fileSystem.NewFile("", "foo/foo.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")
	s.True(s.testFile.Location().Exists())
	s.False(otherFile.Location().Exists())
}

//TestNewLocation ensures that we can create new locations, even with relative dot paths
func (s *memLocationTest) TestNewLocation() {

	otherFile, nerr := s.fileSystem.NewFile("", "/foo/foo.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")
	fileLocation := otherFile.Location()
	subDir, lerr := fileLocation.NewLocation("other/")
	assert.NoError(s.T(),lerr,"Unexpected error creating a new location")

	s.Equal("/foo/other/", subDir.Path())

	relDir, lerr2 := subDir.NewLocation("../../bar/")
	assert.NoError(s.T(),lerr2,"Unexpected error creating a new location")
	s.Equal("/bar/", relDir.Path(), "relative dot path works")
}

//TestNewFile tests that location can create a file at its current path
func (s *memLocationTest) TestNewFile() {
	loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	assert.NoError(s.T(),err,"Unexpected error creating a new location")


	newfile, nerr := loc.NewFile("../../bam/this.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")
	s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")
}

//TestChangeDir tests that we can change the directory on a location but that it doesn't change the file's location
func (s *memLocationTest) TestChangeDir() {

	newFile, nerr := s.fileSystem.NewFile("", "/dir/to/change/change.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")

	WriteZeroBytes(newFile)
	loc := newFile.Location()
	cerr := loc.ChangeDir("extraDir")
	assert.NoError(s.T(), cerr, "Unexpected error while changing directory")
	exists, eerr := loc.Exists()
	assert.NoError(s.T(),eerr,"Unexpected error checking for Existence")
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
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")

	WriteZeroBytes(file)
	location := file.Location()
	s.Equal("/some/file/", location.Path())
	derr := file.Delete()
	assert.NoError(s.T(), derr, "Delete failed unexpectedly")
}

//TestURI ensures that URI's for locations come out in the correct format
func (s *memLocationTest) TestURI() {
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")
	WriteZeroBytes(file)
	location := file.Location()
	_,eerr:=location.Exists()
	assert.NoError(s.T(),eerr,"Unexpected error checking for existence")
	expected := "mem:///some/file/"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
	derr := file.Delete()
	assert.NoError(s.T(), derr, "Delete failed unexpectedly")
}

//TestStringer tests the implementation of io.Stringer
func (s *memLocationTest) TestStringer() {
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")

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
	assert.NoError(s.T(), err, "Unexpected error creating a new file")
	derr1 := newFile.Delete()
	assert.Error(s.T(), derr1, "Unexpected existence error")
	WriteZeroBytes(newFile)
	otherFile, nerr := s.fileSystem.NewFile("", "foo.txt")
	assert.NoError(s.T(),nerr,"Unexpected error creating a new file")

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
	s.True(s.fileSystem.systemMap["/foo.txt"] == nil)

}

func TestMemLocation(t *testing.T) {
	suite.Run(t, new(memLocationTest))
}
