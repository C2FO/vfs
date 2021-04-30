package s3

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/mocks"
)

type fileTestSuite struct {
	suite.Suite
}

var (
	s3apiMock      *mocks.S3API
	fs             FileSystem
	testFile       vfs.File
	defaultOptions Options
)

func (ts *fileTestSuite) SetupTest() {
	var err error
	s3apiMock = &mocks.S3API{}
	defaultOptions = Options{AccessKeyID: "abc"}
	fs = FileSystem{client: s3apiMock, options: defaultOptions}
	testFile, err = fs.NewFile("bucket", "/some/path/to/file.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func (ts *fileTestSuite) TestRead() {
	contents := "hello world!"
	s3apiMock.On("GetObject", mock.AnythingOfType("*s3.GetObjectInput")).Return(&s3.GetObjectOutput{
		Body: nopCloser{bytes.NewBufferString(contents)},
	}, nil)

	file, err := fs.NewFile("bucket", "/some/path/file.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file")
	}

	var localFile = bytes.NewBuffer([]byte{})

	_, copyErr := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr, "no error expected")
	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no error expected")

	s3apiMock.AssertExpectations(ts.T())
	ts.Equal(localFile.String(), contents, "Copying an s3 file to a buffer should fill buffer with file's contents")
}

// TODO: Write on Close() (actual s3 calls wait until file is closed to be made.)
func (ts *fileTestSuite) TestWrite() {
	file, err := fs.NewFile("bucket", "/tmp/hello.txt")
	ts.NoError(err, "Shouldn't fail creating new file")

	contents := []byte("Hello world!")
	count, err := file.Write(contents)

	ts.Equal(len(contents), count, "Returned count of bytes written should match number of bytes passed to Write.")
	ts.Nil(err, "Error should be nil when calling Write")
}

func (ts *fileTestSuite) TestSeek() {
	contents := "hello world!"
	file, err := fs.NewFile("bucket", "/tmp/hello.txt")
	ts.NoError(err, "Shouldn't fail creating new file")

	s3apiMock.On("GetObject", mock.AnythingOfType("*s3.GetObjectInput")).Return(&s3.GetObjectOutput{
		Body: nopCloser{bytes.NewBufferString(contents)},
	}, nil)

	_, seekErr := file.Seek(6, 0)
	assert.NoError(ts.T(), seekErr, "no error expected")

	var localFile = bytes.NewBuffer([]byte{})

	_, copyErr := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr, "no error expected")

	ts.Equal("world!", localFile.String(), "Seeking should download the file and move the cursor as expected")

	localFile = bytes.NewBuffer([]byte{})
	_, seekErr2 := file.Seek(0, 0)
	assert.NoError(ts.T(), seekErr2, "no error expected")

	_, copyErr2 := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr2, "no error expected")
	ts.Equal(contents, localFile.String(), "Subsequent calls to seek work on temp file as expected")

	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no error expected")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestGetLocation() {
	file, err := fs.NewFile("bucket", "/path/hello.txt")
	ts.NoError(err, "Shouldn't fail creating new file.")

	location := file.Location()
	ts.Equal("s3", location.FileSystem().Scheme(), "Should initialize location with FS underlying file.")
	ts.Equal("/path/", location.Path(), "Should initialize path with the location of the file.")
	ts.Equal("bucket", location.Volume(), "Should initialize bucket with the bucket containing the file.")
}

func (ts *fileTestSuite) TestExists() {
	file, err := fs.NewFile("bucket", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)

	exists, err := file.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.Nil(err, "Shouldn't return an error when exists is true")
}

