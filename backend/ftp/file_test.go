package ftp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strconv"
	"testing"
	"time"

	fs "github.com/dsoprea/go-utility/v2/filesystem"
	_ftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/ftp/mocks"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/utils"
)

type fileTestSuite struct {
	suite.Suite
	ftpClientMock *mocks.Client
	fs            FileSystem
	testFile      vfs.File
}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}

func (ts *fileTestSuite) SetupTest() {
	var err error
	ts.ftpClientMock = &mocks.Client{}
	ts.fs = FileSystem{ftpclient: ts.ftpClientMock, options: Options{}}
	ts.testFile, err = ts.fs.NewFile("user@host.com:22", "/some/path/to/file.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test ftp.File instance.")
	}
}

var errClientGetter = errors.New("some dataconn getter error")

func clientGetterReturnsError(_ context.Context, _ utils.Authority, _ Options) (client types.Client, err error) {
	return nil, errClientGetter
}

func (ts *fileTestSuite) TestRead() {

	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(ts.T())

	contents := "hello world!"

	dc := NewFakeDataConn(types.OpenRead)
	ts.NoError(dc.AssertReadContents(contents))

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	ftpfile := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
			dataconn:  dc,
		},
		authority: auth,
		path:      fp,
	}
	// test successful read
	var localFile = bytes.NewBuffer([]byte{})
	b, copyErr := io.Copy(localFile, ftpfile)
	ts.NoError(copyErr, "no error expected")
	ts.EqualValues(len(contents), b, "byte count after copy")
	ts.Equal(contents, localFile.String(), "Copying an ftp file to a buffer should fill buffer with localfile's contents")

	// test read error
	myReadErr := errors.New("some read error")
	dc.AssertReadErr(myReadErr)
	cnt, rErr := ftpfile.Read(make([]byte, 1))
	ts.Error(rErr, "no error expected")
	ts.ErrorIs(rErr, myReadErr, "error is a read error")
	ts.Equal(0, cnt, "byte count is 0")

	// get dataconn error
	dconnErr := errors.New("some getDataConn error")
	dataConnGetterFunc = func(context.Context, utils.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error) {
		return nil, dconnErr
	}
	_, err = ftpfile.Read(make([]byte, 1))
	ts.Error(err, "failure to get dataconn should return an error")
	ts.ErrorIs(err, dconnErr, "should be right kind of error")

	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestClose() {

	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(ts.T())

	contents := "hello world!"

	dc := NewFakeDataConn(types.OpenRead)
	ts.NoError(dc.AssertReadContents(contents))

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	ftpfile := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
			dataconn:  dc,
		},
		authority: auth,
		path:      fp,
		offset:    1234,
	}

	// values set pre-test
	ts.NotNil(ftpfile.fileSystem.dataconn, "dataconn is not nil")
	ts.EqualValues(1234, ftpfile.offset, "non-zero offset")

	// error closing ftpfile
	myCloseErr := errors.New("some close error")
	dc.AssertCloseErr(myCloseErr)
	err = ftpfile.Close()
	ts.Error(err, "close error expected")
	ts.Equal(1, dc.GetCloseCalledCount(), "dataconn.Close() called once")

	// success closing ftpfile
	dc.AssertCloseErr(nil)
	err = ftpfile.Close()
	ts.NoError(err, "no close error expected")

	// values zeroed after successful Close()
	ts.True(ftpfile.fileSystem.resetConn, "resetConn should be true")
	ts.EqualValues(0, ftpfile.offset, "offset should be zero")
	ts.Equal(2, dc.GetCloseCalledCount(), "dataconn.Close() called a second time")
}

func (ts *fileTestSuite) TestWrite() {
	fakeDataConn := NewFakeDataConn(types.OpenWrite)

	auth, err := utils.NewAuthority("user@host.com:22")
	ts.NoError(err)
	file := &File{
		fileSystem: NewFileSystem().WithClient(ts.ftpClientMock),
		authority:  auth,
		path:       "/tmp/hello.txt",
	}

	file.fileSystem.dataconn = fakeDataConn

	contents := "hello world!"

	// test write success
	count, err := file.Write([]byte(contents))
	ts.Equal(len(contents), count, "Returned count of bytes written should match number of bytes passed to Write.")
	ts.Equal(fakeDataConn.GetWriteContents(), contents, "expected contents written")
	ts.Nil(err, "Error should be nil when calling Write")

	// test write failure
	myWriteErr := errors.New("some write error")
	fakeDataConn.AssertWriteErr(myWriteErr)
	count, wErr := file.Write([]byte(contents))
	ts.Error(wErr, "no error expected")
	ts.ErrorIs(wErr, myWriteErr, "error is a write error")
	ts.Equal(0, count, "byte count is 0")

	// get client error
	dconnErr := errors.New("some getDataConn error")
	dataConnGetterFunc = func(context.Context, utils.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error) {
		return nil, dconnErr
	}
	_, err = file.Write([]byte(contents))
	ts.Error(err, "failed delete should return an error")
	ts.ErrorIs(err, dconnErr, "should be right kind of error")
}

