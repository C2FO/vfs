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

	_ftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/ftp/mocks"
	"github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
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
	ts.ftpClientMock = mocks.NewClient(ts.T())
	ts.fs = FileSystem{ftpclient: ts.ftpClientMock, options: Options{}}
	ts.testFile, err = ts.fs.NewFile("user@host.com:22", "/some/path/to/file.txt")
	ts.Require().NoError(err, "Shouldn't return error creating test ftp.File instance.")
	dataConnGetterFunc = getDataConn
}

var errClientGetter = errors.New("some dataconn getter error")

func clientGetterReturnsError(_ context.Context, _ authority.Authority, _ Options) (client types.Client, err error) {
	return nil, errClientGetter
}

func (ts *fileTestSuite) TestRead() {
	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(ts.T())

	contents := "hello world!"

	dc := mocks.NewDataConn(ts.T())
	dc.EXPECT().Mode().Return(types.OpenRead)

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	ftpfile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				ftpclient: client,
				options:   Options{},
				dataconn:  dc,
			},
			authority: auth,
		},
		path: fp,
	}
	// test successful read
	localFile := bytes.NewBuffer([]byte{})
	dc.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	b, copyErr := io.Copy(localFile, ftpfile)
	ts.Require().NoError(copyErr, "no error expected")
	ts.Len(contents, int(b), "byte count after copy")
	ts.Equal(contents, localFile.String(), "Copying an ftp file to a buffer should fill buffer with localfile's contents")

	// test read error
	myReadErr := errors.New("some read error")
	dc.EXPECT().Read(mock.Anything).Return(0, myReadErr).Once()
	cnt, rErr := ftpfile.Read(make([]byte, 1))
	ts.Require().Error(rErr, "no error expected")
	ts.Require().ErrorIs(rErr, myReadErr, "error is a read error")
	ts.Zero(cnt, "byte count is 0")

	// get dataconn error
	dconnErr := errors.New("some getDataConn error")
	dataConnGetterFunc = func(context.Context, authority.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error) {
		return nil, dconnErr
	}
	_, err = ftpfile.Read(make([]byte, 1))
	ts.Require().Error(err, "failure to get dataconn should return an error")
	ts.Require().ErrorIs(err, dconnErr, "should be right kind of error")
}

func (ts *fileTestSuite) TestClose() {
	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(ts.T())

	dc := mocks.NewDataConn(ts.T())

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	ftpfile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				ftpclient: client,
				options:   Options{},
				dataconn:  dc,
			},
			authority: auth,
		},
		path:   fp,
		offset: 1234,
	}

	// values set pre-test
	ts.NotNil(ftpfile.location.fileSystem.dataconn, "dataconn is not nil")
	ts.Equal(int64(1234), ftpfile.offset, "non-zero offset")

	// error closing ftpfile
	myCloseErr := errors.New("some close error")
	dc.EXPECT().Close().Return(myCloseErr).Once()
	err = ftpfile.Close()
	ts.Require().Error(err, "close error expected")

	// success closing ftpfile
	dc.EXPECT().Close().Return(nil).Once()
	err = ftpfile.Close()
	ts.Require().NoError(err, "no close error expected")

	// values zeroed after successful Close()
	ts.Nil(ftpfile.location.fileSystem.dataconn, "dataconn should be nil")
	ts.Zero(ftpfile.offset, "offset should be zero")
}