func (ts *fileTestSuite) TestNotExists() {
	file, err := fs.NewFile("bucket", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).
		Return(&s3.HeadObjectOutput{}, awserr.New(s3.ErrCodeNoSuchKey, "key doesn't exist", nil))

	exists, err := file.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.Nil(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	targetFile := &File{
		fileSystem: &FileSystem{
			client:  s3apiMock,
			options: defaultOptions,
		},
		bucket: "TestBucket",
		key:    "testKey.txt",
	}

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)

	err := testFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestEmptyCopyToFile() {
	targetFile := &mocks.File{}
	targetFile.On("Write", mock.Anything).Return(0, nil)
	targetFile.On("Close").Return(nil)

	expectedSize := int64(0)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{ContentLength: &expectedSize}, nil, nil)

	err := testFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertExpectations(ts.T())

	// Assert that file was still written to and closed when the reader size is 0 bytes.
	targetFile.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile() {
	targetFile := &File{
		fileSystem: &FileSystem{
			client:  s3apiMock,
			options: defaultOptions,
		},
		bucket: "TestBucket",
		key:    "testKey.txt",
	}

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)
	s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)

	err := testFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestGetCopyObject() {
	type getCopyObjectTest struct {
		key, expectedCopySource string
	}
	tests := []getCopyObjectTest{
		{
			key:                "/path/to/nospace.txt",
			expectedCopySource: "%2Fpath%2Fto%2Fnospace.txt",
		},
		{
			key:                "/path/to/has space.txt",
			expectedCopySource: "%2Fpath%2Fto%2Fhas%20space.txt",
		},
		{
			key:                "/path/to/encoded%20space.txt",
			expectedCopySource: "%2Fpath%2Fto%2Fencoded%2520space.txt",
		},
		{
			key:                "/path/to/has space/file.txt",
			expectedCopySource: "%2Fpath%2Fto%2Fhas%20space%2Ffile.txt",
		},
		{
			key:                "/path/to/encoded%20space/file.txt",
			expectedCopySource: "%2Fpath%2Fto%2Fencoded%2520space%2Ffile.txt",
		},
	}

	// ensure spaces are properly encoded (or not)
	for _, t := range tests {
		sourceFile := &File{
			fileSystem: &FileSystem{
				client:  s3apiMock,
				options: defaultOptions,
			},
			bucket: "TestBucket",
			key:    t.key,
		}

		targetFile := &File{
			fileSystem: &FileSystem{
				client:  s3apiMock,
				options: defaultOptions,
			},
			bucket: "TestBucket",
			key:    "source.txt",
		}

		// copy from t.key to /source.txt
		actual, err := sourceFile.getCopyObjectInput(targetFile)
		ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
		ts.Equal("TestBucket"+t.expectedCopySource, *actual.CopySource)
	}

	// test that different options returns nil
	// nil means we can't do s3-to-s3 copy so use TouchCopy
	sourceFile := &File{
		fileSystem: &FileSystem{
			client:  s3apiMock,
			options: defaultOptions,
		},
		bucket: "TestBucket",
		key:    "/path/to/file.txt",
	}

	targetFile := &File{
		fileSystem: &FileSystem{
			client: s3apiMock,
			options: Options{AccessKeyID: "xyz",
				ACL: "SomeCannedACL",
			},
		},
		bucket: "TestBucket",
		key:    "/path/to/otherFile.txt",
	}
	actual, err := sourceFile.getCopyObjectInput(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")
	ts.Nil(actual, "copyOjbectInput should be nil (can't do s3-to-s3 copyObject)")

	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToFile_CopyError() {
	targetFile := &File{
		fileSystem: &FileSystem{
			client:  s3apiMock,
			options: defaultOptions,
		},
		bucket: "TestBucket",
		key:    "testKey.txt",
	}

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, errors.New("some copy error"))

	err := testFile.MoveToFile(targetFile)
	ts.NotNil(err, "Error shouldn't be returned from successful call to CopyToFile")
	s3apiMock.AssertNotCalled(ts.T(), "DeleteObject", mock.Anything)
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestCopyToLocation() {
	s3Mock1 := &mocks.S3API{}
	s3Mock1.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, nil)
	s3Mock1.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)
	f := &File{
		fileSystem: &FileSystem{
			client:  s3Mock1,
			options: defaultOptions,
		},
		bucket: "bucket",
		key:    "/hello.txt",
	}

	defer func() {
		closeErr := f.Close()
		assert.NoError(ts.T(), closeErr, "no error expected")
	}()

	l := &Location{
		fileSystem: &FileSystem{
			client:  &mocks.S3API{},
			options: defaultOptions,
		},
		bucket: "bucket",
		prefix: "/subdir/",
	}

	// no error "copying" objects
	_, err := f.CopyToLocation(l)
	ts.NoError(err, "Shouldn't return error for this call to CopyToLocation")

}

