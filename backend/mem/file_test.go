package mem

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	_os "github.com/c2fo/vfs/v6/backend/os"
)

/**********************************
 ************TESTS*****************
 **********************************/

type memFileTest struct {
	suite.Suite
	testFile   *File
	fileSystem *FileSystem
}

func (s *memFileTest) SetupTest() {
	memfs := NewFileSystem()
	file, nerr := memfs.NewFile("C", "/test_files/test.txt")
	s.NoError(nerr, "unexpected error creating file")

	// initializing our test file. casting it, and bringing it into existence by calling Touch() on it
	s.testFile = file.(*File)
	s.fileSystem = memfs
	s.NoError(s.testFile.Touch(), "unexpected error touching file")
}

func (s *memFileTest) TeardownTest() {
	err := s.testFile.Close()
	s.NoError(err, "close error not expected")
	s.NoError(s.testFile.Delete(), "delete failed unexpectedly")
}

// TestZBR ensures that we can always read zero bytes
func (s *memFileTest) TestZBR() {

	byteSlice := make([]byte, 0)
	_, err := s.testFile.Read(byteSlice)
	s.ErrorIs(err, io.EOF, "read of 0 bytes failed")

}

// TestRARO ensures that read after read w/file still open (and without a seek) will incur an error
func (s *memFileTest) TestRARO() {
	sliceToWrite := "hello world!"
	byteSlice := make([]byte, len(sliceToWrite))
	byteSlice2 := make([]byte, len(sliceToWrite))

	_, err := s.testFile.Write([]byte(sliceToWrite)) // writing our bytes to the buffer so we have something to read
	s.NoError(err, "unexpected write error")
	s.NoError(s.testFile.Close(), "unexpected error closing file") // closing the file so the buffer contents are stored into "contents"

	_, err = s.testFile.Read(byteSlice) // initial read
	s.NoError(err, "unexpected read error")
	_, err = s.testFile.Read(byteSlice2)
	// an error should occur here since the first read
	// moved the cursor all the way through and we did
	// not close the file before reading again
	s.Error(err, "read was expected to fail")
}

// TestRARC tests a read after read w/file closed between reads to see ensure an error occurs
func (s *memFileTest) TestRARC() {
	sliceToWrite := "hello world!"
	byteSlice := make([]byte, len(sliceToWrite))
	byteSlice2 := make([]byte, len(sliceToWrite))

	_, err := s.testFile.Write([]byte(sliceToWrite)) // Writing our bytes to the buffer so we have something to read.
	s.NoError(err, "unexpected write error")
	s.NoError(s.testFile.Close(), "unexpected error closing file") // Closing the file so the buffer contents are stored into "contents".

	// This is the initial read.
	_, err = s.testFile.Read(byteSlice)
	s.NoError(err, "unexpected read error")
	s.NoError(s.testFile.Close(), "unexpected error closing file")
	_, err = s.testFile.Read(byteSlice2)
	s.NoError(err, "unexpected read error")

	// No error should occur here since the first read
	// moved the cursor all the way through but we closed
	// the file before reading again, so it should reset it.
	s.NoError(err, "read after read failed!")
	s.Equal(byteSlice, byteSlice2)

}

// TestNewFileSameName creates two files with the same name and ensures
// that the second creation returns a reference to the first
func (s *memFileTest) TestNewFileSameName() {
	sharedPath := "/path/to/file.txt"
	firstFile, err := s.fileSystem.NewFile("", sharedPath)
	s.NoError(err, "unexpected error creating a file")
	expectedText := "hey y'all!"
	_, err = firstFile.Write([]byte(expectedText))
	s.NoError(err, "unexpected error writing to file")
	secondFile, err := s.fileSystem.NewFile("", sharedPath)
	s.NoError(err, "unexpected error creating a file")
	expectedSlice := make([]byte, len(expectedText))
	// since secondFile references firstFile, reading will throw an error as we never closed or seeked firstFile
	_, err = secondFile.Read(expectedSlice)
	s.Error(err, "expected read error since firstFile was never closed")
	// after this call, we can expect to be able to read from secondFile since its reference, firstFile, was closed
	s.NoError(firstFile.Close(), "unexpected error closing file")
	_, err = secondFile.Read(expectedSlice)
	s.NoError(err, "unexpected read error")
	s.Equal(expectedText, string(expectedSlice))
}

