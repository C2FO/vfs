package mem

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/backend"
	_os "github.com/c2fo/vfs/v5/backend/os"
	"github.com/stretchr/testify/suite"
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
	fs := &FileSystem{}
	fs.Initialize() //initializing to create the fsMap
	file, nerr := fs.NewFile("C", "/test_files/test.txt")
	s.NoError(nerr, "Unexpected error creating file")

	//initializing our test file. casting it, and bringing it into existence by calling Touch() on it
	s.testFile = file.(*File)
	s.fileSystem = fs
	s.testFile.Touch()
}

func (s *memFileTest) TeardownTest() {
	err := s.testFile.Close()
	s.NoError(err, "close error not expected")
	s.NoError(s.testFile.Delete(), "Delete failed unexpectedly")
}

//TestZBR ensures that we can always read zero bytes
func (s *memFileTest) TestZBR() {

	byteSlice := make([]byte, 0)
	_, err := s.testFile.Read(byteSlice)
	s.NoError(err, "Read of 0 bytes failed")

}

//TestRARO ensures that read after read w/file still open (and without a seek) will incur an error
func (s *memFileTest) TestRARO() {
	sliceToWrite := "hello world!"
	byteSlice := make([]byte, len(sliceToWrite))
	byteSlice2 := make([]byte, len(sliceToWrite))

	_, err := s.testFile.Write([]byte(sliceToWrite)) //writing our bytes to the buffer so we have something to read
	s.NoError(err, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file") //closing the file so the buffer contents are stored into "contents"

	_, err = s.testFile.Read(byteSlice) //initial read
	s.NoError(err, "Unexpected read error")
	_, err = s.testFile.Read(byteSlice2)
	/* an error should occur here since the first read
	moved the cursor all the way through and we did
	not close the file before reading again
	*/
	s.Error(err, "Read was expected to fail")
}

// TestRARC tests a read after read w/file closed between reads to see ensure an error occurs
func (s *memFileTest) TestRARC() {
	sliceToWrite := "hello world!"
	byteSlice := make([]byte, len(sliceToWrite))
	byteSlice2 := make([]byte, len(sliceToWrite))

	_, err := s.testFile.Write([]byte(sliceToWrite)) //Writing our bytes to the buffer so we have something to read.
	s.NoError(err, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file") //Closing the file so the buffer contents are stored into "contents".

	_, err = s.testFile.Read(byteSlice) //This is the initial read.
	s.NoError(err, "Unexpected read error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file")
	_, err = s.testFile.Read(byteSlice2)
	s.NoError(err, "Unexpected read error")

	/* No error should occur here since the first read
	moved the cursor all the way through but we closed
	the file before reading again, so it should reset it.
	*/

	s.NoError(err, "Read after read failed!")
	s.Equal(byteSlice, byteSlice2)

}

/*
TestNewFile creates two files with the same name and ensures
that the second creation returns a reference to the first
 */
func (s *memFileTest) TestNewFileSameName(){
	sharedPath := "/path/to/file.txt"
	firstFile,err := s.fileSystem.NewFile("",sharedPath)
	s.NoError(err,"Unexpected error creating a file")

	expectedText := "hey y'all!"
	_, err = firstFile.Write([]byte(expectedText))
	s.NoError(err,"Unexpected error writing to file")

	secondFile, err := s.fileSystem.NewFile("",sharedPath)
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

/*
TestDelete deletes the receiver file, then creates another file and deletes it.
Succeeds only on both successful deletions
*/
func (s *memFileTest) TestDelete() {

	newFile, err := s.fileSystem.NewFile("", "/home/bar.txt")
	s.NoError(err, "Unexpected creation error")
	newFile.(*File).Touch()
	otherFile, err := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(err, "Unexpected creation error")
	otherFile.(*File).Touch()
	existence, err := otherFile.Exists()
	s.True(existence)
	s.NoError(err, "Unexpected existence error")

	//deleting otherFile and ensuring it no longer exists
	s.NoError(otherFile.Delete(), "Delete unexpectedly failed")
	existence1, err := otherFile.Exists()
	s.False(existence1)
	s.NoError(err, "Unexpected existence error")
}

//TestExists1 uses "Exists()" to check for existence of our receiver's file then creates a file and does the same thing.
func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	s.NoError(err, "Unexpected existence error")
	s.True(doesExist) //double check that the error message was right

	otherFile, err := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(err, "Unexpected error creating file")
	otherFile.(*File).Touch()
	doesExist1, err := otherFile.Exists()
	s.NoError(err, "Unexpected existence error")
	s.True(doesExist1)
}

//TestExists2 ensures that "Exists()" detects a non-existent file
func (s *memFileTest) TestExists2() {
	otherFile, err := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.NoError(err, "Unexpected error creating file")
	otherFile.(*File).Touch()
	//deleting otherFile and asserting non-existence
	s.NoError(otherFile.Delete(), "Delete unexpectedly failed")
	existence, err := otherFile.Exists()
	s.NoError(err, "Unexpected existence error")
	s.False(existence)
}

//TestNewFile creates a file and then checks that it exists in our system the the path expected
func (s *memFileTest) TestNewFile() {

	file, err := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.NoError(err, "Unexpected error creating file")
	file.(*File).Touch()
	filePath := file.Path()
	object, ok := s.fileSystem.fsMap[file.Location().Volume()][filePath] //checking our system map for a match to the given fileName
	s.True(ok)
	s.True(object.i.(*File).Name() == "foo.txt") //casting the object to a file so we can call "Name()"

}

//TestSeek2 tests to ensure that we can seek to any part of the file, if it exists
func (s *memFileTest) TestSeek2() {
	newFile, err := s.fileSystem.NewFile("", "/home/test_files/subdir/seekTest.txt")
	s.NoError(err, "Unexpected error creating a new file")
	initText := "Hello world!"

	_, err = newFile.Write([]byte(initText))
	s.NoError(err, "Unexpected write error")

	//without closing, the cursor should be at the end of the file.
	//calling Seek with a whence of '1' and asking to move to cursor forward should throw an error
	_, err = newFile.Seek(1, 1)
	s.Error(err, "Expected seek to throw an error")
	//trying to read should also be an error:
	_, err = newFile.Read(make([]byte, 1))
	s.Error(err, "Expected read error")

	_, err = newFile.Seek(0, 0)
	s.NoError(err, "Unexpected seek error")
	testByte := make([]byte, 1)

	num, err := newFile.Read(testByte)
	s.NoError(err, "Unexpected read error")
	s.True("H" == string(testByte))
	s.Equal(1, num)

	_, err = newFile.Seek(-2, 1)
	s.Error(err, "Seek error expected for seeking into negative bounds")

	_, err = newFile.Seek(1, 1)
	s.NoError(err, "Unexpected seek error")
}

//TestNameToURI creates a file then pulls it out of the fsMap using its name and ensures its file and location URIs are correct
func (s *memFileTest) TestNameToURI() {

	name := "/test_files/examples/foo.txt"
	file, err := s.fileSystem.NewFile("C", name)
	s.NoError(err, "Unexpected error creating file")
	file.(*File).Touch()
	//fsMap returns an "obj" so to get a file we need to cast its member interface to a file
	retObj, ok := s.fileSystem.fsMap["C"]["/test_files/examples/foo.txt"]
	s.True(ok)
	s.True(retObj.isFile) //making sure that this object is a file
	retFile := retObj.i.(*File)

	retFile.Touch()
	//checking existence in the fsMap using "retFile's" path
	existence, err := s.fileSystem.fsMap["C"][retFile.Path()].i.(*File).Exists() //map to obj to file cast
	s.NoError(err, "Unexpected existence error")
	s.True(existence)
	s.Equal("mem://C/test_files/examples/", retFile.location.URI())
	s.Equal("mem://C/test_files/examples/foo.txt", retFile.URI())

}

//TestOpenFile ensures that reading after a write without seeking or closing a file throws an error
func (s *memFileTest) TestOpenFile() {
	expectedText := "hello world"

	_, err := s.testFile.Write([]byte(expectedText))
	data := make([]byte, len(expectedText))
	s.NoError(err, "Write error not expected")

	_, err = s.testFile.Read(data)
	s.Error(err, "read error expected")
	s.False(expectedText == string(data))
}

//TestSeek writes to a file and seeks to the beginning of it to read what it wrote
func (s *memFileTest) TestSeek() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, err := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(err, "Unexpected error creating a file")
	_, err = file.Write([]byte(expectedText))
	s.NoError(err, "write error not expected")

	_, err = file.Seek(0, 0)
	s.NoError(err, "seek error not expected")

	_, err = file.Read(data)
	s.NoError(err, "read error not expected")
	s.NoError(file.Close(), "close error not expected")

	s.Equal(expectedText, string(data))

	found, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.True(found)

	s.NoError(file.Delete(), "File was not deleted properly")

	found2, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.False(found2)

}

/*
TestCopyToLocation copies a file to a location that has
 been passed in. Succeeds on existence of original file and its
contents in new location
*/
func (s *memFileTest) TestCopyToLocation() {

	newFile, err := s.fileSystem.NewFile("", "/home/foo.txt")
	s.NoError(err, "Unexpected error creating file")

	expectedText := "hello world!"
	_, err = s.testFile.Write([]byte(expectedText))
	s.NoError(err, "Unexpected write error")

	//seeking to the start of the file from the end of the file (offset of -12 takes us to the start)
	_, err = s.testFile.Seek(-12, 2)
	s.NoError(err, "Seek unexpectedly threw an error")

	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))

	copiedFile, cerr := s.testFile.CopyToLocation(newFile.Location())
	s.NoError(cerr, "CopyToLocation unexpectedly failed")

	copiedFile.(*File).Touch()
	s.True(copiedFile != nil)
	//making sure the path was correctly updated
	s.EqualValues("/home/test.txt", copiedFile.Path())

	_, err = copiedFile.Read(readSlice1)
	s.NoError(err, "Unexpected read error.")

	_, err = s.testFile.Read(readSlice2)
	s.NoError(err, "Unexpected read error.")
	s.EqualValues(string(readSlice2), string(readSlice1))

}

