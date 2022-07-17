package ftp

import (
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	_ftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/backend/ftp/mocks"
	"github.com/c2fo/vfs/v6/utils"
)

type locationTestSuite struct {
	suite.Suite
	ftpfs  *FileSystem
	client *mocks.Client
}

func (lt *locationTestSuite) SetupTest() {
	lt.client = mocks.NewClient(lt.T())
	lt.ftpfs = NewFileSystem().WithClient(lt.client)
}

func (lt *locationTestSuite) TestList() {
	expectedFileList := []string{"file1.txt", "file2.txt"}

	entries := []*_ftp.Entry{
		{
			Name:   "file1.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "file2.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "subdir",
			Target: "",
			Type:   _ftp.EntryTypeFolder,
			Time:   time.Now().UTC(),
		},
	}
	authority := "host.com"
	locPath := "/dir1/"
	lt.client.On("List", locPath).Return(entries, nil).Once()

	loc, err := lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	fileList, err := loc.List()
	lt.Nil(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in expected file list.")
	}

	// file not found (location doesn't exist)
	lt.client.On("List", locPath).Return([]*_ftp.Entry{}, errors.New("some error")).Once()
	fileList, err = loc.List()
	lt.Error(err, "should return error")
	lt.Len(fileList, 0, "Should return no files on error")

	// file not found (location doesn't exist)
	lt.client.On("List", locPath).Return([]*_ftp.Entry{}, os.ErrNotExist).Once()
	fileList, err = loc.List()
	lt.NoError(err, "Shouldn't return an error on file not found.")
	lt.Len(fileList, 0, "Should return no files on file not found")

	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestListByPrefix() {

	expectedFileList := []string{"file1.txt", "file2.txt"}

	entries := []*_ftp.Entry{
		{
			Name:   "file1.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "file2.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "my_file.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "filedir",
			Target: "",
			Type:   _ftp.EntryTypeFolder,
			Time:   time.Now().UTC(),
		},
	}
	authority := "host.com"
	locPath := "/dir1/"
	lt.client.On("List", locPath).Return(entries, nil).Once()
	loc, err := lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	prefix := "fil"
	fileList, err := loc.ListByPrefix(prefix)
	lt.Nil(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestListByRegex() {
	expectedFileList := []string{"file1.txt", "file2.txt", "stuff.txt"}

	entries := []*_ftp.Entry{
		{
			Name:   "file1.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "file2.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "file.jpg",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "stuff.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   "subdirtxt",
			Target: "",
			Type:   _ftp.EntryTypeFolder,
			Time:   time.Now().UTC(),
		},
	}
	authority := "host.com"
	locPath := "/dir1/"
	lt.client.On("List", locPath).Return(entries, nil).Once()
	loc, err := lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	fileTypeRegex, err := regexp.Compile("txt$")
	if err != nil {
		lt.Fail("Failed to compile regex for test.")
	}
	fileList, err := loc.ListByRegex(fileTypeRegex)
	lt.Nil(err, "Shouldn't return an error on successful call to ListByRegex")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestURI() {
	authority := "user@host.com:21"
	loc, err := lt.ftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("ftp://user@host.com:21/blah/", loc.URI(), "location uri with user, host, port")

	authority = "user:password@host.com"
	file, err := lt.ftpfs.NewFile(authority, "/blah/file.txt")
	lt.NoError(err)
	lt.Equal("ftp://user@host.com/blah/file.txt", file.URI(), "file uri with user, pass, host")
}

func (lt *locationTestSuite) TestString() {
	authority := "user@host.com:21"
	loc, err := lt.ftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("ftp://user@host.com:21/blah/", loc.String(), "location string with user, host, port")

	authority = "user:password@host.com"
	file, err := lt.ftpfs.NewFile(authority, "/blah/file.txt")
	lt.NoError(err)
	lt.Equal("ftp://user@host.com/blah/file.txt", file.String(), "file string with user, pass, host")
}

func (lt *locationTestSuite) TestVolume() {
	authority := "user@host.com:21"
	loc, err := lt.ftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("user@host.com:21", loc.Volume(), "Volume() should return the authority string on location.")

	authority = "user:password@host.com"
	loc, err = lt.ftpfs.NewLocation(authority, "/blah/")
	lt.NoError(err)
	lt.Equal("user@host.com", loc.Volume(), "Volume() should return the authority string on location.")
}

func (lt *locationTestSuite) TestPath() {

	loc, err := lt.ftpfs.NewLocation("host.com", "/path/")
	lt.NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := lt.ftpfs.NewLocation("bucket", "/path/../newpath/")
	lt.NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := lt.ftpfs.NewLocation("bucket", "/path/./to/")
	lt.NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	loc, err := lt.ftpfs.NewLocation("host.com", "/some/path/to/")
	lt.NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

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
	dirs := []*_ftp.Entry{
		{
			Name:   locPath,
			Target: "",
			Type:   _ftp.EntryTypeFolder,
			Time:   time.Now().UTC(),
		},
	}
	lt.client.On("List", locPath).Return(dirs, nil).Once()
	loc, err := lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err := loc.Exists()
	lt.Nil(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")

	// locations does not exist
	locPath = "/my/dir/"
	dirs = []*_ftp.Entry{}
	lt.client.On("List", locPath).Return(dirs, nil).Once()
	loc, err = lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err = loc.Exists()
	lt.Nil(err, "No error expected from Exists")
	lt.True(!exists, "Call to Exists expected to return false.")

	// some error calling list
	lt.client.On("List", locPath).Return(dirs, errors.New("some error")).Once()
	loc, err = lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err = loc.Exists()
	lt.Error(err, "from Exists")
	lt.True(!exists, "Call to Exists expected to return false.")

	// check for not dir -- this shoudln't be possible since NewLocation won't accept non-absolute directories
	dirs = []*_ftp.Entry{
		{
			Name:   locPath,
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
	}
	lt.client.On("List", locPath).Return(dirs, nil).Once()
	loc, err = lt.ftpfs.NewLocation(authority, locPath)
	lt.NoError(err)
	exists, err = loc.Exists()
	lt.Nil(err, "No error expected from Exists")
	lt.True(!exists, "Call to Exists expected to return false.")

	lt.client.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestChangeDir() {

	loc := &Location{fileSystem: lt.ftpfs, path: "/", Authority: utils.Authority{}}

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
	loc, err := lt.ftpfs.NewLocation("ftp.host.com:21", "/old/")
	lt.NoError(err)
	newLoc, err := loc.NewLocation("new/path/")
	lt.NoError(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path/")
	lt.NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.EqualError(err, "non-empty string relativePath is required", "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.EqualError(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestDeleteFile() {
	lt.client.On("Delete", "/old/filename.txt").Return(nil).Once()
	loc, err := lt.ftpfs.NewLocation("ftp.host.com:21", "/old/")
	lt.NoError(err)

	err = loc.DeleteFile("filename.txt")
	lt.NoError(err, "Successful delete should not return an error.")

	// error deleting
	lt.client.On("Delete", "/old/filename.txt").Return(os.ErrNotExist).Once()
	err = loc.DeleteFile("filename.txt")
	lt.Error(err, "failed delete")

	lt.client.AssertExpectations(lt.T())
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))
}
