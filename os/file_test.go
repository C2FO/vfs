package os

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"fmt"
	"github.com/c2fo/vfs"
	"github.com/c2fo/vfs/mocks"
)

/**********************************
 ************TESTS*****************
 **********************************/

type osFileTest struct {
	suite.Suite
	testFile   vfs.File
	fileSystem FileSystem
}

func (s *osFileTest) SetupTest() {
	fs := FileSystem{}
	file, err := fs.NewFile("", "test_files/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file
	s.fileSystem = fs
}

func (s *osFileTest) TeardownTest() {
	err := s.testFile.Close()
	assert.NoError(s.T(), err, "close error not expected")
}

func (s *osFileTest) TestExists() {
	doesExist, err := s.testFile.Exists()
	if err != nil {
		s.Fail("Failed to check for file existance")
	}
	s.True(doesExist)

	otherFile, err := s.fileSystem.NewFile("", "test_files/foo.txt")

	if err != nil {
		s.Fail("Failed to check for file existance")
	}

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

	if err != nil {
		s.Fail(err.Error())
	}

	otherFs.AssertCalled(s.T(), "NewFile", "", "/some/path/test.txt")
	otherFile.AssertExpectations(s.T())
	otherFile.AssertCalled(s.T(), "Write", []uint8(expectedText))
}

func (s *osFileTest) TestCopyToFile() {
	expectedText := "hello world"
	otherFs := new(mocks.FileSystem)
	otherFile := new(mocks.File)

	location := Location{"/some/path", otherFs}

	// Expected behavior
	otherFile.On("Write", mock.Anything).Return(len(expectedText), nil)
	otherFile.On("Close").Return(nil)
	otherFile.On("Name").Return("other.txt")
	otherFile.On("Location").Return(vfs.Location(&location))

	otherFs.On("NewFile", mock.Anything, mock.Anything).Return(otherFile, nil)

	err := s.testFile.CopyToFile(otherFile)

	if err != nil {
		s.Fail(err.Error())
	}

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

	emptyFile, err := s.fileSystem.NewFile("", "test_files/empty.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	err = emptyFile.CopyToFile(otherFile)

	if err != nil {
		s.Fail(err.Error())
	}

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
	dir, terr := ioutil.TempDir("test_files", "example")
	if terr != nil {
		s.Fail(terr.Error())
	}

	origFileName := "test_files/move.txt"
	file, nerr := s.fileSystem.NewFile("", origFileName)
	if nerr != nil {
		s.Fail(nerr.Error())
	}

	defer func() {
		err := os.RemoveAll(dir)
		assert.NoError(s.T(), err, "remove all error not expected")
	}()

	defer func() {
		err := file.Delete()
		assert.NoError(s.T(), err, "delete error not expected")
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
	if err != nil {
		s.Fail(err.Error())
	}

	s.Equal(movedFile.Location().Path(), location.Path())

	//ensure the original file no longer exists
	origFile, _ := s.fileSystem.NewFile(file.Location().Volume(), origFileName)
	origFound, eerr := origFile.Exists()
	assert.NoError(s.T(), eerr, "exists error not expected")
	s.False(origFound)
}

func (s *osFileTest) TestMoveToFile() {
	dir, terr := ioutil.TempDir("test_files", "example")
	if terr != nil {
		s.Fail(terr.Error())
	}

	file1, _ := s.fileSystem.NewFile("", filepath.Join(dir, "original.txt"))
	file2, _ := s.fileSystem.NewFile("", filepath.Join(dir, "move.txt"))

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
	if merr != nil {
		s.Fail(merr.Error())
	}

	f1Exists, _ := file1.Exists()
	f2Exists, _ := file2.Exists()
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
	file, _ := s.fileSystem.NewFile("", "test_files/new.txt")

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

	err := file.Delete()
	if err != nil {
		s.Fail("File was not deleted properly")
	}

	found2, eErr2 := file.Exists()
	assert.NoError(s.T(), eErr2, "exists error not expected")
	s.False(found2)
}

func (s *osFileTest) TestLastModified() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")

	lastModified, err := file.LastModified()
	if err != nil {
		s.Fail(err.Error())
	}
	osStats, err := os.Stat("test_files/test.txt")

	if err != nil {
		s.Fail(err.Error())
	}
	s.NotNil(lastModified)
	s.Equal(osStats.ModTime(), *lastModified)
}

func (s *osFileTest) TestName() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")
	s.Equal("test.txt", file.Name())
}

func (s *osFileTest) TestSize() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")

	size, err := file.Size()
	if err != nil {
		s.Fail(err.Error())
	}

	osStats, err := os.Stat("test_files/test.txt")

	if err != nil {
		s.Fail(err.Error())
	}
	s.NotNil(size)
	s.Equal(osStats.Size(), int64(size))
}

func (s *osFileTest) TestPath() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")
	s.Equal(filepath.Join(file.Location().Path(), file.Name()), file.Path())
}

func (s *osFileTest) TestURI() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	expected := "file:///some/file/test.txt"
	s.Equal(expected, file.URI(), "%s does not match %s", file.URI(), expected)
}

func (s *osFileTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Equal("file:///some/file/test.txt", fmt.Sprintf("%s", file))
}

func TestOSFile(t *testing.T) {
	suite.Run(t, new(osFileTest))
	_ = os.Remove("test_files/new.txt")
}
