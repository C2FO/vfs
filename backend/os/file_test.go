package os

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/mocks"
	"github.com/c2fo/vfs/v6/utils"
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
	dir, err := os.MkdirTemp("", "os_file_test")
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
	s.NoError(err, "close error not expected")
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

func (s *osFileTest) TestTouch() {

	// set up testfile
	testfile, err := s.tmploc.NewFile("test_files/foo.txt")
	s.NoError(err)

	// testfile should NOT exist
	exists, err := testfile.Exists()
	s.NoError(err)
	s.False(exists)

	// touch file
	err = testfile.Touch()
	s.NoError(err)

	// testfile SHOULD exist
	exists, err = testfile.Exists()
	s.NoError(err)
	s.True(exists)

	// size should be zero
	size, err := testfile.Size()
	s.NoError(err)
	s.Zero(size, "size should be zero")

	// capture last_modified
	firstModTime, err := testfile.LastModified()
	s.NoError(err)

	// touch again
	err = testfile.Touch()
	s.NoError(err)

	// size should still be zero
	size, err = testfile.Size()
	s.NoError(err)
	s.Zero(size, "size should be zero")

	// LastModified should be later than previous LastModified
	nextModTime, err := testfile.LastModified()
	s.NoError(err)
	s.True(firstModTime.Before(*nextModTime), "Last Modified was updated")
	s.NoError(testfile.Close())
}

func (s *osFileTest) TestOpenFile() {
	expectedText := "hello world"
	data := make([]byte, len(expectedText))
	_, err := s.testFile.Read(data)
	s.NoError(err, "read error not expected")

	s.Equal(expectedText, string(data))
}

func (s *osFileTest) TestRead() {
	// fail on nonexistent file
	noFile, err := s.tmploc.NewFile("test_files/nonexistent.txt")
	s.NoError(err)
	data := make([]byte, 0)
	_, err = noFile.Read(data)
	s.Error(err, "error trying to read nonexistent file")

	// setup file for reading
	f, err := s.tmploc.NewFile("test_files/readFile.txt")
	s.NoError(err)
	b, err := f.Write([]byte("blah"))
	s.NoError(err)
	s.Equal(4, b)
	s.NoError(f.Close())

	// read from file
	data = make([]byte, 4)
	b, err = f.Read(data)
	s.NoError(err)
	s.Equal(4, b)
	s.Equal("blah", string(data))
	s.NoError(f.Close())

	// setup file for err out of opening
	f, err = s.tmploc.NewFile("test_files/readFileFail.txt")
	s.NoError(err)
	f.(*File).useTempFile = true
	f.(*File).fileOpener = func(filePath string) (*os.File, error) { return nil, errors.New("bad opener") }
	data = make([]byte, 4)
	b, err = f.Read(data)
	s.Error(err)
	s.Zero(b)

	f.(*File).fileOpener = nil
	b, err = f.Write([]byte("blah"))
	s.NoError(err)
	s.Equal(4, b)
	s.NoError(f.Close())
	f.(*File).fileOpener = func(filePath string) (*os.File, error) { return nil, errors.New("bad opener") }
	data = make([]byte, 4)
	b, err = f.Read(data)
	s.Error(err)
	s.Zero(b)
}

func (s *osFileTest) TestSeek() {
	expectedText := "world"
	data := make([]byte, len(expectedText))
	_, serr := s.testFile.Seek(6, 0)
	s.NoError(serr, "seek error not expected")
	_, rerr := s.testFile.Read(data)
	s.NoError(rerr, "read error not expected")
	s.Equal(expectedText, string(data))
	s.NoError(s.testFile.Close())

	// setup file for err out of opening
	f, err := s.tmploc.NewFile("test_files/seekFileFail.txt")
	s.NoError(err)
	f.(*File).useTempFile = true
	f.(*File).fileOpener = func(filePath string) (*os.File, error) { return nil, errors.New("bad opener") }
	_, err = f.Seek(0, 4)
	s.Error(err)
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
	s.Require().NoError(err)

	otherFs.AssertCalled(s.T(), "NewFile", "", "/some/path/test.txt")
}

