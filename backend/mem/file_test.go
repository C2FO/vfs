package mem

import (
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	_os "github.com/c2fo/vfs/v4/backend/os"
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


func (s *memFileTest) SetupTest() {
	fs:=&FileSystem{}
	fs.Initialize() //initializing to create the fsMap
	file, nerr := fs.NewFile("C", "/test_files/test.txt")
	s.NoError(nerr,"Unexpected error creating file")

	//initializing our test file. casting it, and bringing it into existence by writing to it
	s.testFile = file.(*File)
	s.fileSystem = fs
	Touch(file)
}

func (s *memFileTest) TeardownTest() {
	err := s.testFile.Close()
	s.NoError(err, "close error not expected")
	derr := s.testFile.Delete()
	s.NoError(derr, "Delete failed unexpectedly")
}

//TestZBR ensures that we can always read zero bytes
func (s *memFileTest) TestZBR() { //zero byte read

	byteSlice := make([]byte, 0)
	_, err := s.testFile.Read(byteSlice)
	s.NoError(err, "Read of 0 bytes failed")

}
//TestRARO ensures that read after read w/file still open (and without a seek) will incur an error
func (s *memFileTest) TestRARO() {
	sliceToWrite := "hello world!"
	byteSlice := make([]byte, len(sliceToWrite))
	byteSlice2 := make([]byte, len(sliceToWrite))

	_, werr := s.testFile.Write([]byte(sliceToWrite)) //writing our bytes to the buffer so we have something to read
	s.NoError(werr, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file") //closing the file so the buffer contents are stored into "fileContents"

	_, rerr2 := s.testFile.Read(byteSlice) //initial read
	s.NoError(rerr2, "Unexpected read error")
	_, rerr3 := s.testFile.Read(byteSlice2)
	/* an error should occur here since the first read
	moved the cursor all the way through and we did
	not close the file before reading again
	*/
	s.Error(rerr3, "Read was expected to fail")
}

// TestRARC tests a read after read w/file closed between reads to see ensure an error occurs
func (s *memFileTest) TestRARC() {
	sliceToWrite := "hello world!"
	byteSlice := make([]byte, len(sliceToWrite))
	byteSlice2 := make([]byte, len(sliceToWrite))

	_, err := s.testFile.Write([]byte(sliceToWrite)) //Writing our bytes to the buffer so we have something to read.
	s.NoError(err, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file") //Closing the file so the buffer contents are stored into "fileContents".

	_, rerr := s.testFile.Read(byteSlice) //This is the initial read.
	s.NoError(rerr, "Unexpected read error")
	s.NoError(s.testFile.Close(), "Unexpected error closing file")
	_, rerr2 := s.testFile.Read(byteSlice2)
	s.NoError(rerr2, "Unexpected read error")

	/* No error should occur here since the first read
	moved the cursor all the way through but we closed
	the file before reading again, so it should reset it.
	*/

	s.NoError(err, "Read after read failed!")
	s.Equal(byteSlice, byteSlice2)

}

/*
TestDelete deletes the receiver file, then creates another file and deletes it.
Succeeds only on both successful deletions
*/
func (s *memFileTest) TestDelete() {

	newFile, nerr := s.fileSystem.NewFile("", "/home/bar.txt")
	s.NoError(nerr, "Unexpected creation error")
	Touch(newFile)
	otherFile, nerr2 := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(nerr2, "Unexpected creation error")
	Touch(otherFile)
	existence, eerr := otherFile.Exists()
	s.True(existence)
	s.NoError(eerr, "Unexpected existence error")

	//deleting otherFile and ensuring it no longer exists
	s.NoError(otherFile.Delete(), "Delete unexpectedly failed")
	existence1, eerr1 := otherFile.Exists()
	s.False(existence1)
	s.NoError(eerr1, "Unexpected existence error")
}

//TestExists1 uses "Exists()" to check for existence of our receiver's file then creates a file and does the same thing.
func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	s.NoError(err, "Unexpected existence error")
	s.True(doesExist) //double check that the error message was right

	otherFile, nerr := s.fileSystem.NewFile("", "/foo.txt")
	s.NoError(nerr, "Unexpected error creating file")
	Touch(otherFile)
	doesExist1, eerr := otherFile.Exists()
	s.NoError(eerr, "Unexpected existence error")
	s.True(doesExist1)
}

//TestExists2 ensures that "Exists()" detects a non-existent file
func (s *memFileTest) TestExists2() {
	otherFile, nerr := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.NoError(nerr,"Unexpected error creating file")
	Touch(otherFile)
	//deleting otherFile and asserting non-existence
	s.NoError(otherFile.Delete(), "Delete unexpectedly failed")
	existence, eerr := otherFile.Exists()
	s.NoError(eerr, "Unexpected existence error")
	s.False(existence)
}

//TestNewFile creates a file and then checks that it exists in our system the the path expected
func (s *memFileTest) TestNewFile() {

	file, nerr := s.fileSystem.NewFile("", "/test_file/foo.txt")
	s.NoError(nerr,"Unexpected error creating file")
	Touch(file)
	filePath := file.Path()
	object, ok := s.fileSystem.fsMap[file.Location().Volume()][filePath] //checking our system map for a match to the given fileName
	s.True(ok)
	s.True(object.i.(*File).Name() == "foo.txt") //casting the object to a file so we can call "Name()"

}


//TestSeek2 tests to ensure that we can seek to any part of the file, if it exists
func (s *memFileTest) TestSeek2() {
	newFile, nerr := s.fileSystem.NewFile("", "/home/test_files/subdir/seekTest.txt")
	s.NoError(nerr, "Unexpected error creating a new file")
	initText := "Hello world!"

	_, werr := newFile.Write([]byte(initText))
	s.NoError(werr, "Unexpected write error")

	//without closing, the cursor should be at the end of the file.
	//calling Seek with a whence of '1' and asking to move to cursor forward should throw an error
	_, serr := newFile.Seek(1, 1)
	s.Error(serr, "Expected seek to throw an error")
	//trying to read should also be an error:
	_, rerr := newFile.Read(make([]byte, 1))
	s.Error(rerr, "Expected read error")

	_, serr2 := newFile.Seek(0, 0)
	s.NoError(serr2, "Unexpected seek error")
	testByte := make([]byte, 1)

	num, rerr2 := newFile.Read(testByte)
	s.NoError(rerr2, "Unexpected read error")
	s.True("H" == string(testByte))
	s.Equal(1, num)

	_, serr3 := newFile.Seek(-2, 1)
	s.Error(serr3, "Seek error expected for seeking into negative bounds")

	_, serr4 := newFile.Seek(1, 1)
	s.NoError(serr4, "Unexpected seek error")
}

//TestNameToURI creates a file then pulls it out of the fsMap using its name and ensures its file and location URIs are correct
func (s *memFileTest) TestNameToURI() {

	name := "/test_files/examples/foo.txt"
	file, nerr := s.fileSystem.NewFile("C", name)
	s.NoError(nerr,"Unexpected error creating file")
	Touch(file)
	//fsMap returns an "obj" so to get a file we need to cast its member interface to a file
	retObj, ok := s.fileSystem.fsMap["C"]["/test_files/examples/foo.txt"]
	s.True(ok)
	s.True(retObj.isFile)	//making sure that this object is a file
	retFile := retObj.i.(*File)

	Touch(retFile)
	//checking existence in the fsMap using "retFile's" path
	existence, eerr := s.fileSystem.fsMap["C"][retFile.Path()].i.(*File).Exists() //map to obj to file cast
	s.NoError(eerr, "Unexpected existence error")
	s.True(existence)
	s.Equal("mem://C/test_files/examples/", retFile.location.URI())
	s.Equal("mem://C/test_files/examples/foo.txt", retFile.URI())

}

//TestOpenFile ensures that reading after a write without seeking or closing a file throws an error
func (s *memFileTest) TestOpenFile() {
	expectedText := "hello world"

	_, werr := s.testFile.Write([]byte(expectedText))
	data := make([]byte, len(expectedText))
	s.NoError(werr, "Write error not expected")

	_, err := s.testFile.Read(data)
	s.Error(err, "read error expected")
	s.False(expectedText == string(data))
}

//TestSeek writes to a file and seeks to the beginning of it to read what it wrote
func (s *memFileTest) TestSeek() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, nerr := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(nerr, "Unexpected error creating a file")
	_, werr := file.Write([]byte(expectedText))
	s.NoError(werr, "write error not expected")

	_, serr := file.Seek(0, 0)
	s.NoError(serr, "seek error not expected")

	_, rerr := file.Read(data)
	s.NoError(rerr, "read error not expected")
	s.NoError(file.Close(), "close error not expected")

	s.Equal(expectedText, string(data))

	found, eErr := file.Exists()
	s.NoError(eErr, "exists error not expected")
	s.True(found)

	s.NoError(file.Delete(), "File was not deleted properly")

	found2, eErr2 := file.Exists()
	s.NoError(eErr2, "exists error not expected")
	s.False(found2)

}

/*
TestCopyToLocation copies a file to a location that has
 been passed in. Succeeds on existence of original file and its
contents in new location
*/
func (s *memFileTest) TestCopyToLocation() {

	newFile, nerr := s.fileSystem.NewFile("", "/home/foo.txt")
	s.NoError(nerr,"Unexpected error creating file")

	expectedText := "hello world!"
	_, werr := s.testFile.Write([]byte(expectedText))
	s.NoError(werr, "Unexpected write error")

	//seeking to the start of the file from the end of the file (offset of -12 takes us to the start)
	_, serr := s.testFile.Seek(-12, 2)
	s.NoError(serr, "Seek unexpectedly threw an error")

	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))

	copiedFile, cerr := s.testFile.CopyToLocation(newFile.Location())
	s.NoError(cerr, "CopyToLocation unexpectedly failed")

	Touch(copiedFile)
	s.True(copiedFile != nil)
	//making sure the path was correctly updated
	s.EqualValues("/home/test.txt", copiedFile.Path())

	_, rerr := copiedFile.Read(readSlice1)
	s.NoError(rerr, "Unexpected read error.")

	_, rerr2 := s.testFile.Read(readSlice2)
	s.NoError(rerr2, "Unexpected read error.")
	s.EqualValues(string(readSlice2), string(readSlice1))

}

