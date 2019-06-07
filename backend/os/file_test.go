package os

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/mocks"
	"github.com/c2fo/vfs/v4/utils"
)

/**********************************
 ************TESTS*****************
 **********************************/

type osFileTest struct {
	suite.Suite
	testFile   vfs.File
	fileSystem *FileSystem
	tmploc     vfs.Location
}

func (s *osFileTest) SetupSuite() {
	fs := &FileSystem{}
	s.fileSystem = fs
	dir, err := ioutil.TempDir("", "os_file_test")
	dir = utils.EnsureTrailingSlash(dir)
	s.NoError(err)
	s.tmploc, err = fs.NewLocation("", dir)
	s.NoError(err)
	setupTestFiles(s.tmploc)
}

func (s *osFileTest) TearDownSuite() {
	teardownTestFiles(s.tmploc)
}

func (s *osFileTest) SetupTest() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.NoError(err, "No file was opened")
	s.testFile = file
}

func (s *osFileTest) TeardownTest() {
	err := s.testFile.Close()
	assert.NoError(s.T(), err, "close error not expected")
}

func (s *osFileTest) TestExists() {
	doesExist, err := s.testFile.Exists()
	s.NoError(err, "Failed to check for file existence")
	s.True(doesExist)

	otherFile, err := s.tmploc.NewFile("test_files/foo.txt")
	s.NoError(err, "Failed to check for file existence")

	otherFileExists, _ := otherFile.Exists()
	s.False(otherFileExists)
}

func (s *osFileTest) TestOpenFile() {
	expectedText := "hello world"
	data := make([]byte, len(expectedText))
	_, err := s.testFile.Read(data)
	assert.NoError(s.T(), err, "read error not expected")

	s.Equal(expectedText, string(data))
}

func (s *osFileTest) TestSeek() {
	expectedText := "world"
	data := make([]byte, len(expectedText))
	_, serr := s.testFile.Seek(6, 0)
	assert.NoError(s.T(), serr, "seek error not expected")
	_, rerr := s.testFile.Read(data)
	assert.NoError(s.T(), rerr, "read error not expected")
	s.Equal(expectedText, string(data))
}

func (s *osFileTest) TestCopyToLocation() {
	expectedText := "hello world"
	otherFs := new(mocks.FileSystem)
	otherFile := new(mocks.File)

	// Expected behavior
	otherFile.On("Write", mock.Anything).Return(len(expectedText), nil)
	otherFile.On("Close").Return(nil)
	otherFs.On("NewFile", mock.Anything, mock.Anything).Return(otherFile, nil)

	location := Location{"/some/path", otherFs}

	_, err := s.testFile.CopyToLocation(&location)
	s.NoError(err)

	otherFs.AssertCalled(s.T(), "NewFile", "", "/some/path/test.txt")
	otherFile.AssertExpectations(s.T())
	otherFile.AssertCalled(s.T(), "Write", []uint8(expectedText))
}

func (s *osFileTest) TestCopyToFile() {
	expectedText := "hello world"
	otherFs := &mocks.FileSystem{}
	otherFile := new(mocks.File)

	location := Location{"/some/path", otherFs}

	// Expected behavior
	otherFile.On("Write", mock.Anything).Return(len(expectedText), nil)
	otherFile.On("Close").Return(nil)
	otherFile.On("Name").Return("other.txt")
	otherFile.On("Location").Return(vfs.Location(&location))

	otherFs.On("NewFile", mock.Anything, mock.Anything).Return(otherFile, nil)

	err := s.testFile.CopyToFile(otherFile)
	s.NoError(err)

	otherFs.AssertCalled(s.T(), "NewFile", "", "/some/path/other.txt")
	otherFile.AssertExpectations(s.T())
	otherFile.AssertCalled(s.T(), "Write", []uint8(expectedText))
}