/*
TestCopyToLocationOW copies a file to a location that has
 a file with the same name. Succeeds only on overwrite of file
 in specified location
*/
func (s *memFileTest) TestCopyToLocationOW() {

	newFile, err := s.fileSystem.NewFile("C", "/home/test.txt")
	s.NoError(err, "Unexpected error creating a file")
	newFile.(*File).Touch()
	originalText := "goodbye world!"
	_, err = newFile.Write([]byte(originalText))
	s.NoError(err, "Unexpected write error")
	s.NoError(newFile.Close(), "Unexpected Close error")

	expectedText := "hello world!"
	_, err = s.testFile.Write([]byte(expectedText))
	s.NoError(err, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected Close error")

	readSlice := make([]byte, len(expectedText))
	copiedFile, err := s.testFile.CopyToLocation(newFile.Location())
	s.NoError(err, "CopyToLocation unexpectedly failed")
	s.True(copiedFile != nil)

	s.EqualValues("/home/test.txt", copiedFile.Path())
	_, err = copiedFile.Read(readSlice)
	s.NoError(err, "Unexpected read error.")
	s.EqualValues("hello world!", string(readSlice))

}

//TestCopyToNilFile ensures that an error is thrown for trying to copy to a nil file
func (s *memFileTest) TestCopyToNilFile() {

	newFile, err := s.fileSystem.NewFile("", "/nilFile.txt")
	s.NoError(err, "Unexpected error creating file")
	newFile = nil
	err = s.testFile.CopyToFile(newFile)
	s.Error(err, "Expected error for copying to nil file")
}

//TestCopyToLocationOS copies a file from the in-memory FileSystem over
// to a location on the  OS FileSystem and checks for success
func (s *memFileTest) TestCopyToLocationOS() {

	expectedText := "hello world!"
	_, err := s.testFile.Write([]byte(expectedText))
	s.NoError(err, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file")

	var osFile vfs.File
	dir, err := ioutil.TempDir("", "osDir")
	if err != nil {
		log.Fatal(err)
	}
	osFileName := filepath.Join(dir, "osFile.txt")

	osFile, err = backend.Backend(_os.Scheme).NewFile("", osFileName)
	s.NoError(err, "Unexpected error creating osFile")
	_, err = osFile.Write(make([]byte, 0))
	s.NoError(err, "Unexpected error writing zero bytes to osFile")

	exists, err := osFile.Exists()
	s.NoError(err, "Unexpected existence error")
	s.True(exists)

	readSlice := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))

	copiedFile, err := s.testFile.CopyToLocation(osFile.Location())
	s.NoError(err, "CopyToLocation unexpectedly failed")
	s.NoError(copiedFile.Close(), "Unexpected Close error")

	s.True(copiedFile != nil)
	s.EqualValues("/test_files/test.txt", s.testFile.Path())         //testFile's path should be unchanged
	s.EqualValues(filepath.Join(dir, "test.txt"), copiedFile.Path()) //new path should be that

	_, err = copiedFile.Read(readSlice)
	s.NoError(err, "unexpected read error")

	_, err = s.testFile.Read(readSlice2)
	s.NoError(err, "unexpected read error")
	s.EqualValues(string(readSlice2), string(readSlice)) //both reads should be the same
	cleanErr := os.RemoveAll(dir)                        // clean up
	s.NoError(cleanErr, "Unexpected error cleaning up osFiles")
}