func (ts *fileTestSuite) TestTouch() {
	// Copy portion tested through CopyToLocation, just need to test whether or not Delete happens
	// in addition to CopyToLocation
	s3Mock1 := &mocks.S3API{}
	s3Mock1.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)
	s3Mock1.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, nil)
	s3Mock1.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	file := &File{
		fileSystem: &FileSystem{
			client:  s3Mock1,
			options: defaultOptions,
		},
		bucket: "newBucket",
		key:    "/new/file/path/hello.txt",
	}
	terr := file.Touch()
	ts.NoError(terr, "Shouldn't return error creating test s3.File instance.")

	s3Mock1.AssertExpectations(ts.T())

	// test non-existent length
	s3Mock2 := &mocks.S3API{}
	s3Mock2.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).
		Return(&s3.HeadObjectOutput{}, awserr.New(s3.ErrCodeNoSuchKey, "", nil)).Once()
	s3Mock2.On("PutObjectRequest", mock.AnythingOfType("*s3.PutObjectInput")).
		Return(&request.Request{HTTPRequest: &http.Request{Header: make(map[string][]string), URL: &url.URL{}}}, &s3.PutObjectOutput{})
	s3Mock2.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).
		Return(&s3.HeadObjectOutput{}, nil)
	file2 := &File{
		fileSystem: &FileSystem{
			client:  s3Mock2,
			options: defaultOptions,
		},
		bucket: "newBucket",
		key:    "/new/file/path/hello.txt",
	}
	terr2 := file2.Touch()
	ts.NoError(terr2, "Shouldn't return error creating test s3.File instance.")

	s3Mock2.AssertExpectations(ts.T())

}
func (ts *fileTestSuite) TestMoveToLocation() {
	// Copy portion tested through CopyToLocation, just need to test whether or not Delete happens
	// in addition to CopyToLocation
	s3Mock1 := &mocks.S3API{}
	s3Mock1.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, nil)
	s3Mock1.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{}, nil)
	f := &File{
		fileSystem: &FileSystem{
			client:  s3Mock1,
			options: defaultOptions,
		},
		bucket: "newBucket",
		key:    "/new/file/path/hello.txt",
	}
	location := new(mocks.Location)
	location.On("NewFile", mock.Anything).Return(f, nil)

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)
	s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)

	file, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	defer func() {
		closeErr := file.Close()
		ts.NoError(closeErr, "no error expected")
	}()

	_, err = file.MoveToLocation(location)
	ts.NoError(err, "no error expected")

	// test non-scheme MoveToLocation
	mockLocation := new(mocks.Location)
	mockLocation.On("NewFile", mock.Anything).
		Return(&File{fileSystem: &FileSystem{client: s3Mock1}, bucket: "bucket", key: "/new/hello.txt"}, nil)

	s3apiMock2 := &mocks.S3API{}
	s3apiMock2.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(&s3.CopyObjectOutput{}, nil)

	fs = FileSystem{client: s3apiMock2}
	file2, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	_, err = file2.CopyToLocation(mockLocation)
	ts.NoError(err, "MoveToLocation error not expected")

	s3apiMock.AssertExpectations(ts.T())
	location.AssertExpectations(ts.T())
	mockLocation.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestMoveToLocationFail() {
	// If CopyToLocation fails we need to ensure DeleteObject isn't called.
	otherFs := new(mocks.FileSystem)
	location := new(mocks.Location)
	location.On("NewFile", mock.Anything).Return(&File{fileSystem: &fs, bucket: "bucket", key: "/new/hello.txt"}, nil)

	s3apiMock.On("CopyObject", mock.AnythingOfType("*s3.CopyObjectInput")).Return(nil, errors.New("didn't copy, oh noes"))

	file, err := fs.NewFile("bucket", "/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't return error creating test s3.File instance.")
	}

	_, merr := file.MoveToLocation(location)
	ts.Error(merr, "MoveToLocation error not expected")

	closeErr := file.Close()
	ts.NoError(closeErr, "no close error expected")

	s3apiMock.AssertExpectations(ts.T())
	s3apiMock.AssertNotCalled(ts.T(), "DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput"))
	otherFs.AssertExpectations(ts.T())
	location.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestDelete() {
	s3apiMock.On("DeleteObject", mock.AnythingOfType("*s3.DeleteObjectInput")).Return(&s3.DeleteObjectOutput{}, nil)
	err := testFile.Delete()
	ts.Nil(err, "Successful delete should not return an error.")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestLastModified() {
	now := time.Now()
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{
		LastModified: &now,
	}, nil)
	modTime, err := testFile.LastModified()
	ts.Nil(err, "Error should be nil when correctly returning time of object.")
	ts.Equal(&now, modTime, "Returned time matches expected LastModified time.")
}

func (ts *fileTestSuite) TestLastModifiedFail() {
	// setup error on HEAD
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(nil,
		errors.New("boom"))
	m, e := testFile.LastModified()
	ts.Error(e, "got error as exepcted")
	ts.Nil(m, "nil ModTime returned")
}

