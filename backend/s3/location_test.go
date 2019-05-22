package s3

import (
	"path"
	"regexp"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v4/mocks"
)

type locationTestSuite struct {
	suite.Suite
	fs        *FileSystem
	s3apiMock *mocks.S3API
}

func (lt *locationTestSuite) SetupTest() {
	lt.s3apiMock = &mocks.S3API{}
	lt.fs = &FileSystem{client: lt.s3apiMock}
}

func (lt *locationTestSuite) TestList() {
	expectedFileList := []string{"file.txt", "file2.txt"}
	keyListFromAPI := []string{"dir1/file.txt", "dir1/file2.txt"}
	bucket := "bucket"
	locPath := "dir1/"
	delimiter := "/"
	isTruncated := false
	lt.s3apiMock.On("ListObjects", &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &locPath,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(keyListFromAPI),
		IsTruncated: &isTruncated,
		Prefix:      &locPath,
	}, nil).Once()

	loc := &Location{lt.fs, locPath, bucket}
	fileList, err := loc.List()
	lt.Nil(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in expected file list.")
	}
	lt.s3apiMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestList_pagedCall() {
	firstKeyList := []string{"dir1/file.txt", "dir1/file2.txt"}
	firstCallOutputMarker := firstKeyList[len(firstKeyList)-1]
	secondKeyList := []string{"dir1/file3.txt", "dir1/file4.txt"}
	expectedFileList := []string{"file.txt", "file2.txt", "file3.txt", "file4.txt"}
	bucket := "bucket"
	locPath := "dir1/"
	delimiter := "/"
	isTruncatedTrue := true
	isTruncatedFalse := false
	lt.s3apiMock.On("ListObjects", &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &locPath,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(firstKeyList),
		IsTruncated: &isTruncatedTrue,
		NextMarker:  &firstCallOutputMarker,
		Prefix:      &locPath,
	}, nil)

	lt.s3apiMock.On("ListObjects", &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &locPath,
		Delimiter: &delimiter,
		Marker:    &firstCallOutputMarker,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(secondKeyList),
		IsTruncated: &isTruncatedFalse,
		Prefix:      &locPath,
	}, nil)

	loc := &Location{lt.fs, locPath, bucket}
	fileList, err := loc.List()
	lt.Nil(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return the expected number of files.")
	for _, expectedKey := range expectedFileList {
		lt.Contains(fileList, expectedKey, "All returned keys should be in expected file list.")
	}
	lt.s3apiMock.AssertNumberOfCalls(lt.T(), "ListObjects", 2)
}

