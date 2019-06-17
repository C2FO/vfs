package mem

import (
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	_os "github.com/c2fo/vfs/v4/backend/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"
)

/**********************************
 ************TESTS*****************
 **********************************/

type memFileTest struct {
	suite.Suite
	testFile   *File
	fileSystem *FileSystem
}

func (s *memFileTest) SetupSuite() {
}

func (s *memFileTest) TearDownSuite() {
	//teardownTestFiles()
}

func (s *memFileTest) SetupTest() {
	fs := &FileSystem{
		make(map[string][]string),
		make(map[string][]*File),
		make(map[string]objMap),
	}

	file, err := fs.NewFile("C", "/test_files/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file.(*File)
	s.fileSystem = fs
	//s.fileSystem.Initialize()
	s.NoError(WriteZeroBytes(s.testFile), "Unexpected error writing zero bytes")
}

func (s *memFileTest) TeardownTest() {
	err := s.testFile.Close()
	assert.NoError(s.T(), err, "close error not expected")
	derr := s.testFile.Delete()
	assert.NoError(s.T(), derr, "Delete failed unexpectedly")
}

func (s *memFileTest) TestZBR() { //zero byte read

	byteSlice := make([]byte, 0)
	_, err := s.testFile.Read(byteSlice)
	assert.NoError(s.T(), err, "Read of 0 bytes failed")

}

func (s *memFileTest) TestRARO() { //read after read w/file still open to see ensure an error occurs
	byteSlice := make([]byte, 32)
	sliceToWrite := make([]byte, 32)
	byteSlice2 := make([]byte, 32)
	for i := 0; i < 32; i++ { //filling the byte slice
		b := byte(i * 2)
		sliceToWrite[i] = b
	}

	_, _ = s.testFile.Write(sliceToWrite) //writing our bytes to the buffer so we have something to read
	_ = s.testFile.Close()                //closing the file so the buffer contents are stored into "privSlice"
	_, _ = s.testFile.Read(byteSlice)     //initial read
	_, err := s.testFile.Read(byteSlice2)

	/* an error should occur here since the first read
	moved the cursor all the way through and we did
	not close the file before reading again
	*/

	assert.Error(s.T(), err, "Read after read failed!")

}

// TestRARC tests a read after read w/file closed between reads to see ensure an error occurs
func (s *memFileTest) TestRARC() {
	byteSlice := make([]byte, 32)
	sliceToWrite := make([]byte, 32)
	byteSlice2 := make([]byte, 32)
	for i := 0; i < 32; i++ { //filling the byte slice
		b := byte(i * 2)
		sliceToWrite[i] = b
	}

	_, err := s.testFile.Write(sliceToWrite) //Writing our bytes to the buffer so we have something to read.
	assert.NoError(s.T(), err, "Unexpected write error")
	err = s.testFile.Close() //Closing the file so the buffer contents are stored into "privSlice".
	s.True(err == nil)
	_, err = s.testFile.Read(byteSlice) //This is the initial read.
	s.True(err == nil)
	err = s.testFile.Close()
	s.True(err == nil)
	_, err = s.testFile.Read(byteSlice2)

	/* No error should occur here since the first read
	moved the cursor all the way through but we closed
	the file before reading again, so it should reset it.
	*/

	assert.NoError(s.T(), err, "Read after read failed!")
	assert.ObjectsAreEqualValues(byteSlice, byteSlice2)

}

/*
TestDelete deletes the receiver file, then creates another file and deletes it.
Succeeds only on both successful deletions
*/
func (s *memFileTest) TestDelete() {

	newFile, err := s.fileSystem.NewFile("", "/home/bar.txt")
	assert.NoError(s.T(), err, "Unexpected creation error")
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	otherFile, _ := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(WriteZeroBytes(otherFile), "Unexpected error writing zero bytes")
	existence, eerr := otherFile.Exists()
	s.True(existence)
	assert.NoError(s.T(), eerr, "Unexpected existence error")
	derr := otherFile.Delete()
	assert.NoError(s.T(), derr, "Delete unexpectedly failed")
	existence1, eerr1 := otherFile.Exists()
	s.False(existence1)
	assert.NoError(s.T(), eerr1, "Unexpected existence error")
}

//TestExists1 uses "Exists()" to check for existence of our receiver's file then creates a file and does the same thing.
func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	assert.NoError(s.T(), err, "Unexpected existence error")
	s.True(doesExist) //double check that the error message was right
	otherFile, err := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(WriteZeroBytes(otherFile), "Unexpected error writing zero bytes")
	assert.NoError(s.T(), err, "File creation was not successful so it does not exist")
	doesExist1, eerr := otherFile.Exists()
	assert.NoError(s.T(), eerr, "Unexpected existence error")
	s.True(doesExist1)
}