func (s *osFileTest) TestEmptyCopyToFile() {
	expectedText := ""
	otherFs := new(mocks.FileSystem)
	otherFile := new(mocks.File)

	location := Location{"/some/path", otherFs}

	// Expected behavior
	otherFile.On("Write", mock.Anything).Return(len(expectedText), nil)
	otherFile.On("Close").Return(nil)
	otherFile.On("Name").Return("other.txt")
	otherFile.On("Location").Return(vfs.Location(&location))

	otherFs.On("NewFile", mock.Anything, mock.Anything).Return(otherFile, nil)

	emptyFile, err := s.tmploc.NewFile("test_files/empty.txt")
	s.NoError(err, "No file was opened")

	err = emptyFile.CopyToFile(otherFile)
	s.NoError(err)

	otherFs.AssertCalled(s.T(), "NewFile", "", "/some/path/other.txt")
	otherFile.AssertExpectations(s.T())
	otherFile.AssertCalled(s.T(), "Write", []uint8(expectedText))
}

func (s *osFileTest) TestCopyToLocationIgnoreExtraSeparator() {
	expectedText := "hello world"
	otherFs := new(mocks.FileSystem)
	otherFile := new(mocks.File)

	// Expected behavior
	otherFile.On("Write", mock.Anything).Return(len(expectedText), nil)
	otherFile.On("Close").Return(nil)
	otherFs.On("NewFile", mock.Anything, mock.Anything).Return(otherFile, nil)

	// Add trailing slash
	location := Location{"/some/path/", otherFs}

	_, err := s.testFile.CopyToLocation(&location)

	if err != nil {
		s.Fail(err.Error())
	}

	otherFs.AssertCalled(s.T(), "NewFile", "", "/some/path/test.txt")
}

func (s *osFileTest) TestMoveToLocation() {
	expectedText := "moved file"
	dir, terr := ioutil.TempDir(path.Join(s.tmploc.Path(), "test_files"), "example")
	s.NoError(terr)

	origFileName := path.Join(dir, "test_files/move.txt")
	file, nerr := s.fileSystem.NewFile("", origFileName)
	s.NoError(nerr)

	defer func() {
		err := os.RemoveAll(dir)
		assert.NoError(s.T(), err, "remove all error not expected")
	}()

	_, werr := file.Write([]byte(expectedText))
	assert.NoError(s.T(), werr, "write error not expected")

	cerr := file.Close()
	assert.NoError(s.T(), cerr, "close error not expected")

	found, eerr := file.Exists()
	assert.NoError(s.T(), eerr, "exists error not expected")
	s.True(found)

	//setup location
	location := Location{dir, s.fileSystem}

	//move the file to new location
	movedFile, err := file.MoveToLocation(&location)
	s.NoError(err)

	s.Equal(location.Path(), movedFile.Location().Path(), "ensure file location changed")

	//ensure the original file no longer exists
	origFile, _ := s.fileSystem.NewFile(file.Location().Volume(), origFileName)
	origFound, eerr := origFile.Exists()
	assert.NoError(s.T(), eerr, "exists error not expected")
	s.False(origFound)
}

func (s *osFileTest) TestMoveToFile() {
	dir, terr := ioutil.TempDir(path.Join(s.tmploc.Path(), "test_files"), "example")
	s.NoError(terr)

	file1, err := s.fileSystem.NewFile("", filepath.Join(dir, "original.txt"))
	s.NoError(err)

	file2, err := s.fileSystem.NewFile("", filepath.Join(dir, "move.txt"))
	s.NoError(err)

	defer func() {
		err := os.RemoveAll(dir)
		assert.NoError(s.T(), err, "remove all error not expected")
	}()

	text := "original file"
	_, werr := file1.Write([]byte(text))
	assert.NoError(s.T(), werr, "write error not expected")
	cerr := file1.Close()
	assert.NoError(s.T(), cerr, "close error not expected")

	found1, eErr1 := file1.Exists()
	s.True(found1)
	assert.NoError(s.T(), eErr1, "exists error not expected")

	found2, eErr2 := file2.Exists()
	s.False(found2)
	assert.NoError(s.T(), eErr2, "exists error not expected")

	merr := file1.MoveToFile(file2)
	s.NoError(merr)

	f1Exists, err := file1.Exists()
	s.NoError(err)
	f2Exists, err := file2.Exists()
	s.NoError(err)
	s.False(f1Exists)
	s.True(f2Exists)

	data := make([]byte, len(text))
	_, rerr := file2.Read(data)
	assert.NoError(s.T(), rerr, "read error not expected")
	cErr := file2.Close()
	assert.NoError(s.T(), cErr, "close error not expected")

	s.Equal(text, string(data))
}

