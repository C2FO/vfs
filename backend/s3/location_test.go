package s3

import (
	"path"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/backend/s3/mocks"
	"github.com/c2fo/vfs/v6/options/delete"
	"github.com/c2fo/vfs/v6/utils"
)

type locationTestSuite struct {
	suite.Suite
	fs        *FileSystem
	s3cliMock *mocks.Client
}

func (lt *locationTestSuite) SetupTest() {
	lt.s3cliMock = &mocks.Client{}
	lt.fs = &FileSystem{client: lt.s3cliMock}
}

func (lt *locationTestSuite) TestList() {
	expectedFileList := []string{"file.txt", "file2.txt"}
	keyListFromAPI := []string{"dir1/file.txt", "dir1/file2.txt"}
	bucket := "bucket"
	locPath := "/dir1/"
	prefix := "dir1/"
	delimiter := "/"
	isTruncated := false
	lt.s3cliMock.On("ListObjects", matchContext, &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &prefix,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(keyListFromAPI),
		IsTruncated: &isTruncated,
		Prefix:      &prefix,
	}, nil).Once()

	loc, err := lt.fs.NewLocation(bucket, locPath)
	lt.NoError(err)
	fileList, err := loc.List()
	lt.NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in expected file list.")
	}
	lt.s3cliMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestList_pagedCall() {
	firstKeyList := []string{"dir1/file.txt", "dir1/file2.txt"}
	firstCallOutputMarker := firstKeyList[len(firstKeyList)-1]
	secondKeyList := []string{"dir1/file3.txt", "dir1/file4.txt"}
	expectedFileList := []string{"file.txt", "file2.txt", "file3.txt", "file4.txt"}
	bucket := "bucket"
	locPath := "/dir1/"
	prefix := "dir1/"
	delimiter := "/"
	isTruncatedTrue := true
	isTruncatedFalse := false
	lt.s3cliMock.On("ListObjects", matchContext, &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &prefix,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(firstKeyList),
		IsTruncated: &isTruncatedTrue,
		NextMarker:  &firstCallOutputMarker,
		Prefix:      &prefix,
	}, nil)

	lt.s3cliMock.On("ListObjects", matchContext, &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &prefix,
		Delimiter: &delimiter,
		Marker:    &firstCallOutputMarker,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(secondKeyList),
		IsTruncated: &isTruncatedFalse,
		Prefix:      &prefix,
	}, nil)

	loc, err := lt.fs.NewLocation(bucket, locPath)
	lt.NoError(err)
	fileList, err := loc.List()
	lt.NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, expectedKey := range expectedFileList {
		lt.Contains(fileList, expectedKey, "All returned keys should be in expected file list.")
	}
	lt.s3cliMock.AssertNumberOfCalls(lt.T(), "ListObjects", 2)
}

