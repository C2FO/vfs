package sftp

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/sftp/mocks"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

type fileSystemTestSuite struct {
	suite.Suite
	sftpfs *FileSystem
}

func (ts *fileSystemTestSuite) SetupTest() {
	client := mocks.NewClient(ts.T())
	ts.sftpfs = &FileSystem{
		sftpclient: client,
	}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	// test with options
	newFS := NewFileSystem(WithOptions(Options{KeyFilePath: "/some/path/"}))
	ts.NotNil(newFS, "Should return a new fileSystem for sftp")
	ts.Equal("/some/path/", newFS.options.KeyFilePath, "Should set region to us-east-1")

	// test with client
	sftpMock := mocks.NewClient(ts.T())
	newFS = NewFileSystem(WithClient(sftpMock))
	ts.NotNil(newFS, "Should return a new fileSystem for sftp")
	ts.Equal(sftpMock, newFS.sftpclient, "Should set client to s3cliMock")
}

func (ts *fileSystemTestSuite) TestNewFile() {
	filePath := "/path/to/file.txt"
	file, err := ts.sftpfs.NewFile("host.com", filePath)
	ts.Require().NoError(err, "No errors returned by NewFile(%s)", filePath)
	ts.NotNil(file, "sftpfs.NewFile(%s) should assign all but first name component to key", filePath)
}

func (ts *fileSystemTestSuite) TestNewFile_Error() {
	// test nil pointer
	var nilsftpfs *FileSystem
	_, err := nilsftpfs.NewFile("host.com", "/path/to/file.txt")
	ts.Require().ErrorIs(err, errFileSystemRequired, "errors returned by NewFile")

	// test validation error
	file, err := ts.sftpfs.NewFile("host.com", "relative/path/to/file.txt")
	ts.Require().ErrorIs(err, utils.ErrBadAbsFilePath, "errors returned by NewFile")
	ts.Nil(file, "NewFile shouldn't return a file")

	filePath := ""
	file, err = ts.sftpfs.NewFile("", filePath)
	ts.Require().Error(err, "NewFile(%s)", filePath)
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)

	filePath = "/some/file.txt"
	file, err = ts.sftpfs.NewFile("", filePath)
	ts.Require().ErrorIs(err, errAuthorityAndPathRequired, "bad authority")
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)
}

func (ts *fileSystemTestSuite) TestNewLocation() {
	locPath := "/path/to/"
	loc, err := ts.sftpfs.NewLocation("host.com", locPath)
	ts.Require().NoError(err, "No errors returned by NewLocation(%s)", locPath)
	ts.NotNil(loc, "sftpfs.NewLocation(%s) should assign all but first name component to key", locPath)
}

func (ts *fileSystemTestSuite) TestNewLocation_Error() {
	// test nil pointer
	var nilsftpfs *FileSystem
	_, err := nilsftpfs.NewLocation("somehost.com", "/path/to/")
	ts.Require().ErrorIs(err, errFileSystemRequired, "errors returned by NewLocation")

	// test validation error
	file, err := ts.sftpfs.NewLocation("host.com", "relative/path/to/")
	ts.Require().ErrorIs(err, utils.ErrBadAbsLocationPath, "errors returned by NewLocation")
	ts.Nil(file, "NewFile shouldn't return a file")

	locPath := ""
	file, err = ts.sftpfs.NewLocation("host.com", locPath)
	ts.Require().ErrorIs(err, errAuthorityAndPathRequired, "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)

	locPath = "/path/"
	file, err = ts.sftpfs.NewLocation("", locPath)
	ts.Require().ErrorIs(err, errAuthorityAndPathRequired, "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)
}

func (ts *fileSystemTestSuite) TestName() {
	ts.Equal("Secure File Transfer Protocol", ts.sftpfs.Name(), "Name() is sftp.name const")
}

func (ts *fileSystemTestSuite) TestScheme() {
	ts.Equal("sftp", ts.sftpfs.Scheme(), "expected scheme found")
}

func (ts *fileSystemTestSuite) TestRetry() {
	ts.IsType(vfs.DefaultRetryer(), ts.sftpfs.Retry(), "expected scheme found") //nolint:staticcheck // deprecated
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
	client, err := ts.sftpfs.Client(authority.Authority{})
	ts.Require().NoError(err, "no error")
	ts.Equal(ts.sftpfs.sftpclient, client, "client was already set")
}

func (ts *fileSystemTestSuite) TestClientWithAutoDisconnect() {
	getClientCount := 0
	client := mocks.NewClient(ts.T())
	client.On("ReadDir", "/").Return([]os.FileInfo{}, nil).Times(3)
	client.On("Close").Return(nil).Once()
	defaultClientGetter = func(authority.Authority, Options) (Client, io.Closer, error) {
		getClientCount++
		return client, nil, nil
	}

	// setup location with auto-disconnect of one second
	fs := NewFileSystem(WithOptions(Options{AutoDisconnect: 1}))
	loc, err := fs.NewLocation("user@host.com:1234", "/")
	ts.Require().NoError(err)

	// when List is first called, client should be nil, so we ask for a new client
	_, err = loc.List()
	ts.Require().NoError(err)
	// when List is immediately called a second time, client should be cached so no need for new client
	_, err = loc.List()
	ts.Require().NoError(err)
	// sleep for 2 seconds, client should disconnect and become nil
	time.Sleep(2 * time.Second)
	// when List is called a third time, client should be nil so we ask for a new client
	_, err = loc.List()
	ts.Require().NoError(err)
	fs.connTimerStop()
	fs.connTimer = nil
	// list should've been called 3 times

	// newClient should only have been called twice (because it was cached one time).
	ts.Equal(2, getClientCount, "newClient should only have been called twice (because it was cached one time")
}

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