func (ts *fileTestSuite) TestName() {
	ts.Equal("file.txt", testFile.Name(), "Name should return just the name of the file.")
}

func (ts *fileTestSuite) TestSize() {
	contentLength := int64(100)
	s3apiMock.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).Return(&s3.HeadObjectOutput{
		ContentLength: &contentLength,
	}, nil)

	size, err := testFile.Size()
	ts.Nil(err, "Error should be nil when requesting size for file that exists.")
	ts.Equal(uint64(100), size, "Size should return the ContentLength value from s3 HEAD request.")
	s3apiMock.AssertExpectations(ts.T())
}

func (ts *fileTestSuite) TestPath() {
	ts.Equal("/some/path/to/file.txt", testFile.Path(), "Should return file.key (with leading slash)")
}

func (ts *fileTestSuite) TestURI() {
	s3apiMock = &mocks.S3API{}
	fs = FileSystem{client: s3apiMock}
	file, _ := fs.NewFile("mybucket", "/some/file/test.txt")
	expected := "s3://mybucket/some/file/test.txt"
	ts.Equal(expected, file.URI(), "%s does not match %s", file.URI(), expected)
}

func (ts *fileTestSuite) TestStringer() {
	fs = FileSystem{client: &mocks.S3API{}}
	file, _ := fs.NewFile("mybucket", "/some/file/test.txt")
	ts.Equal("s3://mybucket/some/file/test.txt", file.String())
}

func (ts *fileTestSuite) TestUploadInput() {
	fs = FileSystem{client: &mocks.S3API{}}
	file, _ := fs.NewFile("mybucket", "/some/file/test.txt")
	ts.Equal("AES256", *uploadInput(file.(*File)).ServerSideEncryption, "sse was set")
	ts.Equal("/some/file/test.txt", *uploadInput(file.(*File)).Key, "key was set")
	ts.Equal("mybucket", *uploadInput(file.(*File)).Bucket, "bucket was set")
}

func (ts *fileTestSuite) TestNewFile() {
	fs := &FileSystem{}
	// fs is nil
	_, err := fs.NewFile("", "")
	ts.Errorf(err, "non-nil s3.FileSystem pointer is required")

	// bucket is ""
	_, err = fs.NewFile("", "asdf")
	ts.Errorf(err, "non-empty strings for bucket and key are required")
	// key is ""
	_, err = fs.NewFile("asdf", "")
	ts.Errorf(err, "non-empty strings for bucket and key are required")

	//
	bucket := "mybucket"
	key := "/path/to/key"
	file, err := fs.NewFile(bucket, key)
	ts.NoError(err, "newFile should succeed")
	ts.IsType(&File{}, file, "newFile returned a File struct")
	ts.Equal(bucket, file.Location().Volume())
	ts.Equal(key, file.Path())
}

func (ts *fileTestSuite) TestCloseWithoutWrite() {
	fs := &FileSystem{}
	file, err := fs.NewFile("mybucket", "/some/file/test.txt")
	ts.NoError(err)
	ts.NoError(file.Close())
	ts.NoError(err, "file closed without error")
}

func (ts *fileTestSuite) TestCloseWithWrite() {
	s3Mock2 := &mocks.S3API{}
	s3Mock2.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).
		Return(&s3.HeadObjectOutput{}, awserr.New(s3.ErrCodeNoSuchKey, "", nil)).Once()
	s3Mock2.On("PutObjectRequest", mock.AnythingOfType("*s3.PutObjectInput")).
		Return(&request.Request{HTTPRequest: &http.Request{Header: make(map[string][]string), URL: &url.URL{}}}, &s3.PutObjectOutput{})
	s3Mock2.On("HeadObject", mock.AnythingOfType("*s3.HeadObjectInput")).
		Return(&s3.HeadObjectOutput{}, awserr.New(s3.ErrCodeNoSuchKey, "key doesn't exist", nil))
	file := &File{
		fileSystem: &FileSystem{
			client:  s3Mock2,
			options: defaultOptions,
		},
		bucket: "newBucket",
		key:    "/new/file/path/hello.txt",
	}
	contents := []byte("Hello world!")
	_, err := file.Write(contents)
	ts.NoError(err, "Error should be nil when calling Write")
	err = file.Close()
	ts.Error(err, "file doesn't exists , retired 5 times ")

}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}