func (ts *fileTestSuite) TestSeek() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up ftpfile
	fp := "/some/path.txt"
	client := &mocks.Client{}

	contents := "hello world!"

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)
	fakeDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeDataConn.AssertReadContents(contents))
	ftpfile := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
			dataconn:  fakeDataConn,
		},
		authority: auth,
		path:      fp,
	}

	// seek to position 6, whence 0
	_, err = ftpfile.Seek(6, 0)
	ts.NoError(err, "no error expected")
	var localFile = bytes.NewBuffer([]byte{})
	_, err = io.Copy(localFile, ftpfile)
	ts.NoError(err, "no error expected")
	ts.Equal("world!", localFile.String(), "Seeking should move the ftp file cursor as expected")
	localFile = bytes.NewBuffer([]byte{})

	// seek back to start
	_, err = ftpfile.Seek(0, 0)
	ts.NoError(err, "no error expected")
	_, err = io.Copy(localFile, ftpfile)
	ts.NoError(err, "no error expected")
	ts.Equal(contents, localFile.String(), "Subsequent calls to seek work on temp ftp file as expected")

	// whence = 1 (seek relative position), seek 2
	pos, err := ftpfile.Seek(6, 0) // seek to some mid point
	ts.NoError(err, "no error expected")
	ts.EqualValues(6, pos, "position check")
	pos, err = ftpfile.Seek(2, 1) // now seek to relative position
	ts.NoError(err, "no error expected")
	ts.EqualValues(8, pos, "position check")
	localFile.Reset()
	_, err = io.Copy(localFile, ftpfile)
	ts.NoError(err, "no error expected")
	ts.Equal("rld!", localFile.String(), "seek should be position 8, 2 relative to 6")

	// whence = 1, original file offset < 0 (not even sure if this is possible)
	ftpfile.offset = -2 // this SHOULD not be possible
	pos, err = ftpfile.Seek(5, 1)
	ts.NoError(err, "no error expected")
	ts.EqualValues(5, pos, "new offset should be 5")

	// whence = 2 (seek from end)
	ftpfile.fileSystem.dataconn.(*FakeDataConn).AssertSize(uint64(len(contents)))
	pos, err = ftpfile.Seek(8, 2) // seek to some mid point
	ts.NoError(err, "no error expected")
	ts.EqualValues(4, pos, "position check")
	localFile.Reset()
	_, err = io.Copy(localFile, ftpfile)
	ts.NoError(err, "no error expected")
	ts.Equal("o world!", localFile.String(), "seek should be position 8, 2 relative to 6")

	// dataconn != nil, so set file offset and get new dataconn
	ftpfile.offset = 8                  // set it to some offset
	ftpfile.fileSystem.resetConn = true // make dataconn nil
	offset, err := ftpfile.Seek(6, 0)
	ts.NoError(err, "error not expected")
	ts.EqualValues(6, offset, "returned offset should be 6")
	ts.EqualValues(6, ftpfile.offset, "ftp File offset should be 6")
	ts.NotNil(ftpfile.fileSystem.dataconn, "dataconn should no longer be nil")

	// whence = 2, correction of offset to 0 when whence 2 and seek offset > len(contents)
	pos, err = ftpfile.Seek(15, 2)
	ts.NoError(err, "no error expected")
	ts.EqualValues(0, pos, "new offset should be 5")

	// whence = 2, file doesn't exist yet
	ftpfile.fileSystem.dataconn.(*FakeDataConn).AssertExists(false)
	_, err = ftpfile.Seek(15, 2)
	ts.Error(err, "error expected")
	ts.ErrorIs(err, os.ErrNotExist, "os error not exist expected")

	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestSeekError() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up ftpfile
	fp := "/some/path.txt"
	client := &mocks.Client{}

	contents := "hello world!"

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)
	fakeDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeDataConn.AssertReadContents(contents))
	ftpfile := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
		},
		authority: auth,
		path:      fp,
	}

	// error setting dataconn when initially nil
	dconnErr := errors.New("some getDataConn error")
	dataConnGetterFunc = func(context.Context, utils.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error) {
		return nil, dconnErr
	}
	_, err = ftpfile.Seek(6, 0)
	ts.Error(err, "should return an error")
	ts.ErrorIs(err, dconnErr, "should be right kind of error")

	// whence = 1, f.dataconn.Close() error
	dataConnGetterFunc = getFakeDataConn
	fakedconn := NewFakeDataConn(types.OpenRead)
	ftpfile.fileSystem.dataconn = fakedconn
	closeErr := errors.New("some close error")
	fakedconn.AssertCloseErr(closeErr)
	pos, err := ftpfile.Seek(3, 1)
	ts.Error(err, "should return an error")
	ts.ErrorIs(err, closeErr, "should be right kind of error")
	ts.EqualValues(0, pos, "position should be 0 on error")
	fakedconn.AssertCloseErr(nil)

	// whence = 2, f.Size() error (client.GetEntry error)
	dataConnGetterFunc = getDataConn
	ftpfile.fileSystem.resetConn = true
	sizeErr := errors.New("some Size error")

	client.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	client.EXPECT().
		GetEntry(ftpfile.Path()).
		Return(nil, sizeErr). // return non-ErrNotFound error when calling Size()
		Once()
	pos, err = ftpfile.Seek(3, 2)
	ts.Error(err, "should return an error")
	ts.ErrorIs(err, sizeErr, "should be right kind of error")
	ts.EqualValues(0, pos, "position should be 0 on error")

	// whence = 2, f.dataconn.Close() error
	ftpfile.fileSystem.dataconn = fakedconn
	fakedconn.AssertCloseErr(closeErr)
	pos, err = ftpfile.Seek(3, 2)
	ts.Error(err, "should return an error")
	ts.ErrorIs(err, closeErr, "should be right kind of error")
	ts.EqualValues(0, pos, "position should be 0 on error")
	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestExists_noMlst() {
	ftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	entries := []*_ftp.Entry{
		{
			Name: ts.testFile.Name(),
			Type: _ftp.EntryTypeFile,
		},
	}
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(false).
		Once()
	ts.ftpClientMock.EXPECT().
		List(ftpfile.Path()).
		Return(entries, nil).
		Once()

	exists, err := ftpfile.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.NoError(err, "Shouldn't return an error when exists is true")

	// stat client error
	defaultClientGetter = clientGetterReturnsError
	ftpfile.(*File).fileSystem.WithClient(nil)
	ftpfile.(*File).fileSystem.dataconn = nil
	exists, err = ftpfile.Exists()
	ts.Error(err, "error expected")
	ts.ErrorIs(err, errClientGetter, "err should be correct type")
	ts.False(exists, "exists should be false on error")
}

