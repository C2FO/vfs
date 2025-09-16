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

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/sftp/mocks"
	_mocks "github.com/c2fo/vfs/v7/mocks"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

type fileTestSuite struct {
	suite.Suite
	sftpMock *mocks.Client
	fs       FileSystem
	testFile vfs.File
}

func (ts *fileTestSuite) SetupTest() {
	var err error
	ts.sftpMock = mocks.NewClient(ts.T())
	ts.fs = FileSystem{sftpclient: ts.sftpMock, options: Options{}}
	ts.testFile, err = ts.fs.NewFile("user@host.com:22", "/some/path/to/file.txt")
	ts.Require().NoError(err, "Shouldn't return error creating test sftp.File instance.")
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
	client := mocks.NewClient(ts.T())

	contents := "hello world!"
	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)
	sftpfile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: client,
				options:    Options{},
			},
			authority: auth,
		},
		path:     filepath,
		sftpfile: &nopWriteCloser{strings.NewReader(contents)},
	}
	// perform test
	var localFile = bytes.NewBuffer([]byte{})

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	b, copyErr := io.CopyBuffer(localFile, sftpfile, buffer)
	ts.Require().NoError(copyErr, "no error expected")
	ts.Equal(int64(12), b, "byte count after copy")
	ts.Require().NoError(sftpfile.Close(), "no error expected")
	ts.Equal(localFile.String(), contents, "Copying an sftp file to a buffer should fill buffer with localfile's contents")
}

func (ts *fileTestSuite) TestSeek() {
	// set up sftpfile
	filepath := "/some/path.txt"
	client := mocks.NewClient(ts.T())

	contents := "hello world!"
	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sftpfile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: client,
				options:    Options{},
			},
			authority: auth,
		},
		path:     filepath,
		sftpfile: &nopWriteCloser{strings.NewReader(contents)},
	}
	// perform test
	_, seekErr := sftpfile.Seek(6, 0)
	ts.Require().NoError(seekErr, "no error expected")

	var localFile = bytes.NewBuffer([]byte{})

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	_, copyErr := io.CopyBuffer(localFile, sftpfile, buffer)
	ts.Require().NoError(copyErr, "no error expected")

	ts.Equal("world!", localFile.String(), "Seeking should move the sftp file cursor as expected")

	localFile = bytes.NewBuffer([]byte{})
	_, seekErr2 := sftpfile.Seek(0, 0)
	ts.Require().NoError(seekErr2, "no error expected")

	buffer = make([]byte, utils.TouchCopyMinBufferSize)
	_, copyErr2 := io.CopyBuffer(localFile, sftpfile, buffer)
	ts.Require().NoError(copyErr2, "no error expected")
	ts.Equal(contents, localFile.String(), "Subsequent calls to seek work on temp sftp file as expected")

	closeErr := sftpfile.Close()
	ts.Require().NoError(closeErr, "no error expected")
}