//TestCopyToFile tests "CopyToFile()" between two files both in the in-memory FileSystem
func (s *memFileTest) TestCopyToFile() {

	expectedText := "hello world"
	otherFile, err := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(err, "Unexpected error creating file")
	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))
	num, err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	s.NoError(err, "No error expected from Write but got one")
	s.NoError(s.testFile.Close(), "Unexpected error closing a file")

	otherFile.(*File).Touch()
	strPath := otherFile.Path()
	vol := otherFile.Location().Volume()

	err = s.testFile.CopyToFile(otherFile)
	s.NoError(err, "Copy to file failed unexpectedly")

	_, err = s.testFile.Read(readSlice1)
	s.NoError(err, "Unexpected read error")

	_, err = s.fileSystem.fsMap[vol][strPath].i.(*File).Read(readSlice2)
	s.NoError(err, "Unexpected read error")
	s.EqualValues(string(readSlice1), string(readSlice2))

}

//TestCopyToFileOS tests "CopyToFile()" between one file in the in-memory FileSystem and the other in the os FileSystem
func (s *memFileTest) TestCopyToFileOS() {

	expectedText := "Hello World!"
	var osFile vfs.File
	var err error
	dir, err := ioutil.TempDir("", "osDir")
	if err != nil {
		log.Fatal(err)
	}
	osFileName := filepath.Join(dir, "osFile.txt")
	osFile, err = backend.Backend(_os.Scheme).NewFile("", osFileName)
	s.NoError(err, "Unexpected error creating osFile")
	_, err = osFile.Write(make([]byte, 0))
	s.NoError(err, "Unexpected error writing zero bytes to osFile")
	num, err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	s.NoError(err, "No error expected from Write but got one")
	s.NoError(s.testFile.Close(), "Unexpected error closing a  file")

	err = s.testFile.CopyToFile(osFile)
	s.NoError(err, "Copy to file failed unexpectedly")
	s.NoError(osFile.Close(), "Unexpected error closing a  file")

	size1, err := s.testFile.Size()
	s.NoError(err, "Unexpected error retrieving size")
	size2, err := osFile.Size()
	s.NoError(err, "Unexpected error retrieving size")
	s.Equal(size1, size2)

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
	expectedSlice := make([]byte, 0)
	otherFile, err := s.fileSystem.NewFile("", "/some/path/otherfile.txt")

	s.NoError(err, "Unexpected error creating a file")

	_, err = otherFile.Write([]byte("yooooooooooo"))
	s.NoError(err, "Unexpected Write error")
	s.NoError(otherFile.Close(), "Unexpected close error")

	emptyFile, nerr := s.fileSystem.NewFile("C", "/test_files/empty.txt")
	s.NoError(nerr, "File creation was not successful so it does not exist")
	_, err = emptyFile.Write([]byte(""))
	s.NoError(err, "Unexpected Write error")

	s.NoError(emptyFile.Close(), "Unexpected close error")
	//call to CopyToFile
	s.NoError(emptyFile.CopyToFile(otherFile), "Copy to file failed unexpectedly")

	_, err = otherFile.Read(expectedSlice)
	s.NoError(err, "Unexpected Read error")
	s.Equal(expectedText, string(expectedSlice))

}

