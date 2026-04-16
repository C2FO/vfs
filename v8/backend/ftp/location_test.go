package ftp

import (
	"context"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	_ftp "github.com/jlaffaye/ftp"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
	vfs "github.com/c2fo/vfs/v8"
	"github.com/c2fo/vfs/v8/backend/ftp/mocks"
	"github.com/c2fo/vfs/v8/backend/testsuite"
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
	authorityStr := "host.com"
	locPath := "/dir1/"
	lt.client.EXPECT().List(locPath).Return(entries, nil).Once()

	loc, err := lt.ftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	ctx := context.Background()
	fileList, err := testsuite.CollectList(ctx, loc)
	lt.Require().NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in expected file list.")
	}

	// file not found (location doesn't exist)
	lt.client.EXPECT().List(locPath).Return([]*_ftp.Entry{}, errors.New("some error")).Once()
	fileList, err = testsuite.CollectList(ctx, loc)
	lt.Require().Error(err, "should return error")
	lt.Empty(fileList, "Should return no files on error")

	// file not found (location doesn't exist)
	lt.client.EXPECT().List(locPath).Return([]*_ftp.Entry{}, errors.New("550")).Once()
	fileList, err = testsuite.CollectList(ctx, loc)
	lt.Require().NoError(err, "Shouldn't return an error on file not found.")
	lt.Empty(fileList, "Should return no files on file not found")

	// error getting client
	errLoc, err := NewFileSystem(WithClientGetter(clientGetterReturnsError)).NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	fileList, err = testsuite.CollectList(ctx, errLoc)
	lt.Require().Error(err, "error expected")
	lt.Require().ErrorIs(err, errClientGetter, "err should be correct type")
	lt.Empty(fileList, "fileList should be empty on error")
}

func (lt *locationTestSuite) TestListByPrefix() {
	entries := []*_ftp.Entry{
		{
			Name: "myFile1.txt",
			Type: _ftp.EntryTypeFile,
		},
		{
			Name: "myFile2.txt",
			Type: _ftp.EntryTypeFile,
		},
		{
			Name: "mom.csv",
			Type: _ftp.EntryTypeFile,
		},
		{
			Name: "NOTmyFiles.txt",
			Type: _ftp.EntryTypeFile,
		},
		{
			Name: ".config.json",
			Type: _ftp.EntryTypeFile,
		},
		{
			Name: "myDir",
			Type: _ftp.EntryTypeFolder,
		},
		{
			Name: "otherDir",
			Type: _ftp.EntryTypeFolder,
		},
		{
			Name: ".aws",
			Type: _ftp.EntryTypeFolder,
		},
	}
	tests := []struct {
		description   string
		path          string
		prefix        string
		resolvedPath  string
		allEntries    []*_ftp.Entry
		expectedFiles []string
	}{
		{
			description:  "standard prefix",
			path:         "/some/path/",
			prefix:       "my",
			resolvedPath: "/some/path/",
			allEntries:   entries,
			expectedFiles: []string{
				"myFile1.txt",
				"myFile2.txt",
			},
		},
		{
			description:   "standard prefix - none found",
			path:          "/some/path/",
			prefix:        "blah",
			resolvedPath:  "/some/path/",
			allEntries:    entries,
			expectedFiles: []string{},
		},
		{
			description:  "relative prefix",
			path:         "/some/path/",
			prefix:       "to/my",
			resolvedPath: "/some/path/to/",
			allEntries:   entries,
			expectedFiles: []string{
				"myFile1.txt",
				"myFile2.txt",
			},
		},
		{
			description:  "dot prefix",
			path:         "/some/path/",
			prefix:       ".con",
			resolvedPath: "/some/path/",
			allEntries:   entries,
			expectedFiles: []string{
				".config.json",
			},
		},
		{
			description:  "dot-only prefix",
			path:         "/some/path/",
			prefix:       ".",
			resolvedPath: "/some/path/",
			allEntries:   entries,
			expectedFiles: []string{
				".config.json",
			},
		},
		{
			description:   "standard prefix - directories should not match",
			path:          "/some/path/",
			prefix:        "other",
			resolvedPath:  "/some/path/",
			allEntries:    entries,
			expectedFiles: []string{},
		},
	}

	for _, test := range tests {
		lt.Run(test.description, func() {
			// setup location
			loc, err := lt.ftpfs.NewLocation("host.com", test.path)
			lt.Require().NoError(err, test.description)

			// setup mock List
			lt.client.EXPECT().
				List(test.resolvedPath).
				Return(test.allEntries, nil).
				Once()

			fileList, err := testsuite.CollectList(context.Background(), loc, vfs.WithPrefix(test.prefix))
			lt.Require().NoError(err, test.description)
			lt.Equal(test.expectedFiles, fileList, test.description)
		})
	}

	// client.List returns no results, return empty string slice and nil (error)
	locPath := "/dir1/"
	prefix := "fil"
	loc, err := lt.ftpfs.NewLocation("ftp.host.com", locPath)
	lt.Require().NoError(err)
	lt.client.EXPECT().
		List(locPath).
		Return(nil, errors.New("550")).
		Once()
	expectedEmptyStringSlice := make([]string, 0)
	fileList, err := testsuite.CollectList(context.Background(), loc, vfs.WithPrefix(prefix))
	lt.Require().NoError(err, "no error expected expected")
	lt.Equal(expectedEmptyStringSlice, fileList, "fileList should be empty string slice")

	ftpLoc := loc.(*Location)

	// error getting client
	ftpLoc.fileSystem.clientGetter = clientGetterReturnsError
	ftpLoc.fileSystem.WithClient(nil)
	ftpLoc.fileSystem.dataconn = nil
	fileList, err = testsuite.CollectList(context.Background(), loc, vfs.WithPrefix(prefix))
	lt.Require().Error(err, "error expected")
	lt.Require().ErrorIs(err, errClientGetter, "err should be correct type")
	lt.Equal(expectedEmptyStringSlice, fileList, "fileList should be empty string slice")

	// error calling client.List()
	ftpLoc.fileSystem.WithClient(lt.client)
	listErr := errors.New("some error")
	lt.client.EXPECT().
		List(locPath).
		Return([]*_ftp.Entry{}, listErr).
		Once()
	fileList, err = testsuite.CollectList(context.Background(), loc, vfs.WithPrefix(prefix))
	lt.Require().Error(err, "error expected")
	lt.Require().ErrorIs(err, listErr, "err should be correct type")
	lt.Equal(expectedEmptyStringSlice, fileList, "fileList should be empty string slice")
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
	authorityStr := "host.com"
	locPath := "/dir1/"
	lt.client.EXPECT().List(locPath).Return(entries, nil).Once()
	loc, err := lt.ftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)

	fileTypeRegex := regexp.MustCompile("txt$")
	fileList, err := testsuite.CollectList(context.Background(), loc, vfs.WithRegexp(fileTypeRegex))
	lt.Require().NoError(err, "Shouldn't return an error on successful call to List with regexp")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}

	// List returns error
	listErr := errors.New("some list error")
	lt.client.EXPECT().List(locPath).Return(nil, listErr).Once()
	fileList, err = testsuite.CollectList(context.Background(), loc, vfs.WithRegexp(fileTypeRegex))
	lt.Require().Error(err, "error is expected")
	lt.Require().ErrorIs(err, listErr, "error is right kind of error")
	lt.Empty(fileList)
}

