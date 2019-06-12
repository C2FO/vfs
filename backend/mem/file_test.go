package mem

import (
	"fmt"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	_os "github.com/c2fo/vfs/v4/backend/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"path"
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
	file, err := fs.NewFile("", "/test_files/test.txt")

	if err != nil {
		s.Fail("No file was opened")
	}

	s.testFile = file.(*File)
	s.fileSystem = fs
	WriteZeroBytes(s.testFile)
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
// TestRARC tests a read after read w/file closed between reads to see ensure an error occurs
func (s *memFileTest) TestRARC(){
	byteSlice := make([]byte,32)
	sliceToWrite := make([]byte, 32)
	byteSlice2 := make([]byte, 32)
	for i:=0;i<32;i++{	//filling the byte slice
		b := byte(i*2)
		sliceToWrite[i] = b
	}

	_,err := s.testFile.Write(sliceToWrite) 	//Writing our bytes to the buffer so we have something to read.
	assert.NoError(s.T(),err,"Unexpected write error")
	err = s.testFile.Close()				//Closing the file so the buffer contents are stored into "privSlice".
	s.True(err==nil)
	_, err = s.testFile.Read(byteSlice)		//This is the initial read.
	s.True(err==nil)
	err = s.testFile.Close()
	s.True(err==nil)
	_, err = s.testFile.Read(byteSlice2)

	/* No error should occur here since the first read
	moved the cursor all the way through but we closed
	the file before reading again, so it should reset it.
	*/

	assert.NoError(s.T(),err,"Read after read failed!")
	assert.ObjectsAreEqualValues(byteSlice,byteSlice2)

}


/*
TestDelete deletes the receiver file, then creates another file and deletes it.
Succeeds only on both successful deletions
*/
func (s *memFileTest) TestDelete(){


	newFile,err := s.fileSystem.NewFile("","home/bar.txt")
	assert.NoError(s.T(),err,"Unexpected creation error")
	WriteZeroBytes(newFile)
	otherFile,_ := s.fileSystem.NewFile("","foo.txt")
	WriteZeroBytes(otherFile)
	existence,eerr := otherFile.Exists()
	s.True(existence)
	assert.NoError(s.T(),eerr,DoesNotExist())
	derr:=otherFile.Delete()
	assert.NoError(s.T(),derr,DeleteError())
	existence1,eerr1 := otherFile.Exists()
	s.False(existence1)
	assert.Error(s.T(), eerr1, DoesNotExist())
	s.True(systemMap["/foo.txt"]==nil)

}



//TestExists1 uses "Exists()" to check for existence of our receiver's file then creates a file and does the same thing.
func (s *memFileTest) TestExists1() {
	doesExist, err := s.testFile.Exists()
	assert.NoError(s.T(),err,DoesNotExist())
	s.True(doesExist)		//double check that the error message was right
	otherFile, err := s.fileSystem.NewFile("","/foo.txt")
	WriteZeroBytes(otherFile)
	assert.NoError(s.T(), err,"File creation was not successful so it does not exist")
	doesExist1, eerr := otherFile.Exists()
	assert.NoError(s.T(),eerr,DoesNotExist())
	s.True(doesExist1)
}

//TestExists2 ensures that "Exists()" detects a non-existent file
func (s *memFileTest) TestExists2() {
	otherFile, err := s.fileSystem.NewFile("","/test_file/foo.txt")
	s.True(err == nil)
	WriteZeroBytes(otherFile)
	derr := otherFile.Delete()
	assert.NoError(s.T(),derr,DeleteError())
	existence,eerr:=otherFile.Exists()		//should throw an error and existence should be false
	assert.Error(s.T(),eerr,DoesNotExist())
	s.False(existence)
}

func (s *memFileTest) TestNewFile(){

	file, err := s.fileSystem.NewFile("","/test_file/foo.txt")
	s.True(err==nil)
	tmp, ok := systemMap[file.Path()]  //checking our system map for a match to the given fileName
	s.True(ok)
	s.True(tmp!=nil)
}


func (s *memFileTest) TestWrite2(){
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
	assert.Error(s.T(), eErr2, "exists error not expected")
	s.False(found2)
}


func (s* memFileTest) TestNameToURI(){

	name := "/test_files/examples/foo.txt"
	_, err := s.fileSystem.NewFile("," ,name)
	s.True(err==nil)
	retFile, ok := systemMap["/test_files/examples/foo.txt"]
	WriteZeroBytes(systemMap[name])
	s.True(ok)
	existence,eerr:=systemMap[name].Exists()
	assert.NoError(s.T(),eerr,DoesNotExist())
	s.True(existence)
	retFile.location.URI()
	assert.ObjectsAreEqualValues("mem:///test_files/examples/foo.txt",retFile.URI())

}



func (s *memFileTest) TestOpenFile() {
	expectedText := "hello world"
	data := make([]byte, len(expectedText))
	_, err := s.testFile.Read(data)
	assert.NoError(s.T(), err, "read error not expected")
	s.False(expectedText == string(data))
}

func (s *memFileTest) TestSeek() {


	expectedText := "new file"
	data := make([]byte, len(expectedText))
	file, _ := s.fileSystem.NewFile("", "test_files/new.txt")

	_, werr := file.Write([]byte(expectedText))
	assert.NoError(s.T(), werr, "write error not expected")

	_, serr := file.Seek(0, 0)
	assert.NoError(s.T(), serr, "seek error not expected")
	_, rerr := file.Read(data)
	assert.NoError(s.T(), rerr, "read error not expected")
	_ = file.Close()



	/*
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
		assert.Error(s.T(), eErr2, "exists error not expected")
		s.False(found2)

	 */
}
/*
TestCopyToLocationOS copies a file to a location that has
 been passed in. Succeeds on existence of original file and its
contents in new location
*/
func (s *memFileTest) TestCopyToLocation() {

	newFile,_:= s.fileSystem.NewFile("","/home/foo.txt")
	expectedText := "hello world!"
	_,werr:=s.testFile.Write([]byte(expectedText))
	readSlice1 := make([]byte,len(expectedText))
	readSlice2 := make([]byte,len(expectedText))
	assert.NoError(s.T(),werr,"Unexpected write error")
	copiedFile,cerr := s.testFile.CopyToLocation(newFile.Location())
	assert.NoError(s.T(),cerr,"CopyToLocation unexpectedly failed")
	WriteZeroBytes(copiedFile)
	s.True(copiedFile!=nil)
	assert.EqualValues(s.T(),"/home/test.txt",copiedFile.Path())
	_,rerr:=copiedFile.Read(readSlice1)
	assert.NoError(s.T(),rerr,"Unexpected read error.")
	_,rerr2 := s.testFile.Read(readSlice2)
	assert.NoError(s.T(),rerr2,"Unexpected read error.")
	assert.EqualValues(s.T(),string(readSlice2),string(readSlice1))

}
/*
TestCopyToLocationOS copies a file to a location that has
 a file with the same name. Succeeds only on overwrite of file
 in specified location
*/
func (s *memFileTest) TestCopyToLocationOW(){

	newFile,_:= s.fileSystem.NewFile("","/home/test.txt")
	WriteZeroBytes(newFile)
	originalText := "goodbye world!"
	_,werr1 := newFile.Write([]byte(originalText))
	assert.NoError(s.T(),werr1,"Unexpected write error")
	closeErr := newFile.Close()
	assert.NoError(s.T(),closeErr,"Unexpected Close error")
	expectedText := "hello world!"
	_,werr2:=s.testFile.Write([]byte(expectedText))
	assert.NoError(s.T(),werr2,"Unexpected write error")
	closeErr2:=s.testFile.Close()
	assert.NoError(s.T(),closeErr2,"Unexpected Close error")
	readSlice := make([]byte,len(expectedText))
	copiedFile,cerr := s.testFile.CopyToLocation(newFile.Location())
	assert.NoError(s.T(),cerr,"CopyToLocation unexpectedly failed")
	s.True(copiedFile!=nil)
	assert.EqualValues(s.T(),"/home/test.txt",copiedFile.Path())
	_,rerr:=copiedFile.Read(readSlice)
	assert.NoError(s.T(),rerr,"Unexpected read error.")
	assert.EqualValues(s.T(),"hello world!",string(readSlice))

}

//TestCopyToLocationOS copies a file from the mem fs over
// to a location on the  OS fs and checks for success
func (s *memFileTest) TestCopyToLocationOS(){  //Unsure how this should work

/*
	var osFile vfs.File
	var err error
	//content := []byte("temporary file's content")
	dir, err := ioutil.TempDir("", "example")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir) // clean up
	osFileName := filepath.Join(dir, "tmpfile")

	//fmt.Println(content,dir)
	osFile, err  = backend.Backend(_os.Scheme).NewFile("",dir)
	_,zwErr:=osFile.Write([]byte(""))
	assert.NoError(s.T(),zwErr,"Unexpected write error")
	s.True(err == nil)
	expectedText:= "hello world!"
	exists,eerr:=osFile.Exists()
	assert.EqualValues(s.T(),true,exists)
	assert.NoError(s.T(),eerr,DoesNotExist())
	_,werr:=s.testFile.Write([]byte(expectedText))
	assert.NoError(s.T(),werr,"Unexpected write error")

	readSlice := make([]byte,len(expectedText))
	readSlice2:= make([]byte,len(expectedText))

	copiedFile,cerr := s.testFile.CopyToLocation(osFile.Location())
	closeErr:=copiedFile.Close()
	assert.NoError(s.T(),closeErr,"Unexpected Close error")
	assert.NoError(s.T(),cerr,"CopyToLocation unexpectedly failed")
	s.True(copiedFile!=nil)
	assert.EqualValues(s.T(),"/test_files/test.txt",s.testFile.Path()) //testFile's path should be unchanged

	assert.EqualValues(s.T(),"/temporary/test.txt",copiedFile.Path()) //new path should be that

	_,rerr:=copiedFile.Read(readSlice)
	assert.NoError(s.T(),rerr,"unexpected read error")

	_,rerr2:=s.testFile.Read(readSlice2)
	assert.NoError(s.T(),rerr2,"unexpected read error")
	assert.EqualValues(s.T(),string(readSlice2),string(readSlice)) //both reads should be the same


 */


}

//TestCopyToFile tests "CopyToFile()" between two files both in the mem FS
func (s *memFileTest) TestCopyToFile() {
	expectedText := "hello world"
	otherFile,_ := s.fileSystem.NewFile("","/test.txt")
	readSlice1:=make([]byte,len(expectedText))
	readSlice2:=make([]byte,len(expectedText))
	num,err := s.testFile.Write([]byte(expectedText))
	s.False(num==0)
	assert.NoError(s.T(),err,"No error expected from Write but got one")
	_ = s.testFile.Close()
	WriteZeroBytes(otherFile)
	strPath := otherFile.Path()
	err = s.testFile.CopyToFile(otherFile)
	assert.NoError(s.T(),err,CopyFail())
	assert.NoError(s.T(), err, CopyFail())
	_,rerr1:=s.testFile.Read(readSlice1)
	assert.NoError(s.T(),rerr1,"Unexpected read error")
	_,rerr2:=systemMap[strPath].Read(readSlice2)
	assert.NoError(s.T(),rerr2,"Unexpected read error")
	assert.ObjectsAreEqualValues(string(readSlice1),string(readSlice2))
}

//TestCopyToFileOS tests "CopyToFile()" between one file in the mem FS and the other in the os FS
func (s *memFileTest) TestCopyToFileOS(){ //testing copy to a file across file systems
	//inMem and OS

	expectedText := "Hello World!"
	var osFile vfs.File
	var err error
	osFile, err  = backend.Backend(_os.Scheme).NewFile("","test_files/foo.txt")
	WriteZeroBytes(osFile)
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
*/
func (s *memFileTest) TestMoveToLocation() {


		newFile,_ := s.fileSystem.NewFile("","/otherDir/foo.txt")
		WriteZeroBytes(newFile)
		str1 := newFile.Path()
		file, _ := newFile.MoveToLocation(s.testFile.Location())
		WriteZeroBytes(file)
		str2 := file.Path()
		assert.Equal(s.T(),path.Base(str1),path.Base(str2))

}

/*
TestMoveToFile creates a newFile and moves the testFile to it.
Test succeeds if the moved file has the correct data.

*/
func (s *memFileTest) TestMoveToFile() {

	expectedSlice := []byte("Hello World!")
	newFile,_ :=s.fileSystem.NewFile("","/samples/test.txt")
	WriteZeroBytes(newFile)
	_,werr :=s.testFile.Write(expectedSlice)
	assert.NoError(s.T(),werr,WriteError())
	cerr := s.testFile.Close()
	s.True(cerr==nil)
	merr:=s.testFile.MoveToFile(newFile)
	assert.NoError(s.T(),merr,MoveToFileError())
	newFileSlice :=	make([]byte,len("Hello World!"))
	fmt.Println(systemMap[newFile.Path()])
	s.False(s.testFile.Exists())
	_,rerr := systemMap[newFile.Path()].Read(newFileSlice)
	assert.NoError(s.T(),rerr,"Read unexpectedly failed")
	assert.ObjectsAreEqualValues(string(expectedSlice),string(newFileSlice))
	assert.ObjectsAreEqualValues(newFile.Path(),"/samples/foo.txt")


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

func (s *memFileTest) TestPath() {
	str1 := "/home/some/directory/test_files/test.txt"
	_, _ = s.fileSystem.NewFile("", str1)

	_,_ = s.fileSystem.NewFile("","test_files/bar.txt")
	file1,_ := s.fileSystem.NewFile("","/directory/bar.txt")


	str := "directory/test_files/test.txt"
	_, _ = s.fileSystem.NewFile("",str)
	s.Equal("/directory/bar.txt", file1.Path())
}

func (s *memFileTest) TestURI() {
	file, err := s.fileSystem.NewFile("", "test_files/lots/of/directories/here/we/go/test.txt")
	s.True(err==nil)
	WriteZeroBytes(file)
	uri := file.URI()
	s.Equal("mem:///test_files/lots/of/directories/here/we/go/test.txt", uri)
}

func (s *memFileTest) TestStringer() {

	file, err := s.fileSystem.NewFile("", "test_files/lots/of/directories/here/we/go/test.txt")
	s.True(err==nil)
	WriteZeroBytes(file)
	str := file.String()
	s.Equal("mem:///test_files/lots/of/directories/here/we/go/test.txt", str)

}

func TestMemFile(t *testing.T) {
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


func WriteZeroBytes(file vfs.File){

	zB := make([]byte,0)
	_,_ =file.Write(zB)
	_ = file.Close()
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