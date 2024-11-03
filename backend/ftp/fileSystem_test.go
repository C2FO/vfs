package ftp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/ftp/mocks"
	"github.com/c2fo/vfs/v6/utils"
)

type fileSystemTestSuite struct {
	suite.Suite
	ftpfs *FileSystem
}

func (ts *fileSystemTestSuite) SetupTest() {
	client := &mocks.Client{}
	ts.ftpfs = &FileSystem{
		ftpclient: client,
	}
}

func (ts *fileSystemTestSuite) TestNewFileSystem() {
	newFS := NewFileSystem().WithClient(&mocks.Client{})
	ts.NotNil(newFS, "Should return a new fileSystem for ftp")
}

func (ts *fileSystemTestSuite) TestNewFile() {
	filePath := "/path/to/file.txt"
	file, err := ts.ftpfs.NewFile("host.com", filePath)
	ts.NoError(err, "No errors returned by NewFile(%s)", filePath)
	ts.NotNil(file, "ftpfs.NewFile(%s) should assign all but first name component to key", filePath)
}

func (ts *fileSystemTestSuite) TestNewFile_Error() {
	// test nil pointer
	var nilftpfs *FileSystem
	_, err := nilftpfs.NewFile("host.com", "/path/to/file.txt")
	ts.EqualError(err, "non-nil ftp.FileSystem pointer is required", "errors returned by NewFile")

	// test validation error
	file, err := ts.ftpfs.NewFile("host.com", "relative/path/to/file.txt")
	ts.EqualError(err, utils.ErrBadAbsFilePath, "errors returned by NewFile")
	ts.Nil(file, "NewFile shouldn't return a file")

	filePath := ""
	file, err = ts.ftpfs.NewFile("", filePath)
	ts.Error(err, "NewFile(%s)", filePath)
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)

	filePath = "/some/file.txt"
	file, err = ts.ftpfs.NewFile("", filePath)
	ts.EqualError(err, "authority string may not be empty", "bad authority")
	ts.Nil(file, "NewFile(%s) shouldn't return a file", filePath)
}

func (ts *fileSystemTestSuite) TestNewLocation() {
	locPath := "/path/to/"
	loc, err := ts.ftpfs.NewLocation("host.com", locPath)
	ts.NoError(err, "No errors returned by NewLocation(%s)", locPath)
	ts.NotNil(loc, "ftpfs.NewLocation(%s) should assign all but first name component to key", locPath)
}

func (ts *fileSystemTestSuite) TestNewLocation_Error() {
	// test nil pointer
	var nilftpfs *FileSystem
	_, err := nilftpfs.NewLocation("somehost.com", "/path/to/")
	ts.EqualError(err, "non-nil ftp.FileSystem pointer is required", "errors returned by NewLocation")

	// test validation error
	file, err := ts.ftpfs.NewLocation("host.com", "relative/path/to/")
	ts.EqualError(err, utils.ErrBadAbsLocationPath, "errors returned by NewLocation")
	ts.Nil(file, "NewFile shouldn't return a file")

	locPath := ""
	file, err = ts.ftpfs.NewLocation("host.com", locPath)
	ts.EqualError(err, "absolute location path is invalid - must include leading and trailing slashes", "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)

	locPath = "/path/"
	file, err = ts.ftpfs.NewLocation("", locPath)
	ts.EqualError(err, "authority string may not be empty", "NewLocation(%s)", locPath)
	ts.Nil(file, "NewLocation(%s) shouldn't return a file", locPath)
}

func (ts *fileSystemTestSuite) TestName() {
	ts.Equal("File Transfer Protocol", ts.ftpfs.Name(), "Name() is ftp.name const")
}

func (ts *fileSystemTestSuite) TestScheme() {
	ts.Equal("ftp", ts.ftpfs.Scheme(), "expected scheme found")
}

func (ts *fileSystemTestSuite) TestRetry() {
	ts.IsType(vfs.DefaultRetryer(), ts.ftpfs.Retry(), "expected scheme found")
}

func (ts *fileSystemTestSuite) TestWithOptions() {
	// ignore non-ftp.Options
	fs := ts.ftpfs.WithOptions("just a string")
	ts.Equal(ts.ftpfs, fs, "no change for non-ftp.Options")

	// with option
	fs = ts.ftpfs.WithOptions(Options{})
	ts.NotNil(fs.options, "ftpfs.options is not nil")
}

func (ts *fileSystemTestSuite) TestClient() {
	// client already set
	client, err := ts.ftpfs.Client(context.Background(), utils.Authority{})
	ts.NoError(err, "no error")
	ts.Equal(ts.ftpfs.ftpclient, client, "client was already set")

	// bad options
	badOpt := "not an ftp.Options"
	ts.ftpfs.ftpclient = nil
	ts.ftpfs.options = badOpt
	_, err = ts.ftpfs.Client(context.Background(), utils.Authority{})
	ts.EqualError(err, "unable to create client, vfs.Options must be an ftp.Options", "client was already set")
}

func TestFileSystem(t *testing.T) {
	suite.Run(t, new(fileSystemTestSuite))
}