func (ts *fileTestSuite) TestWrite() {
	fakeDataConn := mocks.NewDataConn(ts.T())
	fakeDataConn.EXPECT().Mode().Return(types.OpenWrite)

	auth, err := authority.NewAuthority("user@host.com:22")
	ts.Require().NoError(err)
	file := &File{
		location: &Location{
			fileSystem: NewFileSystem(WithClient(ts.ftpClientMock)),
			authority:  auth,
		},
		path: "/tmp/hello.txt",
	}

	file.location.fileSystem.dataconn = fakeDataConn

	contents := "hello world!"

	// test write success
	var written string
	fakeDataConn.EXPECT().Write(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		written = string(p)
		return len(p), nil
	}).Once()
	count, err := file.Write([]byte(contents))
	ts.Len(contents, count, "Returned count of bytes written should match number of bytes passed to Write.")
	ts.Equal(contents, written, "expected contents written")
	ts.Require().NoError(err, "Error should be nil when calling Write")

	// test write failure
	myWriteErr := errors.New("some write error")
	fakeDataConn.EXPECT().Write(mock.Anything).Return(0, myWriteErr).Once()
	count, wErr := file.Write([]byte(contents))
	ts.Require().Error(wErr, "no error expected")
	ts.Require().ErrorIs(wErr, myWriteErr, "error is a write error")
	ts.Zero(count, "byte count is 0")

	// get client error
	dconnErr := errors.New("some getDataConn error")
	dataConnGetterFunc = func(context.Context, authority.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error) {
		return nil, dconnErr
	}
	_, err = file.Write([]byte(contents))
	ts.Require().Error(err, "failed delete should return an error")
	ts.Require().ErrorIs(err, dconnErr, "should be right kind of error")
}

func (ts *fileTestSuite) TestSeek() {
	mockDataConn := mocks.NewDataConn(ts.T())
	mockDataConn.EXPECT().Mode().Return(types.OpenRead)
	mockDataConn.EXPECT().IsTimePreciseInList().Return(true)
	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		fs.dataconn = mockDataConn
		return mockDataConn, nil
	}

	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(ts.T())

	contents := "hello world!"

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)
	ftpfile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				ftpclient: client,
				options:   Options{},
			},
			authority: auth,
		},
		path: fp,
	}

	// seek to position 6, whence 0
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	_, err = ftpfile.Seek(6, 0)
	ts.Require().NoError(err, "no error expected")
	localFile := bytes.NewBuffer([]byte{})
	mockDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents[6:])
		return len(contents) - 6, io.EOF
	}).Once()
	_, err = io.Copy(localFile, ftpfile)
	ts.Require().NoError(err, "no error expected")
	ts.Equal("world!", localFile.String(), "Seeking should move the ftp file cursor as expected")
	localFile = bytes.NewBuffer([]byte{})

	// seek back to start
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	_, err = ftpfile.Seek(0, 0)
	ts.Require().NoError(err, "no error expected")
	mockDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	_, err = io.Copy(localFile, ftpfile)
	ts.Require().NoError(err, "no error expected")
	ts.Equal(contents, localFile.String(), "Subsequent calls to seek work on temp ftp file as expected")

	// whence = 1 (seek relative position), seek 2
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	pos, err := ftpfile.Seek(6, 0) // seek to some mid point
	ts.Require().NoError(err, "no error expected")
	ts.Equal(int64(6), pos, "position check")
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	mockDataConn.EXPECT().Close().Return(nil).Once()
	pos, err = ftpfile.Seek(2, 1) // now seek to relative position
	ts.Require().NoError(err, "no error expected")
	ts.Equal(int64(8), pos, "position check")
	localFile.Reset()
	mockDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents[8:])
		return len(contents) - 8, io.EOF
	}).Once()
	_, err = io.Copy(localFile, ftpfile)
	ts.Require().NoError(err, "no error expected")
	ts.Equal("rld!", localFile.String(), "seek should be position 8, 2 relative to 6")

	// whence = 1, original file offset < 0 (not even sure if this is possible)
	ftpfile.offset = -2 // this SHOULD not be possible
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	mockDataConn.EXPECT().Close().Return(nil).Once()
	pos, err = ftpfile.Seek(5, 1)
	ts.Require().NoError(err, "no error expected")
	ts.Equal(int64(5), pos, "new offset should be 5")

	// whence = 2 (seek from end)
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{Size: uint64(len(contents))}, nil).Twice()
	mockDataConn.EXPECT().Close().Return(nil).Once()
	pos, err = ftpfile.Seek(8, 2) // seek to some mid point
	ts.Require().NoError(err, "no error expected")
	ts.Equal(int64(4), pos, "position check")
	localFile.Reset()
	mockDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents[4:])
		return len(contents) - 4, io.EOF
	}).Once()
	_, err = io.Copy(localFile, ftpfile)
	ts.Require().NoError(err, "no error expected")
	ts.Equal("o world!", localFile.String(), "seek should be position 8, 2 relative to 6")

	// dataconn != nil, so set file offset and get new dataconn
	ftpfile.offset = 8 // set it to some offset
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	offset, err := ftpfile.Seek(6, 0)
	ts.Require().NoError(err, "error not expected")
	ts.Equal(int64(6), offset, "returned offset should be 6")
	ts.Equal(int64(6), ftpfile.offset, "ftp File offset should be 6")
	ts.NotNil(ftpfile.location.fileSystem.dataconn, "dataconn should no longer be nil")

	// whence = 2, correction of offset to 0 when whence 2 and seek offset > len(contents)
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Twice()
	mockDataConn.EXPECT().Close().Return(nil).Once()
	pos, err = ftpfile.Seek(15, 2)
	ts.Require().NoError(err, "no error expected")
	ts.Zero(pos, "new offset should be 5")

	// whence = 2, file doesn't exist yet
	mockDataConn.EXPECT().GetEntry(fp).Return(nil, os.ErrNotExist).Once()
	_, err = ftpfile.Seek(15, 2)
	ts.Require().Error(err, "error expected")
	ts.Require().ErrorIs(err, os.ErrNotExist, "os error not exist expected")
}