func (s *osFileTest) TestMoveToLocation() {
	expectedText := "moved file"
	dir, terr := os.MkdirTemp(path.Join(s.tmploc.Path(), "test_files"), "example")
	s.NoError(terr)

	origFileName := path.Join(dir, "test_files/move.txt")
	file, nerr := s.fileSystem.NewFile("", origFileName)
	s.NoError(nerr)

	defer func() {
		err := os.RemoveAll(dir)
		s.NoError(err, "remove all error not expected")
	}()

	_, werr := file.Write([]byte(expectedText))
	s.NoError(werr, "write error not expected")

	cerr := file.Close()
	s.NoError(cerr, "close error not expected")

	found, eerr := file.Exists()
	s.NoError(eerr, "exists error not expected")
	s.True(found)

	// setup location
	location := Location{dir, s.fileSystem}

	// move the file to new location
	movedFile, err := file.MoveToLocation(&location)
	s.NoError(err)

	s.Equal(location.Path(), movedFile.Location().Path(), "ensure file location changed")

	// ensure the original file no longer exists
	origFile, _ := s.fileSystem.NewFile(file.Location().Volume(), origFileName)
	origFound, eerr := origFile.Exists()
	s.NoError(eerr, "exists error not expected")
	s.False(origFound)

	// test non-scheme MoveToLocation
	mockLocation := new(mocks.Location)
	mockfs := new(mocks.FileSystem)

	// Expected behavior
	mockfs.On("Scheme").Return("mock")
	fsMockFile := new(mocks.File)
	fsMockFile.On("Write", mock.Anything).Return(10, nil)
	fsMockFile.On("Close").Return(nil)
	mockfs.On("NewFile", mock.Anything, mock.Anything).Return(fsMockFile, nil)
	mockLocation.On("FileSystem").Return(mockfs)
	mockLocation.On("Volume").Return("")
	mockLocation.On("Path").Return("/some/path/to/")
	mockLocation.On("Close").Return(nil)
	mockFile := new(mocks.File)
	mockFile.On("Location").Return(mockLocation, nil)
	mockFile.On("Name").Return("/some/path/to/move.txt")
	mockFile.On("Location").Return(mockLocation, nil)
	mockLocation.On("NewFile", mock.Anything).Return(mockFile, nil)
	mockfs.On("NewLocation", mock.Anything, mock.Anything).Return(mockLocation)

	_, err = movedFile.MoveToLocation(mockLocation)
	s.NoError(err)
}

func (s *osFileTest) TestSafeOsRename() {
	dir, err := os.MkdirTemp(path.Join(s.tmploc.Path(), "test_files"), "example")
	s.NoError(err)
	defer func() {
		err := os.RemoveAll(dir)
		s.NoError(err, "remove all error not expected")
	}()

	// TODO: I haven't figured out a way to test safeOsRename since setting up the scenario is
	//     very os-dependent so here I will actually only ensure non-"invalid cross-device link"
	//     errors are handled correctly and that normal renames work.

	// test that normal rename works
	testfile := path.Join(dir, "original.txt")
	file1, err := s.fileSystem.NewFile("", testfile)
	s.NoError(err)
	var testBytes = []byte("test me")
	_, err = file1.Write(testBytes)
	s.NoError(err)
	s.NoError(file1.Close())

	newFile := path.Join(dir, "new.txt")
	s.NoError(safeOsRename(testfile, newFile))

	exists, err := file1.Exists()
	s.NoError(err)
	s.False(exists)
	file2, err := s.fileSystem.NewFile("", newFile)
	s.NoError(err)
	exists, err = file2.Exists()
	s.NoError(err)
	s.True(exists)

	// test that a rename failure (non-"invalid cross-device link" error) is returned properly
	badfile := path.Join(dir, "this_should_not_exist.txt")

	err = safeOsRename(badfile, newFile)
	s.Error(err)
	s.NotEqual(osCrossDeviceLinkError, err.Error())
}

func (s *osFileTest) TestOsCopy() {
	dir, err := os.MkdirTemp(path.Join(s.tmploc.Path(), "test_files"), "example")
	s.NoError(err)
	defer func() {
		err := os.RemoveAll(dir)
		s.NoError(err, "remove all error not expected")
	}()

	file1, err := s.fileSystem.NewFile("", path.Join(dir, "original.txt"))
	s.NoError(err)
	var testBytes = []byte("test me")
	_, err = file1.Write(testBytes)
	s.NoError(err)
	s.NoError(file1.Close())

	file2, err := s.fileSystem.NewFile("", path.Join(dir, "move.txt"))
	s.NoError(err)

	s.NoError(osCopy(path.Join(file1.Location().Volume(), file1.Path()), path.Join(file2.Location().Volume(), file2.Path())), "test osCopy")

	b, err := io.ReadAll(file2)
	s.NoError(err)
	s.Equal(testBytes, b, "contents match")
}