func (ts *fileTestSuite) Test_openFile() {
	type testCase struct {
		name           string
		flags          int
		setupMocks     func(client *mocks.Client)
		expectedError  bool
		expectedErrMsg string
	}

	tests := []testCase{
		{
			name:  "Open file for read",
			flags: os.O_RDONLY,
			setupMocks: func(client *mocks.Client) {
				client.EXPECT().OpenFile("/some/path.txt", os.O_RDONLY).Return(&sftp.File{}, nil)
			},
			expectedError: false,
		},
		{
			name:  "Open file for write",
			flags: os.O_WRONLY | os.O_CREATE,
			setupMocks: func(client *mocks.Client) {
				client.EXPECT().MkdirAll("/some").Return(nil)
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0644)).Return(nil)
				client.EXPECT().OpenFile("/some/path.txt", os.O_WRONLY|os.O_CREATE).Return(&sftp.File{}, nil)
			},
			expectedError: false,
		},
		{
			name:  "Open file for create",
			flags: os.O_RDWR | os.O_CREATE,
			setupMocks: func(client *mocks.Client) {
				client.EXPECT().MkdirAll(path.Dir("/some/path.txt")).Return(nil)
				client.EXPECT().OpenFile("/some/path.txt", os.O_RDWR|os.O_CREATE).Return(&sftp.File{}, nil)
			},
			expectedError: false,
		},
		{
			name:  "Open file for create with error",
			flags: os.O_RDWR | os.O_CREATE,
			setupMocks: func(client *mocks.Client) {
				client.EXPECT().MkdirAll(path.Dir("/some/path.txt")).Return(errors.New("mkdir error"))
			},
			expectedError:  true,
			expectedErrMsg: "mkdir error",
		},
		{
			name:  "Open file with default permissions",
			flags: os.O_WRONLY,
			setupMocks: func(client *mocks.Client) {
				client.EXPECT().OpenFile("/some/path.txt", os.O_WRONLY).Return(&sftp.File{}, nil)
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0644)).Return(nil)
			},
			expectedError: false,
		},
		{
			name:  "Open file with default permissions error",
			flags: os.O_WRONLY,
			setupMocks: func(client *mocks.Client) {
				client.EXPECT().OpenFile("/some/path.txt", os.O_WRONLY).Return(&sftp.File{}, nil)
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0644)).Return(errors.New("chmod error"))
			},
			expectedError:  true,
			expectedErrMsg: "chmod error",
		},
	}

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			client := mocks.NewClient(ts.T())
			tt.setupMocks(client)

			auth, err := authority.NewAuthority("sftp://user@host:22")
			ts.Require().NoError(err)
			file := &File{
				path: "/some/path.txt",
				location: &Location{
					authority: auth,
					fileSystem: &FileSystem{
						sftpclient: client,
						options:    Options{FilePermissions: utils.Ptr("0644")},
					},
				},
			}

			_, err = file._open(tt.flags)
			if tt.expectedError {
				ts.Require().Error(err)
				ts.Require().ErrorContains(err, tt.expectedErrMsg)
			} else {
				ts.Require().NoError(err)
			}
		})
	}
}

func (ts *fileTestSuite) TestExists() {
	sftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	ts.sftpMock.On("Stat", sftpfile.Path()).Return(nil, nil).Once()

	exists, err := sftpfile.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.Require().NoError(err, "Shouldn't return an error when exists is true")
}

func (ts *fileTestSuite) TestNotExists() {
	sftpfile, err := ts.fs.NewFile("user@host.com", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	ts.sftpMock.On("Stat", sftpfile.Path()).Return(nil, os.ErrNotExist).Once()
	exists, err := sftpfile.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.Require().NoError(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	content := "this is a test"

	// set up source
	sourceClient := mocks.NewClient(ts.T())

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())

	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestCopyToFileBuffered() {
	content := "this is a test"

	// set up source
	sourceClient := mocks.NewClient(ts.T())

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())

	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestCopyToFileEmpty() {
	content := ""

	// set up source
	sourceClient := mocks.NewClient(ts.T())

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestCopyToFileEmptyBuffered() {
	content := ""

	// set up source
	sourceClient := mocks.NewClient(ts.T())

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	err = sourceFile.CopyToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestCopyToLocation() {
	content := "this is a location test"

	// set up source
	sourceClient := mocks.NewClient(ts.T())

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	newFile, err := sourceFile.CopyToLocation(targetMockLocation)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.Equal("sftp://user@host2.com:22/some/path.txt", newFile.URI(), "new file uri check")
}

func (ts *fileTestSuite) TestMoveToFile_differentAuthority() {
	content := "blah"

	// set up source
	sourceClient := mocks.NewClient(ts.T())
	sourceClient.On("Remove", mock.Anything).Return(nil).Once()

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	// run tests
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestMoveToFile_sameAuthority() {
	// set up source
	sourceClient := mocks.NewClient(ts.T())
	sourceClient.On("Rename", mock.Anything, mock.Anything).Return(nil).Once()
	sourceClient.On("MkdirAll", mock.Anything).Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path: "/some/path.txt",
	}

	rws := mocks.NewReadWriteSeekCloser(ts.T())
	sourceFile.opener = func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return rws, nil }

	targetClient := mocks.NewClient(ts.T())
	targetClient.On("Stat", mock.Anything).Return(nil, os.ErrNotExist).Twice()

	auth2, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path: "/some/other/path.txt",
	}

	// run tests
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestMoveToFile_fileExists() {
	// set up source
	sourceClient := mocks.NewClient(ts.T())

	sourceClient.On("Rename", mock.Anything, mock.Anything).Return(nil).Once()
	sourceClient.On("MkdirAll", mock.Anything).Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path: "/some/path.txt",
	}

	rws := mocks.NewReadWriteSeekCloser(ts.T())
	sourceFile.opener = func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return rws, nil }

	// set up target
	targetFileInfo := mocks.NewFileInfo(ts.T())

	targetClient := mocks.NewClient(ts.T())

	auth2, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path: "/some/other/path.txt",
	}
	targetClient.On("Stat", targetFile.Location().Path()).Return(nil, os.ErrNotExist).Once()
	targetClient.On("Stat", targetFile.path).Return(targetFileInfo, nil).Once()
	targetClient.On("Remove", targetFile.path).Return(nil).Once()

	// run tests
	err = sourceFile.MoveToFile(targetFile)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")
}