// TestDelete deletes the receiver file, then creates another file and deletes it.
// Succeeds only on both successful deletions
func (s *memFileTest) TestDelete() {

	newFile, err := s.fileSystem.NewFile("", "/home/bar.txt")
	s.NoError(err, "unexpected creation error")
	s.NoError(newFile.Touch(), "unexpected error touching file")
	otherFile, err := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(err, "unexpected creation error")
	s.NoError(otherFile.Touch(), "unexpected error touching file")
	existence, err := otherFile.Exists()
	s.True(existence)
	s.NoError(err, "unexpected existence error")

	// deleting otherFile and ensuring it no longer exists
	s.NoError(otherFile.Delete(), "Delete unexpectedly failed")
	existence1, err := otherFile.Exists()
	s.False(existence1)
	s.NoError(err, "unexpected existence error")
}

// TestExists1 uses "Exists()" to check for existence of our receiver's file then creates a file and does the same thing.
func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	s.NoError(err, "unexpected existence error")
	s.True(doesExist) // double check that the error message was right

	otherFile, err := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(otherFile.Touch(), "unexpected error touching file")
	doesExist1, err := otherFile.Exists()
	s.NoError(err, "unexpected existence error")
	s.True(doesExist1)
}

// TestExists2 ensures that "Exists()" detects a non-existent file
func (s *memFileTest) TestExists2() {
	otherFile, err := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(otherFile.Touch(), "unexpected error touching file")
	// deleting otherFile and asserting non-existence
	s.NoError(otherFile.Delete(), "delete unexpectedly failed")
	existence, err := otherFile.Exists()
	s.NoError(err, "unexpected existence error")
	s.False(existence)
}

// TestNewFile creates a file and then checks that it exists in our system at the path expected
func (s *memFileTest) TestNewFile() {

	file, err := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(file.Touch(), "unexpected error touching file")
	filePath := file.Path()
	returnedFile, err := s.fileSystem.NewFile(file.Location().Volume(), filePath) // checking our system map for a match to the given fileName
	s.NoError(err, "unexpected error retrieving file")
	s.Equal("foo.txt", returnedFile.Name()) // casting the object to a file so we can call "Name()"

}

// TestSeek2 tests to ensure that we can seek to any part of the file, if it exists
func (s *memFileTest) TestSeek2() {
	newFile, err := s.fileSystem.NewFile("", "/home/test_files/subdir/seekTest.txt")
	s.NoError(err, "unexpected error creating a new file")
	initText := "Hello world!"

	_, err = newFile.Write([]byte(initText))
	s.NoError(err, "unexpected write error")

	// without closing, the cursor should be at the end of the file.
	// calling Seek with a whence of '1' and asking to move to cursor forward should throw an error
	_, err = newFile.Seek(1, 1)
	s.Error(err, "expected seek to throw an error")
	// trying to read should also be an error:
	_, err = newFile.Read(make([]byte, 1))
	s.Error(err, "expected read error")
	s.NoError(newFile.Close(), "unexpected close error")

	_, err = newFile.Seek(0, 0)
	s.NoError(err, "unexpected seek error")
	testByte := make([]byte, 1)

	num, err := newFile.Read(testByte)
	s.NoError(err, "unexpected read error")
	s.Equal("H", string(testByte))
	s.Equal(1, num)

	_, err = newFile.Seek(-2, 1)
	s.Error(err, "seek error expected for seeking into negative bounds")

	_, err = newFile.Seek(1, 1)
	s.NoError(err, "unexpected seek error")

	// test for seek with whence of '2'
	pos, err := newFile.Seek(-2, 2)
	s.NoError(err, "unexpected seek error")
	s.Equal(int64(len(initText)-2), pos)

	// test for invalid whence
	_, err = newFile.Seek(0, 3)
	s.ErrorIs(err, vfs.ErrSeekInvalidWhence, "expected Seek error")

	// test for seeking on non-existent file
	f, err := s.fileSystem.NewFile("", "/some/non-existent/file.txt")
	s.NoError(err, "unexpected error creating a new file")
	_, err = f.Seek(0, 0)
	s.ErrorIs(err, fs.ErrNotExist, "expected Seek error")

}

