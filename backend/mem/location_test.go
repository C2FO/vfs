package mem

import (
	"io"
	"path"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
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
	fs := NewFileSystem()

	file, nerr := fs.NewFile("", "/test_files/test.txt")
	s.Require().NoError(nerr, "file creation was not successful so it does not exist")

	s.testFile = file.(*File)
	s.fileSystem = fs
	s.Require().NoError(s.testFile.Touch(), "unexpected error touching file")
}

// TestFSName tests out whether or not the location knows what filesystem it is on
func (s *memLocationTest) TestFSName() {
	s.Equal("In-Memory FileSystem", s.testFile.Location().FileSystem().Name())
	s.Equal("mem", s.testFile.Location().FileSystem().Scheme())
}

// TestList tests that a location is capable of finding all files that exist there
func (s *memLocationTest) TestList() {
	expected := []string{"test.txt"}
	actual, _ := s.testFile.Location().List()
	s.Equal(expected, actual)
}

// TestList_NonExistentDirectory is a test copied over from OS
// that creates locations and ensures that they do not exist
// by showing that no files live on those directories
func (s *memLocationTest) TestList_NonExistentDirectory() {
	location, err := s.testFile.Location().NewLocation("not/a/directory/")
	s.Require().NoError(err, "error isn't expected")

	exists, err := location.Exists()
	s.Require().NoError(err, "error isn't expected")
	s.True(exists, "location should return true for Exists")

	contents, err := location.List()
	s.Require().NoError(err, "error isn't expected")
	s.Empty(contents, "list should return empty slice for non-existent directory")

	prefixContents, err := location.ListByPrefix("anything")
	s.Require().NoError(err, "error isn't expected")
	s.Empty(prefixContents, "ListByPrefix should return empty slice for non-existent directory")

	regex := regexp.MustCompile("-+")
	regexContents, err := location.ListByRegex(regex)
	s.Require().NoError(err, "error isn't expected")
	s.Empty(regexContents, "ListByRegex should return empty slice for non-existent directory")
}

