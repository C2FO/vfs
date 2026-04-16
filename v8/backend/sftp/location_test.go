package sftp

import (
	"context"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/utils"
	vfs "github.com/c2fo/vfs/v8"
	"github.com/c2fo/vfs/v8/backend/sftp/mocks"
	"github.com/c2fo/vfs/v8/backend/testsuite"
)

type locationTestSuite struct {
	suite.Suite
	sftpfs *FileSystem
	client *mocks.Client
}

func (lt *locationTestSuite) SetupTest() {
	lt.client = mocks.NewClient(lt.T())
	lt.sftpfs = &FileSystem{
		sftpclient: lt.client,
	}
}

func (lt *locationTestSuite) TestList() {
	expectedFileList := []string{"file.txt", "file2.txt"}

	file1 := mocks.NewFileInfo(lt.T())
	file1.
		On("Name").Return("file.txt").
		On("IsDir").Return(false)
	file2 := mocks.NewFileInfo(lt.T())
	file2.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	subdir1 := mocks.NewFileInfo(lt.T())
	subdir1.EXPECT().IsDir().Return(true)
	keyListFromAPI := []*mocks.FileInfo{subdir1, file1, file2}
	authorityStr := "host.com"
	locPath := "/dir1/"
	lt.client.EXPECT().ReadDir(locPath).Return(sliceImplementationToInterface(keyListFromAPI), nil).Once()

	loc, err := lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	ctx := context.Background()
	fileList, err := testsuite.CollectList(ctx, loc)
	lt.Require().NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in expected file list.")
	}

	// file not found (location doesn't exist)
	lt.client.EXPECT().ReadDir(locPath).Return(make([]os.FileInfo, 0), errors.New("some error")).Once()
	fileList, err = testsuite.CollectList(ctx, loc)
	lt.Require().Error(err, "should return error")
	lt.Empty(fileList, "Should return no files on error")

	// file not found (location doesn't exist)
	lt.client.EXPECT().ReadDir(locPath).Return(make([]os.FileInfo, 0), os.ErrNotExist).Once()
	fileList, err = testsuite.CollectList(ctx, loc)
	lt.Require().NoError(err, "Shouldn't return an error on file not found.")
	lt.Empty(fileList, "Should return no files on file not found")
}

func (lt *locationTestSuite) TestListByPrefix() {
	expectedFileList := []string{"file.txt", "file2.txt"}

	file1 := mocks.NewFileInfo(lt.T())
	file1.
		On("Name").Return("file.txt").
		On("IsDir").Return(false)
	file2 := mocks.NewFileInfo(lt.T())
	file2.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file3 := mocks.NewFileInfo(lt.T())
	file3.
		On("Name").Return("my_file.txt").
		On("IsDir").Return(false)
	subdir1 := mocks.NewFileInfo(lt.T())
	subdir1.EXPECT().IsDir().Return(true)
	keyListFromAPI := []*mocks.FileInfo{subdir1, file1, file2, file3}
	authorityStr := "host.com"
	locPath := "/dir1/"
	lt.client.EXPECT().ReadDir(locPath).Return(sliceImplementationToInterface(keyListFromAPI), nil).Once()
	loc, err := lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	prefix := "fil"
	fileList, err := testsuite.CollectList(context.Background(), loc, vfs.WithPrefix(prefix))
	lt.Require().NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
}

func (lt *locationTestSuite) TestListByRegex() {
	expectedFileList := []string{"file1.txt", "file2.txt", "stuff.txt"}

	file1 := mocks.NewFileInfo(lt.T())
	file1.
		On("Name").Return("file1.txt").
		On("IsDir").Return(false)
	file2 := mocks.NewFileInfo(lt.T())
	file2.
		On("Name").Return("file2.txt").
		On("IsDir").Return(false)
	file4 := mocks.NewFileInfo(lt.T())
	file4.
		On("Name").Return("stuff.txt").
		On("IsDir").Return(false)
	subdir1 := mocks.NewFileInfo(lt.T())
	subdir1.EXPECT().IsDir().Return(true)
	keyListFromAPI := []*mocks.FileInfo{subdir1, file1, file2, file4}
	authorityStr := "host.com"
	locPath := "/dir1/"
	lt.client.EXPECT().ReadDir(locPath).Return(sliceImplementationToInterface(keyListFromAPI), nil).Once()
	loc, err := lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	fileTypeRegex := regexp.MustCompile("txt$")

	fileList, err := testsuite.CollectList(context.Background(), loc, vfs.WithRegexp(fileTypeRegex))
	lt.Require().NoError(err, "Shouldn't return an error on successful call to List with regexp")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
}