func (ts *fileTestSuite) TestExists_mlst() {
	ftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	entry := &_ftp.Entry{
		Name: ts.testFile.Name(),
		Type: _ftp.EntryTypeFile,
	}
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ftpfile.Path()).
		Return(entry, nil).
		Once()

	exists, err := ftpfile.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.NoError(err, "Shouldn't return an error when exists is true")

	// stat client error
	defaultClientGetter = clientGetterReturnsError
	ftpfile.(*File).fileSystem.WithClient(nil)
	ftpfile.(*File).fileSystem.dataconn = nil
	exists, err = ftpfile.Exists()
	ts.Error(err, "error expected")
	ts.ErrorIs(err, errClientGetter, "err should be correct type")
	ts.False(exists, "exists should be false on error")
}

func (ts *fileTestSuite) TestNotExists_noMlst() {
	ftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(false).
		Once()
	ts.ftpClientMock.EXPECT().
		List(ftpfile.Path()).
		Return(nil, os.ErrNotExist).Once()
	exists, err := ftpfile.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.NoError(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestNotExists_mlst() {
	ftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ftpfile.Path()).
		Return(nil, os.ErrNotExist).Once()
	exists, err := ftpfile.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.NoError(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up source
	contents := "hello world!"
	fakeReadDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeReadDataConn.AssertReadContents(contents))
	auth2, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	sourceFile := &File{
		fileSystem: NewFileSystem().WithClient(ts.ftpClientMock),
		authority:  auth2,
		path:       "/src/hello.txt",
	}
	sourceFile.fileSystem.dataconn = fakeReadDataConn

	// set up target
	fakeWriteDataConn := NewFakeDataConn(types.OpenWrite)
	auth, err := utils.NewAuthority("user@host.com:22")
	ts.NoError(err)
	targetFile := &File{
		fileSystem: NewFileSystem().WithClient(ts.ftpClientMock),
		authority:  auth,
		path:       "/targ/hello.txt",
	}
	targetFile.fileSystem.dataconn = fakeWriteDataConn

	// successful copy
	err = sourceFile.CopyToFile(targetFile)
	ts.NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
	ts.Equal(contents, targetFile.fileSystem.dataconn.(*FakeDataConn).GetWriteContents(), "contents match")

	// file doesn't exist error while copying
	fakeSingleOpDataConn := NewFakeDataConn(types.SingleOp)
	fakeSingleOpDataConn.AssertExists(false)
	sourceFile.fileSystem.resetConn = false
	sourceFile.fileSystem.dataconn = fakeSingleOpDataConn
	err = sourceFile.CopyToFile(targetFile)
	ts.Error(err, "error is expected")
	ts.ErrorIs(err, os.ErrNotExist, "error is expected kind of error")

	// writer close error while copying
	fakeReadDataConn = NewFakeDataConn(types.OpenRead)
	sourceFile.fileSystem.dataconn = fakeReadDataConn
	sourceFile.fileSystem.resetConn = false
	ts.NoError(fakeReadDataConn.AssertReadContents(contents))
	fakeWriteDataConn = NewFakeDataConn(types.OpenWrite)
	targetFile.fileSystem.dataconn = fakeWriteDataConn
	targetFile.fileSystem.resetConn = false
	closeErr := errors.New("some close error")
	fakeWriteDataConn.AssertCloseErr(closeErr) // assert writer close error
	err = sourceFile.CopyToFile(targetFile)
	ts.Error(err, "error is expected")
	ts.ErrorIs(err, closeErr, "error is expected kind of error")

}

func (ts *fileTestSuite) TestCopyToLocation() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up source
	contents := "hello world!"
	fakeReadDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeReadDataConn.AssertReadContents(contents))
	auth2, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	sourceFile := &File{
		fileSystem: NewFileSystem().WithClient(ts.ftpClientMock),
		authority:  auth2,
		path:       "/src/hello.txt",
	}
	sourceFile.fileSystem.dataconn = fakeReadDataConn

	// set up target
	auth, err := utils.NewAuthority("user@host.com:22")
	ts.NoError(err)
	targetLocation := &Location{
		fileSystem: &FileSystem{
			ftpclient: ts.ftpClientMock,
		},
		Authority: auth,
		path:      "/targ/",
	}

	// copy to location success
	newFile, err := sourceFile.CopyToLocation(targetLocation)
	ts.NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
	ts.Equal(newFile.URI(), "ftp://user@host.com:22/targ/hello.txt", "new file uri check")
	ts.Equal(contents, newFile.(*File).fileSystem.dataconn.(*FakeDataConn).GetWriteContents(), "contents match")

	// copy to location newfile failure
	fakeReadDataConn = NewFakeDataConn(types.OpenRead)
	sourceFile.fileSystem.dataconn = fakeReadDataConn
	sourceFile.path = ""
	newFile, err = sourceFile.CopyToLocation(targetLocation)
	ts.Error(err, "error is expected")
	ts.Nil(newFile, "newFile is nil")
	ts.ErrorContains(err, utils.ErrBadRelFilePath, "error is correct type")
}