// TestListByPrefix creates some files and provides a prefix. Succeeds on correct string slice returned
func (s *memLocationTest) TestListByPrefix() {
	f1, nerr := s.fileSystem.NewFile("", "/foo.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Require().NoError(f1.Touch(), "unexpected error touching file")

	f2, nerr := s.fileSystem.NewFile("", "/home/test_files/subdir/file1.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Require().NoError(f2.Touch(), "unexpected error touching file")

	f3, nerr := s.fileSystem.NewFile("", "/home/test_files/subdir/file2.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Require().NoError(f3.Touch(), "unexpected error touching file")

	f4, nerr := s.fileSystem.NewFile("", "/home/directories/test/mat.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Require().NoError(f4.Touch(), "unexpected error touching file")

	f5, nerr := s.fileSystem.NewFile("", "/test/files/car.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Require().NoError(f5.Touch(), "unexpected error touching file")

	loc, lerr := s.fileSystem.NewLocation("", "/home/test_files/subdir/")
	s.Require().NoError(lerr, "unexpected error creating a location")

	nameSlice, lerr := loc.ListByPrefix("f")
	s.Require().NoError(lerr, "unexpected error obtaining list by prefix")
	expectedSlice := []string{"file1.txt", "file2.txt"}
	s.Equal(expectedSlice, nameSlice)
	emptySlice, err := s.testFile.Location().ListByPrefix("m")
	s.Require().NoError(err, "unexpected error retrieving files by prefix")
	s.Empty(emptySlice) // no files should be found with this prefix at this location
}

// TestListByRegex provides a simple regular expression and ensures that the correct fileNames matched that regEx
func (s *memLocationTest) TestListByRegex() {
	newFile, nerr := s.fileSystem.NewFile("", "/test_files/test.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")

	s.Require().NoError(newFile.Touch(), "unexpected error touching file")
	expected := []string{"test.txt"}

	regex := regexp.MustCompile("[est]+")

	actual, err := newFile.Location().ListByRegex(regex)
	s.Require().NoError(err, "unexpected error listing by regEx")

	s.Equal(expected, actual)
	newLoc, err := newFile.Location().NewLocation("../")
	s.Require().NoError(err, "unexpected error changing directories")

	regex2 := regexp.MustCompile("test.txt")

	actual2, err := newLoc.ListByRegex(regex2)
	s.Require().NoError(err, "unexpected error listing by regEx")
	s.Equal([]string{}, actual2)
}

// TestExists ensures that a real location exists, and one that was simply created does not
func (s *memLocationTest) TestExists() {
	otherFile, nerr := s.fileSystem.NewFile("", "/foo/foo.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.True(s.testFile.Location().Exists())
	s.True(otherFile.Location().Exists())
}

// TestNewLocation ensures that we can create new locations, even with relative dot paths
func (s *memLocationTest) TestNewLocation() {
	otherFile, nerr := s.fileSystem.NewFile("", "/foo/foo.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	fileLocation := otherFile.Location()
	subDir, lerr := fileLocation.NewLocation("other/")
	s.Require().NoError(lerr, "unexpected error creating a new location")

	s.Equal("/foo/other/", subDir.Path())

	relDir, lerr2 := subDir.NewLocation("../../bar/")
	s.Require().NoError(lerr2, "unexpected error creating a new location")
	s.Equal("/bar/", relDir.Path(), "relative dot path works")
}

// TestNewLocationRelativePath tests to see whether a file can be made by passing a
// relative path to a location object that technically does not exist
func (s *memLocationTest) TestNewLocationRelativePath() {
	newFile, nerr := s.fileSystem.NewFile("C", "/newLocTest/dir/file.txt")
	s.Require().NoError(nerr, "unexpected error creating a file")

	_, werr := newFile.Write([]byte("Hello world!"))
	s.Require().NoError(werr, "unexpected write error")

	s.Require().NoError(newFile.Close(), "unexpected error closing file")

	loc, nerr2 := s.fileSystem.NewLocation("C", "/newLocTest/")
	s.Require().NoError(nerr2, "unexpected error creating a new location")
	s.True(loc.Exists())

	otherFile, lerr := loc.NewFile("dir/file2.txt")
	s.Require().NoError(lerr, "unexpected error creating a file from location")

	s.Require().NoError(otherFile.Touch(), "unexpected error touching file")
	s.Equal(newFile.Location().Path(), otherFile.Location().Path(), "absolute location paths should be equal")
}

// TestNewFile tests that location can create a file at its current path
func (s *memLocationTest) TestNewFile() {
	loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	s.Require().NoError(err, "unexpected error creating a new location")

	newfile, nerr := loc.NewFile("../../bam/this.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")

	// new tests for location update
	s.Run("new file with relative path updates location", func() {
		newFile, err := loc.NewFile("../newfile.txt")
		s.Require().NoError(err)
		s.Equal("/foo/bar/newfile.txt", newFile.Path(), "NewFile with relative path should update location correctly")
		s.Equal("/foo/bar/", newFile.Location().Path(), "NewFile with relative path should update location correctly")
	})

	s.Run("new file with relative path to root", func() {
		newFile, err := loc.NewFile("../../../../newrootfile.txt")
		s.Require().NoError(err)
		s.Equal("/newrootfile.txt", newFile.Path(), "NewFile with relative path to root should update location correctly")
		s.Equal("/", newFile.Location().Path(), "NewFile with relative path to root should update location correctly")
	})
}

// TestNewFile creates two files with the same name and ensures
// that the second creation returns a reference to the first
func (s *memLocationTest) TestNewFileSameName() {
	sharedPath := "/path/to/file.txt"
	firstFile, err := s.fileSystem.NewFile("", sharedPath)

	s.Require().NoError(err, "unexpected error creating a file")
	location := firstFile.Location()

	expectedText := "hey y'all!"
	_, err = firstFile.Write([]byte(expectedText))
	s.Require().NoError(err, "unexpected error writing to file")

	secondFile, err := location.NewFile(path.Base(sharedPath))
	s.Require().NoError(err, "unexpected error creating a file")
	expectedSlice := make([]byte, len(expectedText))

	// since secondFile references firstFile, reading will throw an error as we never closed or seeked firstFile
	_, err = secondFile.Read(expectedSlice)
	s.Require().Error(err, "expected read error since firstFile was never closed")

	// after this call, we can expect to be able to read from secondFile since its reference, firstFile, was closed
	s.Require().NoError(firstFile.Close(), "unexpected error closing file")

	_, err = secondFile.Read(expectedSlice)
	s.Require().NoError(err, "unexpected read error")

	s.Equal(expectedText, string(expectedSlice))
}

//nolint:staticcheck // deprecated method test
func (s *memLocationTest) TestChangeDir() {
	newFile, nerr := s.fileSystem.NewFile("", "/dir/to/change/change.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")

	s.Require().NoError(newFile.Touch(), "unexpected error touching file")
	loc := newFile.Location()

	// changing directory
	s.Require().NoError(loc.ChangeDir("extraDir/"), "unexpected error while changing directory")
	exists, eerr := loc.Exists()
	s.Require().NoError(eerr, "unexpected error checking for Existence")
	s.True(exists)
	s.NotEqual(newFile.Location().Path(), loc.Path())
}

//nolint:staticcheck // deprecated method test
func (s *memLocationTest) TestVolume() {
	newFile, nerr := s.fileSystem.NewFile("D:", "/path/to/file/example.txt")
	s.Require().NoError(nerr, "unexpected error creating a file")
	s.Require().NoError(newFile.Touch(), "unexpected error touching file")
	s.Require().NoError(newFile.Close(), "unexpected error closing file")
	// For Unix, this returns an empty string. For windows, it would be something like 'C:'
	s.Equal("D", newFile.Location().Volume())
}

// TestPath makes sure that locations return the correct paths, along with leading and trailing slashes
func (s *memLocationTest) TestPath() {
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")

	s.Require().NoError(file.Touch(), "unexpected error touching file")
	location := file.Location()
	s.Equal("/some/file/", location.Path())
	// deleting file
	s.Require().NoError(file.Delete(), "Delete failed unexpectedly")
}

// TestURI ensures that URI's for locations come out in the correct format
func (s *memLocationTest) TestURI() {
	file, nerr := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")
	s.Require().NoError(file.Touch(), "unexpected error touching file")
	location := file.Location()
	_, eerr := location.Exists()
	s.Require().NoError(eerr, "unexpected error checking for existence")
	expected := "mem:///some/file/"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
	s.Require().NoError(file.Delete(), "delete failed unexpectedly")
}

// TestStringer tests the implementation of io.Stringer
func (s *memLocationTest) TestStringer() {
	file, nerr := s.fileSystem.NewFile("C", "/some/file/test.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")

	s.Require().NoError(file.Touch(), "unexpected error touching file")
	location := file.Location()
	expected := "mem://C/some/file/"
	s.Equal(expected, location.String(), "%s does not match %s", location.String(), expected)
	// deleting file
	s.Require().NoError(file.Delete(), "delete failed unexpectedly")
}

// TestDeleteFile makes files, writes to them, deletes them, all while asserting things like existence and errors
func (s *memLocationTest) TestDeleteFile() {
	newFile, err := s.fileSystem.NewFile("", "/home/bar.txt")
	s.Require().NoError(err, "unexpected error creating a new file")
	// attempt to delete newFile
	s.Require().Error(newFile.Delete(), "expected existence error") // expected an error since newFile does not yet exist
	s.Require().NoError(newFile.Touch(), "unexpected error touching file")

	otherFile, nerr := s.fileSystem.NewFile("", "/foo.txt")
	s.Require().NoError(nerr, "unexpected error creating a new file")

	// attempt to delete otherFile
	s.Require().Error(otherFile.Location().DeleteFile(otherFile.Name()), "expected existence error") // want to catch the delete error
	s.Require().NoError(otherFile.Touch(), "unexpected error touching file")                         // bring it to existence with a touch
	existence, eerr := otherFile.Exists()
	s.True(existence)
	s.Require().NoError(eerr, "unexpected existence error")

	// deleting otherFile
	s.Require().NoError(otherFile.Location().DeleteFile(otherFile.Name()), "unexpected existence error")
	existence1, eerr1 := otherFile.Exists()
	s.False(existence1)
	s.Require().NoError(eerr1, "unexpected existence error")
}

// TestWriteExistingFile tests that initializing a pre-existing file from a location, using a relative path will not result
// in a blank file
func (s *memLocationTest) TestWriteExistingFile() {
	newFile, err := s.fileSystem.NewFile("", "/path/to/file/bar.txt")
	s.Require().NoError(err, "unexpected error creating a new file")

	_, err = newFile.Write([]byte("hello world"))
	s.Require().NoError(err)
	s.Require().NoError(newFile.Close())

	location, err := s.fileSystem.NewLocation("", "/path/")
	s.Require().NoError(err)

	file, err := location.NewFile("to/file/bar.txt")
	s.Require().NoError(err)

	exists, err := file.Exists()
	s.Require().NoError(err)
	s.True(exists)

	data, err := io.ReadAll(file)
	s.Require().NoError(err)
	s.Equal("hello world", string(data))
}

func TestMemLocation(t *testing.T) {
	suite.Run(t, new(memLocationTest))
}
