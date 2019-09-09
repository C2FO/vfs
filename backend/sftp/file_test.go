package sftp

import (
	"bytes"
	"errors"
	"github.com/pkg/sftp"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/backend/sftp/mocks"
	_mocks "github.com/c2fo/vfs/v5/mocks"
	"github.com/c2fo/vfs/v5/utils"
)

type fileTestSuite struct {
	suite.Suite
	sftpMock *mocks.Client
	fs       FileSystem
	testFile vfs.File
}

func (ts *fileTestSuite) SetupTest() {
	var err error
	ts.sftpMock = &mocks.Client{}
	ts.fs = FileSystem{sftpclient: ts.sftpMock, options: Options{}}
	ts.testFile, err = ts.fs.NewFile("user@host.com:22", "/some/path/to/file.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test sftp.File instance.")
	}
}

//this wraps strings.Reader to satisfy SFTPFile interface
type nopWriteCloser struct {
	io.ReadSeeker
}

func (nopWriteCloser) Close() error                      { return nil }
func (nopWriteCloser) Write(p []byte) (n int, err error) { return 0, nil }

func (ts *fileTestSuite) TestRead() {

	// set up sftpfile
	filepath := "/some/path.txt"
	client := &mocks.Client{}

	contents := "hello world!"

	sftpfile := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     filepath,
		sftpfile: &nopWriteCloser{strings.NewReader(contents)},
	}
	// perform test
	var localFile = bytes.NewBuffer([]byte{})

	b, copyErr := io.Copy(localFile, sftpfile)
	ts.NoError(copyErr, "no error expected")
	ts.Equal(b, int64(12), "byte count after copy")
	ts.NoError(sftpfile.Close(), "no error expected")
	ts.Equal(localFile.String(), contents, "Copying an sftp file to a buffer should fill buffer with localfile's contents")

	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestSeek() {

	// set up sftpfile
	filepath := "/some/path.txt"
	client := &mocks.Client{}

	contents := "hello world!"

	sftpfile := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     filepath,
		sftpfile: &nopWriteCloser{strings.NewReader(contents)},
	}
	// perform test
	_, seekErr := sftpfile.Seek(6, 0)
	ts.NoError(seekErr, "no error expected")

	var localFile = bytes.NewBuffer([]byte{})

	_, copyErr := io.Copy(localFile, sftpfile)
	ts.NoError(copyErr, "no error expected")

	ts.Equal("world!", localFile.String(), "Seeking should move the sftp file cursor as expected")

	localFile = bytes.NewBuffer([]byte{})
	_, seekErr2 := sftpfile.Seek(0, 0)
	ts.NoError(seekErr2, "no error expected")

	_, copyErr2 := io.Copy(localFile, sftpfile)
	ts.NoError(copyErr2, "no error expected")
	ts.Equal(contents, localFile.String(), "Subsequent calls to seek work on temp sftp file as expected")

	closeErr := sftpfile.Close()
	ts.NoError(closeErr, "no error expected")
	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) Test_openFile() {

	// set up sftpfile
	filepath := "/some/path.txt"
	client := &mocks.Client{}

	file1 := &nopWriteCloser{strings.NewReader("file 1")}

	file := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     filepath,
		sftpfile: file1,
	}

	// file already opened
	f, err := file.openFile(os.O_RDONLY)
	ts.NoError(err, "no error expected")
	b, err := ioutil.ReadAll(f)
	ts.NoError(err, "no error expected")
	ts.Equal(string(b), "file 1", "mock returned")

	//file not open, open for read
	file.sftpfile = nil
	file.opener = func(c Client, p string, f int) (SFTPFile, error) { return file1, nil }
	file1.Seek(0, 0) //reset file
	f, err = file.openFile(os.O_RDONLY)
	ts.NoError(err, "no error expected")
	b, err = ioutil.ReadAll(f)
	ts.NoError(err, "no error expected")
	ts.Equal(string(b), "file 1", "mock returned")

	//file not open, user default opener
	file.sftpfile = nil
	file.opener = nil
	client.On("OpenFile", filepath, os.O_RDONLY).Return(&sftp.File{}, nil)
	f, err = file.openFile(os.O_RDONLY)
	ts.NoError(err, "no error expected")
	ts.IsType(&sftp.File{}, f, "type check")

	//file not open, open for create/write
	file.sftpfile = nil
	file.opener = func(c Client, p string, f int) (SFTPFile, error) { return file1, nil }
	file1.Seek(0, 0) //reset file
	client.On("MkdirAll", path.Dir(filepath)).Return(nil)
	f, err = file.openFile(os.O_RDWR|os.O_CREATE)
	ts.NoError(err, "no error expected")
	b, err = ioutil.ReadAll(f)
	ts.NoError(err, "no error expected")
	ts.Equal(string(b), "file 1", "mock returned")

	client.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestExists() {
	sftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	ts.sftpMock.On("MkdirAll", sftpfile.Location().Path()).Return(nil).Once()
	ts.sftpMock.On("Stat", sftpfile.Path()).Return(nil, nil).Once()

	exists, err := sftpfile.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.Nil(err, "Shouldn't return an error when exists is true")
}