//TestExists2 ensures that "Exists()" detects a non-existent file
func (s *memFileTest) TestExists2() {
	otherFile, err := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.True(err == nil)
	s.NoError(WriteZeroBytes(otherFile), "Unexpected error writing zero bytes")
	derr := otherFile.Delete()
	assert.NoError(s.T(), derr, "Delete unexpectedly failed")
	existence, eerr := otherFile.Exists() //should throw an error and existence should be false
	assert.NoError(s.T(), eerr, "Unexpected existence error")
	s.False(existence)
}

func (s *memFileTest) TestNewFile() {

	file, err := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.True(err == nil)
	filePath := file.Path()
	object, ok := s.fileSystem.fsMap[file.Location().Volume()][filePath] //checking our system map for a match to the given fileName
	s.True(ok)
	s.True(object.i.(*File).Name() == "foo.txt") //casting the object to a file so we can call "Name()"

}

func (s *memFileTest) TestWrite2() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, nerr := s.fileSystem.NewFile("", "/test_files/new.txt")
	assert.NoError(s.T(), nerr, "Unexpected error creating a new file")

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

//TestSeek tests to ensure that we can seek to any part of the file, if it exists
func (s *memFileTest) TestSeek2() {
	newFile, nerr := s.fileSystem.NewFile("", "/home/test_files/subdir/seekTest.txt")
	assert.NoError(s.T(), nerr, "Unexpected error creating a new file")
	initText := "Hello world!"
	_, werr := newFile.Write([]byte(initText))
	assert.NoError(s.T(), werr, "Unexpected write error")
	//without closing, the cursor should be at the end of the file.
	//calling Seek with a whence of '1' and asking to move to cursor forward should throw an error
	_, serr := newFile.Seek(1, 1)
	assert.Error(s.T(), serr, "Expected seek to throw an error")
	//trying to read should also be an error:
	_, rerr := newFile.Read(make([]byte, 1))
	assert.Error(s.T(), rerr, "Expected read error")
	_, serr2 := newFile.Seek(0, 0)
	assert.NoError(s.T(), serr2, "Unexpected seek error")
	testByte := make([]byte, 1)
	num, rerr2 := newFile.Read(testByte)
	assert.NoError(s.T(), rerr2, "Unexpected read error")
	s.True("H" == string(testByte))
	assert.ObjectsAreEqual(1, num)
	_, serr3 := newFile.Seek(-2, 1)
	assert.Error(s.T(), serr3, "Seek error expected for seeking into negative bounds")
	_, serr4 := newFile.Seek(1, 1)
	assert.NoError(s.T(), serr4, "Unexpected seek error")

}

func (s *memFileTest) TestNameToURI() {

	name := "/test_files/examples/foo.txt"
	_, err := s.fileSystem.NewFile("C", name)
	s.True(err == nil)
	retObj, ok := s.fileSystem.fsMap["C"]["/test_files/examples/foo.txt"]
	retFile := retObj.i.(*File)
	s.True(ok)
	s.NoError(WriteZeroBytes(retFile), "Unexpected error writing zero bytes")
	s.True(ok)
	existence, eerr := s.fileSystem.fsMap["C"][retFile.Path()].i.(*File).Exists() //map to obj to file cast
	assert.NoError(s.T(), eerr, "Unexpected existence error")
	s.True(existence)
	s.Equal("mem://C/test_files/examples/", retFile.location.URI())
	s.Equal("mem://C/test_files/examples/foo.txt", retFile.URI())

}

func (s *memFileTest) TestOpenFile() {
	expectedText := "hello world"
	_, werr := s.testFile.Write([]byte(expectedText))
	data := make([]byte, len(expectedText))
	assert.NoError(s.T(), werr, "Write error not expected")
	_, err := s.testFile.Read(data)
	assert.Error(s.T(), err, "read error expected")
	s.False(expectedText == string(data))
}