func (lt *locationTestSuite) TestListByPrefix() {
	expectedFileList := []string{"file1.txt", "file2.txt"}
	keyListFromAPI := []string{"dir1/file1.txt", "dir1/file2.txt"}
	bucket := "bucket"
	locPath := "/dir1/"
	prefix := "fil"
	apiCallPrefix := utils.RemoveLeadingSlash(path.Join(locPath, prefix))
	delimiter := "/"
	isTruncated := false
	lt.s3cliMock.On("ListObjects", matchContext, &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &apiCallPrefix,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(keyListFromAPI),
		IsTruncated: &isTruncated,
		Prefix:      &apiCallPrefix,
	}, nil).Once()
	loc, err := lt.fs.NewLocation(bucket, locPath)
	lt.NoError(err)
	fileList, err := loc.ListByPrefix(prefix)
	lt.NoError(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.s3cliMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestListByRegex() {
	expectedFileList := []string{"file1.txt", "file2.txt"}
	keysReturnedFromAPI := []string{"file1.txt", "file2.txt", "file3.png", "file4.jpg"}
	bucket := "bucket"
	locPath := "/blah/"
	prefix := "blah/"
	delimiter := "/"
	isTruncated := false
	lt.s3cliMock.On("ListObjects", matchContext, &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &prefix,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(keysReturnedFromAPI),
		IsTruncated: &isTruncated,
		Prefix:      &prefix,
	}, nil).Once()
	loc, err := lt.fs.NewLocation(bucket, locPath)
	lt.NoError(err)
	fileTypeRegex := regexp.MustCompile("txt$")

	fileList, err := loc.ListByRegex(fileTypeRegex)
	lt.NoError(err, "Shouldn't return an error on successful call to ListByRegex")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.s3cliMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestVolume() {
	bucket := "bucket"
	loc, err := lt.fs.NewLocation(bucket, "/")
	lt.NoError(err)
	lt.Equal(bucket, loc.Volume(), "Volume() should return the bucket name on location.")
}

func (lt *locationTestSuite) TestPath() {
	loc, err := lt.fs.NewLocation("bucket", "/path/")
	lt.NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := lt.fs.NewLocation("bucket", "/path/../newpath/")
	lt.NoError(err2)
	lt.Equal("/newpath/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := lt.fs.NewLocation("bucket", "/path/./to/")
	lt.NoError(err3)
	lt.Equal("/path/to/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	loc, err := lt.fs.NewLocation("bucket", "/some/path/to/")
	lt.NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")

	// test nil pointer
	var nilLoc *Location
	_, err = nilLoc.NewFile("/path/to/file.txt")
	lt.EqualError(err, "non-nil s3.Location pointer is required", "errors returned by NewFile")

	// test empty path error
	_, err = loc.NewFile("")
	lt.EqualError(err, "non-empty string filePath is required", "errors returned by NewFile")

	// test validation error
	_, err = loc.NewFile("/absolute/path/to/file.txt")
	lt.EqualError(err, utils.ErrBadRelFilePath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestExists_true() {
	bucket := "foo"
	lt.s3cliMock.On("HeadBucket", matchContext, &s3.HeadBucketInput{
		Bucket: &bucket,
	}).Return(&s3.HeadBucketOutput{}, nil).Once()
	loc, err := lt.fs.NewLocation(bucket, "/")
	lt.NoError(err)
	exists, err := loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")
	lt.s3cliMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestExists_false() {
	bucket := "foo"
	lt.s3cliMock.On("HeadBucket", matchContext, &s3.HeadBucketInput{
		Bucket: &bucket,
	}).Return(nil, &types.NoSuchBucket{}).Once()
	loc, err := lt.fs.NewLocation(bucket, "/")
	lt.NoError(err)
	exists, err := loc.Exists()
	lt.NoError(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return true.")
	lt.s3cliMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestChangeDir() {
	// test nil Location
	var nilLoc *Location
	err := nilLoc.ChangeDir("path/to/")
	lt.EqualErrorf(err, "non-nil s3.Location pointer is required", "error expected for nil location")

	loc := &Location{fileSystem: lt.fs, prefix: "/", bucket: "bucket"}

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
	loc, err := lt.fs.NewLocation("bucket", "/old/")
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
	lt.EqualError(err, "non-nil s3.Location pointer is required", "errors returned by NewLocation")

	// test empty path error
	_, err = loc.NewLocation("")
	lt.EqualError(err, "non-empty string relativePath is required", "errors returned by NewLocation")

	// test validation error
	_, err = loc.NewLocation("/absolute/path/to/")
	lt.EqualError(err, utils.ErrBadRelLocationPath, "errors returned by NewLocation")
}

func (lt *locationTestSuite) TestStringURI() {
	loc := &Location{fileSystem: lt.fs, prefix: "some/path/to/location", bucket: "mybucket"}
	lt.Equal("s3://mybucket/some/path/to/location/", loc.String(), "uri is returned")
}

func (lt *locationTestSuite) TestDeleteFile() {
	lt.s3cliMock.On("DeleteObject", matchContext, mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	loc, err := lt.fs.NewLocation("bucket", "/old/")
	lt.NoError(err)

	err = loc.DeleteFile("filename.txt")
	lt.NoError(err, "Successful delete should not return an error.")
	lt.s3cliMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestDeleteFileWithAllVersionsOption() {
	var versions []types.ObjectVersion
	verIds := [...]string{"ver1", "ver2"}
	for i := range verIds {
		versions = append(versions, types.ObjectVersion{VersionId: &verIds[i]})
	}
	versOutput := s3.ListObjectVersionsOutput{
		Versions: versions,
	}
	lt.s3cliMock.On("ListObjectVersions", matchContext, mock.AnythingOfType("*s3.ListObjectVersionsInput")).Return(&versOutput, nil)
	lt.s3cliMock.On("DeleteObject", matchContext, mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	loc, err := lt.fs.NewLocation("bucket", "/old/")
	lt.NoError(err)

	err = loc.DeleteFile("filename.txt", delete.WithAllVersions())
	lt.NoError(err, "Successful delete should not return an error.")
	lt.s3cliMock.AssertExpectations(lt.T())
	lt.s3cliMock.AssertNumberOfCalls(lt.T(), "DeleteObject", 3)
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))
}

/*
Helpers
*/
func convertKeysToS3Objects(keys []string) []types.Object {
	var objects []types.Object
	for _, key := range keys {
		object := types.Object{Key: aws.String(key)}
		objects = append(objects, object)
	}
	return objects
}
