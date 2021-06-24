package gs

import (
	"bytes"
	"context"
	"io"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type fileTestSuite struct {
	suite.Suite
}

func objectExists(bucket *storage.BucketHandle, objectName string) bool {
	objectHandle := bucket.Object(objectName)
	ctx := context.Background()
	_, err := objectHandle.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false
		}
		panic(err)
	}
	return true
}

func fsFileNameExists(fs *FileSystem, bucketName, objectName string) bool {
	file, err := fs.NewFile(bucketName, "/"+objectName)
	if err != nil {
		panic(err)
	}
	exists, err := file.Exists()
	if err != nil {
		panic(err)
	}
	return exists
}

func (ts *fileTestSuite) TestRead() {
	contents := "hello world!"
	bucketName := "bucki"
	objectName := "some/path/file.txt"
	server := fakestorage.NewServer(Objects{{
		BucketName:      bucketName,
		Name:            objectName,
		ContentType:     "text/plain",
		ContentEncoding: "utf8",
		Content:         []byte(contents),
	}})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile(bucketName, "/"+objectName)
	if err != nil {
		ts.Fail("Shouldn't fail creating new file")
	}

	var localFile = bytes.NewBuffer([]byte{})

	_, copyErr := io.Copy(localFile, file)
	assert.NoError(ts.T(), copyErr, "no error expected")
	closeErr := file.Close()
	assert.NoError(ts.T(), closeErr, "no error expected")

	ts.Equal(localFile.String(), contents, "Copying an gs file to a buffer should fill buffer with file's contents")
}

func (ts *fileTestSuite) TestWrite() {
	contents := "hello world!"
	bucketName := "bucki"
	objectName := "some/path/file.txt"
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile(bucketName, "/"+objectName)
	ts.NoError(err, "Shouldn't fail creating new file")

	count, err := file.Write([]byte(contents))

	ts.Equal(len(contents), count, "Returned count of bytes written should match number of bytes passed to Write.")
	ts.Nil(err, "Error should be nil when calling Write")
}

func (ts *fileTestSuite) TestGetLocation() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile("bucket", "/path/hello.txt")
	ts.NoError(err, "Shouldn't fail creating new file.")

	location := file.Location()
	ts.Equal("gs", location.FileSystem().Scheme(), "Should initialize location with FS underlying file.")
	ts.Equal("/path/", location.Path(), "Should initialize path with the location of the file.")
	ts.Equal("bucket", location.Volume(), "Should initialize bucket with the bucket containing the file.")
}

func (ts *fileTestSuite) TestExists() {
	bucketName := "bucki"
	objectName := "some/path/file.txt"

	server := fakestorage.NewServer(Objects{{
		BucketName:      bucketName,
		Name:            objectName,
		ContentType:     "text/plain",
		ContentEncoding: "utf8",
		Content:         []byte("content"),
	}})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile(bucketName, "/"+objectName)
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	exists, err := file.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.Nil(err, "Shouldn't return an error when exists is true")
}

func (ts *fileTestSuite) TestNotExists() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile("bucket", "/path/hello.txt")
	if err != nil {
		ts.Fail("Shouldn't fail creating new file.")
	}

	exists, err := file.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.Nil(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestCopyToFile() {
	sourceName := "source.txt"
	targetName := "target.txt"
	bucketName := "bucket-a"
	server := fakestorage.NewServer(Objects{{
		BucketName:      bucketName,
		Name:            sourceName,
		ContentType:     "text/plain",
		ContentEncoding: "utf8",
		Content:         []byte("content"),
	}})
	defer server.Stop()
	client := server.Client()
	fs := NewFileSystem().WithClient(client)
	bucket := client.Bucket(bucketName)

	ts.True(objectExists(bucket, sourceName), "source should exist")
	ts.True(fsFileNameExists(fs, bucketName, sourceName), "source should exist")

	ts.False(objectExists(bucket, targetName), "target should not exist")
	ts.False(fsFileNameExists(fs, bucketName, targetName), "target should not exist")

	sourceFile, err := fs.NewFile(bucketName, "/"+sourceName)
	ts.NoError(err)
	targetFile, err := fs.NewFile(bucketName, "/"+targetName)
	ts.NoError(err)
	err = sourceFile.CopyToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to CopyToFile")

	ts.True(objectExists(bucket, targetName), "target should exist")
	ts.True(fsFileNameExists(fs, bucketName, targetName), "target should exist")
}

func (ts *fileTestSuite) TestMoveToFile() {
	sourceName := "source.txt"
	targetName := "target.txt"
	bucketName := "bucket-b"
	server := fakestorage.NewServer(Objects{{
		BucketName:      bucketName,
		Name:            sourceName,
		ContentType:     "text/plain",
		ContentEncoding: "utf8",
		Content:         []byte("content"),
	}})
	defer server.Stop()
	client := server.Client()
	fs := NewFileSystem().WithClient(client)
	bucket := client.Bucket(bucketName)

	ts.True(objectExists(bucket, sourceName), "source should exist")
	ts.True(fsFileNameExists(fs, bucketName, sourceName), "source should exist")

	ts.False(objectExists(bucket, targetName), "target should not exist")
	ts.False(fsFileNameExists(fs, bucketName, targetName), "target should not exist")

	sourceFile, err := fs.NewFile(bucketName, "/"+sourceName)
	ts.NoError(err)
	targetFile, err := fs.NewFile(bucketName, "/"+targetName)
	ts.NoError(err)
	err = sourceFile.MoveToFile(targetFile)
	ts.Nil(err, "Error shouldn't be returned from successful call to MoveToFile")

	ts.True(objectExists(bucket, targetName), "target should exist")
	ts.True(fsFileNameExists(fs, bucketName, targetName), "target should exist")
}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}
