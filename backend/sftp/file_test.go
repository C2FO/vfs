package sftp

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/sftp/mocks"
	_mocks "github.com/c2fo/vfs/v6/mocks"
	"github.com/c2fo/vfs/v6/utils"
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

// this wraps strings.Reader to satisfy ReadWriteSeekCloser interface
type nopWriteCloser struct {
	io.ReadSeeker
}

func (nopWriteCloser) Close() error                      { return nil }
func (nopWriteCloser) Write(_ []byte) (n int, err error) { return 0, nil }

func (ts *fileTestSuite) TestRead() {

	// set up sftpfile
	filepath := "/some/path.txt"
	client := &mocks.Client{}

	contents := "hello world!"
	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)
	sftpfile := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: auth,
		path:      filepath,
		sftpfile:  &nopWriteCloser{strings.NewReader(contents)},
	}
	// perform test
	var localFile = bytes.NewBuffer([]byte{})

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	b, copyErr := io.CopyBuffer(localFile, sftpfile, buffer)
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
	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sftpfile := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: auth,
		path:      filepath,
		sftpfile:  &nopWriteCloser{strings.NewReader(contents)},
	}
	// perform test
	_, seekErr := sftpfile.Seek(6, 0)
	ts.NoError(seekErr, "no error expected")

	var localFile = bytes.NewBuffer([]byte{})

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	_, copyErr := io.CopyBuffer(localFile, sftpfile, buffer)
	ts.NoError(copyErr, "no error expected")

	ts.Equal("world!", localFile.String(), "Seeking should move the sftp file cursor as expected")

	localFile = bytes.NewBuffer([]byte{})
	_, seekErr2 := sftpfile.Seek(0, 0)
	ts.NoError(seekErr2, "no error expected")

	buffer = make([]byte, utils.TouchCopyMinBufferSize)
	_, copyErr2 := io.CopyBuffer(localFile, sftpfile, buffer)
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

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	file := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: auth,
		path:      filepath,
		sftpfile:  file1,
	}

	// file already opened
	f, err := file.openFile(os.O_RDONLY)
	ts.NoError(err, "no error expected")
	b, err := io.ReadAll(f)
	ts.NoError(err, "no error expected")
	ts.Equal(string(b), "file 1", "mock returned")

	// file not open, open for read
	file.sftpfile = nil
	file.opener = func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return file1, nil }
	_, err = file1.Seek(0, 0) // reset file
	ts.NoError(err, "no error expected")
	f, err = file.openFile(os.O_RDONLY)
	ts.NoError(err, "no error expected")
	b, err = io.ReadAll(f)
	ts.NoError(err, "no error expected")
	ts.Equal(string(b), "file 1", "mock returned")

	// file not open, user default opener
	file.sftpfile = nil
	file.opener = nil
	client.On("OpenFile", filepath, os.O_RDONLY).Return(&sftp.File{}, nil)
	f, err = file.openFile(os.O_RDONLY)
	ts.NoError(err, "no error expected")
	ts.IsType(&sftp.File{}, f, "type check")

	// file not open, open for create/write
	file.sftpfile = nil
	file.opener = func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return file1, nil }
	_, err = file1.Seek(0, 0) // reset file
	ts.NoError(err, "no error expected")
	client.On("MkdirAll", path.Dir(filepath)).Return(nil)
	f, err = file.openFile(os.O_RDWR | os.O_CREATE)
	ts.NoError(err, "no error expected")
	b, err = io.ReadAll(f)
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
	content := "this is a test"

	// set up source
	sourceClient := &mocks.Client{}

	sourceSftpFile := &mocks.SFTPFile{}

	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToFileBuffered() {
	content := "this is a test"

	// set up source
	sourceClient := &mocks.Client{}

	sourceSftpFile := &mocks.SFTPFile{}

	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToFileEmpty() {
	content := ""

	// set up source
	sourceClient := &mocks.Client{}

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToFileEmptyBuffered() {
	content := ""

	// set up source
	sourceClient := &mocks.Client{}

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToLocation() {
	content := "this is a location test"

	// set up source
	sourceClient := &mocks.Client{}

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	newFile, err := sourceFile.CopyToLocation(targetMockLocation)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.Equal(newFile.URI(), "sftp://user@host2.com:22/some/path.txt", "new file uri check")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_differentAuthority() {
	content := "blah"

	// set up source
	sourceClient := &mocks.Client{}
	sourceClient.On("Remove", mock.Anything).Return(nil).Once()

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	// run tests
	err = sourceFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_sameAuthority() {
	// set up source
	sourceClient := &mocks.Client{}
	sourceClient.On("Rename", mock.Anything, mock.Anything).Return(nil).Once()
	sourceClient.On("MkdirAll", mock.Anything).Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
	}

	rws := &mocks.SFTPFile{}
	rws.On("Seek", int64(0), 1).Return(int64(0), nil)
	sourceFile.opener = func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return rws, nil }

	// set up target
	targetFileInfo := &mocks.FileInfo{}
	targetFileInfo.On("IsDir").Return(true).Once()

	targetClient := &mocks.Client{}
	targetClient.On("Stat", mock.Anything).Return(nil, os.ErrNotExist).Twice()

	auth2, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/other/path.txt",
	}

	// run tests
	err = sourceFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_fileExists() {
	// set up source
	sourceClient := &mocks.Client{}

	sourceClient.On("Rename", mock.Anything, mock.Anything).Return(nil).Once()
	sourceClient.On("MkdirAll", mock.Anything).Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
	}

	rws := &mocks.SFTPFile{}
	rws.On("Seek", int64(0), 1).Return(int64(0), nil)
	sourceFile.opener = func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return rws, nil }

	// set up target
	targetFileInfo := &mocks.FileInfo{}
	targetFileInfo.On("IsDir").Return(true).Once()

	targetClient := &mocks.Client{}

	auth2, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/other/path.txt",
	}
	targetClient.On("Stat", targetFile.Location().Path()).Return(nil, os.ErrNotExist).Once()
	targetClient.On("Stat", targetFile.path).Return(targetFileInfo, nil).Once()
	targetClient.On("Remove", targetFile.path).Return(nil).Once()

	// run tests
	err = sourceFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	sourceClient.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToLocation() {
	content := "loc test"

	// set up source
	sourceClient := &mocks.Client{}
	sourceClient.On("Remove", mock.Anything).Return(nil).Once()

	sourceSftpFile := &mocks.SFTPFile{}
	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	sourceFile := &File{
		fileSystem: &FileSystem{
			sftpclient: sourceClient,
			options:    Options{},
		},
		Authority: auth,
		path:      "/some/path.txt",
		sftpfile:  sourceSftpFile,
	}

	// set up target
	targetClient := &mocks.Client{}

	targetSftpFile := &mocks.SFTPFile{}
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := utils.NewAuthority("user@host2.com:22")
	ts.NoError(err)

	targetFile := &File{
		fileSystem: &FileSystem{
			sftpclient: targetClient,
			options:    Options{},
		},
		Authority: auth2,
		path:      "/some/other/path.txt",
		sftpfile:  targetSftpFile,
		opener:    func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	newFile, err := sourceFile.MoveToLocation(targetMockLocation)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.Equal(newFile.URI(), "sftp://user@host2.com:22/some/other/path.txt", "new file uri check")

	sourceClient.AssertExpectations(ts.T())
	sourceSftpFile.AssertExpectations(ts.T())
	targetClient.AssertExpectations(ts.T())
	targetSftpFile.AssertExpectations(ts.T())
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

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	file := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: auth,
		path:      filepath,
		sftpfile:  sftpFile,
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

	auth, err := utils.NewAuthority("user@host1.com:22")
	ts.NoError(err)

	file := &File{
		fileSystem: &FileSystem{
			sftpclient: client,
			options:    Options{},
		},
		Authority: auth,
		path:      filepath,
		sftpfile:  sftpFile,
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