//TestMoveToLocation ensures that we can move files to specified locations
func (s *memFileTest) TestMoveToLocation() {

	newFile, err := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(err, "Unexpected error creating file")
	newFile.(*File).Touch()
	str1 := newFile.Path()
	file, err := newFile.MoveToLocation(s.testFile.Location())
	s.NoError(err, "Unexpected move to location error")

	exists, err := newFile.Exists() //newFile shouldn't exist since MovetoLocation deletes the calling object
	s.NoError(err, "Unexpected existence error")
	s.False(exists)

	newFile.(*File).Touch()
	str2 := file.Path()
	s.Equal(path.Base(str1), path.Base(str2))
	s.Equal("/test_files/", file.Location().Path())

}

//TestMoveToLocation2 creates two files with the same name in different places and moves one to the other
func (s *memFileTest) TestMoveToLocation2() {

	expectedText := "Who ya calling pinhead?"
	newFile, err := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(err, "Unexpected error creating file")
	newFile.(*File).Touch()

	otherFile, err := s.fileSystem.NewFile("", "/thisDir/foo.txt")
	s.NoError(err, "Unexpected error creating file")
	_, err = otherFile.Write([]byte(expectedText))
	s.NoError(err, "Unexpected write error")

	str1 := newFile.Path()
	file, _ := otherFile.MoveToLocation(newFile.Location())
	str2 := file.Path()
	s.Equal(path.Base(str1), path.Base(str2))

	exists, _ := otherFile.Exists()
	s.False(exists)
	readSlice := make([]byte, len(expectedText))

	_, err = file.Read(readSlice)
	s.NoError(err, "Unexpected read error")
	s.Equal(expectedText, string(readSlice))

}