func (s *memFileTest) TestSeek() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, nerr := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(nerr, "Unexpected error creating a file")
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

/*
TestCopyToLocationOS copies a file to a location that has
 been passed in. Succeeds on existence of original file and its
contents in new location
*/
func (s *memFileTest) TestCopyToLocation() {

	newFile, _ := s.fileSystem.NewFile("", "/home/foo.txt")
	expectedText := "hello world!"
	_, werr := s.testFile.Write([]byte(expectedText))
	_, serr := s.testFile.Seek(-12, 2)
	assert.NoError(s.T(), serr, "Seek unexpectedly threw an error")
	//assert.NoError(s.T(),cerr,"Unexpected close error")
	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))
	assert.NoError(s.T(), werr, "Unexpected write error")
	copiedFile, cerr := s.testFile.CopyToLocation(newFile.Location())
	assert.NoError(s.T(), cerr, "CopyToLocation unexpectedly failed")
	s.NoError(WriteZeroBytes(copiedFile), "Unexpected error writing zero bytes")
	s.True(copiedFile != nil)
	assert.EqualValues(s.T(), "/home/test.txt", copiedFile.Path())
	_, rerr := copiedFile.Read(readSlice1)
	assert.NoError(s.T(), rerr, "Unexpected read error.")
	_, rerr2 := s.testFile.Read(readSlice2)
	assert.NoError(s.T(), rerr2, "Unexpected read error.")
	assert.EqualValues(s.T(), string(readSlice2), string(readSlice1))

}

/*
TestCopyToLocationOS copies a file to a location that has
 a file with the same name. Succeeds only on overwrite of file
 in specified location
*/
func (s *memFileTest) TestCopyToLocationOW() {

	newFile, _ := s.fileSystem.NewFile("", "/home/test.txt")
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	originalText := "goodbye world!"
	_, werr1 := newFile.Write([]byte(originalText))
	assert.NoError(s.T(), werr1, "Unexpected write error")
	closeErr := newFile.Close()
	assert.NoError(s.T(), closeErr, "Unexpected Close error")
	expectedText := "hello world!"
	_, werr2 := s.testFile.Write([]byte(expectedText))
	assert.NoError(s.T(), werr2, "Unexpected write error")
	closeErr2 := s.testFile.Close()
	assert.NoError(s.T(), closeErr2, "Unexpected Close error")
	readSlice := make([]byte, len(expectedText))
	copiedFile, cerr := s.testFile.CopyToLocation(newFile.Location())
	assert.NoError(s.T(), cerr, "CopyToLocation unexpectedly failed")
	s.True(copiedFile != nil)
	assert.EqualValues(s.T(), "/home/test.txt", copiedFile.Path())
	_, rerr := copiedFile.Read(readSlice)
	assert.NoError(s.T(), rerr, "Unexpected read error.")
	assert.EqualValues(s.T(), "hello world!", string(readSlice))

}

//TestCopyToNilFile ensures that an error is thrown for trying to copy to a nil file
func (s *memFileTest) TestCopyToNilFile() {

	newFile, _ := s.fileSystem.NewFile("", "/nilFile.txt")
	err := s.testFile.CopyToFile(newFile)
	assert.Error(s.T(), err, "Expected error for copying to nil file")
}

//TestCopyToLocationOS copies a file from the mem fs over
// to a location on the  OS fs and checks for success
func (s *memFileTest) TestCopyToLocationOS() { //Unsure how this should work

	expectedText := "hello world!"
	_, werr1 := s.testFile.Write([]byte(expectedText))
	assert.NoError(s.T(), werr1, "Unexpected write error")
	cerr1 := s.testFile.Close()
	assert.NoError(s.T(), cerr1, "Unexpected close error")
	var osFile vfs.File
	var err error
	dir, err := ioutil.TempDir("", "osDir")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir) // clean up
	osFileName := filepath.Join(dir, "osFile.txt")

	//fmt.Println(content,dir)
	osFile, err = backend.Backend(_os.Scheme).NewFile("", osFileName)
	_, zwErr := osFile.Write([]byte(""))
	assert.NoError(s.T(), zwErr, "Unexpected write error")
	s.True(err == nil)
	exists, eerr := osFile.Exists()
	assert.EqualValues(s.T(), true, exists)
	assert.NoError(s.T(), eerr, "Unexpected existence error")
	readSlice := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))
	copiedFile, cerr := s.testFile.CopyToLocation(osFile.Location())
	closeErr := copiedFile.Close()
	assert.NoError(s.T(), closeErr, "Unexpected Close error")
	assert.NoError(s.T(), cerr, "CopyToLocation unexpectedly failed")
	s.True(copiedFile != nil)
	assert.EqualValues(s.T(), "/test_files/test.txt", s.testFile.Path()) //testFile's path should be unchanged

	assert.EqualValues(s.T(), filepath.Join(dir, "test.txt"), copiedFile.Path()) //new path should be that

	_, rerr := copiedFile.Read(readSlice)
	assert.NoError(s.T(), rerr, "unexpected read error")

	_, rerr2 := s.testFile.Read(readSlice2)
	assert.NoError(s.T(), rerr2, "unexpected read error")
	assert.EqualValues(s.T(), string(readSlice2), string(readSlice)) //both reads should be the same

}