func (ts *fileTestSuite) TestMoveToFile_differentAuthority() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up source
	contents := "hello world!"
	fakeReadDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeReadDataConn.AssertReadContents(contents))
	auth2, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	sourceFile := &File{
		fileSystem: NewFileSystem().WithClient(ts.ftpClientMock),
		authority:  auth2,
		path:       "/src/hello.txt",
	}
	sourceFile.fileSystem.dataconn = fakeReadDataConn

	// set up target
	fakeWriteDataConn := NewFakeDataConn(types.OpenWrite)
	auth, err := utils.NewAuthority("user@host.com:22")
	ts.NoError(err)
	targetFile := &File{
		fileSystem: NewFileSystem().WithClient(ts.ftpClientMock),
		authority:  auth,
		path:       "/targ/hello.txt",
	}
	targetFile.fileSystem.dataconn = fakeWriteDataConn

	// successfully MoveToFile for different authorities (copy-delete)
	err = sourceFile.MoveToFile(targetFile)
	ts.NoError(err, "Error shouldn't be returned from successful call to MoveToFile")
	ts.Equal(contents, targetFile.fileSystem.dataconn.(*FakeDataConn).GetWriteContents(), "contents match")
	ts.Equal("ftp://user@host.com:22/targ/hello.txt", targetFile.URI(), "expected uri")

	// CopyToFile failure on MoveToFile
	fakeReadDataConn = NewFakeDataConn(types.SingleOp)
	sourceFile.fileSystem.dataconn = fakeReadDataConn
	sourceFile.fileSystem.resetConn = false
	readErr := errors.New("some read error")
	fakeReadDataConn.AssertExists(true)
	fakeReadDataConn.AssertSingleOpErr(readErr)
	err = sourceFile.MoveToFile(targetFile)
	ts.Error(err, "error should be returned from successful call to MoveToFile")
	ts.ErrorIs(err, readErr, "correct kind of error")

	ts.ftpClientMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_sameAuthority() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up source
	contents := "hello world!"
	fakeReadDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeReadDataConn.AssertReadContents(contents))
	auth2, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	srcMockFTPClient := &mocks.Client{}
	sourceFile := &File{
		fileSystem: NewFileSystem().WithClient(srcMockFTPClient),
		authority:  auth2,
		path:       "/src/hello.txt",
	}
	sourceFile.fileSystem.dataconn = fakeReadDataConn

	// set up target
	tgtMockFTPClient := &mocks.Client{}
	fakeWriteDataConn := NewFakeDataConn(types.OpenWrite)
	auth, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	targetFile := &File{
		fileSystem: NewFileSystem().WithClient(tgtMockFTPClient),
		authority:  auth,
		path:       "/targ/hello.txt",
	}
	targetFile.fileSystem.dataconn = fakeWriteDataConn

	// successfully MoveToFile for same authorities (rename) - dir exists
	entries := []*_ftp.Entry{
		{
			Name: path.Base(targetFile.Location().Path()),
			Type: _ftp.EntryTypeFolder,
		},
	}
	tgtMockFTPClient.EXPECT().
		List("/").
		Return(entries, nil).
		Once()
	srcMockFTPClient.EXPECT().
		Rename(sourceFile.Path(), targetFile.Path()).
		Return(nil).
		Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.NoError(err, "Error shouldn't be returned from successful call to MoveToFile")
	ts.Equal("ftp://123@xyz.com:3022/targ/hello.txt", targetFile.URI(), "expected uri")

	// successfully MoveToFile for same authorities (rename) - dir doesn't exist
	tgtMockFTPClient.EXPECT().
		List(targetFile.Location().Path()).
		Return([]*_ftp.Entry{}, nil).
		Once()
	srcMockFTPClient.EXPECT().
		MakeDir(targetFile.Location().Path()).
		Return(nil).
		Once()
	srcMockFTPClient.EXPECT().
		Rename(sourceFile.Path(), targetFile.Path()).
		Return(nil).
		Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.NoError(err, "Error shouldn't be returned from successful call to MoveToFile")
	ts.Equal("ftp://123@xyz.com:3022/targ/hello.txt", targetFile.URI(), "expected uri")

	// get client failure
	defaultClientGetter = clientGetterReturnsError
	dataConnGetterFunc = getDataConn
	sourceFile.fileSystem.WithClient(nil)
	sourceFile.fileSystem.resetConn = true
	err = sourceFile.MoveToFile(targetFile)
	ts.Error(err, "error is expected")
	ts.ErrorIs(err, errClientGetter, "error is the right kind of error")
	defaultClientGetter = getClient
	targetFile.fileSystem.WithClient(tgtMockFTPClient)
	dataConnGetterFunc = getFakeDataConn

	// Exists failure
	existsErr := errors.New("some exists error")
	targetFile.fileSystem.dataconn = NewFakeDataConn(types.SingleOp)
	targetFile.fileSystem.dataconn.(*FakeDataConn).AssertSingleOpErr(existsErr)
	targetFile.fileSystem.dataconn.(*FakeDataConn).AssertExists(true)
	err = sourceFile.MoveToFile(targetFile)
	ts.Error(err, "error is expected")
	ts.ErrorIs(err, existsErr, "error is the right kind of error")
	targetFile.fileSystem.dataconn = NewFakeDataConn(types.SingleOp)

	// Mkdir failure
	mkdirErr := errors.New("some mkdir error")
	tgtMockFTPClient.EXPECT().
		List(targetFile.Location().Path()).
		Return([]*_ftp.Entry{}, errors.New("550")).
		Once()
	sourceFile.fileSystem.dataconn = NewFakeDataConn(types.SingleOp)
	sourceFile.fileSystem.dataconn.(*FakeDataConn).AssertSingleOpErr(mkdirErr)
	sourceFile.fileSystem.resetConn = false
	err = sourceFile.MoveToFile(targetFile)
	ts.Error(err, "error is expected")
	ts.ErrorIs(err, mkdirErr, "error is the right kind of error")
}