// TestNameToURI creates a file then pulls it out of the fsMap using its name and ensures its file and location URIs are correct
func (s *memFileTest) TestNameToURI() {

	name := "/test_files/examples/foo.txt"
	file, err := s.fileSystem.NewFile("C", name)
	s.NoError(err, "unexpected error creating file")
	s.NoError(file.Touch(), "unexpected error touching file")
	// fsMap returns an "fsObject" so to get a file we need to cast its member interface to a file
	returnedFile, err := s.fileSystem.NewFile("C", "/test_files/examples/foo.txt")
	s.NoError(err, "unexpected error retrieving file")

	s.NoError(returnedFile.Touch(), "unexpected error touching file")
	// checking existence in the fsMap using "retFile's" path
	existence, err := returnedFile.Exists()
	s.NoError(err, "unexpected existence error")
	s.True(existence)
	s.Equal("mem://C/test_files/examples/", returnedFile.Location().URI())
	s.Equal("mem://C/test_files/examples/foo.txt", returnedFile.URI())

}

// TestOpenFile ensures that reading after a write without seeking or closing a file throws an error
func (s *memFileTest) TestOpenFile() {
	expectedText := "hello world"

	_, err := s.testFile.Write([]byte(expectedText))
	data := make([]byte, len(expectedText))
	s.NoError(err, "write error not expected")

	_, err = s.testFile.Read(data)
	s.Error(err, "read error expected")
	s.NotEqual(expectedText, string(data))
}

// TestSeek writes to a file and seeks to the beginning of it to read what it wrote
func (s *memFileTest) TestSeek() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, err := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(err, "unexpected error creating a file")
	_, err = file.Write([]byte(expectedText))
	s.NoError(err, "write error not expected")

	_, err = file.Seek(0, 0)
	s.NoError(err, "seek error not expected")
	s.NoError(file.Close(), "unexpected close error")
	_, err = file.Read(data)
	s.NoError(err, "read error not expected")
	s.NoError(file.Close(), "close error not expected")

	s.Equal(expectedText, string(data))

	found, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.True(found)

	s.NoError(file.Delete(), "file was not deleted properly")

	found2, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.False(found2)

}

// TestCopyToLocation copies a file to a location that has
// been passed in. Succeeds on existence of original file and its
// contents in new location
func (s *memFileTest) TestCopyToLocation() {

	newFile, err := s.fileSystem.NewFile("", "/home/foo.txt")
	s.NoError(err, "unexpected error creating file")
	expectedText := "hello world!"
	_, err = s.testFile.Write([]byte(expectedText))
	s.NoError(err, "unexpected write error")

	s.NoError(s.testFile.Close(), "unexpected close error")

	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))

	copiedFile, cerr := s.testFile.CopyToLocation(newFile.Location())
	s.NoError(cerr, "CopyToLocation unexpectedly failed")

	s.NoError(copiedFile.Touch(), "unexpected error touching file")
	s.NotNil(copiedFile)
	// making sure the path was correctly updated
	s.Equal("/home/test.txt", copiedFile.Path())

	_, err = copiedFile.Read(readSlice1)
	s.NoError(err, "unexpected read error")

	_, err = s.testFile.Read(readSlice2)
	s.NoError(err, "unexpected read error")
	s.Equal(readSlice2, readSlice1)

}

// TestCopyToLocationOW copies a file to a location that has
// a file with the same name. Succeeds only on overwrite of file
// in specified location
func (s *memFileTest) TestCopyToLocationOW() {

	newFile, err := s.fileSystem.NewFile("C", "/home/test.txt")
	s.NoError(err, "unexpected error creating a file")
	s.NoError(newFile.Touch(), "unexpected error touching file")
	originalText := "goodbye world!"
	_, err = newFile.Write([]byte(originalText))
	s.NoError(err, "unexpected write error")
	s.NoError(newFile.Close(), "unexpected Close error")

	expectedText := "hello world!"
	_, err = s.testFile.Write([]byte(expectedText))
	s.NoError(err, "unexpected write error")
	s.NoError(s.testFile.Close(), "unexpected Close error")

	readSlice := make([]byte, len(expectedText))
	copiedFile, err := s.testFile.CopyToLocation(newFile.Location())
	s.NoError(err, "CopyToLocation unexpectedly failed")
	s.NotNil(copiedFile)
	s.NoError(copiedFile.Close(), "unexpected close error")

	s.Equal("/home/test.txt", copiedFile.Path())
	_, err = copiedFile.Read(readSlice)
	s.NoError(err, "unexpected read error")
	s.Equal("hello world!", string(readSlice))

}