/*
TestCopyToLocationOW copies a file to a location that has
 a file with the same name. Succeeds only on overwrite of file
 in specified location
*/
func (s *memFileTest) TestCopyToLocationOW() {

	newFile, nerr := s.fileSystem.NewFile("", "/home/test.txt")
	s.NoError(nerr,"Unexpected error creating a file")
	Touch(newFile)
	originalText := "goodbye world!"
	_, werr1 := newFile.Write([]byte(originalText))
	s.NoError(werr1, "Unexpected write error")
	s.NoError(newFile.Close(), "Unexpected Close error")

	expectedText := "hello world!"
	_, werr2 := s.testFile.Write([]byte(expectedText))
	s.NoError(werr2, "Unexpected write error")
	s.NoError(s.testFile.Close(), "Unexpected Close error")

	readSlice := make([]byte, len(expectedText))
	copiedFile, cerr := s.testFile.CopyToLocation(newFile.Location())
	s.NoError(cerr, "CopyToLocation unexpectedly failed")
	s.True(copiedFile != nil)

	s.EqualValues("/home/test.txt", copiedFile.Path())
	_, rerr := copiedFile.Read(readSlice)
	s.NoError(rerr, "Unexpected read error.")
	s.EqualValues("hello world!", string(readSlice))

}