func (ts *fileTestSuite) TestMoveToLocation() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	// set up source
	contents := "hello world!"
	fakeReadDataConn := NewFakeDataConn(types.OpenRead)
	ts.NoError(fakeReadDataConn.AssertReadContents(contents))
	auth, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	srcMockFTPClient := &mocks.Client{}
	sourceFile := &File{
		fileSystem: NewFileSystem().WithClient(srcMockFTPClient),
		authority:  auth,
		path:       "/src/hello.txt",
	}
	sourceFile.fileSystem.dataconn = fakeReadDataConn

	// set up target
	auth2, err := utils.NewAuthority("user@host.com:22")
	ts.NoError(err)
	targetLocation := &Location{
		fileSystem: &FileSystem{
			ftpclient: ts.ftpClientMock,
		},
		Authority: auth2,
		path:      "/targ/",
	}

	// successful MoveToLocation
	newFile, err := sourceFile.MoveToLocation(targetLocation)
	ts.NoError(err, "error shouldn't be returned from successful call to MoveToFile")
	ts.Equal(newFile.URI(), "ftp://user@host.com:22/targ/hello.txt", "new file uri check")

	// failed to MoveToLocation (read error while copying)
	sourceFile.path = ""
	newFile, err = sourceFile.MoveToLocation(targetLocation)
	ts.Error(err, "error is expected")
	ts.ErrorContains(err, utils.ErrBadRelFilePath, "error is the right type of error")
	ts.Nil(newFile, "newFile should be nil on error")

	srcMockFTPClient.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestTouch_exists() {
	filepath := "/some/path.txt"
	// set up source
	client := &mocks.Client{}
	dconn := NewFakeDataConn(types.OpenRead)
	auth, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	file := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
			dataconn:  dconn,
		},
		authority: auth,
		path:      filepath,
	}

	entry := &_ftp.Entry{
		Name: file.Name(),
		Type: _ftp.EntryTypeFile,
	}
	entries := []*_ftp.Entry{
		{
			Name: file.Name(),
			Type: _ftp.EntryTypeFolder,
		},
	}
	parentEntries := []*_ftp.Entry{
		{
			Name: "some",
			Type: _ftp.EntryTypeFolder,
		},
	}

	// success calling Touch
	p := file.Path()
	// time precision check in Touch() -> file.Exists() -> stat()
	client.EXPECT().
		IsTimePreciseInList().
		Return(false).
		Once()
	// file exists check in Touch() -> file.Exists() -> stat()
	client.EXPECT().
		List(p).
		Return(entries, nil).
		Once()
	// reject set time method each time Touch() is called
	client.EXPECT().
		IsSetTimeSupported().
		Return(false).
		Once()
	// location exists check in Touch() -> MoveToFile() -> location.Exists()
	client.EXPECT().
		List("/").
		Return(parentEntries, nil).
		Twice() // location should always exist for this test - no mkdir call

	// Move to temp file (to update last modified)
	n := time.Now()
	now = func() time.Time { return n } // override global var 'now' for test
	defer func() {
		now = time.Now
	}()
	tempFileName := file.Name() + strconv.FormatInt(now().UnixNano(), 10)
	client.EXPECT().
		Rename(p, file.Location().Path()+tempFileName).
		Return(nil).
		Once()

	// Move temp file back to original file (to update last modified)
	client.EXPECT().
		Rename(file.Location().Path()+tempFileName, file.Path()).
		Return(nil).
		Once()
	ts.NoError(file.Touch())

	// success calling Touch when SetTime is supported
	client.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	client.EXPECT().
		GetEntry(p). // initial exists check
		Return(entry, nil).
		Once()
	client.EXPECT().
		IsSetTimeSupported().
		Return(true).
		Once()
	client.EXPECT().
		SetTime(p, mock.Anything).
		Return(nil).
		Once()

	ts.NoError(file.Touch())

	// error calling Touch when SetTime is called
	setTimeErr := errors.New("some SetTime error")

	client.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	client.EXPECT().
		GetEntry(p). // initial exists check
		Return(entry, nil).
		Once()
	client.EXPECT().
		IsSetTimeSupported().
		Return(true).
		Once()
	client.EXPECT().
		SetTime(p, mock.Anything).
		Return(setTimeErr).
		Once()

	err = file.Touch()
	ts.Error(err, "expected error")
	ts.ErrorIs(err, setTimeErr, "error is correct error type")

	// failure calling Touch (exists check failed)
	listErr := errors.New("some list error")
	client.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	client.EXPECT().
		GetEntry(p). // MoveToFile exists check
		Return(nil, listErr).
		Once()
	err = file.Touch()
	ts.Error(err, "expected error")
	ts.ErrorIs(err, listErr, "error is correct error type")

	// failure calling MoveToFile in Touch func
	client.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	client.EXPECT().
		GetEntry(p). // initial exists check
		Return(entry, nil).
		Once()
	client.EXPECT().
		IsSetTimeSupported().
		Return(false).
		Once()
	client.EXPECT().
		List("/").
		Return(nil, listErr).
		Once()

	err = file.Touch()
	ts.Error(err, "expected error")
	ts.ErrorIs(err, listErr, "error is correct error type")

	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestTouch_notExists() {
	dataConnGetterFunc = getFakeDataConn
	defer func() {
		dataConnGetterFunc = getDataConn
	}()

	filepath := "/some/path.txt"
	// set up source
	client := &mocks.Client{}
	dconn := NewFakeDataConn(types.SingleOp)
	auth, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	file := &File{
		fileSystem: &FileSystem{
			ftpclient: client,
			options:   Options{},
			dataconn:  dconn,
		},
		authority: auth,
		path:      filepath,
	}

	// success calling Touch when file does not exist
	dconn.AssertExists(false)
	err = file.Touch()
	ts.NoError(err, "no error expected")

	// failure to Write in Touch func
	dataConnGetterFunc = getDataConn
	client.EXPECT().
		List("/"). // initial exists check
		Return([]*_ftp.Entry{
			{
				Name: "some",
				Type: _ftp.EntryTypeFolder,
			},
		}, nil).
		Once()
	client.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	client.EXPECT().
		GetEntry(file.Path()). // initial exists check
		Return(&_ftp.Entry{}, errors.New("550")).
		Once()
	wErr := errors.New("some write error")
	// error in StorFrom should cause future writes to error because of a closed pipe
	client.EXPECT().
		StorFrom(file.Path(), mock.Anything, uint64(0)).
		Return(wErr).
		Once()
	file.fileSystem.resetConn = false

	err = file.Touch()
	ts.Error(err, "expected error")
	ts.ErrorIs(err, io.ErrClosedPipe, "error is correct error type")

	// allow goroutines to complete
	time.Sleep(50 * time.Millisecond)

	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestDelete() {
	auth, err := utils.NewAuthority("123@xyz.com:3022")
	ts.NoError(err)
	mockFTPClient := &mocks.Client{}
	testFile := &File{
		fileSystem: NewFileSystem().WithClient(mockFTPClient),
		authority:  auth,
		path:       "/src/hello.txt",
	}

	// successful delete
	mockFTPClient.EXPECT().
		Delete(testFile.Path()).
		Return(nil).
		Once()
	ts.NoError(testFile.Delete(), "successful delete should not return an error")

	// failure deleting
	delErr := errors.New("some delete error")
	mockFTPClient.EXPECT().
		Delete(testFile.Path()).
		Return(delErr).
		Once()
	err = testFile.Delete()
	ts.Error(err, "failed delete should return an error")
	ts.ErrorIs(err, delErr, "should be right kind of error")

	// failure getting client
	defaultClientGetter = clientGetterReturnsError
	testFile.fileSystem.WithClient(nil)
	testFile.fileSystem.resetConn = true
	err = testFile.Delete()
	ts.Error(err, "failed delete should return an error")
	ts.ErrorIs(err, errClientGetter, "should be right kind of error")

	ts.ftpClientMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestLastModified() {
	now := time.Now()
	entry := &_ftp.Entry{

		Name: ts.testFile.Name(),
		Type: _ftp.EntryTypeFile,
		Time: now,
	}

	// successfully retrieved LastModified
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ts.testFile.Path()).
		Return(entry, nil).
		Once()
	modTime, err := ts.testFile.LastModified()
	ts.NoError(err, "Error should be nil when correctly returning time of object.")
	ts.Equal(&now, modTime, "Returned time matches expected LastModified time.")

	// file not found calling stat
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ts.testFile.Path()).
		Return(&_ftp.Entry{}, errors.New("550 file unavailable")).
		Once()
	modTime, err = ts.testFile.LastModified()
	ts.Error(err, "error expected")
	ts.ErrorIs(err, os.ErrNotExist, "err should be os.ErrNotExist")
	ts.Nil(modTime, "LastModified's time should be nil on error")

	// stat client error
	defaultClientGetter = clientGetterReturnsError
	ts.testFile.(*File).fileSystem.WithClient(nil)
	ts.testFile.(*File).fileSystem.resetConn = true
	modTime, err = ts.testFile.LastModified()
	ts.Error(err, "error expected")
	ts.ErrorIs(err, errClientGetter, "err should be correct type")
	ts.Nil(modTime, "LastModified's time should be nil on error")
}