func (ts *fileTestSuite) TestSeekError() {
	mockDataConn := mocks.NewDataConn(ts.T())
	mockDataConn.EXPECT().Mode().Return(types.OpenRead)
	mockDataConn.EXPECT().IsTimePreciseInList().Return(true)
	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		fs.dataconn = mockDataConn
		return mockDataConn, nil
	}

	// set up ftpfile
	fp := "/some/path.txt"
	client := mocks.NewClient(ts.T())
	client.EXPECT().IsTimePreciseInList().Return(true)

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)
	ftpfile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				ftpclient: client,
				options:   Options{},
			},
			authority: auth,
		},
		path: fp,
	}

	// error setting dataconn when initially nil
	dconnErr := errors.New("some getDataConn error")
	dataConnGetterFunc = func(context.Context, authority.Authority, *FileSystem, *File, types.OpenType) (types.DataConn, error) {
		return nil, dconnErr
	}
	_, err = ftpfile.Seek(6, 0)
	ts.Require().Error(err, "should return an error")
	ts.Require().ErrorIs(err, dconnErr, "should be right kind of error")

	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		fs.dataconn = mockDataConn
		return mockDataConn, nil
	}

	// whence = 1, f.dataconn.Close() error
	mockDataConn.EXPECT().GetEntry(fp).Return(&_ftp.Entry{}, nil).Once()
	closeErr := errors.New("some close error")
	mockDataConn.EXPECT().Close().Return(closeErr).Once()
	pos, err := ftpfile.Seek(3, 1)
	ts.Require().Error(err, "should return an error")
	ts.Require().ErrorIs(err, closeErr, "should be right kind of error")
	ts.Zero(pos, "position should be 0 on error")

	// whence = 2, f.Size() error (client.GetEntry error)
	dataConnGetterFunc = getDataConn
	ftpfile.Location().FileSystem().(*FileSystem).dataconn = nil
	sizeErr := errors.New("some Size error")

	client.EXPECT().
		GetEntry(ftpfile.Path()).
		Return(nil, sizeErr). // return non-ErrNotFound error when calling Size()
		Once()
	pos, err = ftpfile.Seek(3, 2)
	ts.Require().Error(err, "should return an error")
	ts.Require().ErrorIs(err, sizeErr, "should be right kind of error")
	ts.Zero(pos, "position should be 0 on error")

	// whence = 2, f.dataconn.Close() error
	ftpfile.Location().FileSystem().(*FileSystem).dataconn = mockDataConn
	mockDataConn.EXPECT().Close().Return(closeErr).Once()
	pos, err = ftpfile.Seek(3, 2)
	ts.Require().Error(err, "should return an error")
	ts.Require().ErrorIs(err, closeErr, "should be right kind of error")
	ts.EqualValues(0, pos, "position should be 0 on error")
}