//TestCopyToNilFile ensures that an error is thrown for trying to copy to a nil file
func (s *memFileTest) TestCopyToNilFile() {

	newFile, nerr := s.fileSystem.NewFile("", "/nilFile.txt")
	s.NoError(nerr,"Unexpected error creating file")
	newFile=nil
	err := s.testFile.CopyToFile(newFile)
	s.Error(err, "Expected error for copying to nil file")
}

//TestCopyToLocationOS copies a file from the mem fs over
// to a location on the  OS fs and checks for success
func (s *memFileTest) TestCopyToLocationOS() {

	expectedText := "hello world!"
	_, werr1 := s.testFile.Write([]byte(expectedText))
	s.NoError(werr1, "Unexpected write error")
	s.NoError(s.testFile.Close(),"Unexpected error closing file")

	var osFile vfs.File
	var err error
	dir, err := ioutil.TempDir("", "osDir")
	if err != nil {
		log.Fatal(err)
	}
	osFileName := filepath.Join(dir, "osFile.txt")

	osFile, err = backend.Backend(_os.Scheme).NewFile("", osFileName)
	s.NoError(err,"Unexpected error creating osFile")
	_,werr:=osFile.Write(make([]byte,0))
	s.NoError(werr,"Unexpected error writing zero bytes to osFile")

	exists, eerr := osFile.Exists()
	s.NoError(eerr, "Unexpected existence error")
	s.True(exists)

	readSlice := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))

	copiedFile, cerr := s.testFile.CopyToLocation(osFile.Location())
	s.NoError(cerr, "CopyToLocation unexpectedly failed")
	s.NoError(copiedFile.Close(), "Unexpected Close error")

	s.True(copiedFile != nil)
	s.EqualValues("/test_files/test.txt", s.testFile.Path()) //testFile's path should be unchanged
	s.EqualValues(filepath.Join(dir, "test.txt"), copiedFile.Path()) //new path should be that

	_, rerr := copiedFile.Read(readSlice)
	s.NoError(rerr, "unexpected read error")

	_, rerr2 := s.testFile.Read(readSlice2)
	s.NoError(rerr2, "unexpected read error")
	s.EqualValues(string(readSlice2), string(readSlice)) //both reads should be the same
	cleanErr:=os.RemoveAll(dir) // clean up
	s.NoError(cleanErr,"Unexpected error cleaning up osFiles")
}