func (ts *fileTestSuite) TestLastModifiedFail() {
	myErr := errors.New("some error")
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ts.testFile.Path()).
		Return(nil, myErr)
	m, e := ts.testFile.LastModified()
	ts.Error(e, "got error as expected")
	ts.ErrorIs(e, myErr, "got correct error")
	ts.Nil(m, "nil ModTime returned")
}

func (ts *fileTestSuite) TestName() {
	ts.Equal("file.txt", ts.testFile.Name(), "Name should return just the name of the file.")
}

func (ts *fileTestSuite) TestSize() {
	contentLength := uint64(100)
	entry := &_ftp.Entry{

		Name: ts.testFile.Name(),
		Type: _ftp.EntryTypeFile,
		Size: contentLength,
	}
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ts.testFile.Path()).
		Return(entry, nil).
		Once()

	size, err := ts.testFile.Size()
	ts.NoError(err, "Error should be nil when requesting size for file that exists.")
	ts.Equal(contentLength, size, "Size should return the ContentLength value from s3 HEAD request.")

	myErr := errors.New("some error")
	ts.ftpClientMock.EXPECT().
		IsTimePreciseInList().
		Return(true).
		Once()
	ts.ftpClientMock.EXPECT().
		GetEntry(ts.testFile.Path()).
		Return(&_ftp.Entry{}, myErr).
		Once()
	size, err = ts.testFile.Size()
	ts.Error(err, "expect error")
	ts.ErrorIs(err, myErr, "got correct error")
	ts.Equal(uint64(0), size, "Size should be 0 on error")

	ts.ftpClientMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestPath() {
	ts.Equal("/some/path/to/file.txt", ts.testFile.Path(), "Should return file.key (with leading slash)")
}