func (ts *fileTestSuite) TestExists_noMLST() {
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
	ts.Require().NoError(err, "Shouldn't return an error when exists is true")

	// stat client error
	defaultClientGetter = clientGetterReturnsError
	ftpfile.(*File).location.fileSystem.ftpclient = nil
	ftpfile.(*File).location.fileSystem.dataconn = nil
	exists, err = ftpfile.Exists()
	ts.Require().Error(err, "error expected")
	ts.Require().ErrorIs(err, errClientGetter, "err should be correct type")
	ts.False(exists, "exists should be false on error")
}

func (ts *fileTestSuite) TestExists_MLST() {
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
	ts.Require().NoError(err, "Shouldn't return an error when exists is true")

	// stat client error
	defaultClientGetter = clientGetterReturnsError
	ftpfile.(*File).location.fileSystem.ftpclient = nil
	ftpfile.(*File).location.fileSystem.dataconn = nil
	exists, err = ftpfile.Exists()
	ts.Require().Error(err, "error expected")
	ts.Require().ErrorIs(err, errClientGetter, "err should be correct type")
	ts.False(exists, "exists should be false on error")
}

func (ts *fileTestSuite) TestNotExists_noMLST() {
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
	ts.Require().NoError(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestNotExists_MLST() {
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
	ts.Require().NoError(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	// set up source
	contents := "hello world!"
	fakeReadDataConn := mocks.NewDataConn(ts.T())
	fakeReadDataConn.EXPECT().IsTimePreciseInList().Return(true)
	auth2, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	sourceFS := NewFileSystem(WithClient(ts.ftpClientMock))
	sourceFile := &File{
		location: &Location{
			fileSystem: sourceFS,
			authority:  auth2,
		},
		path: "/src/hello.txt",
	}

	// set up target
	fakeWriteDataConn := mocks.NewDataConn(ts.T())
	auth, err := authority.NewAuthority("user@host.com:22")
	ts.Require().NoError(err)
	targetFS := NewFileSystem(WithClient(ts.ftpClientMock))
	targetFile := &File{
		location: &Location{
			fileSystem: targetFS,
			authority:  auth,
		},
		path: "/targ/hello.txt",
	}

	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		if fs == sourceFS {
			return fakeReadDataConn, nil
		}
		if fs == targetFS {
			return fakeWriteDataConn, nil
		}
		return fs.dataconn, nil
	}

	// successful copy
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(&_ftp.Entry{}, nil).Once()
	fakeReadDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	var written string
	fakeWriteDataConn.EXPECT().Write(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		written = string(p)
		return len(contents), nil
	}).Once()
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
	ts.Equal(contents, written, "contents match")

	// file doesn't exist error while copying
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(&_ftp.Entry{}, nil).Once()
	fakeReadDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	fakeWriteDataConn.EXPECT().Write(mock.Anything).Return(len(contents), os.ErrNotExist).Once()
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().Error(err, "error is expected")
	ts.Require().ErrorIs(err, os.ErrNotExist, "error is expected kind of error")

	// writer close error while copying
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(&_ftp.Entry{}, nil).Once()
	fakeReadDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	targetFile.location.fileSystem.dataconn = fakeWriteDataConn
	closeErr := errors.New("some close error")
	fakeWriteDataConn.EXPECT().Write(mock.Anything).Return(len(contents), nil).Once()
	fakeWriteDataConn.EXPECT().Close().Return(closeErr).Twice()
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().Error(err, "error is expected")
	ts.Require().ErrorIs(err, closeErr, "error is expected kind of error")
}

func (ts *fileTestSuite) TestCopyToLocation() {
	// set up source
	contents := "hello world!"
	fakeReadDataConn := mocks.NewDataConn(ts.T())
	fakeReadDataConn.EXPECT().IsTimePreciseInList().Return(true)
	auth2, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	sourceFS := NewFileSystem(WithClient(ts.ftpClientMock))
	sourceFile := &File{
		location: &Location{
			fileSystem: sourceFS,
			authority:  auth2,
		},
		path: "/src/hello.txt",
	}

	// set up target
	fakeWriteDataConn := mocks.NewDataConn(ts.T())
	auth, err := authority.NewAuthority("user@host.com:22")
	ts.Require().NoError(err)
	targetFS := NewFileSystem(WithClient(ts.ftpClientMock))
	targetLocation := &Location{
		fileSystem: targetFS,
		authority:  auth,
		path:       "/targ/",
	}

	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		if fs == sourceFS {
			return fakeReadDataConn, nil
		}
		if fs == targetFS {
			return fakeWriteDataConn, nil
		}
		return fs.dataconn, nil
	}

	// copy to location success
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(&_ftp.Entry{}, nil).Once()
	fakeReadDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	var written string
	fakeWriteDataConn.EXPECT().Write(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		written = string(p)
		return len(p), nil
	}).Once()
	newFile, err := sourceFile.CopyToLocation(targetLocation)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
	ts.Equal("ftp://user@host.com:22/targ/hello.txt", newFile.URI(), "new file uri check")
	ts.Equal(contents, written, "contents match")

	// copy to location newfile failure
	sourceFile.path = ""
	newFile, err = sourceFile.CopyToLocation(targetLocation)
	ts.Require().Error(err, "error is expected")
	ts.Nil(newFile, "newFile is nil")
	ts.Require().ErrorIs(err, utils.ErrBadRelFilePath, "error is correct type")
}