/*
TestMoveToFile creates a newFile and moves the testFile to it.
Test succeeds if the moved file has the correct data.  They share the same name, so this is effectively a "CopyToFile" call
*/
func (s *memFileTest) TestMoveToFile() {

	expectedSlice := []byte("Hello World!")
	newFile, err := s.fileSystem.NewFile("", "/samples/test.txt")
	s.NoError(err, "Unexpected error creating file")
	newFile.(*File).Touch()

	_, err = s.testFile.Write(expectedSlice)
	s.NoError(err, "Write failed unexpectedly")
	s.NoError(s.testFile.Close(), "Unexpected close error")
	/*
		after this call, newFile and "s.testFile" will be deleted.
		we re-obtain the newFile pointer by calling it from our fsMap by giving it the (new) path and volume
	*/
	err = s.testFile.MoveToFile(newFile)
	s.NoError(err, "Move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	newFile = s.fileSystem.fsMap[""]["/samples/test.txt"].i.(*File)
	s.False(s.testFile.Exists())

	_, err = newFile.Read(newFileSlice)
	s.NoError(err, "Read unexpectedly failed")

	s.Equal(string(expectedSlice), string(newFileSlice))
	s.Equal("/samples/test.txt", newFile.Path())

}

//TestMoveToFile2 ensures that if a call is made on a files who don't share the same name, a new file is produced with the caller's name
func (s *memFileTest) TestMoveToFile2() {

	expectedSlice := []byte("Hello World!")
	newFile, err := s.fileSystem.NewFile("", "/samples/diffName.txt")
	s.NoError(err, "File creation was not successful so it does not exist")
	//newPath := "/samples/test.txt" //this is the path used to retrieve the file after it has been moved
	newFile.(*File).Touch()
	_, err = s.testFile.Write(expectedSlice)
	s.NoError(err, "Write failed unexpectedly")
	s.NoError(s.testFile.Close(), "Unexpected close error")
	/*
		after this call, newFile and "s.testFile" will be deleted.
		we re-obtain the newFile by calling it from our fsMap by giving it the (new) path and volume
	*/
	err = s.testFile.MoveToFile(newFile)
	s.NoError(err, "Move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	//newFile = s.fileSystem.fsMap[""][newPath].i.(*File)
	s.False(s.testFile.Exists())

	_, err = newFile.Read(newFileSlice)
	s.NoError(err, "Read unexpectedly failed")
	s.Equal(string(expectedSlice), string(newFileSlice))
	s.Equal("/samples/diffName.txt", newFile.Path())

}

/*
TestWrite writes a string to a file and checks for success by comparing the number of bytes
written by "Write()" to the length of the slice it wrote from
*/
func (s *memFileTest) TestWrite() {
	expectedText := "I'm fed up with this world" //-Tommy Wiseau
	bSlice := []byte(expectedText)
	length := len(bSlice)
	num, err := s.testFile.Write(bSlice)
	s.NoError(err, "Write did not work as expected")
	s.EqualValues(length, num)

}

//TestWrite2 writes to a file, and reads from it without closing it by seeking to the start
func (s *memFileTest) TestWrite2() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, err := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(err, "Unexpected error creating a new file")

	_, err = file.Write([]byte(expectedText))
	s.NoError(err, "write error not expected")

	_, err = file.Seek(0, 0)
	s.NoError(err, "seek error not expected")
	_, err = file.Read(data)
	s.NoError(err, "read error not expected")
	err = file.Close()
	s.NoError(err, "close error not expected")

	s.Equal(expectedText, string(data))

	found, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.True(found)

	//deleting file and asserting non-existence
	s.NoError(file.Delete(), "Unexpected error deleting file")
	found2, err := file.Exists()
	s.NoError(err, "exists error not expected")
	s.False(found2)
}

/*
TestLastModified Writes to a file then retrives the value that LastModified() returns and the lastModified value
stored in the File struct and compares them against eachother.  Successful if they are equal.
*/

func (s *memFileTest) TestLastModified() {
	data := "Hello World!"
	sliceData := []byte(data)
	_, err := s.testFile.Write(sliceData)
	s.NoError(err, "Write did not work as expected!")

	t, _ := s.testFile.LastModified()

	s.Equal(s.testFile.lastModified, *t)

}

//TestName creates a file and names it and then asserts that the given name and the return of Name() match.
func (s *memFileTest) TestName() {
	newFile, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(err, "Unexpected error creating file")
	newFile.(*File).Touch()
	s.Equal("test.txt", newFile.Name())
}

//TestSize allocates 64 bytes in one file and 32 in the other.  It makes sure that Size() returns the correct sizes.
func (s *memFileTest) TestSize() {
	otherFile, err := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(err, "Unexpected error creting file")
	_, err = s.testFile.Write(make([]byte, 64))
	s.NoError(err, "Unexpected write error")
	_, err = otherFile.Write(make([]byte, 32))
	s.NoError(err, "Unexpected write error")
	size1, err := s.testFile.Size()
	s.NoError(err, "Unexpected error retrieving size")
	size2, err := otherFile.Size()
	s.NoError(err, "Unexpected error retrieving size")
	s.True(size1 > size2)

}

//TestPath makes sure that locations return the correct paths, along with leading and trailing slashes
func (s *memFileTest) TestPath() {
	str1 := "/home/some/directory/test_files/test.txt"
	_, err := s.fileSystem.NewFile("", str1)
	s.NoError(err, "Unexpected error creating a new file")

	_, err = s.fileSystem.NewFile("", "/test_files/bar.txt")
	s.NoError(err, "Unexpected error creating a new file")

	file1, err := s.fileSystem.NewFile("", "/directory/bar.txt")
	s.NoError(err, "Unexpected error creating a new file")
	file1.(*File).Touch()

	str := "/directory/test_files/test.txt"
	_, err = s.fileSystem.NewFile("", str)
	s.NoError(err, "Unexpected error creating a new file")
	s.Equal("/directory/bar.txt", file1.Path())
}

//TestURI ensures that URI's for locations come out in the correct format
func (s *memFileTest) TestURI() {
	file, err := s.fileSystem.NewFile("C", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(err, "Unexpected error creating file")
	file.(*File).Touch()
	uri := file.URI()
	s.Equal("mem://C/test_files/lots/of/directories/here/we/go/test.txt", uri)
}

//TestStringer tests the implementation of io.Stringer
func (s *memFileTest) TestStringer() {

	file, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(err, "Unexpected error creating file")
	file.(*File).Touch()
	str := file.String()
	s.Equal("mem:///test_files/lots/of/directories/here/we/go/test.txt", str)

}

func TestMemFile(t *testing.T) {
	suite.Run(t, new(memFileTest))
	_ = os.Remove("test_files/new.txt")
}

func (s *memFileTest) teardownTestFiles() {

	s.fileSystem.fsMap = nil
}