func (lt *locationTestSuite) TestURI() {
	authorityStr := "user@host.com:21"
	loc, err := lt.ftpfs.NewLocation(authorityStr, "/blah/")
	lt.Require().NoError(err)
	lt.Equal("ftp://user@host.com:21/blah/", loc.URI(), "location uri with user, host, port")

	authorityStr = "user:password@host.com"
	file, err := lt.ftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().NoError(err)
	lt.Equal("ftp://user@host.com/blah/file.txt", file.URI(), "file uri with user, pass, host")

	authorityStr = `domain.com\user@host.com`
	_, err = lt.ftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().Error(err)
	lt.Require().ErrorContains(err, "net/url: invalid userinfo", "file uri with bad user")

	authorityStr = `domain.com%5Cuser@host.com`
	file, err = lt.ftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().NoError(err)
	lt.Equal(`ftp://domain.com%5Cuser@host.com/blah/file.txt`, file.URI(), "file uri with percent-encoded character in user")
}

func (lt *locationTestSuite) TestString() {
	authorityStr := "user@host.com:21"
	loc, err := lt.ftpfs.NewLocation(authorityStr, "/blah/")
	lt.Require().NoError(err)
	lt.Equal("ftp://user@host.com:21/blah/", loc.String(), "location string with user, host, port")

	authorityStr = "user:password@host.com"
	file, err := lt.ftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().NoError(err)
	lt.Equal("ftp://user@host.com/blah/file.txt", file.String(), "file string with user, pass, host")
}