func (lt *locationTestSuite) TestURI() {
	authorityStr := "user@host.com:22"
	loc, err := lt.sftpfs.NewLocation(authorityStr, "/blah/")
	lt.Require().NoError(err)
	lt.Equal("sftp://user@host.com:22/blah/", loc.URI(), "location uri with user, host, port")

	authorityStr = "user:password@host.com"
	file, err := lt.sftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().NoError(err)
	lt.Equal("sftp://user@host.com/blah/file.txt", file.URI(), "file uri with user, pass, host")

	authorityStr = `domain.com\user@host.com`
	_, err = lt.sftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().Error(err)
	lt.Require().ErrorContains(err, "net/url: invalid userinfo", "file uri with bad user")

	authorityStr = `domain.com%5Cuser@host.com`
	file, err = lt.sftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().NoError(err)
	lt.Equal(`sftp://domain.com%5Cuser@host.com/blah/file.txt`, file.URI(), "file uri with percent-encoded character in user")
}

func (lt *locationTestSuite) TestString() {
	authorityStr := "user@host.com:22"
	loc, err := lt.sftpfs.NewLocation(authorityStr, "/blah/")
	lt.Require().NoError(err)
	lt.Equal("sftp://user@host.com:22/blah/", loc.String(), "location string with user, host, port")

	authorityStr = "user:password@host.com"
	file, err := lt.sftpfs.NewFile(authorityStr, "/blah/file.txt")
	lt.Require().NoError(err)
	lt.Equal("sftp://user@host.com/blah/file.txt", file.String(), "file string with user, pass, host")
}

func (lt *locationTestSuite) TestPath() {
	loc, err := lt.sftpfs.NewLocation("host.com", "/path/")
	lt.Require().NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := lt.sftpfs.NewLocation("bucket", "/path/../newpath/")
	lt.Require().NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := lt.sftpfs.NewLocation("bucket", "/path/./to/")
	lt.Require().NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	loc, err := lt.sftpfs.NewLocation("bucket", "/some/path/to/")
	lt.Require().NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewFile("/path/to/file.txt")
	lt.Require().ErrorIs(err, errLocationRequired, "errors returned by NewFile")

	// test empty path error
	_, err = loc.NewFile("")
	lt.Require().ErrorIs(err, errPathRequired, "errors returned by NewFile")

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
	dir1 := mocks.NewFileInfo(lt.T())
	dir1.EXPECT().IsDir().Return(true)
	lt.client.EXPECT().Stat(locPath).Return(dir1, nil).Once()
	loc, err := lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err := loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")

	// locations does not exist
	locPath = "/my/dir/"
	dir1 = mocks.NewFileInfo(lt.T())
	lt.client.EXPECT().Stat(locPath).Return(dir1, os.ErrNotExist).Once()
	loc, err = lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err = loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return false.")

	// some error calling stat
	lt.client.EXPECT().Stat(locPath).Return(dir1, errors.New("some error")).Once()
	loc, err = lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err = loc.Exists()
	lt.Require().Error(err, "from Exists")
	lt.False(exists, "Call to Exists expected to return false.")

	// check for not dir -- this shouldn't be possible since NewLocation won't accept non-absolute directories
	dir1 = mocks.NewFileInfo(lt.T())
	dir1.EXPECT().IsDir().Return(false) // set isdir false
	lt.client.EXPECT().Stat(locPath).Return(dir1, nil).Once()
	loc, err = lt.sftpfs.NewLocation(authorityStr, locPath)
	lt.Require().NoError(err)
	exists, err = loc.Exists()
	lt.Require().NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return false.")
}

func (lt *locationTestSuite) TestNewLocation() {
	loc, err := lt.sftpfs.NewLocation("bucket", "/old/")
	lt.Require().NoError(err)
	newLoc, err := loc.NewLocation("new/path/")
	lt.Require().NoError(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path/")
	lt.Require().NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewLocation("/path/to/")
	lt.Require().ErrorIs(err, errLocationRequired, "errors returned by NewLocation")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.Require().ErrorIs(err, errPathRequired, "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.Require().ErrorIs(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestDeleteFile() {
	lt.client.EXPECT().Remove("/old/filename.txt").Return(nil).Once()
	loc, err := lt.sftpfs.NewLocation("bucket", "/old/")
	lt.Require().NoError(err)

	err = loc.DeleteFile("filename.txt")
	lt.Require().NoError(err, "Successful delete should not return an error.")

	// error deleting
	lt.client.EXPECT().Remove("/old/filename.txt").Return(os.ErrNotExist).Once()
	err = loc.DeleteFile("filename.txt")
	lt.Require().Error(err, "failed delete")
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