//TestCopyToFile tests "CopyToFile()" between two files both in the mem FS
func (s *memFileTest) TestCopyToFile() {

	expectedText := "hello world"
	otherFile, nerr := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(nerr,"Unexpected error creating file")
	readSlice1 := make([]byte, len(expectedText))
	readSlice2 := make([]byte, len(expectedText))
	num, err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	s.NoError(err, "No error expected from Write but got one")
	s.NoError(s.testFile.Close(), "Unexpected error closing a file")

	Touch(otherFile)
	strPath := otherFile.Path()
	vol := otherFile.Location().Volume()

	err = s.testFile.CopyToFile(otherFile)
	s.NoError(err, "Copy to file failed unexpectedly")

	_, rerr1 := s.testFile.Read(readSlice1)
	s.NoError(rerr1, "Unexpected read error")

	_, rerr2 := s.fileSystem.fsMap[vol][strPath].i.(*File).Read(readSlice2)
	s.NoError(rerr2, "Unexpected read error")
	s.EqualValues(string(readSlice1), string(readSlice2))

}

//TestCopyToFileOS tests "CopyToFile()" between one file in the mem FS and the other in the os FS
func (s *memFileTest) TestCopyToFileOS() {

	expectedText := "Hello World!"
	var osFile vfs.File
	var err error
	osFile, err = backend.Backend(_os.Scheme).NewFile("", "test_files/foo.txt")
	s.NoError(err,"Unexpected error creating osFile")
	_,werr:=osFile.Write(make([]byte,0))
	s.NoError(werr,"Unexpected error writing zero bytes to osFile")
	num, err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	s.NoError(err, "No error expected from Write but got one")
	s.NoError(s.testFile.Close(), "Unexpected error closing a  file")

	cerr := s.testFile.CopyToFile(osFile)
	s.NoError(cerr, "Copy to file failed unexpectedly")
	s.NoError(osFile.Close(), "Unexpected error closing a  file")

	size1, serr := s.testFile.Size()
	s.NoError(serr,"Unexpected error retrieving size")
	size2, serr2 := osFile.Size()
	s.NoError(serr2,"Unexpected error retrieving size")
	s.Equal(size1,size2)

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
	otherFile, nerr := s.fileSystem.NewFile("", "/some/path/otherfile.txt")

	s.NoError(nerr,"Unexpected error creating a file")

	_, werr := otherFile.Write([]byte("yooooooooooo"))
	s.NoError(werr, "Unexpected Write error")
	s.NoError(otherFile.Close(), "Unexpected close error")

	emptyFile, nerr := s.fileSystem.NewFile("C", "/test_files/empty.txt")
	s.NoError(nerr, "File creation was not successful so it does not exist")
	_, werr = emptyFile.Write([]byte(""))
	s.NoError(werr, "Unexpected Write error")

	s.NoError(emptyFile.Close(), "Unexpected close error")
	//call to CopyToFile
	s.NoError( emptyFile.CopyToFile(otherFile), "Copy to file failed unexpectedly")

	_, rerr := otherFile.Read(expectedSlice)
	s.NoError(rerr, "Unexpected Read error")
	s.Equal(expectedText, string(expectedSlice))

}