func (lt *locationTestSuite) TestPath() {
	loc, err := lt.ftpfs.NewLocation("host.com", "/path/")
	lt.Require().NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := lt.ftpfs.NewLocation("bucket", "/path/../newpath/")
	lt.Require().NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := lt.ftpfs.NewLocation("bucket", "/path/./to/")
	lt.Require().NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	loc, err := lt.ftpfs.NewLocation("host.com", "/some/path/to/")
	lt.Require().NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

	// test empty path error
	_, err = loc.NewFile("")
	lt.Require().ErrorIs(err, utils.ErrBadRelFilePath, "errors returned by NewFile")

	// test validation error
	_, err = loc.NewFile("/absolute/path/to/file.txt")
	lt.Require().ErrorIs(err, utils.ErrBadRelFilePath, "errors returned by NewLocation")

	// new tests for location update
	lt.Run("new file with relative path updates location", func() {
		newFile, err := loc.NewFile("../newfile.txt")
		lt.Require().NoError(err)
		lt.Equal("/some/path/newfile.txt", newFile.Path(), "NewFile with relative path should update location correctly")
		lt.Equal("/some/path/", newFile.Location().Path(), "NewFile with relative path should update location correctly")
	})

	lt.Run("new file with relative path to root", func() {
		newFile, err := loc.NewFile("../../../../newrootfile.txt")
		lt.Require().NoError(err)
		lt.Equal("/newrootfile.txt", newFile.Path(), "NewFile with relative path to root should update location correctly")
		lt.Equal("/", newFile.Location().Path(), "NewFile with relative path to root should update location correctly")
	})
}

func (lt *locationTestSuite) TestExists() {
	authorityStr := "host.com"

	// location exists
	locPath := "/"
	entries := []*_ftp.Entry{
		{
			Name:   "file.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   locPath,
			Target: "",
			Type:   _ftp.EntryTypeFolder,
			Time:   time.Now().UTC(),
		},
	}
	lt.client.EXPECT().List(locPath).Return(entries, nil).Once()
	loc, err := lt.ftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err := loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")

	// locations does not exist
	locPath = "/my/dir/"
	entries = []*_ftp.Entry{
		{
			Name:   "file.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
	}
	lt.client.EXPECT().List("/my/").Return(entries, nil).Once()
	loc, err = lt.ftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err = loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return false.")

	// some error calling list
	lt.client.EXPECT().List("/my/").Return(entries, errors.New("some error")).Once()
	loc, err = lt.ftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err = loc.Exists()
	lt.Require().Error(err, "from Exists")
	lt.False(exists, "Call to Exists expected to return false.")

	// check for not dir -- this shouldn't be possible since NewLocation won't accept non-absolute directories
	entries = []*_ftp.Entry{
		{
			Name:   "file.txt",
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
		{
			Name:   locPath,
			Target: "",
			Type:   _ftp.EntryTypeFile,
			Time:   time.Now().UTC(),
		},
	}
	lt.client.EXPECT().List("/my/").Return(entries, nil).Once()
	loc, err = lt.ftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err = loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return false.")

	// error getting client
	loc.(*Location).fileSystem.clientGetter = clientGetterReturnsError
	loc.(*Location).fileSystem.WithClient(nil)
	loc.(*Location).fileSystem.dataconn = nil
	exists, err = loc.Exists()
	lt.Require().Error(err, "error expected")
	lt.Require().ErrorIs(err, errClientGetter, "err should be correct type")
	lt.False(exists, "exists should be false on error")
}

func (lt *locationTestSuite) TestNewLocation() {
	loc, err := lt.ftpfs.NewLocation("ftp.host.com:21", "/old/")
	lt.Require().NoError(err)
	newLoc, err := loc.NewLocation("new/path/")
	lt.Require().NoError(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path/")
	lt.Require().NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.Require().ErrorIs(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.Require().ErrorIs(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestDeleteFile() {
	lt.ftpfs.dataConnGetter = getFakeDataConn
	defer func() { lt.ftpfs.dataConnGetter = nil }()
	loc, err := lt.ftpfs.NewLocation("ftp.host.com:21", "/old/")
	lt.Require().NoError(err)

	err = loc.DeleteFile("filename.txt")
	lt.Require().NoError(err, "Successful delete should not return an error.")

	// error deleting
	lt.ftpfs.dataConnGetter = nil
	loc.(*Location).fileSystem.dataconn = nil
	lt.client.EXPECT().Delete("/old/filename.txt").Return(os.ErrNotExist).Once()
	err = loc.DeleteFile("filename.txt")
	lt.Require().Error(err, "failed delete")
	lt.Require().ErrorIs(err, os.ErrNotExist, "error should be right kind of error")

	// getting NewFile
	err = loc.DeleteFile("")
	lt.Require().Error(err, "failed delete")
	lt.Require().ErrorIs(err, utils.ErrBadRelFilePath, "failed delete")
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))
}