func (ts *fileTestSuite) TestMoveToFile_differentAuthority() {
	// set up source
	contents := "hello world!"
	fakeReadDataConn := mocks.NewDataConn(ts.T())
	fakeReadDataConn.EXPECT().IsTimePreciseInList().Return(true)
	auth2, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	sourceFS := NewFileSystem(WithClient(ts.ftpClientMock))
	sourceFile := &File{
		location: &Location{
			fileSystem: sourceFS,
			authority:  auth2,
		},
		path: "/src/hello.txt",
	}

	// set up target
	fakeWriteDataConn := mocks.NewDataConn(ts.T())
	auth, err := authority.NewAuthority("user@host.com:22")
	ts.Require().NoError(err)
	targetFS := NewFileSystem(WithClient(ts.ftpClientMock))
	targetFile := &File{
		location: &Location{
			fileSystem: targetFS,
			authority:  auth,
		},
		path: "/targ/hello.txt",
	}

	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		if fs == sourceFS {
			return fakeReadDataConn, nil
		}
		if fs == targetFS {
			return fakeWriteDataConn, nil
		}
		return fs.dataconn, nil
	}

	// successfully MoveToFile for different authorities (copy-delete)
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(&_ftp.Entry{}, nil).Once()
	fakeReadDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	fakeReadDataConn.EXPECT().Delete("/src/hello.txt").Return(nil).Once()
	var written string
	fakeWriteDataConn.EXPECT().Write(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		written = string(p)
		return len(p), nil
	}).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to MoveToFile")
	ts.Equal(contents, written, "contents match")
	ts.Equal("ftp://user@host.com:22/targ/hello.txt", targetFile.URI(), "expected uri")

	// CopyToFile failure on MoveToFile
	readErr := errors.New("some read error")
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(nil, readErr).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().Error(err, "error should be returned from successful call to MoveToFile")
	ts.Require().ErrorIs(err, readErr, "correct kind of error")
}