func (s *osFileTest) TestWrite() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, err := s.tmploc.NewFile("test_files/new.txt")
	s.NoError(err)

	_, werr := file.Write([]byte(expectedText))
	assert.NoError(s.T(), werr, "write error not expected")

	_, serr := file.Seek(0, 0)
	assert.NoError(s.T(), serr, "seek error not expected")
	_, rerr := file.Read(data)
	assert.NoError(s.T(), rerr, "read error not expected")
	cerr := file.Close()
	assert.NoError(s.T(), cerr, "close error not expected")

	s.Equal(expectedText, string(data))

	found, eErr := file.Exists()
	assert.NoError(s.T(), eErr, "exists error not expected")
	s.True(found)

	err = file.Delete()
	s.NoError(err, "File was not deleted properly")

	found2, eErr2 := file.Exists()
	assert.NoError(s.T(), eErr2, "exists error not expected")
	s.False(found2)
}

func (s *osFileTest) TestLastModified() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.NoError(err)

	lastModified, err := file.LastModified()
	s.NoError(err)
	osStats, err := os.Stat(path.Join(s.tmploc.Path(), "test_files/test.txt"))
	s.NoError(err)
	s.NotNil(lastModified)
	s.Equal(osStats.ModTime(), *lastModified)
}

func (s *osFileTest) TestName() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.NoError(err)
	s.Equal("test.txt", file.Name())
}

func (s *osFileTest) TestSize() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.NoError(err)

	size, err := file.Size()
	s.NoError(err)

	osStats, err := os.Stat(path.Join(s.tmploc.Path(), "test_files/test.txt"))
	s.NoError(err)
	s.NotNil(size)
	s.Equal(osStats.Size(), int64(size))
}

func (s *osFileTest) TestPath() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.NoError(err)
	s.Equal(filepath.Join(file.Location().Path(), file.Name()), file.Path())
}

func (s *osFileTest) TestURI() {
	file, err := s.tmploc.NewFile("some/file/test.txt")
	s.NoError(err)
	expected := fmt.Sprintf("file://%s", path.Join(s.tmploc.Path(), "some/file/test.txt"))
	s.Equal(expected, file.URI(), "%s does not match %s", file.URI(), expected)
}

func (s *osFileTest) TestStringer() {
	file, err := s.tmploc.NewFile("some/file/test.txt")
	s.NoError(err)
	s.Equal(fmt.Sprintf("file://%s", path.Join(s.tmploc.Path(), "some/file/test.txt")), file.String())
}

func TestOSFile(t *testing.T) {
	suite.Run(t, new(osFileTest))
	_ = os.Remove("test_files/new.txt")
}

/*
	Setup TEST FILES
*/
func setupTestFiles(baseLoc vfs.Location) {

	// setup "test_files" dir
	createDir(baseLoc, "test_files")

	// setup "test_files/test.txt"
	writeStringFile(baseLoc, "test_files/empty.txt", ``)

	// setup "test_files/test.txt"
	writeStringFile(baseLoc, "test_files/prefix-file.txt", `hello, Dave`)

	// setup "test_files/test.txt"
	writeStringFile(baseLoc, "test_files/test.txt", `hello world`)

	// setup "test_files/subdir" dir
	createDir(baseLoc, "test_files/subdir")

	// setup "test_files/subdir/test.txt"
	writeStringFile(baseLoc, "test_files/subdir/test.txt", `hello world too`)
}

func teardownTestFiles(baseLoc vfs.Location) {
	err := os.RemoveAll(baseLoc.Path())
	if err != nil {
		panic(err)
	}
}

func createDir(baseLoc vfs.Location, dirname string) {
	dir := path.Join(baseLoc.Path(), dirname)
	perm := os.FileMode(0755)
	err := os.Mkdir(dir, perm)
	if err != nil {
		teardownTestFiles(baseLoc)
		panic(err)
	}
}

func writeStringFile(baseLoc vfs.Location, filename, data string) {
	file := path.Join(baseLoc.Path(), filename)
	f, err := os.Create(file)
	if err != nil {
		teardownTestFiles(baseLoc)
		panic(err)
	}
	_, err = f.WriteString(data)
	if err != nil {
		teardownTestFiles(baseLoc)
		panic(err)
	}
	err = f.Close()
	if err != nil {
		teardownTestFiles(baseLoc)
		panic(err)
	}
}