func (ts *fileTestSuite) TestMoveToLocation() {
	content := "loc test"

	// set up source
	sourceClient := mocks.NewClient(ts.T())
	sourceClient.On("Remove", mock.Anything).Return(nil).Once()

	sourceSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	sourceSftpFile.On("Read", mock.Anything).Return(len(content), nil).Once()
	sourceSftpFile.On("Read", mock.Anything).Return(0, io.EOF).Once()
	sourceSftpFile.On("Close").Return(nil).Once()

	auth, err := authority.NewAuthority("user@host1.com:22")
	ts.Require().NoError(err)

	sourceFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: sourceClient,
				options:    Options{},
			},
			authority: auth,
		},
		path:     "/some/path.txt",
		sftpfile: sourceSftpFile,
	}

	// set up target
	targetClient := mocks.NewClient(ts.T())

	targetSftpFile := mocks.NewReadWriteSeekCloser(ts.T())
	targetSftpFile.On("Write", mock.Anything).Return(len(content), nil).Once()
	targetSftpFile.On("Close").Return(nil).Once()

	auth2, err := authority.NewAuthority("user@host2.com:22")
	ts.Require().NoError(err)

	targetFile := &File{
		location: &Location{
			fileSystem: &FileSystem{
				sftpclient: targetClient,
				options:    Options{},
			},
			authority: auth2,
		},
		path:     "/some/other/path.txt",
		sftpfile: targetSftpFile,
		opener:   func(c Client, p string, f int) (ReadWriteSeekCloser, error) { return targetSftpFile, nil },
	}

	targetMockLocation := &_mocks.Location{}
	targetMockLocation.On("NewFile", mock.Anything).Return(targetFile, nil)

	// run tests
	newFile, err := sourceFile.MoveToLocation(targetMockLocation)
	ts.Require().NoError(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.Equal("sftp://user@host2.com:22/some/other/path.txt", newFile.URI(), "new file uri check")
}