func (lt *locationTestSuite) TestListByPrefix() {
	expectedFileList := []string{"file1.txt", "file2.txt"}
	keyListFromAPI := []string{"dir1/file1.txt", "dir1/file2.txt"}
	bucket := "bucket"
	locPath := "dir1/"
	prefix := "fil"
	apiCallPrefix := path.Join(locPath, prefix)
	delimiter := "/"
	isTruncated := false
	lt.s3apiMock.On("ListObjects", &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &apiCallPrefix,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(keyListFromAPI),
		IsTruncated: &isTruncated,
		Prefix:      &apiCallPrefix,
	}, nil).Once()
	loc := &Location{lt.fs, locPath, bucket}
	fileList, err := loc.ListByPrefix(prefix)
	lt.Nil(err, "Shouldn't return an error when successfully returning list.")
	lt.Len(fileList, len(expectedFileList), "Should return expected number of file keys.")
	for _, fileKey := range fileList {
		lt.Contains(expectedFileList, fileKey, "All returned keys should be in the expected list.")
	}
	lt.s3apiMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestListByRegex() {
	expectedFileList := []string{"file1.txt", "file2.txt"}
	keysReturnedFromAPI := append(expectedFileList, "file3.png", "file4.jpg")
	bucket := "bucket"
	locPath := ""
	delimiter := "/"
	isTruncated := false
	lt.s3apiMock.On("ListObjects", &s3.ListObjectsInput{
		Bucket:    &bucket,
		Prefix:    &locPath,
		Delimiter: &delimiter,
	}).Return(&s3.ListObjectsOutput{
		Contents:    convertKeysToS3Objects(keysReturnedFromAPI),
		IsTruncated: &isTruncated,
		Prefix:      &locPath,
	}, nil).Once()
	loc := &Location{lt.fs, locPath, bucket}
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
	lt.s3apiMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestVolume() {
	bucket := "bucket"
	loc := &Location{lt.fs, "", bucket}
	lt.Equal(bucket, loc.Volume(), "Volume() should return the bucket name on location.")
}

func (lt *locationTestSuite) TestPath() {
	loc, err := lt.fs.NewLocation("bucket", "path/")
	lt.NoError(err)
	lt.Equal("/path/", loc.Path(), "Path() should return the path on location.")

	loc2, err2 := lt.fs.NewLocation("bucket", "/path/")
	lt.NoError(err2)
	lt.Equal("/path/", loc2.Path(), "Path() should return the path on location.")

	loc3, err3 := lt.fs.NewLocation("bucket", "path")
	lt.NoError(err3)
	lt.Equal("/path/", loc3.Path(), "Path() should return the path on location.")
}

func (lt *locationTestSuite) TestNewFile() {
	loc, err := lt.fs.NewLocation("bucket", "some/path/to/")
	lt.NoError(err)
	lt.Equal("/some/path/to/", loc.Path(), "Path() should return the path on location.")

	newfile, _ := loc.NewFile("a/file.txt")
	lt.Equal("/some/path/to/a/file.txt", newfile.Path(), "NewFile relative path works")

	newrelfile, _ := loc.NewFile("../../where/file.txt")
	lt.Equal("/some/where/file.txt", newrelfile.Path(), "Newfile relative dot path works")
}

func (lt *locationTestSuite) TestExists_true() {
	bucket := "foo"
	lt.s3apiMock.On("HeadBucket", &s3.HeadBucketInput{
		Bucket: &bucket,
	}).Return(&s3.HeadBucketOutput{}, nil).Once()
	loc := &Location{lt.fs, "", bucket}
	exists, err := loc.Exists()
	lt.Nil(err, "No error expected from Exists")
	lt.True(exists, "Call to Exists expected to return true.")
	lt.s3apiMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestExists_false() {
	bucket := "foo"
	lt.s3apiMock.On("HeadBucket", &s3.HeadBucketInput{
		Bucket: &bucket,
	}).Return(nil, awserr.New(s3.ErrCodeNoSuchBucket, "NoSuchBucket", nil)).Once()
	loc := &Location{lt.fs, "", bucket}
	exists, err := loc.Exists()
	lt.Nil(err, "No error expected from Exists")
	lt.False(exists, "Call to Exists expected to return true.")
	lt.s3apiMock.AssertExpectations(lt.T())
}

func (lt *locationTestSuite) TestChangeDir() {
	loc := &Location{lt.fs, "", "bucket"}

	err1 := loc.ChangeDir("..")
	assert.NoError(lt.T(), err1, "no error expected")
	lt.Equal("/", loc.Path())

	err2 := loc.ChangeDir("/hello")
	assert.NoError(lt.T(), err2, "no error expected")
	lt.Equal("/hello/", loc.Path())

	err3 := loc.ChangeDir("../.././..")
	assert.NoError(lt.T(), err3, "no error expected")
	lt.Equal("/", loc.Path())

	err4 := loc.ChangeDir("here/is/a/path/")
	assert.NoError(lt.T(), err4, "no error expected")
	lt.Equal("/here/is/a/path/", loc.Path())

	err5 := loc.ChangeDir("../")
	assert.NoError(lt.T(), err5, "no error expected")
	lt.Equal("/here/is/a/", loc.Path())
}

func (lt *locationTestSuite) TestNewLocation() {
	loc := &Location{lt.fs, "old", "bucket"}
	newLoc, err := loc.NewLocation("new/path")
	lt.Nil(err, "No error from successful call to NewLocation")
	lt.Equal("/old/new/path/", newLoc.Path(), "New location should have correct path set")
	lt.Equal("/old/", loc.Path(), "Ensure original path is unchanged.")

	newRelLoc, err := newLoc.NewLocation("../../some/path")
	lt.NoError(err)
	lt.Equal("/old/some/path/", newRelLoc.Path(), "NewLocation works with rel dot paths")
}

func (lt *locationTestSuite) TestStringURI() {
	loc := &Location{fileSystem: lt.fs, prefix: "some/path/to/location", bucket: "mybucket"}
	lt.Equal("s3://mybucket/some/path/to/location/", loc.String(), "uri is returned")
}

func (lt *locationTestSuite) TestDeleteFile() {
	lt.s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)
	lt.s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	loc := &Location{lt.fs, "old", "bucket"}

	err := loc.DeleteFile("filename.txt")
	lt.Nil(err, "Successful delete should not return an error.")
	lt.s3apiMock.AssertExpectations(lt.T())
}

func TestLocation(t *testing.T) {
	suite.Run(t, new(locationTestSuite))
}

/*
	Helpers
*/
func convertKeysToS3Objects(keys []string) []*s3.Object {
	var objects []*s3.Object
	for _, key := range keys {
		object := &s3.Object{}
		objects = append(objects, object.SetKey(key))
	}
	return objects
}