// TestCopyToNilFile ensures that an error is thrown for trying to copy to a nil file
func (s *memFileTest) TestCopyToNilFile() {

	newFile, err := s.fileSystem.NewFile("", "nilFile.txt")
	s.Error(err, "expected error creating file (bad path)")
	err = s.testFile.CopyToFile(newFile)
	s.Error(err, "expected error for copying to nil file")
}

// TestCopyToLocationOS copies a file from the in-memory FileSystem over
// to a location on the  OS FileSystem and checks for success
func (s *memFileTest) TestCopyToLocationOS() {

	expectedText := "hello world!"
	_, err := s.testFile.Write([]byte(expectedText))
	s.NoError(err, "unexpected write error")
	s.NoError(s.testFile.Close(), "unexpected error closing file")

	var osFile vfs.File
	dir, err := os.MkdirTemp("", "osDir")
	s.Require().NoError(err)
	osFileName := filepath.Join(dir, "osFile.txt")

	osFile, err = backend.Backend(_os.Scheme).NewFile("", osFileName)
	s.NoError(err, "unexpected error creating osFile")
	_, err = osFile.Write(make([]byte, 0))
	s.NoError(err, "unexpected error writing zero bytes to osFile")
	s.NoError(osFile.Close())

	exists, err := osFile.Exists()
	s.NoError(err, "unexpected existence error")
	s.True(exists)

	readSlice := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))

	copiedFile, err := s.testFile.CopyToLocation(osFile.Location())
	s.NoError(err, "CopyToLocation unexpectedly failed")
	s.NoError(copiedFile.Close(), "unexpected Close error")

	s.NotNil(copiedFile)
	s.Equal("/test_files/test.txt", s.testFile.Path())         // testFile's path should be unchanged
	s.Equal(filepath.Join(dir, "test.txt"), copiedFile.Path()) // new path should be that

	_, err = copiedFile.Read(readSlice)
	s.NoError(err, "unexpected read error")

	_, err = s.testFile.Read(readSlice2)
	s.NoError(err, "unexpected read error")
	s.Equal(readSlice2, readSlice) // both reads should be the same
	cleanErr := os.RemoveAll(dir)  // clean up
	s.NoError(cleanErr, "unexpected error cleaning up osFiles")
}

// TestCopyToFile tests "CopyToFile()" between two files both in the in-memory FileSystem
func (s *memFileTest) TestCopyToFile() {

	expectedText := "hello world"
	otherFile, err := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(err, "unexpected error creating file")
	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))
	num, err := s.testFile.Write([]byte(expectedText))
	s.NotZero(num)
	s.NoError(err, "no error expected from Write but got one")
	s.NoError(s.testFile.Close(), "unexpected error closing a file")

	s.NoError(otherFile.Touch(), "unexpected error touching file")
	err = s.testFile.CopyToFile(otherFile)
	s.NoError(err, "copy to file failed unexpectedly")

	_, err = s.testFile.Read(readSlice1)
	s.NoError(err, "unexpected read error")
	otherFile, err = s.fileSystem.NewFile("", "/test.txt")
	s.NoError(err, "unexpected creation error")
	_, err = otherFile.Read(readSlice2)
	s.NoError(err, "unexpected read error")
	s.Equal(readSlice1, readSlice2)

}

// TestCopyToFileOS tests "CopyToFile()" between one file in the in-memory FileSystem and the other in the os FileSystem
func (s *memFileTest) TestCopyToFileOS() {

	expectedText := "Hello World!"
	var osFile vfs.File
	var err error
	dir, err := os.MkdirTemp("", "osDir")
	s.Require().NoError(err)
	osFileName := filepath.Join(dir, "osFile.txt")
	osFile, err = backend.Backend(_os.Scheme).NewFile("", osFileName)
	s.NoError(err, "unexpected error creating osFile")
	_, err = osFile.Write(make([]byte, 0))
	s.NoError(err, "unexpected error writing zero bytes to osFile")
	num, err := s.testFile.Write([]byte(expectedText))
	s.NotZero(num)
	s.NoError(err, "no error expected from Write but got one")
	s.NoError(s.testFile.Close(), "unexpected error closing a file")

	err = s.testFile.CopyToFile(osFile)
	s.NoError(err, "copy to file failed unexpectedly")
	s.NoError(osFile.Close(), "unexpected error closing a file")

	size1, err := s.testFile.Size()
	s.NoError(err, "unexpected error retrieving size")
	size2, err := osFile.Size()
	s.NoError(err, "unexpected error retrieving size")
	s.Equal(size1, size2)

}

