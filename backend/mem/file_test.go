package mem

import (
	"bytes"
	"fmt"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	"github.com/c2fo/vfs/v4/backend/os"
	"github.com/c2fo/vfs/v4/backend/s3"
	"github.com/c2fo/vfs/v4/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
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

func (s *memFileTest) TestZBR(){ //zero byte read



		byteSlice := make([]byte, 0)
		_, err := s.testFile.byteBuf.Read(byteSlice)
			assert.NoError(s.T(), err, "Read of 0 bytes failed")


}

func (s *memFileTest) TestRARO(){ //read after read w/file still open to see ensure an error occurs
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

func (s *memFileTest) TestRARC(){ //read after read w/file closed between reads to see ensure an error occurs
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




func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	if err != nil {
		fmt.Println(DoesNotExist())	//error message
	}
	s.True(doesExist)		//double check that the error message was right

	otherFile, err := s.fileSystem.NewFile("","test_file/foo.txt")
	doesExist1, eerr := otherFile.Exists()
	if err!=nil {
		s.Fail("File was not succesfully created so it does not exist")
	}
	//s.True(doesExist1)
	if eerr != nil {
		fmt.Println(DoesNotExist())
	}
	s.True(doesExist1)
}

func (s *memFileTest) TestExists2() { //ensures that Exists() detects a non-existent file
	otherFile, _ := s.fileSystem.NewFile("","test_file/foo.txt")

	_ = otherFile.Delete()
	s.False(otherFile.Exists())
}



func (s *memFileTest) TestOpenFile() {
	expectedText := "hello world"
	data := make([]byte, len(expectedText))
	_, err := s.testFile.Read(data)
	assert.NoError(s.T(), err, "read error not expected")

	s.Equal(expectedText, string(data))
}

func (s *memFileTest) TestSeek() {
	expectedText := "world"
	data := make([]byte, len(expectedText))
	_, serr := s.testFile.Seek(6, 0)
	assert.NoError(s.T(), serr, "seek error not expected")
	_, rerr := s.testFile.Read(data)
	assert.NoError(s.T(), rerr, "read error not expected")
	s.Equal(expectedText, string(data))
}

func (s *memFileTest) TestCopyToLocation() {
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
	s.testFile.Write([]byte(expectedText))
	s.testFile.Close()
	s.testFile.CopyToFile(osFile)
	osFile.Close()

	size1,_ := s.testFile.Size()
	size2,err := osFile.Size()
	s.True(err == nil)
	s.True(size1 == size2)

}


func (s *memFileTest) TestCopyToFileS3(){ //testing copy to a file across file systems
	//inMem and S3  NOT WORKING

	expectedText := "Hello World!"
	//expectedSlice := []byte(expectedText)
	size := len([]byte(expectedText))
	sliceForRead := make([]byte, size)
	var s3File vfs.File
	var err error
	s3File, err  = backend.Backend(s3.Scheme).NewFile("myBucket","test_files/foo.txt")
	s.True(err==nil)
	s.testFile.Write([]byte(expectedText))
	s.testFile.Close()
	fmt.Println(s.testFile.byteBuf.Len())
	s.testFile.CopyToFile(s3File)
	num, _:= s3File.Read(sliceForRead)
	s.True(num ==size )
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
*/
func (s *memFileTest) TestWrite1() { //writes a simple string to a file. succeeds on successful write
	expectedText := "yoyoyo!"
	buf := bytes.NewBufferString(expectedText)
	num,werr1 := s.testFile.byteBuf.Write(buf.Bytes())
	assert.NoError(s.T(),werr1,"Write did not work as expected")
	assert.ObjectsAreEqualValues(0,num)

	if num == 0{
		fmt.Println("yo")
	}

}

func (s *memFileTest) TestLastModified() {
	data := "Hello World!"
	sliceData := []byte(data)
	_, err := s.testFile.Write(sliceData)
	t := time.Since(s.testFile.timeStamp)  //getting the duration between the times of write and this moment
	if t > 1000{ //if both times captured were within 1000 nanoseconds, that means LastModified was successful
		s.Fail("Last modified did not successfully capture the last modification")
	}
	assert.NoError(s.T(),err,"Last modified was unsuccessfully updated")

}

func (s *memFileTest) TestName() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")
	s.Equal("test.txt", file.Name())
}

func (s *memFileTest) TestSize() {
	tmp, _ := s.fileSystem.NewFile("", "test_files/test.txt")
	otherFile := tmp.(*File)
	s.testFile.privSlice = make([]byte, 64)
	otherFile.privSlice = make([]byte,32)
	size1, _ := s.testFile.Size()
	size2, _ := otherFile.Size()
	fmt.Println(size1,size2)
	s.True(size1>size2)

}
/*
func (s *memFileTest) TestPath() {
	file, _ := s.fileSystem.NewFile("", "test_files/test.txt")
	s.Equal(filepath.Join(file.Location().Path(), file.Name()), file.Path())
}
*/
func (s *memFileTest) TestURI() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	expected := "file:///some/file/test.txt"
	s.Equal(expected, file.URI(), "%s does not match %s", file.URI(), expected)
}

func (s *memFileTest) TestStringer() {
	file, _ := s.fileSystem.NewFile("", "/some/file/test.txt")
	s.Equal("file:///some/file/test.txt", file.String())
}

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