//TestMoveToLocation ensures that we can move files to specified locations
func (s *memFileTest) TestMoveToLocation() {

	newFile, nerr := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(nerr,"Unexpected error creating file")
	Touch(newFile)
	str1 := newFile.Path()
	file, merr := newFile.MoveToLocation(s.testFile.Location())
	s.NoError(merr, "Unexpected move to location error")

	exists, eerr := newFile.Exists() //newFile shouldn't exist since MovetoLocation deletes the calling object
	s.NoError(eerr,"Unexpected existence error")
	s.False(exists)

	Touch(file)
	str2 := file.Path()
	s.Equal(path.Base(str1), path.Base(str2))
	s.Equal("/test_files/", file.Location().Path())

}

//TestMoveToLocation2 creates two files with the same name in different places and moves one to the other
func (s *memFileTest) TestMoveToLocation2() {

	expectedText := "Who ya calling pinhead?"
	newFile, nerr := s.fileSystem.NewFile("", "/otherDir/foo.txt")
	s.NoError(nerr,"Unexpected error creating file")
	Touch(newFile)

	otherFile, nerr2 := s.fileSystem.NewFile("", "/thisDir/foo.txt")
	s.NoError(nerr2,"Unexpected error creating file")
	_, werr := otherFile.Write([]byte(expectedText))
	s.NoError(werr, "Unexpected write error")

	str1 := newFile.Path()
	file, _ := otherFile.MoveToLocation(newFile.Location())
	Touch(file)
	str2 := file.Path()
	s.Equal(path.Base(str1), path.Base(str2))

	exists, _ := otherFile.Exists()
	s.False(exists)
	readSlice := make([]byte, len(expectedText))

	_, rerr := file.Read(readSlice)
	s.NoError(rerr, "Unexpected read error")
	s.Equal(expectedText, string(readSlice))

}