func (ts *fileTestSuite) TestTouch() {
	type testCase struct {
		name           string
		filePath       string
		fileExists     bool
		setPermissions bool
		expectedError  error
		setupMocks     func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo)
	}
	err := errors.New("some error")
	testCases := []testCase{
		{
			name:       "file exists",
			filePath:   "/some/path.txt",
			fileExists: true,
			setupMocks: func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo) {
				client.EXPECT().Stat("/some/path.txt").Return(fileInfo, nil).Once()
				client.EXPECT().Chtimes("/some/path.txt", mock.Anything, mock.Anything).Return(nil).Once()
			},
		},
		{
			name:       "file does not exist",
			filePath:   "/some/path.txt",
			fileExists: false,
			setupMocks: func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo) {
				client.EXPECT().Stat("/some/path.txt").Return(nil, os.ErrNotExist).Once()
				sftpFile.EXPECT().Close().Return(nil).Once()
			},
		},
		{
			name:           "set default permissions",
			filePath:       "/some/path.txt",
			fileExists:     true,
			setPermissions: true,
			setupMocks: func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo) {
				client.EXPECT().Stat("/some/path.txt").Return(fileInfo, nil).Once()
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0666)).Return(nil).Once()
				client.EXPECT().Chtimes("/some/path.txt", mock.Anything, mock.Anything).Return(nil).Once()
			},
		},
		{
			name:          "error on stat",
			filePath:      "/some/path.txt",
			expectedError: err,
			setupMocks: func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo) {
				client.EXPECT().Stat("/some/path.txt").Return(nil, err).Once()
			},
		},
		{
			name:          "error on chtimes",
			filePath:      "/some/path.txt",
			fileExists:    true,
			expectedError: err,
			setupMocks: func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo) {
				client.EXPECT().Stat("/some/path.txt").Return(fileInfo, nil).Once()
				client.EXPECT().Chtimes("/some/path.txt", mock.Anything, mock.Anything).Return(err).Once()
			},
		},
		{
			name:     "setPermissions returns error",
			filePath: "/some/path.txt",
			setupMocks: func(client *mocks.Client, sftpFile *mocks.ReadWriteSeekCloser, fileInfo *mocks.FileInfo) {
				client.EXPECT().Stat("/some/path.txt").Return(fileInfo, nil).Once()
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0666)).Return(err).Once()
			},
			expectedError:  err,
			setPermissions: true,
		},
	}

	for _, tc := range testCases {
		ts.Run(tc.name, func() {
			client := mocks.NewClient(ts.T())
			sftpFile := mocks.NewReadWriteSeekCloser(ts.T())
			fileInfo := mocks.NewFileInfo(ts.T())

			auth, err := authority.NewAuthority("user@host1.com:22")
			ts.Require().NoError(err)

			file := &File{
				location: &Location{
					fileSystem: &FileSystem{
						sftpclient: client,
						options: Options{
							FilePermissions: func() *string {
								if tc.setPermissions {
									return utils.Ptr("0666")
								}
								return nil
							}(),
						},
					},
					authority: auth,
				},
				path:     tc.filePath,
				sftpfile: sftpFile,
			}

			tc.setupMocks(client, sftpFile, fileInfo)

			err = file.Touch()
			if tc.expectedError != nil {
				ts.Require().ErrorIs(err, tc.expectedError)
			} else {
				ts.Require().NoError(err)
			}
		})
	}
}

func (ts *fileTestSuite) TestDelete() {
	ts.sftpMock.On("Remove", ts.testFile.Path()).Return(nil).Once()
	err := ts.testFile.Delete()
	ts.Require().NoError(err, "Successful delete should not return an error.")
}

func (ts *fileTestSuite) TestLastModified() {
	now := time.Now()
	file1 := mocks.NewFileInfo(ts.T())
	file1.On("ModTime").Return(now, nil)
	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(file1, nil)
	modTime, err := ts.testFile.LastModified()
	ts.Require().NoError(err, "Error should be nil when correctly returning time of object.")
	ts.Equal(&now, modTime, "Returned time matches expected LastModified time.")
}

func (ts *fileTestSuite) TestLastModifiedFail() {
	myErr := errors.New("some error")
	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(nil, myErr)
	m, e := ts.testFile.LastModified()
	ts.Require().Error(e, "got error as expected")
	ts.Nil(m, "nil ModTime returned")
}

func (ts *fileTestSuite) TestName() {
	ts.Equal("file.txt", ts.testFile.Name(), "Name should return just the name of the file.")
}