func (ts *fileTestSuite) TestMoveToFile_sameAuthority() {
	// set up source
	fakeReadDataConn := mocks.NewDataConn(ts.T())
	auth2, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	srcMockFTPClient := mocks.NewClient(ts.T())
	sourceFS := NewFileSystem(WithClient(srcMockFTPClient))
	sourceFile := &File{
		location: &Location{
			fileSystem: sourceFS,
			authority:  auth2,
		},
		path: "/src/hello.txt",
	}

	// set up target
	tgtMockFTPClient := mocks.NewClient(ts.T())
	fakeWriteDataConn := mocks.NewDataConn(ts.T())
	auth, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	targetFS := NewFileSystem(WithClient(tgtMockFTPClient))
	targetFile := &File{
		location: &Location{
			fileSystem: targetFS,
			authority:  auth,
		},
		path: "/targ/hello.txt",
	}

	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		if fs == sourceFS {
			return fakeReadDataConn, nil
		}
		if fs == targetFS {
			return fakeWriteDataConn, nil
		}
		return fs.dataconn, nil
	}

	// successfully MoveToFile for same authorities (rename) - dir exists
	entries := []*_ftp.Entry{
		{
			Name: path.Base(targetFile.Location().Path()),
			Type: _ftp.EntryTypeFolder,
		},
	}
	fakeReadDataConn.EXPECT().Rename("/src/hello.txt", "/targ/hello.txt").Return(nil).Once()
	fakeWriteDataConn.EXPECT().List("/").Return(entries, nil).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to MoveToFile")
	ts.Equal("ftp://123@xyz.com:3022/targ/hello.txt", targetFile.URI(), "expected uri")

	// successfully MoveToFile for same authorities (rename) - dir doesn't exist
	fakeReadDataConn.EXPECT().Rename("/src/hello.txt", "/targ/hello.txt").Return(nil).Once()
	fakeWriteDataConn.EXPECT().List("/").Return(entries, nil).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to MoveToFile")
	ts.Equal("ftp://123@xyz.com:3022/targ/hello.txt", targetFile.URI(), "expected uri")

	// get client failure
	defaultClientGetter = clientGetterReturnsError
	dataConnGetterFunc = getDataConn
	sourceFile.Location().FileSystem().(*FileSystem).ftpclient = nil
	tgtMockFTPClient.EXPECT().List("/").Return(entries, nil).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().Error(err, "error is expected")
	ts.Require().ErrorIs(err, errClientGetter, "error is the right kind of error")
	defaultClientGetter = func(ctx context.Context, auth authority.Authority, opts Options) (client types.Client, err error) {
		return GetClient(ctx, auth, opts)
	}
	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		return fs.dataconn, nil
	}
	sourceFile.location.fileSystem.dataconn = fakeReadDataConn

	// Exists failure
	existsErr := errors.New("some exists error")
	fakeReadDataConn.EXPECT().Rename("/src/hello.txt", "/targ/hello.txt").Return(existsErr).Once()
	tgtMockFTPClient.EXPECT().List("/").Return(entries, nil).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().Error(err, "error is expected")
	ts.Require().ErrorIs(err, existsErr, "error is the right kind of error")

	// Mkdir failure
	mkdirErr := errors.New("some mkdir error")
	fakeReadDataConn.EXPECT().Rename("/src/hello.txt", "/targ/hello.txt").Return(mkdirErr).Once()
	tgtMockFTPClient.EXPECT().List("/").Return(entries, nil).Once()
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().Error(err, "error is expected")
	ts.Require().ErrorIs(err, mkdirErr, "error is the right kind of error")
}