func (ts *fileTestSuite) TestNotExists() {
	sftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	ts.sftpMock.On("MkdirAll", sftpfile.Location().Path()).Return(nil).Once()
	ts.sftpMock.On("Stat", sftpfile.Path()).Return(nil, os.ErrNotExist).Once()
	exists, err := sftpfile.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.Nil(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	contentLength := 0

	// set up source
	sourceFileInfo := &mocks.FileInfo{}
	sourceFileInfo.On("Size").Return(int64(contentLength))

	sourceClient := &mocks.Client{}
	sourceClient.On("Stat", mock.Anything).Return(sourceFileInfo, nil).Once()

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Close").Return(nil).Once()

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(contentLength, nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host2.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err := sourceFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceFileInfo.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToLocation() {
	contentLength := 0

	// set up source
	sourceFileInfo := &mocks.FileInfo{}
	sourceFileInfo.On("Size").Return(int64(contentLength))

	sourceClient := &mocks.Client{}
	sourceClient.On("Stat", mock.Anything).Return(sourceFileInfo, nil).Once()

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Close").Return(nil).Once()

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(contentLength, nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host2.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
	}

	targetMockFileSystem := &_mocks.FileSystem{}
	targetMockFileSystem.On("NewFile", mock.Anything, mock.Anything).Return(targetFile, nil)

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("FileSystem").Return(targetMockFileSystem)
	targetMockLocation.On("Volume").Return(targetFile.Authority.String())
	targetMockLocation.On("Path").Return("/some/")

	// run tests
	newFile, err := sourceFile.CopyToLocation(targetMockLocation)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.Equal(newFile.URI(), "sftp://user@host2.com:22/some/path.txt", "new file uri check")

	sourceClient.AssertExpectations(ts.T())
	sourceFileInfo.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_differentAuthority() {
	contentLength := 0

	// set up source
	sourceFileInfo := &mocks.FileInfo{}
	sourceFileInfo.On("Size").Return(int64(contentLength))

	sourceClient := &mocks.Client{}
	sourceClient.On("Stat", mock.Anything).Return(sourceFileInfo, nil).Once()
	sourceClient.On("Remove", mock.Anything).Return(nil).Once()

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Close").Return(nil).Once()

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(contentLength, nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host2.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
	}

	// run tests
	err := sourceFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceFileInfo.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_sameAuthority() {
	// set up source
	sourceClient := &mocks.Client{}
	sourceClient.On("Rename", mock.Anything, mock.Anything).Return(nil).Once()
	sourceClient.On("MkdirAll", mock.Anything).Return(nil).Once()

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path: "/some/path.txt",
	}

	// set up target
	targetFileInfo := &mocks.FileInfo{}
	targetFileInfo.On("IsDir").Return(true).Once()

	targetClient := &mocks.Client{}
	targetClient.On("Stat", mock.Anything).Return(nil, os.ErrNotExist).Once()

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path: "/some/other/path.txt",
	}

	// run tests
	err := sourceFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToLocation() {
	contentLength := 0

	// set up source
	sourceFileInfo := &mocks.FileInfo{}
	sourceFileInfo.On("Size").Return(int64(contentLength))

	sourceClient := &mocks.Client{}
	sourceClient.On("Stat", mock.Anything).Return(sourceFileInfo, nil).Once()
	sourceClient.On("Remove", mock.Anything).Return(nil).Once()

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Close").Return(nil).Once()

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(contentLength, nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host2.com:22",
			User: "user",
		},
		path:     "/some/other/path.txt",
		sftpfile: targetSftpFile,
	}

	targetMockFileSystem := &_mocks.FileSystem{}
	targetMockFileSystem.On("NewFile", mock.Anything, mock.Anything).Return(targetFile, nil)

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("FileSystem").Return(targetMockFileSystem)
	targetMockLocation.On("Volume").Return(targetFile.Authority.String())
	targetMockLocation.On("Path").Return("/some/other/")

	// run tests
	newFile, err := sourceFile.MoveToLocation(targetMockLocation)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.Equal(newFile.URI(), "sftp://user@host2.com:22/some/other/path.txt", "new file uri check")

	sourceClient.AssertExpectations(ts.T())
	sourceFileInfo.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
	targetMockFileSystem.AssertExpectations(ts.T())
	targetMockLocation.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestTouch_exists() {
	filepath := "/some/path.txt"
	// set up source
	sourceFileInfo := &mocks.FileInfo{}

	client := &mocks.Client{}
	client.On("Stat", filepath).Return(sourceFileInfo, nil).Once()
	client.On("Chtimes", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	sftpFile := &mocks.SFTPFile{}

	file := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     filepath,
		sftpfile: sftpFile,
	}

	ts.NoError(file.Touch())

	client.AssertExpectations(ts.T())
	sftpFile.AssertExpectations(ts.T())
	sourceFileInfo.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestTouch_notExists() {
	filepath := "/some/path.txt"
	// set up source
	sourceFileInfo := &mocks.FileInfo{}

	client := &mocks.Client{}
	client.On("Stat", filepath).Return(sourceFileInfo, os.ErrNotExist).Once()

	sftpFile := &mocks.SFTPFile{}
	sftpFile.On("Close").Return(nil).Once()

	file := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: utils.Authority{
			Host: "host1.com:22",
			User: "user",
		},
		path:     filepath,
		sftpfile: sftpFile,
	}

	ts.NoError(file.Touch())

	client.AssertExpectations(ts.T())
	sftpFile.AssertExpectations(ts.T())
	sourceFileInfo.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestDelete() {
	ts.sftpMock.On("Remove", ts.testFile.Path()).Return(nil).Once()
	err := ts.testFile.Delete()
	ts.Nil(err, "Successful delete should not return an error.")
	ts.sftpMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestLastModified() {
	now := time.Now()
	file1 := &mocks.FileInfo{}
	file1.On("ModTime").Return(now, nil)
	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(file1, nil)
	modTime, err := ts.testFile.LastModified()
	ts.Nil(err, "Error should be nil when correctly returning time of object.")
	ts.Equal(&now, modTime, "Returned time matches expected LastModified time.")
}

func (ts *fileTestSuite) TestLastModifiedFail() {
	file1 := &mocks.FileInfo{}
	myErr := errors.New("some error")
	file1.On("ModTime").Return(time.Time{}, myErr)
	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(nil, myErr)
	m, e := ts.testFile.LastModified()
	ts.Error(e, "got error as expected")
	ts.Nil(m, "nil ModTime returned")
}

func (ts *fileTestSuite) TestName() {
	ts.Equal("file.txt", ts.testFile.Name(), "Name should return just the name of the file.")
}

func (ts *fileTestSuite) TestSize() {
	contentLength := int64(100)
	file1 := &mocks.FileInfo{}
	file1.On("Size").Return(contentLength)
	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(file1, nil).Once()
	size, err := ts.testFile.Size()
	ts.Nil(err, "Error should be nil when requesting size for file that exists.")
	ts.Equal(uint64(contentLength), size, "Size should return the ContentLength value from s3 HEAD request.")

	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(&mocks.FileInfo{}, errors.New("some error")).Once()
	size, err = ts.testFile.Size()
	ts.Error(err, "expect error")
	ts.Equal(uint64(0), size, "Size should be 0 on error")

	ts.sftpMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestPath() {
	ts.Equal("/some/path/to/file.txt", ts.testFile.Path(), "Should return file.key (with leading slash)")
}

func (ts *fileTestSuite) TestURI() {
	expected := "sftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.URI(), "URI test")
}

func (ts *fileTestSuite) TestStringer() {
	expected := "sftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.String(), "String test")
}

func (ts *fileTestSuite) TestNewFile() {
	fs := &FileSystem{}
	// fs is nil
	_, err := fs.NewFile("user@host.com", "")
	ts.Errorf(err, "non-nil sftp.FileSystem pointer is required")

	// authority is ""
	_, err = fs.NewFile("", "asdf")
	ts.Errorf(err, "non-empty strings for bucket and key are required")
	// path is ""
	_, err = fs.NewFile("user@host.com", "")
	ts.Errorf(err, "non-empty strings for bucket and key are required")

	authority := "user@host.com"
	key := "/path/to/file"
	sftpFile, err := fs.NewFile(authority, key)
	ts.NoError(err, "newFile should succeed")
	ts.IsType(&File{}, sftpFile, "newFile returned a File struct")
	ts.Equal(authority, sftpFile.Location().Volume())
	ts.Equal(key, sftpFile.Path())
}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}
