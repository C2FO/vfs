package mem

import (
	"fmt"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	"github.com/c2fo/vfs/v4/backend/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
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
	//setupTestFiles()
}

func (s *memFileTest) TearDownSuite() {
	teardownTestFiles()
}

func (s *memFileTest) SetupTest() {
	fs := &FileSystem{}
	file, err := fs.NewFile("", "test_files/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file.(*File)
	s.fileSystem = fs
}

func (s *memFileTest) TeardownTest() {
	err := s.testFile.Close()
	assert.NoError(s.T(), err, "close error not expected")
}
//TestZBR ensures that a zero byte read is valid
func (s *memFileTest) TestZBR(){



		byteSlice := make([]byte, 0)
		_, err := s.testFile.byteBuf.Read(byteSlice)
			assert.NoError(s.T(), err, "Read of 0 bytes failed")


}
//TestRARO expects an error to occur when reading after a full read of the file without first closing it
func (s *memFileTest) TestRARO(){
		byteSlice := make([]byte,32)
		sliceToWrite := make([]byte, 32)
		byteSlice2 := make([]byte, 32)
		for i:=0;i<32;i++{	//filling the byte slice
			 b := byte(i*2)
			sliceToWrite[i] = b
		}

	_,_ = s.testFile.Write(sliceToWrite) 	//writing our bytes to the buffer so we have something to read
	_ =s.testFile.Close()				//closing the file so the buffer contents are stored into "privSlice"
	_,_ =s.testFile.Read(byteSlice)		//initial read
	fmt.Println(s.testFile.byteBuf.Len())
	_, err := s.testFile.Read(byteSlice2)

	/* an error should occur here since the first read
		moved the cursor all the way through and we did
		not close the file before reading again
	*/

	assert.Error(s.T(),err,"Read after read failed!")

}
// TestRARC tests a read after read w/file closed between reads to see ensure no error occurs
func (s *memFileTest) TestRARC(){
		byteSlice := make([]byte,32)
		sliceToWrite := make([]byte, 32)
		byteSlice2 := make([]byte, 32)
		for i:=0;i<32;i++{	//filling the byte slice
		b := byte(i*2)
		sliceToWrite[i] = b
		}

		_,err := s.testFile.Write(sliceToWrite) 	//writing our bytes to the buffer so we have something to read
		s.True(err==nil)
		err = s.testFile.Close()				//closing the file so the buffer contents are stored into "privSlice"
		s.True(err==nil)
	_, err = s.testFile.Read(byteSlice)		//initial read
	s.True(err==nil)
	fmt.Println(s.testFile.byteBuf.Len())
	err = s.testFile.Close()
	s.True(err==nil)
	_, err = s.testFile.Read(byteSlice2)

		/* no error should occur here since the first read
		moved the cursor all the way through but we closed
		the file before reading again, so it should reset it
		*/
		assert.NoError(s.T(),err,"Read after read failed!")

}

/*
TestDelete deletes the receiver file, then creates another file and deletes it.
Succeeds only on both successful deletions
*/
func (s *memFileTest) TestDelete(){
	err := s.testFile.Delete()
	assert.NoError(s.T(),err,DeleteError())
	tmp,_ := s.fileSystem.NewFile("","foo.txt")
	otherFile := tmp.(*File)
	existence,eerr := otherFile.Exists()
	s.True(existence)
	assert.NoError(s.T(),eerr,DoesNotExist())
	otherFile.Delete()
	existence1,eerr1 := otherFile.Exists()
	s.False(existence1)
	assert.Error(s.T(), eerr1, DoesNotExist())

}



//TestExists1 uses "Exists()" to check for existence of our receiver's file then creates a file and does the same thing.
func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	assert.NoError(s.T(),err,DoesNotExist())
	s.True(doesExist)		//double check that the error message was right
	otherFile, err := s.fileSystem.NewFile("","foo.txt")
	assert.NoError(s.T(), err,"File creation was not successful so it does not exist")
	doesExist1, eerr := otherFile.Exists()
	assert.NoError(s.T(),eerr,DoesNotExist())
	s.True(doesExist1)
}

//TestExists2 ensures that "Exists()" detects a non-existent file
func (s *memFileTest) TestExists2() {
	otherFile, err := s.fileSystem.NewFile("","test_file/foo.txt")
	s.True(err == nil)
	derr := otherFile.Delete()
	assert.NoError(s.T(),derr,DoesNotExist())
}



func (s *memFileTest) TestOpenFile() {
	/*
	expectedText := "hello world"
	data := make([]byte, len(expectedText))
	_, err := s.testFile.Read(data)
	assert.NoError(s.T(), err, "read error not expected")

	s.Equal(expectedText, string(data))

	 */
}

func (s *memFileTest) TestSeek() {
	/*
	expectedText := "world"
	data := make([]byte, len(expectedText))
	_, serr := s.testFile.Seek(6, 0)
	assert.NoError(s.T(), serr, "seek error not expected")
	_, rerr := s.testFile.Read(data)
	assert.NoError(s.T(), rerr, "read error not expected")
	s.Equal(expectedText, string(data))

	 */
}

func (s *memFileTest) TestCopyToLocation() {
	/*
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

	 */
}

func (s *memFileTest) TestCopyToFile() {
	expectedText := "hello world"
	tmp,_ := s.fileSystem.NewFile("","/test_files/test.txt")

	otherFile := tmp.(*File)
	_,_ = s.testFile.Write([]byte(expectedText))
	_ = s.testFile.Close()
	_ = s.testFile.CopyToFile(otherFile)
	length:=len(s.testFile.privSlice)
	var err error = nil
	for i:=0;i<length;i++{
		if s.testFile.privSlice[i] != otherFile.privSlice[i]{
			err = CopyFail()
		}

	}
	assert.NoError(s.T(),err,CopyFail())
}

