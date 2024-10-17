package sftp

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/sftp/mocks"
	"github.com/c2fo/vfs/v6/utils"
)

type fileSystemTestSuite struct {
	suite.Suite
	sftpfs *FileSystem
}

func (ts *fileSystemTestSuite) SetupTest() {
	client := &mocks.Client{}
	ts.sftpfs = &FileSystem{
		sftpclient: client,
	}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	newFS := NewFileSystem().WithClient(&mocks.Client{})
	ts.NotNil(newFS, "Should return a new fileSystem for sftp")
}

func (ts *fileSystemTestSuite) TestNewFile() {
	filePath := "/path/to/file.txt"
	file, err := ts.sftpfs.NewFile("host.com", filePath)
	ts.Nil(err, "No errors returned by NewFile(%s)", filePath)
	ts.NotNil(file, "sftpfs.NewFile(%s) should assign all but first name component to key", filePath)
}

func (ts *fileSystemTestSuite) TestNewFile_Error() {
	// test nil pointer
	var nilsftpfs *FileSystem
	_, err := nilsftpfs.NewFile("host.com", "/path/to/file.txt")
	ts.EqualError(err, "non-nil sftp.FileSystem pointer is required", "errors returned by NewFile")

	// test validation error
	file, err := ts.sftpfs.NewFile("host.com", "relative/path/to/file.txt")
	ts.EqualError(err, utils.ErrBadAbsFilePath, "errors returned by NewFile")
	ts.Nil(file, "NewFile shouldn't return a file")

	filePath := ""
	file, err = ts.sftpfs.NewFile("", filePath)
	ts.Error(err, "NewFile(%s)", filePath)
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)

	filePath = "/some/file.txt"
	file, err = ts.sftpfs.NewFile("", filePath)
	ts.EqualError(err, "authority string may not be empty", "bad authority")
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)
}

func (ts *fileSystemTestSuite) TestNewLocation() {
	locPath := "/path/to/"
	loc, err := ts.sftpfs.NewLocation("host.com", locPath)
	ts.NoError(err, "No errors returned by NewLocation(%s)", locPath)
	ts.NotNil(loc, "sftpfs.NewLocation(%s) should assign all but first name component to key", locPath)
}

func (ts *fileSystemTestSuite) TestNewLocation_Error() {
	// test nil pointer
	var nilsftpfs *FileSystem
	_, err := nilsftpfs.NewLocation("somehost.com", "/path/to/")
	ts.EqualError(err, "non-nil sftp.FileSystem pointer is required", "errors returned by NewLocation")

	// test validation error
	file, err := ts.sftpfs.NewLocation("host.com", "relative/path/to/")
	ts.EqualError(err, utils.ErrBadAbsLocationPath, "errors returned by NewLocation")
	ts.Nil(file, "NewFile shouldn't return a file")

	locPath := ""
	file, err = ts.sftpfs.NewLocation("host.com", locPath)
	ts.EqualError(err, "absolute location path is invalid - must include leading and trailing slashes", "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)

	locPath = "/path/"
	file, err = ts.sftpfs.NewLocation("", locPath)
	ts.EqualError(err, "authority string may not be empty", "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)
}

func (ts *fileSystemTestSuite) TestName() {
	ts.Equal("Secure File Transfer Protocol", ts.sftpfs.Name(), "Name() is sftp.name const")
}

func (ts *fileSystemTestSuite) TestScheme() {
	ts.Equal("sftp", ts.sftpfs.Scheme(), "expected scheme found")
}

func (ts *fileSystemTestSuite) TestRetry() {
	ts.IsType(vfs.DefaultRetryer(), ts.sftpfs.Retry(), "expected scheme found")
}

func (ts *fileSystemTestSuite) TestWithOptions() {
	// ignore non-sftp.Options
	fs := ts.sftpfs.WithOptions("just a string")
	ts.Equal(ts.sftpfs, fs, "no change for non-sftp.Options")

	// with option
	fs = ts.sftpfs.WithOptions(Options{})
	ts.NotNil(fs.options, "sftpfs.options is not nil")
}

func (ts *fileSystemTestSuite) TestClient() {
	// client already set
	client, err := ts.sftpfs.Client(utils.Authority{})
	ts.NoError(err, "no error")
	ts.Equal(ts.sftpfs.sftpclient, client, "client was already set")

	// bad options
	badOpt := "not an sftp.Options"
	ts.sftpfs.sftpclient = nil
	ts.sftpfs.options = badOpt
	_, err = ts.sftpfs.Client(utils.Authority{})
	ts.Error(err, "error found")
	ts.Equal("unable to create client, vfs.Options must be an sftp.Options", err.Error(), "client was already set")
}

func (ts *fileSystemTestSuite) TestClientWithAutoDisconnect() {
	getClientCount := 0
	client := &mocks.Client{}
	client.On("ReadDir", "/").Return([]os.FileInfo{}, nil).Times(3)
	client.On("Close").Return(nil).Times(1)
	defaultClientGetter = func(utils.Authority, Options) (Client, io.Closer, error) {
		getClientCount++
		return client, nil, nil
	}

	// setup location with auto-disconnect of one second
	fs := &FileSystem{}
	fs.WithOptions(Options{AutoDisconnect: 1})
	loc, err := fs.NewLocation("user@host.com:1234", "/")
	ts.NoError(err)

	// when List is first called, client should be nil, so we ask for a new client
	_, err = loc.List()
	ts.NoError(err)
	// when List is immediately called a second time, client should be cached so no need for new client
	_, err = loc.List()
	ts.NoError(err)
	// sleep for 2 seconds, client should disconnect and become nil
	time.Sleep(2 * time.Second)
	// when List is called a third time, client should be nil so we ask for a new client
	_, err = loc.List()
	ts.NoError(err)
	fs.connTimerStop()
	fs.connTimer = nil
	// list should've been called 3 times
	client.AssertExpectations(ts.T())

	// newClient should only have been called twice (because it was cached one time).
	ts.Equal(2, getClientCount, "newClient should only have been called twice (because it was cached one time")
}

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