// TestEmptyCopy to file creates two files,
// one is empty the other is not. Calls
// CopyToFile using the empty one on the
// non-empty one. Succeeds on the non-empty
// file becoming empty
func (s *memFileTest) TestEmptyCopyToFile() {

	expectedText := ""
	expectedSlice := make([]byte, 0)
	otherFile, err := s.fileSystem.NewFile("", "/some/path/otherfile.txt")

	s.NoError(err, "unexpected error creating a file")

	_, err = otherFile.Write([]byte("yooooooooooo"))
	s.NoError(err, "unexpected Write error")
	s.NoError(otherFile.Close(), "unexpected close error")

	emptyFile, nerr := s.fileSystem.NewFile("C", "/test_files/empty.txt")
	s.NoError(nerr, "file creation was not successful so it does not exist")
	_, err = emptyFile.Write([]byte(""))
	s.NoError(err, "unexpected Write error")

	s.NoError(emptyFile.Close(), "unexpected close error")
	// call to CopyToFile
	s.NoError(emptyFile.CopyToFile(otherFile), "CopyToFile failed unexpectedly")

	_, err = otherFile.Read(expectedSlice)
	s.ErrorIs(err, io.EOF, "expected EOF error")
	s.Equal(expectedText, string(expectedSlice))

}

// TestMoveToLocation ensures that we can move files to specified locations
func (s *memFileTest) TestMoveToLocation() {

	newFile, err := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(newFile.Touch(), "unexpected error touching file")
	str1 := newFile.Path()
	file, err := newFile.MoveToLocation(s.testFile.Location())
	s.NoError(err, "unexpected move to location error")

	exists, err := newFile.Exists() // newFile shouldn't exist since MovetoLocation deletes the calling object
	s.NoError(err, "unexpected existence error")
	s.False(exists)

	s.NoError(newFile.Touch(), "unexpected error touching file")
	str2 := file.Path()
	s.Equal(path.Base(str1), path.Base(str2))
	s.Equal("/test_files/", file.Location().Path())

}

// TestMoveToLocation2 creates two files with the same name in different places and moves one to the other
func (s *memFileTest) TestMoveToLocation2() {

	expectedText := "Who ya calling pinhead?"
	newFile, err := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(newFile.Touch(), "unexpected error touching file")

	otherFile, err := s.fileSystem.NewFile("", "/thisDir/foo.txt")
	s.NoError(err, "unexpected error creating file")
	_, err = otherFile.Write([]byte(expectedText))
	s.NoError(err, "unexpected write error")
	s.NoError(otherFile.Close(), "unexpected close error")
	str1 := newFile.Path()
	file, _ := otherFile.MoveToLocation(newFile.Location())
	str2 := file.Path()
	s.Equal(path.Base(str1), path.Base(str2))

	exists, _ := otherFile.Exists()
	s.False(exists)
	readSlice := make([]byte, len(expectedText))

	_, err = file.Read(readSlice)
	s.NoError(err, "unexpected read error")
	s.Equal(expectedText, string(readSlice))

}

