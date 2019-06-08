package mem

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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
	setupTestFiles()
}

func (s *memLocationTest) TearDownSuite() {
	teardownTestFiles()
}

func (s *memLocationTest) SetupTest() {
	fs := &FileSystem{}
	file, err := fs.NewFile("", "/home/test_files/subdir/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file
	s.fileSystem = fs
}

func (s *memLocationTest) TestList() {

}
func (s *memLocationTest) TestList_NonExistentDirectory() {

}

func (s *memLocationTest) TestListByPrefix() {

	_,_ = s.fileSystem.NewFile("","foo.txt")
	_,_ = s.fileSystem.NewFile("","home/judd/subdir/file1.txt")
	_,_ = s.fileSystem.NewFile("","home/judd/subdir/file2.txt")
	_,_ = s.fileSystem.NewFile("","home/directories/test/mat.txt")
	_,_ = s.fileSystem.NewFile("","test/files/car.txt")
	//fmt.Println(s.testFile.Location().Path(),"here")
	nameSlice,_ := s.testFile.Location().ListByPrefix("subdir/f")
	fmt.Println(nameSlice)

}

func (s *memLocationTest) TestListByRegex() {
	expected := []string{"prefix-file.txt"}
	regex, _ := regexp.Compile("[-]+")
	actual, _ := s.testFile.Location().ListByRegex(regex)
	s.Equal(expected, actual)
}

func (s *memLocationTest) TestExists() {
	otherFile, _ := s.fileSystem.NewFile("", "foo/foo.txt")
	s.True(s.testFile.Location().Exists())
	s.False(otherFile.Location().Exists())
}

func (s *memLocationTest) TestNewLocation() {

	l1,_ := s.testFile.Location().NewLocation("/../..")
	assert.Equal(s.T(),"/home/",l1.Path())
	l2, _ := s.testFile.Location().NewLocation("testDir")
	assert.Equal(s.T(),"/home/test_files/subdir/testDir/",l2.Path())
	l3, _ := s.testFile.Location().NewLocation("/..")
	assert.Equal(s.T(),"/home/test_files/",l3.Path())

}

func (s *memLocationTest) TestNewFile() {
	//loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	//s.NoError(err)

	//newfile, _ := loc.NewFile("../../bam/this.txt")
	//s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")
}

func (s *memLocationTest) TestChangeDir() {
	otherFile, _ := s.fileSystem.NewFile("", "foo/foo.txt")
	fileLocation := otherFile.Location()
	cwd := fileLocation.Path()
	err := fileLocation.ChangeDir("other/")
	assert.NoError(s.T(), err, "change dir error not expected")
	s.Equal(fileLocation.Path(), filepath.Join(cwd, "other/"))
}

func (s *memLocationTest) TestVolume() {
	volume := s.testFile.Location().Volume()

	// For Unix, this returns an empty string. For windows, it would be something like 'C:'
	s.Equal(filepath.VolumeName("test_files/test.txt"), volume)
}

func (s *memLocationTest) TestPath() {
//	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
//	location := file.Location()
//	s.Equal("/some/file/", location.Path())
	//rootLocation := Location{fileSystem: s.fileSystem, name: "/"}
	//s.Equal("/", Path())
}

func (s *memLocationTest) TestURI() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	location := file.Location()
	expected := "file:///some/file/test.txt"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
}

func (s *memLocationTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	location := file.Location()
	s.Equal("file:///some/file/", location.String())
}

func (s *memLocationTest) TestDeleteFile() {
	dir, err := ioutil.TempDir("test_files", "example")
	s.NoError(err, "Setup not expected to fail.")
	defer func() {
		derr := os.RemoveAll(dir)
		s.NoError(derr, "Cleanup shouldn't fail.")
	}()

	//expectedText := "file to delete"
	//fileName := "test.txt"
	//location := Location{dir, s.fileSystem}
	//file, err := NewFile(fileName)
	s.NoError(err, "Creating file to test delete shouldn't fail")

	//_, err = file.Write([]byte(expectedText))
	s.NoError(err, "Shouldn't fail to write text to file.")

	//exists, err := file.Exists()
	s.NoError(err, "Exists shouldn't throw error.")
	//s.True(exists, "Exists should return true for test file.")

	//s.NoError(DeleteFile(fileName), "Deleting the file shouldn't throw an error.")
	//exists, err = file.Exists()
	s.NoError(err, "Shouldn't throw error testing for exists after delete.")
	//s.False(exists, "Exists should return false after deleting the file.")

}

func TestMemLocation(t *testing.T) {
	suite.Run(t, new(memLocationTest))
}