//TestCopyToFile tests "CopyToFile()" between two files both in the mem FS
func (s *memFileTest) TestCopyToFile() {

	expectedText := "hello world"
	otherFile, _ := s.fileSystem.NewFile("", "/test.txt")
	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))
	num, err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	assert.NoError(s.T(), err, "No error expected from Write but got one")
	closeErr := s.testFile.Close()
	assert.NoError(s.T(), closeErr, "Unexpected error closing a file")

	s.NoError(WriteZeroBytes(otherFile), "Unexpected error writing zero bytes")
	strPath := otherFile.Path()
	vol := otherFile.Location().Volume()
	err = s.testFile.CopyToFile(otherFile)
	assert.NoError(s.T(), err, "Copy to file failed unexpectedly")
	assert.NoError(s.T(), err, "Copy to file failed unexpectedly")
	_, rerr1 := s.testFile.Read(readSlice1)
	assert.NoError(s.T(), rerr1, "Unexpected read error")
	_, rerr2 := s.fileSystem.fsMap[vol][strPath].i.(*File).Read(readSlice2)
	assert.NoError(s.T(), rerr2, "Unexpected read error")
	assert.ObjectsAreEqualValues(string(readSlice1), string(readSlice2))

}

//TestCopyToFileOS tests "CopyToFile()" between one file in the mem FS and the other in the os FS
func (s *memFileTest) TestCopyToFileOS() { //testing copy to a file across file systems
	//inMem and OS

	expectedText := "Hello World!"
	var osFile vfs.File
	var err error
	osFile, err = backend.Backend(_os.Scheme).NewFile("", "test_files/foo.txt")
	s.NoError(WriteZeroBytes(osFile), "Unexpected error writing zero bytes")
	s.True(err == nil)
	num, err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	assert.NoError(s.T(), err, "No error expected from Write but got one")
	closeErr := s.testFile.Close()
	assert.NoError(s.T(), closeErr, "Unexpected error closing a  file")

	err = s.testFile.CopyToFile(osFile)
	assert.NoError(s.T(), err, "Copy to file failed unexpectedly")
	closeErr2 := osFile.Close()
	assert.NoError(s.T(), closeErr2, "Unexpected error closing a  file")

	size1, _ := s.testFile.Size()
	size2, err := osFile.Size()
	s.True(err == nil)
	s.True(size1 == size2)

}

/*
 TestEmptyCopy to file creates two files,
 one is empty the other is not. Calls
 CopyToFile using the empty one on t
 he non-empty one. Succeeds on the non-empty
 file becoming empty
*/
func (s *memFileTest) TestEmptyCopyToFile() {

	expectedText := ""
	expectedSlice := make([]byte, 1)
	otherFile, _ := s.fileSystem.NewFile("", "/some/path/otherfile.txt")
	_, werr := otherFile.Write([]byte("yooooooooo"))
	assert.NoError(s.T(), werr, "Unexpected Write error")
	cerr := otherFile.Close()
	assert.NoError(s.T(), cerr, "Unexpected close error")

	emptyFile, _ := s.fileSystem.NewFile("", "/test_files/empty.txt")
	_, werr = emptyFile.Write([]byte(""))
	assert.NoError(s.T(), werr, "Unexpected Write error")
	cerr = emptyFile.Close()
	assert.NoError(s.T(), cerr, "Unexpected close error")
	copyErr := emptyFile.CopyToFile(otherFile)
	assert.NoError(s.T(), copyErr, "Copy to file failed unexpectedly")
	_, rerr := otherFile.Read(expectedSlice)
	assert.NoError(s.T(), rerr, "Unexpected Read error")
	assert.ObjectsAreEqual(expectedText, string(expectedSlice))

}

