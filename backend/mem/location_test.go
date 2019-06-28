package mem

import (
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v5"
)

/**********************************
 ************TESTS*****************
 **********************************/

type memLocationTest struct {
	suite.Suite
	testFile   vfs.File
	fileSystem *FileSystem
}

func (s *memLocationTest) SetupTest() {
	fs := &FileSystem{}
	fs.Initialize()

	file, nerr := fs.NewFile("", "/test_files/test.txt")
	s.NoError(nerr, "File creation was not successful so it does not exist")

	s.testFile = file.(*File)
	s.fileSystem = fs
	s.testFile.(*File).Touch()
}

//TestFSName tests out whether or not the location knows what filesystem it is on
func (s *memLocationTest) TestFSName() {

	s.Equal(s.testFile.Location().FileSystem().Name(), "In-Memory Filesystem")
	s.Equal(s.testFile.Location().FileSystem().Scheme(), "mem")
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
	s.True(exists, "location should return true for Exists")

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

	f1, nerr := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(nerr, "Unexpected error creating a new file")
	f1.(*File).Touch()

	f2, nerr2 := s.fileSystem.NewFile("", "/home/test_files/subdir/file1.txt")
	s.NoError(nerr2, "Unexpected error creating a new file")
	f2.(*File).Touch()

	f3, nerr3 := s.fileSystem.NewFile("", "/home/test_files/subdir/file2.txt")
	s.NoError(nerr3, "Unexpected error creating a new file")
	f3.(*File).Touch()

	f4, nerr4 := s.fileSystem.NewFile("", "/home/directories/test/mat.txt")
	s.NoError(nerr4, "Unexpected error creating a new file")
	f4.(*File).Touch()

	f5, nerr5 := s.fileSystem.NewFile("", "/test/files/car.txt")
	s.NoError(nerr5, "Unexpected error creating a new file")
	f5.(*File).Touch()

	loc, lerr := s.fileSystem.NewLocation("", "/home/test_files/subdir/")
	s.NoError(lerr, "Unexpected error creating a location")

	nameSlice, lerr := loc.ListByPrefix("f")
	s.NoError(lerr, "Unexpected error obtaining list by prefix")
	expectedSlice := []string{"file1.txt", "file2.txt"}
	s.Equal(expectedSlice, nameSlice)
	emptySlice, _ := s.testFile.Location().ListByPrefix("m")
	s.Equal(make([]string, 0), emptySlice) //no files should be found with this prefix at this location
}

//TestListByRegex provides a simple regular expression and ensures that the correct fileNames matched that regEx
func (s *memLocationTest) TestListByRegex() {

	newFile, nerr := s.fileSystem.NewFile("", "/test_files/test.txt")
	s.NoError(nerr, "Unexpected error creating a new file")

	newFile.(*File).Touch()
	expected := []string{"test.txt"}

	regex, comperr := regexp.Compile("[est]+")
	s.NoError(comperr, "Unexpected error compiling regEx")

	actual, regerr := newFile.Location().ListByRegex(regex)
	s.NoError(regerr, "Unexpected error listing by regEx")

	s.Equal(expected, actual)
	cerr := newFile.Location().ChangeDir("../")
	s.NoError(cerr, "Unexpected error changing directories")

	regex2, regerr2 := regexp.Compile("[test.txt]")
	s.NoError(regerr2, "Unexpected error compiling regEx")

	actual2, regerr3 := newFile.Location().ListByRegex(regex2)
	s.NoError(regerr3, "Unexpected error listing by regEx")
	s.Equal(expected, actual2)

}

//TestExists ensures that a real location exists, and one that was simply created does not
func (s *memLocationTest) TestExists() {
	otherFile, nerr := s.fileSystem.NewFile("", "/foo/foo.txt")
	s.NoError(nerr, "Unexpected error creating a new file")
	s.True(s.testFile.Location().Exists())
	s.True(otherFile.Location().Exists())
}

//TestNewLocation ensures that we can create new locations, even with relative dot paths
func (s *memLocationTest) TestNewLocation() {

	otherFile, nerr := s.fileSystem.NewFile("", "/foo/foo.txt")
	s.NoError(nerr, "Unexpected error creating a new file")
	fileLocation := otherFile.Location()
	subDir, lerr := fileLocation.NewLocation("other/")
	s.NoError(lerr, "Unexpected error creating a new location")

	s.Equal("/foo/other/", subDir.Path())

	relDir, lerr2 := subDir.NewLocation("../../bar/")
	s.NoError(lerr2, "Unexpected error creating a new location")
	s.Equal("/bar/", relDir.Path(), "relative dot path works")
}

/*
 TestNewLocation2 tests to see whether a file can be made by passing a
relative path to a location object that technically does not exist
*/
func (s *memLocationTest) TestNewLocation2() {

	newFile, nerr := s.fileSystem.NewFile("C", "/newLocTest/dir/file.txt")
	s.NoError(nerr, "Unexpected error creating a file")

	_, werr := newFile.Write([]byte("Hello world!"))
	s.NoError(werr, "Unexpected write error")

	s.NoError(newFile.Close(), "Unexpected error closing file")

	loc, nerr2 := s.fileSystem.NewLocation("C", "/newLocTest/")
	s.NoError(nerr2, "Unexpected error creating a new location")
	s.True(loc.Exists())

	otherFile, lerr := loc.NewFile("dir/file2.txt")
	s.NoError(lerr, "Unexpected error creating a file from location")

	otherFile.(*File).Touch()
	s.Equal(newFile.Location().Path(), otherFile.Location().Path(), "Absolute location paths should be equal")

}