// TestMoveToFile creates a newFile and moves the testFile to it.
// Test succeeds if the moved file has the correct data.  They share the same name, so this is effectively a "CopyToFile" call
func (s *memFileTest) TestMoveToFile() {

	expectedSlice := []byte("Hello World!")
	newFile, err := s.fileSystem.NewFile("", "/samples/test.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(newFile.Touch(), "unexpected error touching file")

	_, err = s.testFile.Write(expectedSlice)
	s.NoError(err, "Write failed unexpectedly")
	s.NoError(s.testFile.Close(), "unexpected close error")

	// after this call, newFile and "s.testFile" will be deleted.
	// we re-obtain the newFile pointer by calling it from our fsMap by giving it the (new) path and volume
	err = s.testFile.MoveToFile(newFile)
	s.NoError(err, "move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	newFile, err = s.fileSystem.NewFile("", "/samples/test.txt")
	s.NoError(err, "unexpected creation error")

	s.False(s.testFile.Exists())

	_, err = newFile.Read(newFileSlice)
	s.NoError(err, "read unexpectedly failed")

	s.Equal(expectedSlice, newFileSlice)
	s.Equal("/samples/test.txt", newFile.Path())

}

// TestMoveToFile2 ensures that if a call is made on a files who don't share the same name, a new file is produced with the caller's name
func (s *memFileTest) TestMoveToFile2() {

	expectedSlice := []byte("Hello World!")
	newFile, err := s.fileSystem.NewFile("", "/samples/diffName.txt")
	s.NoError(err, "file creation was not successful so it does not exist")
	s.NoError(newFile.Touch(), "unexpected error touching file")
	_, err = s.testFile.Write(expectedSlice)
	s.NoError(err, "write failed unexpectedly")
	s.NoError(s.testFile.Close(), "unexpected close error")

	// after this call, newFile and "s.testFile" will be deleted.
	// we re-obtain the newFile by calling it from our fsMap by giving it the (new) path and volume
	err = s.testFile.MoveToFile(newFile)
	s.NoError(err, "move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	s.False(s.testFile.Exists())

	_, err = newFile.Read(newFileSlice)
	s.NoError(err, "read unexpectedly failed")
	s.Equal(expectedSlice, newFileSlice)
	s.Equal("/samples/diffName.txt", newFile.Path())

}

// TestWrite writes a string to a file and checks for success by comparing the number of bytes
// written by "Write()" to the length of the slice it wrote from
func (s *memFileTest) TestWrite() {
	expectedText := "I'm fed up with this world" // -Tommy Wiseau
	bSlice := []byte(expectedText)
	length := len(bSlice)
	num, err := s.testFile.Write(bSlice)
	s.NoError(err, "unexpected write error")
	s.EqualValues(length, num)
	s.NoError(s.testFile.Close(), "unexpected close error")

	// test write after Seek (edit mode)
	_, err = s.testFile.Seek(0, 0)
	s.NoError(err, "unexpected seek error")
	_, err = s.testFile.Write([]byte("Hello World!"))
	s.NoError(err, "unexpected write error")
	s.NoError(s.testFile.Close(), "unexpected close error")
	actualText, err := io.ReadAll(s.testFile)
	s.NoError(err, "unexpected read error")
	s.Equal("Hello World!ith this world", string(actualText))
}

// TestRead ensures read can be called successively to get an entire file's contents in chunks
func (s *memFileTest) TestRead() {
	expectedSlice := []byte("Hello World!")
	length := len(expectedSlice)
	fileToRead, err := s.fileSystem.NewFile("", "/fileToRead.txt")
	s.NoError(err, "unexpected new file error")
	num, err := fileToRead.Write(expectedSlice)
	s.NoError(err, "unexpected write error")
	s.EqualValues(length, num)
	s.NoError(fileToRead.Close(), "close error not expected")

	b, err := io.ReadAll(fileToRead)
	s.NoError(err, "read error not expected")
	s.Len(b, 12)
	s.Equal(expectedSlice, b)

	// test for reading from a non-existent file
	f, err := s.fileSystem.NewFile("", "/some/non-existent/file.txt")
	s.NoError(err, "unexpected error creating a new file")
	_, err = f.Read(make([]byte, 1))
	s.ErrorIs(err, fs.ErrNotExist, "expected Read error")
}

// TestWriteThenReadNoClose writes to a file, and reads from it without closing it by seeking to the start
func (s *memFileTest) TestWriteThenReadNoClose() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, err := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(err, "unexpected error creating a new file")

	_, err = file.Write([]byte(expectedText))
	s.NoError(err, "write error not expected")

	_, err = file.Seek(0, 0)
	s.NoError(err, "seek error not expected")
	_, err = file.Read(data)
	s.NoError(err, "no error expected")

	s.NoError(file.Close(), "close error not expected")
	_, err = file.Read(data)
	s.NoError(err, "read error not expected")

	s.Equal(expectedText, string(data))

	found, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.True(found)

	// deleting file and asserting non-existence
	s.NoError(file.Delete(), "unexpected error deleting file")
	found2, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.False(found2)
}

// TestLastModified Writes to a file then retrieves the value that LastModified() returns and the lastModified value
// stored in the File struct and compares them against eachother.  Successful if they are equal.
func (s *memFileTest) TestLastModified() {
	_, err := s.testFile.Write([]byte("Hello World!"))
	s.NoError(err, "write did not work as expected!")
	s.NoError(s.testFile.Close(), "close error not expected")

	t, _ := s.testFile.LastModified()
	firstTime := *t
	time.Sleep(1 * time.Second)
	_, err = s.testFile.Write([]byte("hey!"))
	s.NoError(err, "unexpected write error")
	s.NoError(s.testFile.Close(), "close error not expected")

	secondTime := *t

	s.True(secondTime.UnixNano() > firstTime.UnixNano())

}

// TestName creates a file and names it and then asserts that the given name and the return of Name() match.
func (s *memFileTest) TestName() {
	newFile, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(newFile.Touch(), "unexpected error touching file")
	s.Equal("test.txt", newFile.Name())
}

// TestSize allocates 64 bytes in one file and 32 in the other.  It makes sure that Size() returns the correct sizes.
func (s *memFileTest) TestSize() {
	otherFile, err := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(err, "unexpected error creating file")
	_, err = s.testFile.Write(make([]byte, 64))
	s.NoError(err, "unexpected write error")
	_, err = otherFile.Write(make([]byte, 32))
	s.NoError(err, "unexpected write error")

	s.NoError(s.testFile.Close(), "unexpected close error")
	s.NoError(otherFile.Close(), "unexpected close error")

	size1, err := s.testFile.Size()
	s.NoError(err, "unexpected error retrieving size")
	size2, err := otherFile.Size()
	s.NoError(err, "unexpected error retrieving size")
	s.True(size1 > size2)

}

// TestPath makes sure that locations return the correct paths, along with leading and trailing slashes
func (s *memFileTest) TestPath() {
	str1 := "/home/some/directory/test_files/test.txt"
	_, err := s.fileSystem.NewFile("", str1)
	s.NoError(err, "unexpected error creating a new file")

	_, err = s.fileSystem.NewFile("", "/test_files/bar.txt")
	s.NoError(err, "unexpected error creating a new file")

	file1, err := s.fileSystem.NewFile("", "/directory/bar.txt")
	s.NoError(err, "unexpected error creating a new file")
	s.NoError(file1.Touch(), "unexpected error touching file")

	str := "/directory/test_files/test.txt"
	_, err = s.fileSystem.NewFile("", str)
	s.NoError(err, "unexpected error creating a new file")
	s.Equal("/directory/bar.txt", file1.Path())
}

// TestURI ensures that URI's for locations come out in the correct format
func (s *memFileTest) TestURI() {
	file, err := s.fileSystem.NewFile("C", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(file.Touch(), "unexpected error touching file")
	uri := file.URI()
	s.Equal("mem://C/test_files/lots/of/directories/here/we/go/test.txt", uri)
}

// TestStringer tests the implementation of io.Stringer
func (s *memFileTest) TestStringer() {

	file, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(err, "unexpected error creating file")
	s.NoError(file.Touch(), "unexpected error touching file")
	str := file.String()
	s.Equal("mem:///test_files/lots/of/directories/here/we/go/test.txt", str)

}

func (s *memFileTest) TestFileNewWrite() {
	file, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.Require().NoError(err)
	_, err = file.Write([]byte("hello world"))
	s.Require().NoError(err)
	s.Require().NoError(file.Close())

	data, err := io.ReadAll(file)
	s.Require().NoError(err)
	s.Equal("hello world", string(data))
	s.Require().NoError(file.Close())

	// Re-write the same data should not append
	_, err = file.Write([]byte("hello world"))
	s.Require().NoError(err)
	s.Require().NoError(file.Close())

	data, err = io.ReadAll(file)
	s.Require().NoError(err)
	s.Equal("hello world", string(data))
}

func TestMemFile(t *testing.T) {
	suite.Run(t, new(memFileTest))
	_ = os.Remove("test_files/new.txt")
}