func (ts *fileTestSuite) TestMoveToLocation() {
	// set up source
	contents := "hello world!"
	fakeReadDataConn := mocks.NewDataConn(ts.T())
	fakeReadDataConn.EXPECT().IsTimePreciseInList().Return(true)
	auth, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	srcMockFTPClient := mocks.NewClient(ts.T())
	sourceFS := NewFileSystem(WithClient(srcMockFTPClient))
	sourceFile := &File{
		location: &Location{
			fileSystem: sourceFS,
			authority:  auth,
		},
		path: "/src/hello.txt",
	}

	// set up target
	fakeWriteDataConn := mocks.NewDataConn(ts.T())
	auth2, err := authority.NewAuthority("user@host.com:22")
	ts.Require().NoError(err)
	targetFS := NewFileSystem(WithClient(srcMockFTPClient))
	targetLocation := &Location{
		fileSystem: targetFS,
		authority:  auth2,
		path:       "/targ/",
	}

	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		if fs == sourceFS {
			return fakeReadDataConn, nil
		}
		if fs == targetFS {
			return fakeWriteDataConn, nil
		}
		return fs.dataconn, nil
	}

	// successful MoveToLocation
	fakeReadDataConn.EXPECT().GetEntry("/src/hello.txt").Return(&_ftp.Entry{}, nil).Once()
	fakeReadDataConn.EXPECT().Read(mock.Anything).RunAndReturn(func(p []byte) (int, error) {
		copy(p, contents)
		return len(contents), io.EOF
	}).Once()
	fakeReadDataConn.EXPECT().Delete("/src/hello.txt").Return(nil).Once()
	fakeWriteDataConn.EXPECT().Write(mock.Anything).Return(len(contents), nil).Once()
	newFile, err := sourceFile.MoveToLocation(targetLocation)
	ts.Require().NoError(err, "error shouldn't be returned from successful call to MoveToFile")
	ts.Equal("ftp://user@host.com:22/targ/hello.txt", newFile.URI(), "new file uri check")

	// failed to MoveToLocation (read error while copying)
	sourceFile.path = ""
	newFile, err = sourceFile.MoveToLocation(targetLocation)
	ts.Require().Error(err, "error is expected")
	ts.Require().ErrorIs(err, utils.ErrBadRelFilePath, "error is the right type of error")
	ts.Nil(newFile, "newFile should be nil on error")
}