//TestNewFile tests that location can create a file at its current path
func (s *memLocationTest) TestNewFile() {
	loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	s.NoError(err, "Unexpected error creating a new location")

	newfile, nerr := loc.NewFile("../../bam/this.txt")
	s.NoError(nerr, "Unexpected error creating a new file")
	s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")
}
/*
TestNewFile creates two files with the same name and ensures
that the second creation returns a reference to the first
*/
func (s *memLocationTest) TestNewFileSameName(){
	sharedPath := "/path/to/file.txt"
	firstFile,err := s.fileSystem.NewFile("",sharedPath)

	s.NoError(err,"Unexpected error creating a file")
	location := firstFile.Location()

	expectedText := "hey y'all!"
	_, err = firstFile.Write([]byte(expectedText))
	s.NoError(err,"Unexpected error writing to file")

	secondFile, err := location.NewFile(path.Base(sharedPath))
	s.NoError(err,"Unexpected error creating a file")
	expectedSlice := make([]byte,len(expectedText))

	//since secondFile references firstFile, reading will throw an error as we never closed or seeked firstFile
	_, err = secondFile.Read(expectedSlice)
	s.Error(err,"Expected read error since firstFile was never closed")

	//after this call, we can expect to be able to read from secondFile since its reference, firstFile, was closed
	s.NoError(firstFile.Close(),"Unexpected error closing file")

	_, err = secondFile.Read(expectedSlice)
	s.NoError(err,"Unexpected read error")

	s.Equal(expectedText,string(expectedSlice))

}

//TestChangeDir tests that we can change the directory on a location but that it doesn't change the file's location
func (s *memLocationTest) TestChangeDir() {

	newFile, nerr := s.fileSystem.NewFile("", "/dir/to/change/change.txt")
	s.NoError(nerr, "Unexpected error creating a new file")

	newFile.(*File).Touch()
	loc := newFile.Location()

	//changing directory
	s.NoError(loc.ChangeDir("extraDir/"), "Unexpected error while changing directory")
	exists, eerr := loc.Exists()
	s.NoError(eerr, "Unexpected error checking for Existence")
	s.True(exists)
	s.False(newFile.Location().Path() == loc.Path())
}

//TestVolume makes sure that the mem-fs returns the empty string for its volume
func (s *memLocationTest) TestVolume() {

	newFile, nerr := s.fileSystem.NewFile("D:", "/path/to/file/example.txt")
	s.NoError(nerr, "Unexpected error creating a file")
	newFile.(*File).Touch()
	s.NoError(newFile.Close(), "Unexpected error closing file")
	// For Unix, this returns an empty string. For windows, it would be something like 'C:'
	s.Equal("D:", newFile.Location().Volume())
}

//TestPath makes sure that locations return the correct paths, along with leading and trailing slashes
func (s *memLocationTest) TestPath() {
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.NoError(nerr, "Unexpected error creating a new file")

	file.(*File).Touch()
	location := file.Location()
	s.Equal("/some/file/", location.Path())
	//deleting file
	s.NoError(file.Delete(), "Delete failed unexpectedly")
}

//TestURI ensures that URI's for locations come out in the correct format
func (s *memLocationTest) TestURI() {
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.NoError(nerr, "Unexpected error creating a new file")
	file.(*File).Touch()
	location := file.Location()
	_, eerr := location.Exists()
	s.NoError(eerr, "Unexpected error checking for existence")
	expected := "mem:///some/file/"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
	s.NoError(file.Delete(), "Delete failed unexpectedly")
}

//TestStringer tests the implementation of io.Stringer
func (s *memLocationTest) TestStringer() {
	file, nerr := s.fileSystem.NewFile("C", "/some/file/test.txt")
	s.NoError(nerr, "Unexpected error creating a new file")

	file.(*File).Touch()
	location := file.Location()
	expected := "mem://C/some/file/"
	s.Equal(expected, location.String(), "%s does not match %s", location.String(), expected)
	//deleting file
	s.NoError(file.Delete(), "Delete failed unexpectedly")
}

//TestDeleteFile makes files, writes to them, deletes them, all while asserting things like existence and errors
func (s *memLocationTest) TestDeleteFile() {

	newFile, err := s.fileSystem.NewFile("", "/home/bar.txt")
	s.NoError(err, "Unexpected error creating a new file")
	//attempt to delete newFile
	s.Error(newFile.Delete(), "Expected existence error") //expected an error since newFile does not yet exist
	newFile.(*File).Touch()

	otherFile, nerr := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(nerr, "Unexpected error creating a new file")

	//attempt to delete otherFile
	s.Error(otherFile.Location().DeleteFile(otherFile.Name()), "Expected existence error") //want to catch the delete error
	otherFile.(*File).Touch()                                                              //bring it to existence with a touch
	existence, eerr := otherFile.Exists()
	s.True(existence)
	s.NoError(eerr, "Unexpected existence error")

	//deleting otherFile
	s.NoError(otherFile.Location().DeleteFile(otherFile.Name()), "Unexpected existence error")
	existence1, eerr1 := otherFile.Exists()
	s.False(existence1)
	s.NoError(eerr1, "Unexpected existence error")

}

func TestMemLocation(t *testing.T) {
	suite.Run(t, new(memLocationTest))
}