func (s *memFileTest) TestCopyToFileOS(){ //testing copy to a file across file systems
//inMem and OS

	expectedText := "Hello World!"
	var osFile vfs.File
	var err error
	osFile, err  = backend.Backend(os.Scheme).NewFile("","test_files/foo.txt")
	s.True(err == nil)
	num,err := s.testFile.Write([]byte(expectedText))
	s.False(num == 0)
	assert.NoError(s.T(),err,"No error expected from Write but got one")
	_ = s.testFile.Close()
	err = s.testFile.CopyToFile(osFile)
	assert.NoError(s.T(),err,CopyFail())
	_ = osFile.Close()

	size1,_ := s.testFile.Size()
	size2,err := osFile.Size()
	s.True(err == nil)
	s.True(size1 == size2)

}




/*
func (s *memFileTest) TestEmptyCopyToFile() {
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

func (s *memFileTest) TestCopyToLocationIgnoreExtraSeparator() {
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

func (s *memFileTest) TestMoveToLocation() {
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
	//location := Location{dir, s.fileSystem}

	//move the file to new location
	//movedFile, err := file.MoveToLocation(&location)
	//if err != nil {
	//	s.Fail(err.Error())
	//}

	//s.Equal(movedFile.Location().Path(), location.Path())

	//ensure the original file no longer exists
	origFile, _ := s.fileSystem.NewFile(file.Location().Volume(), origFileName)
	origFound, eerr := origFile.Exists()
	assert.NoError(s.T(), eerr, "exists error not expected")
	s.False(origFound)
}

func (s *memFileTest) TestMoveToFile() {
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
/*
TestWrite writes a string to a file and checks for success by comparing the number of bytes
written by "Write()" to the length of the slice it wrote from
*/
func (s *memFileTest) TestWrite() {
	expectedText := "I'm fed up with this world"			//-Tommy Wiseau
	bSlice := []byte(expectedText)
	length := len(bSlice)
	num,werr1 := s.testFile.Write(bSlice)
	assert.NoError(s.T(),werr1,"Write did not work as expected")
	assert.ObjectsAreEqualValues(length,num)


}

/*
TestLastModified Writes to a file then retrives the value that LastModified() returns and the timeStamp value
stored in the File struct and compares them against eachother.  Successful if they are equal.
*/

func (s *memFileTest) TestLastModified() {
	data := "Hello World!"
	sliceData := []byte(data)
	_, err := s.testFile.Write(sliceData)
	assert.NoError(s.T(),err,"Write did not work as expected!")

	t,_ :=s.testFile.LastModified()

	assert.ObjectsAreEqualValues(s.testFile.timeStamp,*t)

}

//TestName creates a file and names it and then asserts that the given name and the return of Name() match.
func (s *memFileTest) TestName() {
	tmp, err := s.fileSystem.NewFile("", "test_files/lots/of/directories/here/we/go/test.txt")
	s.True(err==nil)
	file := tmp.(*File)
	s.Equal("test.txt", file.Name())
}

//TestSize allocates 64 bytes in one file and 32 in the other.  It makes sure that Size() returns the correct sizes.
func (s *memFileTest) TestSize() {
	tmp, err := s.fileSystem.NewFile("", "test.txt")
	s.True(err == nil)
	otherFile := tmp.(*File)
	s.testFile.privSlice = make([]byte, 64)
	otherFile.privSlice = make([]byte,32)
	size1, serr1 := s.testFile.Size()
	s.True(serr1 == nil)
	size2, serr2 := otherFile.Size()
	s.True(serr2 == nil)
	s.True(size1>size2)

}
/*
func (s *memFileTest) TestPath() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")
	s.Equal(filepath.Join(file.Location().Path(), file.Name()), file.Path())
}
*/

func (s *memFileTest) TestURI() {
	tmp, err := s.fileSystem.NewFile("", "/test_files/lots/of/directories/here/we/go/test.txt")
	s.True(err==nil)
	file := tmp.(*File)
	fmt.Println(file.URI())
	s.Equal("file:///test_files/lots/of/directories/here/we/go/test.txt", file.URI())
}
/*
func (s *memFileTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Equal("file:///some/file/test.txt", file.String())
}
*/
func TestOSFile(t *testing.T) {
	suite.Run(t, new(memFileTest))
	//_ = os.Remove("test_files/new.txt")
}

/*
	Setup TEST FILES
*/

func setupTestFiles() {
/*
	// setup "test_files" dir
	createDir("test_files")

	// setup "test_files/test.txt"
	writeStringFile("test_files/empty.txt", ``)

	// setup "test_files/test.txt"
	writeStringFile("test_files/prefix-file.txt", `hello, Dave`)

	// setup "test_files/test.txt"
	writeStringFile("test_files/test.txt", `hello world`)

	// setup "test_files/subdir" dir
	createDir("test_files/subdir")

	// setup "test_files/subdir/test.txt"
	writeStringFile("test_files/subdir/test.txt", `hello world too`)
*/
}


func teardownTestFiles() {
//	err := os.RemoveAll("test_files")
	//if err != nil {
	//	panic(err)
	//}
}

func createDir(dirname string) {

	//perm := os.FileMode(0755)
	//err := os.Mkdir(dirname, perm)
	//if err != nil {
	//	teardownTestFiles()
	//	panic(err)
	//}
}

func writeStringFile(filename, data string) {
	/*
	f, err := os.Create(filename)
	if err != nil {
		teardownTestFiles()
		panic(err)
	}
	_, err = f.WriteString(data)
	if err != nil {
		teardownTestFiles()
		panic(err)
	}
	err = f.Close()
	if err != nil {
		teardownTestFiles()
		panic(err)
	}

	 */
}