func (ts *fileTestSuite) TestTouch_exists() {
	filepath := "/some/path.txt"
	// set up source
	client := mocks.NewClient(ts.T())
	dconn := mocks.NewDataConn(ts.T())
	dconn.EXPECT().Mode().Return(types.OpenRead)
	auth, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	file := &File{
		location: &Location{
			fileSystem: &FileSystem{
				ftpclient: client,
				options:   Options{},
				dataconn:  dconn,
			},
			authority: auth,
			path:      "/some/",
		},
		path: filepath,
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
	dconn.EXPECT().Close().Return(nil)
	ts.Require().NoError(file.Touch())

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

	ts.Require().NoError(file.Touch())

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
	ts.Require().Error(err, "expected error")
	ts.Require().ErrorIs(err, setTimeErr, "error is correct error type")

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
	ts.Require().Error(err, "expected error")
	ts.Require().ErrorIs(err, listErr, "error is correct error type")

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
	ts.Require().Error(err, "expected error")
	ts.Require().ErrorIs(err, listErr, "error is correct error type")
}

func (ts *fileTestSuite) TestTouch_notExists() {
	mockDataConn := mocks.NewDataConn(ts.T())
	mockDataConn.EXPECT().IsTimePreciseInList().Return(true)
	dataConnGetterFunc = func(_ context.Context, _ authority.Authority, fs *FileSystem, _ *File, _ types.OpenType) (types.DataConn, error) {
		fs.dataconn = mockDataConn
		return mockDataConn, nil
	}

	filepath := "/some/path.txt"
	// set up source
	client := mocks.NewClient(ts.T())
	client.EXPECT().IsTimePreciseInList().Return(true)
	auth, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	file := &File{
		location: &Location{
			fileSystem: &FileSystem{
				ftpclient: client,
				options:   Options{},
			},
			authority: auth,
			path:      "/some/",
		},
		path: filepath,
	}

	// success calling Touch when file does not exist
	mockDataConn.EXPECT().GetEntry(filepath).Return(nil, errors.New("550")).Once()
	mockDataConn.EXPECT().Write(mock.Anything).Return(0, nil).Once()
	mockDataConn.EXPECT().Close().Return(nil).Once()
	err = file.Touch()
	ts.Require().NoError(err, "no error expected")

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
		GetEntry(file.Path()). // initial exists check
		Return(&_ftp.Entry{}, errors.New("550")).
		Once()
	wErr := errors.New("some write error")
	// error in StorFrom should cause future writes to error because of a closed pipe
	client.EXPECT().
		StorFrom(file.Path(), mock.Anything, uint64(0)).
		Return(wErr).
		Once()

	err = file.Touch()
	ts.Require().Error(err, "expected error")
	ts.Require().ErrorIs(err, io.ErrClosedPipe, "error is correct error type")

	// allow goroutines to complete
	time.Sleep(50 * time.Millisecond)
}

func (ts *fileTestSuite) TestDelete() {
	auth, err := authority.NewAuthority("123@xyz.com:3022")
	ts.Require().NoError(err)
	mockFTPClient := mocks.NewClient(ts.T())
	testFile := &File{
		location: &Location{
			fileSystem: NewFileSystem(WithClient(mockFTPClient)),
			authority:  auth,
		},
		path: "/src/hello.txt",
	}

	// successful delete
	mockFTPClient.EXPECT().
		Delete(testFile.Path()).
		Return(nil).
		Once()
	ts.Require().NoError(testFile.Delete(), "successful delete should not return an error")

	// failure deleting
	delErr := errors.New("some delete error")
	mockFTPClient.EXPECT().
		Delete(testFile.Path()).
		Return(delErr).
		Once()
	err = testFile.Delete()
	ts.Require().Error(err, "failed delete should return an error")
	ts.Require().ErrorIs(err, delErr, "should be right kind of error")

	// failure getting client
	defaultClientGetter = clientGetterReturnsError
	testFile.Location().FileSystem().(*FileSystem).ftpclient = nil
	testFile.Location().FileSystem().(*FileSystem).dataconn = nil
	err = testFile.Delete()
	ts.Require().Error(err, "failed delete should return an error")
	ts.Require().ErrorIs(err, errClientGetter, "should be right kind of error")
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
	ts.Require().NoError(err, "Error should be nil when correctly returning time of object.")
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
	ts.Require().Error(err, "error expected")
	ts.Require().ErrorIs(err, os.ErrNotExist, "err should be os.ErrNotExist")
	ts.Nil(modTime, "LastModified's time should be nil on error")

	// stat client error
	defaultClientGetter = clientGetterReturnsError
	ts.testFile.(*File).Location().FileSystem().(*FileSystem).ftpclient = nil
	ts.testFile.(*File).Location().FileSystem().(*FileSystem).dataconn = nil
	modTime, err = ts.testFile.LastModified()
	ts.Require().Error(err, "error expected")
	ts.Require().ErrorIs(err, errClientGetter, "err should be correct type")
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
	ts.Require().Error(e, "got error as expected")
	ts.Require().ErrorIs(e, myErr, "got correct error")
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
	ts.Require().NoError(err, "Error should be nil when requesting size for file that exists.")
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
	ts.Require().Error(err, "expect error")
	ts.Require().ErrorIs(err, myErr, "got correct error")
	ts.Zero(size, "Size should be 0 on error")
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
	ts.Require().NoError(err)
	ts.Equal(expected, f.URI(), "URI test")
}

func (ts *fileTestSuite) TestStringer() {
	expected := "ftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.String(), "String test")
}

func (ts *fileTestSuite) TestNewFile() {
	var ftpFS *FileSystem
	// ftpFS is nil
	_, err := ftpFS.NewFile("user@host.com", "")
	ts.Require().ErrorIs(err, errFileSystemRequired)

	ftpFS = &FileSystem{}
	// authority is ""
	_, err = ftpFS.NewFile("", "asdf")
	ts.Require().ErrorIs(err, errAuthorityAndPathRequired)
	// path is ""
	_, err = ftpFS.NewFile("user@host.com", "")
	ts.Require().ErrorIs(err, errAuthorityAndPathRequired)

	authorityStr := "user@host.com"
	key := "/path/to/file"
	ftpFile, err := ftpFS.NewFile(authorityStr, key)
	ts.Require().NoError(err, "newFile should succeed")
	ts.IsType((*File)(nil), ftpFile, "newFile returned a File struct")
	ts.Equal(authorityStr, ftpFile.Location().Authority().String())
	ts.Equal(key, ftpFile.Path())
}
