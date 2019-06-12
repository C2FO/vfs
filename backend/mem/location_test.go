package mem

import (
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
	WriteZeroBytes(s.testFile)

}

func (s *memLocationTest) TestList() {

}
func (s *memLocationTest) TestList_NonExistentDirectory() {

}

func (s *memLocationTest) TestListByPrefix() {

	_,_ = s.fileSystem.NewFile("","foo.txt")
	_,_ = s.fileSystem.NewFile("","/home/test_files/subdir/file1.txt")
	_,_ = s.fileSystem.NewFile("","/home/test_files/subdir/file2.txt")
	_,_ = s.fileSystem.NewFile("","/home/directories/test/mat.txt")
	_,_ = s.fileSystem.NewFile("","/test/files/car.txt")
	//fmt.Println(s.testFile.Location().Path(),"here")
	nameSlice,_ := s.testFile.Location().ListByPrefix("f")
	expectedSlice := make([]string,2)
	expectedSlice[0] = "file1.txt"
	expectedSlice[1] = "file2.txt"
	assert.ObjectsAreEqual(expectedSlice,nameSlice)

	emptySlice,_ := s.testFile.Location().ListByPrefix("m")
	assert.ObjectsAreEqual(make([]string,0),emptySlice)		//no files should be found with this prefix at this location
}


func (s *memLocationTest) TestListByRegex() {

}

func (s *memLocationTest) TestExists() {
	otherFile, _ := s.fileSystem.NewFile("", "foo/foo.txt")
	s.True(s.testFile.Location().Exists())
	s.False(otherFile.Location().Exists())
}

func (s *memLocationTest) TestNewLocation() {

	otherFile, _ := s.fileSystem.NewFile("", "/foo/foo.txt")
	fileLocation := otherFile.Location()
	subDir, _ := fileLocation.NewLocation("other/")
	s.Equal("/foo/other/", subDir.Path())

	relDir, _ := subDir.NewLocation("../../bar/")
	s.Equal("/bar/", relDir.Path(), "relative dot path works")
/*
	l1,_ := s.testFile.Location().NewLocation("/../..")
	assert.Equal(s.T(),"/home/",l1.Path())
	l2, _ := s.testFile.Location().NewLocation("testDir")
	assert.Equal(s.T(),"/home/test_files/subdir/testDir/",l2.Path())
	l3, _ := s.testFile.Location().NewLocation("/..")
	assert.Equal(s.T(),"/home/test_files/",l3.Path())


 */
}

func (s *memLocationTest) TestNewFile() {
	loc, err := s.fileSystem.NewLocation("", "/foo/bar/baz/")
	s.NoError(err)

	newfile, _ := loc.NewFile("../../bam/this.txt")
	s.Equal("/foo/bam/this.txt", newfile.Path(), "relative dot path works")
}

func (s *memLocationTest) TestChangeDir() {

}

func (s *memLocationTest) TestVolume() {
	volume := s.testFile.Location().Volume()

	// For Unix, this returns an empty string. For windows, it would be something like 'C:'
	s.Equal("",volume)
}

func (s *memLocationTest) TestPath() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	location := file.Location()
	s.Equal("/some/file/", location.Path())
}

func (s *memLocationTest) TestURI() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	WriteZeroBytes(file)
	location := file.Location()
	expected := "mem:///some/file/"
	s.Equal(expected, location.URI(), "%s does not match %s", location.URI(), expected)
}

func (s *memLocationTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	WriteZeroBytes(file)
	location := file.Location()
	expected := "mem:///some/file/"
	s.Equal(expected, location.String(), "%s does not match %s", location.String(), expected)

}

func (s *memLocationTest) TestDeleteFile() {

	newFile,err := s.fileSystem.NewFile("","home/bar.txt")
	assert.NoError(s.T(),err,"Unexpected creation error")
	derr1:=newFile.Delete()
	assert.Error(s.T(),derr1,DoesNotExist())
	WriteZeroBytes(newFile)
	otherFile,_ := s.fileSystem.NewFile("","foo.txt")
	derr2:=otherFile.Delete()
	assert.Error(s.T(),derr2,DoesNotExist())
	WriteZeroBytes(otherFile)
	existence,eerr := otherFile.Exists()
	s.True(existence)
	assert.NoError(s.T(),eerr,DoesNotExist())
	derr3:=otherFile.Location().DeleteFile(otherFile.Name())
	assert.NoError(s.T(),derr3,DeleteError())
	existence1,eerr1 := otherFile.Exists()
	s.False(existence1)
	assert.Error(s.T(), eerr1, DoesNotExist())
	s.True(systemMap["/foo.txt"]==nil)

}

func TestMemLocation(t *testing.T) {
	suite.Run(t, new(memLocationTest))
}