func (s *osFileTest) TestMoveToFile() {
	dir, terr := os.MkdirTemp(path.Join(s.tmploc.Path(), "test_files"), "example")
	s.NoError(terr)

	file1, err := s.fileSystem.NewFile("", path.Join(dir, "original.txt"))
	s.NoError(err)

	file2, err := s.fileSystem.NewFile("", path.Join(dir, "move.txt"))
	s.NoError(err)

	defer func() {
		err := os.RemoveAll(dir)
		s.NoError(err, "remove all error not expected")
	}()

	text := "original file"
	_, werr := file1.Write([]byte(text))
	s.NoError(werr, "write error not expected")
	cerr := file1.Close()
	s.NoError(cerr, "close error not expected")

	found1, eErr1 := file1.Exists()
	s.True(found1)
	s.NoError(eErr1, "exists error not expected")

	found2, eErr2 := file2.Exists()
	s.False(found2)
	s.NoError(eErr2, "exists error not expected")

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
	s.NoError(rerr, "read error not expected")
	cErr := file2.Close()
	s.NoError(cErr, "close error not expected")
	s.Equal(text, string(data))

	// test non-scheme MoveToFile
	mockFile := new(mocks.File)
	mockLocation := new(mocks.Location)
	mockfs := new(mocks.FileSystem)

	// Expected behavior
	mockfs.On("Scheme").Return("mock")
	fsMockFile := new(mocks.File)
	fsMockFile.On("Write", mock.Anything).Return(13, nil)
	fsMockFile.On("Close").Return(nil)
	mockfs.On("NewFile", mock.Anything, mock.Anything).Return(fsMockFile, nil)
	mockLocation.On("FileSystem").Return(mockfs)
	mockLocation.On("Volume").Return("")
	mockLocation.On("Path").Return("/some/path/to/")
	mockLocation.On("Close").Return(nil)
	mockFile.On("Location").Return(mockLocation, nil)
	mockFile.On("Name").Return("/some/path/to/file.txt")
	mockFile.On("Location").Return(mockLocation, nil)
	mockfs.On("NewLocation", mock.Anything, mock.Anything).Return(mockLocation)

	s.NoError(file2.MoveToFile(mockFile))
}

func (s *osFileTest) TestWrite() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, err := s.tmploc.NewFile("test_files/new.txt")
	s.NoError(err)

	_, werr := file.Write([]byte(expectedText))
	s.NoError(werr, "write error not expected")

	_, serr := file.Seek(0, 0)
	s.NoError(serr, "seek error not expected")
	_, rerr := file.Read(data)
	s.NoError(rerr, "read error not expected")
	cerr := file.Close()
	s.NoError(cerr, "close error not expected")

	s.Equal(expectedText, string(data))

	found, eErr := file.Exists()
	s.NoError(eErr, "exists error not expected")
	s.True(found)

	err = file.Delete()
	s.NoError(err, "File was not deleted properly")

	found2, eErr2 := file.Exists()
	s.NoError(eErr2, "exists error not expected")
	s.False(found2)

	// setup file for err out of opening
	f, err := s.tmploc.NewFile("test_files/writeFileFail.txt")
	s.NoError(err)
	s.NoError(f.Touch())
	_, err = f.Seek(0, 0)
	s.NoError(err)
	f.(*File).fileOpener = func(filePath string) (*os.File, error) { return nil, errors.New("bad opener") }
	data = make([]byte, 4)
	_, err = f.Write(data)
	s.Error(err)
}

