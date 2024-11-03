package sftp

import (
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/backend/sftp/mocks"
	"github.com/c2fo/vfs/v6/utils"
)

type locationTestSuite struct {
	suite.Suite
	sftpfs *FileSystem
	client *mocks.Client
}

func (lt *locationTestSuite) SetupTest() {
	lt.client = &mocks.Client{}
	lt.sftpfs = &FileSystem{
		sftpclient: lt.client,
	}
}

func (lt *locationTestSuite) TestList() {
	expectedFileList := []string{"file.txt", "file2.txt"}

	file1 := &mocks.FileInfo{}
	file1.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file2 := &mocks.FileInfo{}
	file2.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	subdir1 := &mocks.FileInfo{}
	subdir1.
		On("Name").Return("subdir").
		On("IsDir").Return(true)
	keyListFromAPI := []*mocks.FileInfo{subdir1, file1, file2}
	authority := "host.com"
	locPath := "/dir1/"
	lt.client.On("ReadDir", locPath).Return(sliceImplementationToInterface(keyListFromAPI), nil).Once()

	loc, err := lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	fileList, err := loc.List()
	lt.NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in expected file list.")
	}

	// file not found (location doesn't exist)
	lt.client.On("ReadDir", locPath).Return(make([]os.FileInfo, 0), errors.New("some error")).Once()
	fileList, err = loc.List()
	lt.Error(err, "should return error")
	lt.Len(fileList, 0, "Should return no files on error")

	// file not found (location doesn't exist)
	lt.client.On("ReadDir", locPath).Return(make([]os.FileInfo, 0), os.ErrNotExist).Once()
	fileList, err = loc.List()
	lt.NoError(err, "Shouldn't return an error on file not found.")
	lt.Len(fileList, 0, "Should return no files on file not found")

	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestListByPrefix() {
	expectedFileList := []string{"file.txt", "file2.txt"}

	file1 := &mocks.FileInfo{}
	file1.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file2 := &mocks.FileInfo{}
	file2.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file3 := &mocks.FileInfo{}
	file3.
		On("Name").Return("my_file.txt").
		On("IsDir").Return(false)
	subdir1 := &mocks.FileInfo{}
	subdir1.
		On("Name").Return("filedir").
		On("IsDir").Return(true)
	keyListFromAPI := []*mocks.FileInfo{subdir1, file1, file2, file3}
	authority := "host.com"
	locPath := "/dir1/"
	lt.client.On("ReadDir", locPath).Return(sliceImplementationToInterface(keyListFromAPI), nil).Once()
	loc, err := lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	prefix := "fil"
	fileList, err := loc.ListByPrefix(prefix)
	lt.NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestListByRegex() {
	expectedFileList := []string{"file1.txt", "file2.txt", "stuff.txt"}

	file1 := &mocks.FileInfo{}
	file1.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file2 := &mocks.FileInfo{}
	file2.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file3 := &mocks.FileInfo{}
	file3.
		On("Name").Return("file.jpg").
		On("IsDir").Return(false)
	file4 := &mocks.FileInfo{}
	file4.
		On("Name").Return("stuff.txt").
		On("IsDir").Return(false)
	subdir1 := &mocks.FileInfo{}
	subdir1.
		On("Name").Return("subdirtxt").
		On("IsDir").Return(true)
	keyListFromAPI := []*mocks.FileInfo{subdir1, file1, file2, file4}
	authority := "host.com"
	locPath := "/dir1/"
	lt.client.On("ReadDir", locPath).Return(sliceImplementationToInterface(keyListFromAPI), nil).Once()
	loc, err := lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	fileTypeRegex := regexp.MustCompile("txt$")

	fileList, err := loc.ListByRegex(fileTypeRegex)
	lt.NoError(err, "Shouldn't return an error on successful call to ListByRegex")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestURI() {
	authority := "user@host.com:22"
	loc, err := lt.sftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("sftp://user@host.com:22/blah/", loc.URI(), "location uri with user, host, port")

	authority = "user:password@host.com"
	file, err := lt.sftpfs.NewFile(authority, "/blah/file.txt")
	lt.NoError(err)
	lt.Equal("sftp://user@host.com/blah/file.txt", file.URI(), "file uri with user, pass, host")

	authority = `domain.com\user@host.com`
	_, err = lt.sftpfs.NewFile(authority, "/blah/file.txt")
	lt.Error(err)
	lt.ErrorContains(err, "net/url: invalid userinfo", "file uri with bad user")

	authority = `domain.com%5Cuser@host.com`
	file, err = lt.sftpfs.NewFile(authority, "/blah/file.txt")
	lt.NoError(err)
	lt.Equal(`sftp://domain.com%5Cuser@host.com/blah/file.txt`, file.URI(), "file uri with percent-encoded character in user")
}

func (lt *locationTestSuite) TestString() {
	authority := "user@host.com:22"
	loc, err := lt.sftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("sftp://user@host.com:22/blah/", loc.String(), "location string with user, host, port")

	authority = "user:password@host.com"
	file, err := lt.sftpfs.NewFile(authority, "/blah/file.txt")
	lt.NoError(err)
	lt.Equal("sftp://user@host.com/blah/file.txt", file.String(), "file string with user, pass, host")
}

func (lt *locationTestSuite) TestVolume() {
	authority := "user@host.com:22"
	loc, err := lt.sftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("user@host.com:22", loc.Volume(), "Volume() should return the authority string on location.")

	authority = "user:password@host.com"
	loc, err = lt.sftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("user@host.com", loc.Volume(), "Volume() should return the authority string on location.")
}

func (lt *locationTestSuite) TestPath() {
	loc, err := lt.sftpfs.NewLocation("host.com", "/path/")
	lt.NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := lt.sftpfs.NewLocation("bucket", "/path/../newpath/")
	lt.NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := lt.sftpfs.NewLocation("bucket", "/path/./to/")
	lt.NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	loc, err := lt.sftpfs.NewLocation("bucket", "/some/path/to/")
	lt.NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewFile("/path/to/file.txt")
	lt.EqualError(err, "non-nil sftp.Location pointer receiver is required", "errors returned by NewFile")

	// test empty path error
	_, err = loc.NewFile("")
	lt.EqualError(err, "non-empty string filePath is required", "errors returned by NewFile")

	// test validation error
	_, err = loc.NewFile("/absolute/path/to/file.txt")
	lt.EqualError(err, utils.ErrBadRelFilePath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestExists() {
	authority := "host.com"

	// location exists
	locPath := "/"
	dir1 := &mocks.FileInfo{}
	dir1.
		On("Name").Return(locPath).
		On("IsDir").Return(true)
	lt.client.On("Stat", locPath).Return(dir1, nil).Once()
	loc, err := lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err := loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")

	// locations does not exist
	locPath = "/my/dir/"
	dir1 = &mocks.FileInfo{}
	lt.client.On("Stat", locPath).Return(dir1, os.ErrNotExist).Once()
	loc, err = lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err = loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.True(!exists, "Call to Exists expected to return false.")

	// some error calling stat
	lt.client.On("Stat", locPath).Return(dir1, errors.New("some error")).Once()
	loc, err = lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err = loc.Exists()
	lt.Error(err, "from Exists")
	lt.True(!exists, "Call to Exists expected to return false.")

	// check for not dir -- this shouldn't be possible since NewLocation won't accept non-absolute directories
	dir1 = &mocks.FileInfo{}
	dir1.
		On("Name").Return(locPath).
		On("IsDir").Return(false) // set isdir false
	lt.client.On("Stat", locPath).Return(dir1, nil).Once()
	loc, err = lt.sftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err = loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.True(!exists, "Call to Exists expected to return false.")

	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestChangeDir() {
	// test nil Location
	var nilLoc *Location
	err := nilLoc.ChangeDir("path/to/")
	lt.EqualErrorf(err, "non-nil sftp.Location pointer receiver is required", "error expected for nil location")

	loc := &Location{fileSystem: lt.sftpfs, path: "/", Authority: utils.Authority{}}

	err1 := loc.ChangeDir("../")
	lt.NoError(err1, "no error expected")
	lt.Equal("/", loc.Path())

	err2 := loc.ChangeDir("hello/")
	lt.NoError(err2, "no error expected")
	lt.Equal("/hello/", loc.Path())

	err3 := loc.ChangeDir("../.././../")
	lt.NoError(err3, "no error expected")
	lt.Equal("/", loc.Path())

	err4 := loc.ChangeDir("here/is/a/path/")
	lt.NoError(err4, "no error expected")
	lt.Equal("/here/is/a/path/", loc.Path())

	err5 := loc.ChangeDir("../")
	lt.NoError(err5, "no error expected")
	lt.Equal("/here/is/a/", loc.Path())
}

func (lt *locationTestSuite) TestNewLocation() {
	loc, err := lt.sftpfs.NewLocation("bucket", "/old/")
	lt.NoError(err)
	newLoc, err := loc.NewLocation("new/path/")
	lt.NoError(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path/")
	lt.NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewLocation("/path/to/")
	lt.EqualError(err, "non-nil sftp.Location pointer receiver is required", "errors returned by NewLocation")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.EqualError(err, "non-empty string relativePath is required", "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.EqualError(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestDeleteFile() {
	lt.client.On("Remove", "/old/filename.txt").Return(nil).Once()
	loc, err := lt.sftpfs.NewLocation("bucket", "/old/")
	lt.NoError(err)

	err = loc.DeleteFile("filename.txt")
	lt.NoError(err, "Successful delete should not return an error.")

	// error deleting
	lt.client.On("Remove", "/old/filename.txt").Return(os.ErrNotExist).Once()
	err = loc.DeleteFile("filename.txt")
	lt.Error(err, "failed delete")

	lt.client.AssertExpectations(lt.T())
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))
}

/*
	Helpers
*/

func sliceImplementationToInterface(fis []*mocks.FileInfo) []os.FileInfo {
	osFIs := make([]os.FileInfo, len(fis))
	for i, v := range fis {
		osFIs[i] = os.FileInfo(v)
	}
	return osFIs
}