func (ts *fileTestSuite) TestURI() {
	expected := "ftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.URI(), "URI test")

	expected = "ftp://domain.com%5Cuser@host.com:22/some/path/to/file.txt"
	fsys := NewFileSystem()
	f, err := fsys.NewFile("domain.com%5Cuser@host.com:22", "/some/path/to/file.txt")
	ts.NoError(err)
	ts.Equal(expected, f.URI(), "URI test")
}

func (ts *fileTestSuite) TestStringer() {
	expected := "ftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.String(), "String test")
}

func (ts *fileTestSuite) TestNewFile() {
	ftpFS := &FileSystem{}
	// ftpFS is nil
	_, err := ftpFS.NewFile("user@host.com", "")
	ts.Error(err, "non-nil ftp.FileSystem pointer is required")

	// authority is ""
	_, err = ftpFS.NewFile("", "asdf")
	ts.Error(err, "non-empty strings for bucket and key are required")
	// path is ""
	_, err = ftpFS.NewFile("user@host.com", "")
	ts.Error(err, "non-empty strings for bucket and key are required")

	authority := "user@host.com"
	key := "/path/to/file"
	ftpFile, err := ftpFS.NewFile(authority, key)
	ts.NoError(err, "newFile should succeed")
	ts.IsType(&File{}, ftpFile, "newFile returned a File struct")
	ts.Equal(authority, ftpFile.Location().Volume())
	ts.Equal(key, ftpFile.Path())
}