func (s *memFileTest) TestMoveToLocation() {

	newFile, _ := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	str1 := newFile.Path()
	file, merr := newFile.MoveToLocation(s.testFile.Location())
	s.NoError(merr, "Unexpected move to location error")

	exists, eerr := newFile.Exists() //newFile shouldn't exist since MovetoLocation deletes the calling object
	s.NoError(eerr)
	s.False(exists)
	s.NoError(WriteZeroBytes(file), "Unexpected error writing zero bytes") //writing ZB to ensure existence
	str2 := file.Path()
	assert.Equal(s.T(), path.Base(str1), path.Base(str2))
	assert.Equal(s.T(), "/test_files/", file.Location().Path())

}

//TestMoveToLocation2 creates two files with the same name in different places and moves one to the other
func (s *memFileTest) TestMoveToLocation2() {

	expectedText := "Who ya calling pinhead?"
	newFile, _ := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	otherFile, _ := s.fileSystem.NewFile("", "/thisDir/foo.txt")
	_, werr := otherFile.Write([]byte(expectedText))
	assert.NoError(s.T(), werr, "Unexpected write error")
	str1 := newFile.Path()
	file, _ := otherFile.MoveToLocation(newFile.Location())
	s.NoError(WriteZeroBytes(file), "Unexpected error writing zero bytes")
	str2 := file.Path()
	assert.Equal(s.T(), path.Base(str1), path.Base(str2))
	exists, _ := otherFile.Exists()
	s.False(exists)
	readSlice := make([]byte, len(expectedText))
	_, rerr := file.Read(readSlice)
	assert.NoError(s.T(), rerr, "Unexpected read error")
	assert.Equal(s.T(), expectedText, string(readSlice))

}