func (ts *fileTestSuite) TestSize() {
	contentLength := int64(100)
	file1 := mocks.NewFileInfo(ts.T())
	file1.On("Size").Return(contentLength)
	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(file1, nil).Once()
	size, err := ts.testFile.Size()
	ts.Require().NoError(err, "Error should be nil when requesting size for file that exists.")
	ts.Equal(uint64(contentLength), size, "Size should return the ContentLength value from s3 HEAD request.")

	ts.sftpMock.On("Stat", ts.testFile.Path()).Return(mocks.NewFileInfo(ts.T()), errors.New("some error")).Once()
	size, err = ts.testFile.Size()
	ts.Require().Error(err, "expect error")
	ts.Zero(size, "Size should be 0 on error")
}

func (ts *fileTestSuite) TestPath() {
	ts.Equal("/some/path/to/file.txt", ts.testFile.Path(), "Should return file.key (with leading slash)")
}

func (ts *fileTestSuite) TestURI() {
	expected := "sftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.URI(), "URI test")

	expected = "sftp://domain.com%5Cuser@host.com:22/some/path/to/file.txt"
	fs := NewFileSystem()
	f, err := fs.NewFile("domain.com%5Cuser@host.com:22", "/some/path/to/file.txt")
	ts.Require().NoError(err)
	ts.Equal(expected, f.URI(), "URI test")
}

func (ts *fileTestSuite) TestStringer() {
	expected := "sftp://user@host.com:22/some/path/to/file.txt"
	ts.Equal(expected, ts.testFile.String(), "String test")
}

func (ts *fileTestSuite) TestNewFile() {
	fs := &FileSystem{}
	// fs is nil
	_, err := fs.NewFile("user@host.com", "")
	ts.Require().Errorf(err, "non-nil sftp.FileSystem pointer is required")

	// authority is ""
	_, err = fs.NewFile("", "asdf")
	ts.Require().Errorf(err, "non-empty strings for bucket and key are required")
	// path is ""
	_, err = fs.NewFile("user@host.com", "")
	ts.Require().Errorf(err, "non-empty strings for bucket and key are required")

	authorityStr := "user@host.com"
	key := "/path/to/file"
	sftpFile, err := fs.NewFile(authorityStr, key)
	ts.Require().NoError(err, "newFile should succeed")
	ts.IsType((*File)(nil), sftpFile, "newFile returned a File struct")
	ts.Equal(authorityStr, sftpFile.Location().Authority().String())
	ts.Equal(key, sftpFile.Path())
}

func (ts *fileTestSuite) TestSetDefaultPermissions() {
	type testCase struct {
		name           string
		client         *mocks.Client
		options        Options
		expectedError  bool
		expectedErrMsg string
	}

	tests := []testCase{
		{
			name: "No options provided",
			client: func() *mocks.Client {
				client := mocks.NewClient(ts.T())
				return client
			}(),
			options:       Options{},
			expectedError: false,
		},
		{
			name: "Default permissions set",
			client: func() *mocks.Client {
				client := mocks.NewClient(ts.T())
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0644)).Return(nil)
				return client
			}(),
			options: func() Options {
				opts := Options{FilePermissions: utils.Ptr("0644")}
				return opts
			}(),
			expectedError: false,
		},
		{
			name: "Chmod returns error",
			client: func() *mocks.Client {
				client := mocks.NewClient(ts.T())
				client.EXPECT().Chmod("/some/path.txt", os.FileMode(0644)).Return(errors.New("chmod error"))
				return client
			}(),
			options: func() Options {
				opts := Options{FilePermissions: utils.Ptr("0644")}
				return opts
			}(),
			expectedError:  true,
			expectedErrMsg: "chmod error",
		},
	}

	for i := range tests {
		ts.Run(tests[i].name, func() {
			file := &File{
				path: "/some/path.txt",
				location: &Location{
					fileSystem: &FileSystem{options: tests[i].options},
				},
			}

			err := file.setPermissions(tests[i].client, tests[i].options)
			if tests[i].expectedError {
				ts.Require().Error(err)
				ts.Require().ErrorContains(err, tests[i].expectedErrMsg)
			} else {
				ts.Require().NoError(err)
			}
		})
	}
}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}