/*
TestMoveToFile creates a newFile and moves the testFile to it.
Test succeeds if the moved file has the correct data.  They share the same name, so this is effectively a "CopyToFile" call
*/
func (s *memFileTest) TestMoveToFile() {

	expectedSlice := []byte("Hello World!")
	newFile, nerr := s.fileSystem.NewFile("", "/samples/test.txt")
	s.NoError(nerr,"Unexpected error creating file")
	Touch(newFile)

	_, werr := s.testFile.Write(expectedSlice)
	s.NoError(werr, "Write failed unexpectedly")
	s.NoError(s.testFile.Close(),"Unexpected close error")
	/*
		after this call, newFile and "s.testFile" will be deleted.
		we re-obtain the newFile pointer by calling it from our fsMap by giving it the (new) path and volume
	*/
	merr := s.testFile.MoveToFile(newFile)
	s.NoError(merr, "Move to file failed")
	newFileSlice := make([]byte, len("Hello World!"))
	newFile = s.fileSystem.fsMap[""]["/samples/test.txt"].i.(*File)
	s.False(s.testFile.Exists())

	_, rerr := newFile.Read(newFileSlice)
	s.NoError(rerr, "Read unexpectedly failed")

	s.Equal(string(expectedSlice), string(newFileSlice))
	s.Equal("/samples/test.txt", newFile.Path())

}
//TestMoveToFile2 ensures that if a call is made on a files who don't share the same name, a new file is produced with the caller's name
func (s *memFileTest) TestMoveToFile2() {

	expectedSlice := []byte("Hello World!")
	newFile, nerr := s.fileSystem.NewFile("", "/samples/diffName.txt")
	s.NoError(nerr, "File creation was not successful so it does not exist")
	newPath := "/samples/test.txt" //this is the path used to retrieve the file after it has been moved
	Touch(newFile)
	_, werr := s.testFile.Write(expectedSlice)
	s.NoError(werr, "Write failed unexpectedly")
	s.NoError(s.testFile.Close(),"Unexpected close error")
	/*
		after this call, newFile and "s.testFile" will be deleted.
		we re-obtain the newFile by calling it from our fsMap by giving it the (new) path and volume
	*/
	merr := s.testFile.MoveToFile(newFile)
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
	s.NoError(werr1, "Write did not work as expected")
	s.EqualValues(length-1, num)

}
//TestWrite2 writes to a file, and reads from it without closing it by seeking to the start
func (s *memFileTest) TestWrite2() {
	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, nerr := s.fileSystem.NewFile("", "/test_files/new.txt")
	s.NoError(nerr, "Unexpected error creating a new file")

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

	//deleting file and asserting non-existence
	s.NoError(file.Delete(),"Unexpected error deleting file")
	found2, eErr2 := file.Exists()
	s.NoError(eErr2, "exists error not expected")
	s.False(found2)
}

/*
TestLastModified Writes to a file then retrives the value that LastModified() returns and the timeStamp value
stored in the File struct and compares them against eachother.  Successful if they are equal.
*/

func (s *memFileTest) TestLastModified() {
	data := "Hello World!"
	sliceData := []byte(data)
	_, err := s.testFile.Write(sliceData)
	s.NoError(err, "Write did not work as expected!")

	t, _ := s.testFile.LastModified()

	s.Equal(s.testFile.timeStamp, *t)

}

//TestName creates a file and names it and then asserts that the given name and the return of Name() match.
func (s *memFileTest) TestName() {
	newFile, nerr := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(nerr, "Unexpected error creating file")
	Touch(newFile)
	s.Equal("test.txt", newFile.Name())
}

//TestSize allocates 64 bytes in one file and 32 in the other.  It makes sure that Size() returns the correct sizes.
func (s *memFileTest) TestSize() {
	otherFile, nerr := s.fileSystem.NewFile("", "/test.txt")
	s.NoError(nerr, "Unexpected error creting file")
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
	s.NoError(nerr, "Unexpected error creating a new file")

	_, nerr2 := s.fileSystem.NewFile("", "/test_files/bar.txt")
	s.NoError(nerr2, "Unexpected error creating a new file")

	file1, nerr3 := s.fileSystem.NewFile("", "/directory/bar.txt")
	s.NoError(nerr3, "Unexpected error creating a new file")
	Touch(file1)

	str := "/directory/test_files/test.txt"
	_, nerr4 := s.fileSystem.NewFile("", str)
	s.NoError(nerr4, "Unexpected error creating a new file")
	s.Equal("/directory/bar.txt", file1.Path())
}

//TestURI ensures that URI's for locations come out in the correct format
func (s *memFileTest) TestURI() {
	file, nerr := s.fileSystem.NewFile("C", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(nerr, "Unexpected error creating file")
	Touch(file)
	uri := file.URI()
	s.Equal("mem://C/test_files/lots/of/directories/here/we/go/test.txt", uri)
}

//TestStringer tests the implementation of io.Stringer
func (s *memFileTest) TestStringer() {

	file, nerr := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.NoError(nerr,"Unexpected error creating file")
	Touch(file)
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