/*
TestMoveToFile creates a newFile and moves the testFile to it.
Test succeeds if the moved file has the correct data.
*/
func (s *memFileTest) TestMoveToFile() {

	expectedSlice := []byte("Hello World!")
	newFile, _ := s.fileSystem.NewFile("", "/samples/test.txt")
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	_, werr := s.testFile.Write(expectedSlice)
	assert.NoError(s.T(), werr, "Write failed unexpectedly")
	cerr := s.testFile.Close()
	s.True(cerr == nil)
	merr := s.testFile.MoveToFile(newFile)
	/*
		after this call, newFile and "s.testFile" will be deleted.
		we re-obtain the newFile pointer by calling it from our fsMap by giving it the (new) path and volume
	*/
	s.NoError(merr, "Move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	newFile = s.fileSystem.fsMap[""]["/samples/test.txt"].i.(*File)
	s.False(s.testFile.Exists())

	_, rerr := newFile.Read(newFileSlice)
	s.NoError(rerr, "Read unexpectedly failed")

	s.Equal(string(expectedSlice), string(newFileSlice))
	s.Equal("/samples/test.txt", newFile.Path())

}

func (s *memFileTest) TestMoveToFile2() {

	expectedSlice := []byte("Hello World!")
	newFile, _ := s.fileSystem.NewFile("", "/samples/diffName.txt")
	newPath := "/samples/test.txt" //this is the path used to retrieve the file after it has been moved
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	_, werr := s.testFile.Write(expectedSlice)
	s.NoError(werr, "Write failed unexpectedly")
	cerr := s.testFile.Close()
	s.True(cerr == nil)
	merr := s.testFile.MoveToFile(newFile)
	/*
		after this call, newFile and "s.testFile" will be deleted.
		we re-obtain the newFile by calling it from our fsMap by giving it the (new) path and volume
	*/
	s.NoError(merr, "Move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	newFile = s.fileSystem.fsMap[""][newPath].i.(*File)
	s.False(s.testFile.Exists())
	_, rerr := newFile.Read(newFileSlice)
	s.NoError(rerr, "Read unexpectedly failed")
	s.Equal(string(expectedSlice), string(newFileSlice))
	s.Equal("/samples/test.txt", newFile.Path())

}

/*
TestWrite writes a string to a file and checks for success by comparing the number of bytes
written by "Write()" to the length of the slice it wrote from
*/
func (s *memFileTest) TestWrite() {
	expectedText := "I'm fed up with this world" //-Tommy Wiseau
	bSlice := []byte(expectedText)
	length := len(bSlice)
	num, werr1 := s.testFile.Write(bSlice)
	assert.NoError(s.T(), werr1, "Write did not work as expected")
	assert.ObjectsAreEqualValues(length, num)

}

/*
TestLastModified Writes to a file then retrives the value that LastModified() returns and the timeStamp value
stored in the File struct and compares them against eachother.  Successful if they are equal.
*/

func (s *memFileTest) TestLastModified() {
	data := "Hello World!"
	sliceData := []byte(data)
	_, err := s.testFile.Write(sliceData)
	assert.NoError(s.T(), err, "Write did not work as expected!")

	t, _ := s.testFile.LastModified()

	s.Equal(s.testFile.timeStamp, *t)

}

//TestName creates a file and names it and then asserts that the given name and the return of Name() match.
func (s *memFileTest) TestName() {
	newFile, nerr := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(nerr, "Unexpected error creating file")
	s.NoError(WriteZeroBytes(newFile), "Unexpected error writing zero bytes")
	s.Equal("test.txt", newFile.Name())
}

//TestSize allocates 64 bytes in one file and 32 in the other.  It makes sure that Size() returns the correct sizes.
func (s *memFileTest) TestSize() {
	otherFile, err := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(err, "Unexpected error creting file")
	_, werr := s.testFile.Write(make([]byte, 64))
	s.NoError(werr, "Unexpected write error")
	_, werr2 := otherFile.Write(make([]byte, 32))
	s.NoError(werr2, "Unexpected write error")
	size1, serr1 := s.testFile.Size()
	s.NoError(serr1, "Unexpected error retrieving size")
	size2, serr2 := otherFile.Size()
	s.NoError(serr2, "Unexpected error retrieving size")
	s.True(size1 > size2)

}

//TestPath makes sure that locations return the correct paths, along with leading and trailing slashes
func (s *memFileTest) TestPath() {
	str1 := "/home/some/directory/test_files/test.txt"
	_, nerr := s.fileSystem.NewFile("", str1)
	assert.NoError(s.T(), nerr, "Unexpected error creating a new file")

	_, nerr2 := s.fileSystem.NewFile("", "/test_files/bar.txt")
	assert.NoError(s.T(), nerr2, "Unexpected error creating a new file")

	file1, nerr3 := s.fileSystem.NewFile("", "/directory/bar.txt")
	assert.NoError(s.T(), nerr3, "Unexpected error creating a new file")
	s.NoError(WriteZeroBytes(file1), "Unexpected error writing zero bytes")
	str := "/directory/test_files/test.txt"
	_, nerr4 := s.fileSystem.NewFile("", str)
	assert.NoError(s.T(), nerr4, "Unexpected error creating a new file")
	s.Equal("/directory/bar.txt", file1.Path())
}

//TestURI ensures that URI's for locations come out in the correct format
func (s *memFileTest) TestURI() {
	file, nerr := s.fileSystem.NewFile("C", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(nerr, "Unexpected error creating file")
	s.NoError(WriteZeroBytes(file), "Unexpected error writing zero bytes")
	uri := file.URI()
	s.Equal("mem://C/test_files/lots/of/directories/here/we/go/test.txt", uri)
}

//TestStringer tests the implementation of io.Stringer

func (s *memFileTest) TestStringer() {

	file, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.True(err == nil)
	s.NoError(WriteZeroBytes(file), "Unexpected error writing zero bytes")
	str := file.String()
	s.Equal("mem:///test_files/lots/of/directories/here/we/go/test.txt", str)

}

func TestMemFile(t *testing.T) {
	suite.Run(t, new(memFileTest))
	_ = os.Remove("test_files/new.txt")
}

/*
	Setup TEST FILES
*/

func WriteZeroBytes(file vfs.File) error {

	zB := make([]byte, 0)
	_, werr := file.Write(zB)
	if werr != nil {
		return werr
	}
	cerr := file.Close()
	return cerr
}

func (s *memFileTest) teardownTestFiles() {
	/*
		for _, v := range s.fileSystem.fileList {
			if v != nil {
				s.NoError(v.Delete(),"Unexpected error cleaning up testFiles")

			}

		}
	*/
}