// FakeDataConn implements a types.DataConn
type FakeDataConn struct {
	rw               *fs.SeekableBuffer
	mode             types.OpenType
	closeErr         error
	writeErr         error
	readErr          error
	singleOpErr      error
	exists           bool
	mlst             bool
	size             uint64
	closeCalledCount int
}

func (f *FakeDataConn) Delete(p string) error {
	return f.singleOpErr
}

func (f *FakeDataConn) GetEntry(p string) (*_ftp.Entry, error) {
	if f.exists {
		return &_ftp.Entry{
			Size: f.size,
		}, f.singleOpErr
	} else {
		return nil, errors.New("550")
	}
}

func (f *FakeDataConn) List(p string) ([]*_ftp.Entry, error) {
	if f.exists {
		return []*_ftp.Entry{
			{
				Type: _ftp.EntryTypeFolder,
			},
		}, f.singleOpErr

	}
	return nil, errors.New("550")

}

func (f *FakeDataConn) MakeDir(p string) error {
	return f.singleOpErr

}

func (f *FakeDataConn) Rename(from, to string) error {
	return f.singleOpErr

}

func (f *FakeDataConn) IsSetTimeSupported() bool {
	return false

}

func (f *FakeDataConn) SetTime(p string, t time.Time) error {
	return f.singleOpErr

}

func (f *FakeDataConn) IsTimePreciseInList() bool {
	return f.mlst

}

func (f *FakeDataConn) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.rw.Read(p)
}

func (f *FakeDataConn) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.rw.Write(p)
}

func (f *FakeDataConn) Close() error {
	f.closeCalledCount++
	return f.closeErr
}

func (f *FakeDataConn) Mode() types.OpenType {
	return f.mode
}

func NewFakeDataConn(mode types.OpenType) *FakeDataConn {
	buf := fs.NewSeekableBuffer()
	return &FakeDataConn{
		mode: mode,
		rw:   buf,
	}
}

func (f *FakeDataConn) AssertReadErr(err error) {
	f.readErr = err
}

func (f *FakeDataConn) AssertWriteErr(err error) {
	f.writeErr = err
}

func (f *FakeDataConn) AssertCloseErr(err error) {
	f.closeErr = err
}

func (f *FakeDataConn) AssertExists(exists bool) {
	f.exists = exists
}

func (f *FakeDataConn) AssertSingleOpErr(err error) {
	f.singleOpErr = err
}

func (f *FakeDataConn) AssertMlst(mlst bool) {
	f.mlst = mlst
}

func (f *FakeDataConn) AssertSize(size uint64) {
	f.size = size
}

func (f *FakeDataConn) AssertReadContents(contents string) error {
	// write contents to buffer
	_, err := f.rw.Write([]byte(contents))
	if err != nil {
		return err
	}

	// reset cursor after writing contents
	_, err = f.rw.Seek(0, 0)

	return err
}

func (f *FakeDataConn) GetWriteContents() string {
	return string(f.rw.Bytes())
}

func (f *FakeDataConn) GetCloseCalledCount() int {
	return f.closeCalledCount
}

func getFakeDataConn(_ context.Context, a utils.Authority, fileSystem *FileSystem, f *File, t types.OpenType) (types.DataConn, error) {
	if fileSystem.dataconn != nil {
		if fileSystem.dataconn.Mode() != t {
			// wrong session type ... close current session and unset it (so we can set a new one after)
			err := fileSystem.dataconn.Close()
			if err != nil {
				return fileSystem.dataconn, err
			}
			if f != nil {
				f.fileSystem.resetConn = true
			}
		}
	}

	if f != nil && f.fileSystem.resetConn {
		if f != nil {
			f.fileSystem.resetConn = false
		}
		contents := fileSystem.dataconn.(*FakeDataConn).rw.Bytes()
		fileSystem.dataconn = NewFakeDataConn(t)
		_, err := fileSystem.dataconn.Write(contents)
		if err != nil {
			return nil, err
		}
		_, err = fileSystem.dataconn.(*FakeDataConn).rw.Seek(0, 0)
		if err != nil {
			return nil, err
		}
		fileSystem.dataconn.(*FakeDataConn).exists = true
		fileSystem.dataconn.(*FakeDataConn).mlst = true
	}

	if fileSystem.dataconn == nil {
		fileSystem.dataconn = NewFakeDataConn(t)
	}

	// Seek to offset (whence is always zero because of the way file.Seek calculates it for you)
	if f != nil {
		_, err := fileSystem.dataconn.(*FakeDataConn).rw.Seek(f.offset, 0)
		if err != nil {
			return nil, err
		}
	}

	return fileSystem.dataconn, nil
}