func (s *osFileTest) TestCursor() {
	file, err := s.tmploc.NewFile("test_files/originalFile.txt")
	s.NoError(err)

	expectedText := "mary had \na little lamb\n"
	write, werr := file.Write([]byte(expectedText))
	s.NoError(werr, "write error not expected")
	s.Equal(24, write)
	s.NoError(file.Close())

	_, serr := file.Seek(5, 0) // cursor 5 - opens fd to orig file
	s.Equal(int64(5), file.(*File).cursorPos)
	s.NoError(serr)

	data := make([]byte, 3)
	sz, rerr := file.Read(data) // cursor 8 - orig file - data: "had"
	s.NoError(rerr)
	s.Equal(int64(8), file.(*File).cursorPos)
	s.Equal("had", string(data)) // orig file contents = "had"

	negsz := int64(-sz)
	_, serr2 := file.Seek(negsz, 1) // cursor 5 - orig file
	s.Equal(int64(5), file.(*File).cursorPos)
	s.NoError(serr2)

	// because seek and/or read were called before write, write is now in in-place edit mode (not truncate-write)
	sz, werr = file.Write([]byte("has")) // cursor 8 - tempfile copy of orig - write on tempfile has occurred
	s.NoError(werr)
	s.Equal(int64(8), file.(*File).cursorPos)
	s.Equal(3, sz)

	_, serr = file.Seek(5, 0) // cursor 5 - in temp file
	s.Equal(int64(5), file.(*File).cursorPos)
	s.NoError(serr)

	data = make([]byte, 3)
	sz, rerr = file.Read(data)
	s.NoError(rerr)
	s.Equal(int64(8), file.(*File).cursorPos)
	s.Equal("has", string(data)) // tempFile contents = "has"
	s.Equal(3, sz)

	s.NoError(file.Close()) // moves tempfile containing "has" over original file

	final := make([]byte, 8)
	rd, err := file.Read(final)
	s.NoError(err)
	s.Equal(8, rd)
	s.Equal("mary has", string(final))
	s.NoError(file.Close())

	// if a file exists and we overwrite with a smaller # of text, then it isn't completely overwritten
	//	//somefile.txt contains "the quick brown"
	file, err = s.tmploc.NewFile("test_files/someFile.txt")
	s.NoError(err)

	expectedText = "the quick brown"
	write, werr = file.Write([]byte(expectedText))
	s.NoError(werr, "write error not expected")
	s.Equal(15, write)

	s.NoError(file.Close())

	overwrite, err := file.Write([]byte("hello")) // cursor 5 of tempfile
	s.NoError(err)
	s.Equal(int64(5), file.(*File).cursorPos)
	s.Equal(5, overwrite)

	data = make([]byte, 5)
	_, serr = file.Seek(0, 0) // cursor 0 of tempfile
	s.NoError(serr)

	_, rerr = file.Read(data) // cursor 5 of tempfile - data: "hello"
	s.NoError(rerr)
	s.Equal("hello", string(data))

	data = make([]byte, 3)
	sought, serr := file.Seek(-3, 2) // cursor 3 from end of tempfile
	s.NoError(serr)
	s.Equal(int64(2), sought) // seek returns position relative to beginning of file

	rd, rerr = file.Read(data) // cursor 0 from end of tempfile - data: "llo"
	s.NoError(rerr)
	s.Equal(3, rd)
	s.Equal("llo", string(data))
	s.Equal(int64(5), file.(*File).cursorPos)

	_, serr = file.Seek(0, 0)
	s.NoError(serr)
	final = make([]byte, 5)
	rd, err = file.Read(final)
	s.NoError(err)
	s.Equal(5, rd)
	s.Equal("hello", string(final))
	s.NoError(file.Close()) // moves tempfile containing "hello" over somefile.txt
}

func (s *osFileTest) TestCursorErrs() {
	noFile, err := s.tmploc.NewFile("test_files/nonet.txt")
	s.NoError(err)
	data := make([]byte, 10)
	_, err = noFile.Read(data)
	s.Error(err)
	s.NoError(noFile.Close())

	_, err = noFile.Seek(10, 10)
	s.Error(err)

	_, err = noFile.Seek(-10, 2)
	s.Error(err)
	s.NoError(noFile.Close())

	noFile.(*File).fileOpener = nil
	noFile.(*File).useTempFile = false
	b, err := noFile.Write([]byte("blah"))
	s.NoError(err)
	s.Equal(4, b)
	noFile.(*File).fileOpener = func(filePath string) (*os.File, error) { return nil, errors.New("bad opener") }
	s.Error(noFile.Close())
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

	noFile, err := s.tmploc.NewFile("test_files/nonexistent.txt")
	s.NoError(err)
	lastModified, err = noFile.LastModified()
	s.Error(err)
	s.Nil(lastModified)
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

	noFile, err := s.tmploc.NewFile("test_files/nonexistent.txt")
	s.NoError(err)
	size, err = noFile.Size()
	s.Error(err)
	s.Zero(size)
}

func (s *osFileTest) TestPath() {
	file, err := s.tmploc.NewFile("test_files/test.txt")
	s.NoError(err)
	s.Equal(path.Join(file.Location().Path(), file.Name()), file.Path())
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

func (s *osFileTest) TestLocationRightAfterChangeDir() {
	file, err := s.tmploc.NewFile("chdTest.txt")
	s.NoError(err)
	chDir := "someDir/"

	loc := file.Location()
	s.NotContains(loc.Path(), "someDir/", "location should not contain 'someDir/'")

	err = loc.ChangeDir(chDir)
	s.NoError(err)
	s.Contains(loc.Path(), "someDir/", "location now should contain 'someDir/'")

	// file location shouldn't be affected by ChangeDir() on Location
	s.NotContains(file.Location().Path(), "someDir/", "file location should NOT contain 'someDir/'")
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
	f, err := os.Create(file) //nolint:gosec
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
