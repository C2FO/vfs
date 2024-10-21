package gs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6/options/delete"
	"github.com/c2fo/vfs/v6/utils"
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

func mustReadObject(bucket *storage.BucketHandle, objectName string) []byte {
	objectHandle := bucket.Object(objectName)
	ctx := context.Background()
	reader, err := objectHandle.NewReader(ctx)
	if err != nil {
		panic(err)
	}
	defer func(reader *storage.Reader) {
		err := reader.Close()
		if err != nil {
			panic(err)
		}
	}(reader)
	data, err := io.ReadAll(reader)
	if err != nil {
		panic(err)
	}
	return data
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

func fsMustReadFileName(fs *FileSystem, bucketName, objectName string) []byte {
	file, err := fs.NewFile(bucketName, "/"+objectName)
	if err != nil {
		panic(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		panic(err)
	}
	return data
}

func (ts *fileTestSuite) TestRead() {
	contents := "hello world!"
	bucketName := "bucki"
	objectName := "some/path/file.txt"
	server := fakestorage.NewServer(
		Objects{
			fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      bucketName,
					Name:            objectName,
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte(contents),
			},
		},
	)
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile(bucketName, "/"+objectName)
	ts.Require().NoError(err, "Shouldn't fail creating new file")

	var localFile = bytes.NewBuffer([]byte{})

	buffer := make([]byte, utils.TouchCopyMinBufferSize)
	_, copyErr := io.CopyBuffer(localFile, file, buffer)
	ts.NoError(copyErr, "no error expected")
	closeErr := file.Close()
	ts.NoError(closeErr, "no error expected")

	ts.Equal(localFile.String(), contents, "Copying an gs file to a buffer should fill buffer with file's contents")
}

func (ts *fileTestSuite) TestDelete() {
	contents := "hello world!"
	bucketName := "bucki"
	objectName := "some/path/file.txt"
	server := fakestorage.NewServer(
		Objects{
			fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      bucketName,
					Name:            objectName,
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte(contents),
			},
		},
	)
	defer server.Stop()
	client := server.Client()
	fs := NewFileSystem().WithClient(client)

	file, err := fs.NewFile(bucketName, "/"+objectName)
	ts.Require().NoError(err, "Shouldn't fail creating new file")

	err = file.Delete()
	ts.Require().NoError(err, "Shouldn't fail deleting the file")

	bucket := client.Bucket(bucketName)
	ts.False(objectExists(bucket, objectName))
}

func (ts *fileTestSuite) TestDeleteError() {
	contents := "hello world!"
	bucketName := "bucki"
	objectName := "some/path/file.txt"
	server := fakestorage.NewServer(
		Objects{
			fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      bucketName,
					Name:            objectName,
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte(contents),
			},
		},
	)
	defer server.Stop()
	client := server.Client()
	fs := NewFileSystem().WithClient(client)

	file, err := fs.NewFile(bucketName, "/invalidObject")
	ts.Require().NoError(err, "Shouldn't fail creating new file")

	err = file.Delete()
	ts.Error(err, "Should return an error if gs client had error")
}

func (ts *fileTestSuite) TestDeleteRemoveAllVersions() {
	contents := "hello world!"
	bucketName := "bucki"
	objectName := "some/path/file.txt"
	server := fakestorage.NewServer(
		Objects{
			fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      bucketName,
					Name:            objectName,
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte(contents),
			},
		},
	)
	defer server.Stop()
	client := server.Client()
	fs := NewFileSystem().WithClient(client)

	file, err := fs.NewFile(bucketName, "/"+objectName)
	ts.Require().NoError(err, "Shouldn't fail creating new file")

	f := file.(*File)
	handles, err := f.getObjectGenerationHandles()
	ts.Require().NoError(err, "Shouldn't fail getting object generation handles")
	ts.Len(handles, 1)

	err = file.Delete(delete.WithDeleteAllVersions())
	ts.Require().NoError(err, "Shouldn't fail deleting the file")

	bucket := client.Bucket(bucketName)
	ts.False(objectExists(bucket, objectName))
	handles, err = f.getObjectGenerationHandles()
	ts.Require().NoError(err, "Shouldn't fail getting object generation handles")
	ts.Nil(handles)
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

	ts.Len(contents, count, "Returned count of bytes written should match number of bytes passed to Write.")
	ts.NoError(err, "Error should be nil when calling Write")
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

	server := fakestorage.NewServer(Objects{
		fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName:      bucketName,
				Name:            objectName,
				ContentType:     "text/plain",
				ContentEncoding: "utf8",
			},
			Content: []byte("content"),
		}})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile(bucketName, "/"+objectName)
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	exists, err := file.Exists()
	ts.True(exists, "Should return true for exists based on this setup")
	ts.NoError(err, "Shouldn't return an error when exists is true")
}

func (ts *fileTestSuite) TestNotExists() {
	server := fakestorage.NewServer(Objects{})
	defer server.Stop()
	fs := NewFileSystem().WithClient(server.Client())

	file, err := fs.NewFile("bucket", "/path/hello.txt")
	ts.Require().NoError(err, "Shouldn't fail creating new file.")

	exists, err := file.Exists()
	ts.False(exists, "Should return false for exists based on setup")
	ts.NoError(err, "Error from key not existing should be hidden since it just confirms it doesn't")
}

func (ts *fileTestSuite) TestMoveAndCopy() {
	type TestCase struct {
		move       bool
		readFirst  bool
		sameBucket bool
	}
	type TestCases []TestCase

	testCases := TestCases{}

	for idx := 0; idx <= (1<<3)-1; idx++ {
		testCases = append(testCases, TestCase{
			move:       (idx & (1 << 0)) != 0,
			readFirst:  (idx & (1 << 1)) != 0,
			sameBucket: (idx & (1 << 2)) != 0,
		})
	}

	for _, testCase := range testCases {
		ts.Run(fmt.Sprintf("%#v", testCase), func() {
			sourceName := "source.txt"
			targetName := "target.txt"
			sourceBucketName := "bucket-source"
			var targetBucketName string
			if testCase.sameBucket {
				targetBucketName = sourceBucketName
			} else {
				targetBucketName = "bucket-target"
			}

			content := []byte("content")
			fakeObjects := Objects{
				fakestorage.Object{
					ObjectAttrs: fakestorage.ObjectAttrs{
						BucketName:      sourceBucketName,
						Name:            sourceName,
						ContentType:     "text/plain",
						ContentEncoding: "utf8",
					},
					Content: content,
				}}
			fakeObjects = append(fakeObjects, fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      targetBucketName,
					Name:            "place.holder",
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte{},
			})
			server := fakestorage.NewServer(fakeObjects)
			defer server.Stop()
			client := server.Client()
			fs := NewFileSystem().WithClient(client)
			sourceBucket := client.Bucket(sourceBucketName)
			targetBucket := client.Bucket(targetBucketName)

			ts.True(objectExists(sourceBucket, sourceName), "source should exist")
			ts.True(fsFileNameExists(fs, sourceBucketName, sourceName), "source should exist")
			ts.Equal(content, mustReadObject(sourceBucket, sourceName))
			ts.Equal(content, fsMustReadFileName(fs, sourceBucketName, sourceName))

			ts.False(objectExists(targetBucket, targetName), "target should not exist")
			ts.False(fsFileNameExists(fs, sourceBucketName, targetName), "target should not exist")

			sourceFile, err := fs.NewFile(sourceBucketName, "/"+sourceName)
			ts.NoError(err)
			targetFile, err := fs.NewFile(targetBucketName, "/"+targetName)
			ts.NoError(err)

			if testCase.readFirst {
				_, err := io.ReadAll(sourceFile)
				ts.NoError(err)
			}

			if testCase.move {
				err = sourceFile.MoveToFile(targetFile)
			} else {
				err = sourceFile.CopyToFile(targetFile)
			}

			if testCase.readFirst {
				ts.Error(err, "Error should be returned for operation on file that has been read (i.e. has non 0 cursor position)")
			} else {
				ts.NoError(err, "Error shouldn't be returned from successful operation")

				if testCase.move {
					ts.False(objectExists(sourceBucket, sourceName), "source should not exist")
					ts.False(fsFileNameExists(fs, sourceBucketName, sourceName), "source should not exist")
				} else {
					ts.True(objectExists(sourceBucket, sourceName), "source should exist")
					ts.True(fsFileNameExists(fs, sourceBucketName, sourceName), "source should exist")
					ts.Equal(content, mustReadObject(sourceBucket, sourceName))
					ts.Equal(content, fsMustReadFileName(fs, sourceBucketName, sourceName))
				}

				ts.True(objectExists(targetBucket, targetName), "target should exist")
				ts.True(fsFileNameExists(fs, targetBucketName, targetName), "target should exist")
				ts.Equal(content, mustReadObject(targetBucket, targetName))
				ts.Equal(content, fsMustReadFileName(fs, targetBucketName, targetName))
			}
		})
	}
}

func (ts *fileTestSuite) TestMoveAndCopyBuffered() {
	type TestCase struct {
		move       bool
		readFirst  bool
		sameBucket bool
	}
	type TestCases []TestCase
	testCases := TestCases{}

	for idx := 0; idx <= (1<<3)-1; idx++ {
		testCases = append(testCases, TestCase{
			move:       (idx & (1 << 0)) != 0,
			readFirst:  (idx & (1 << 1)) != 0,
			sameBucket: (idx & (1 << 2)) != 0,
		})
	}

	for _, testCase := range testCases {
		ts.Run(fmt.Sprintf("%#v", testCase), func() {
			sourceName := "source.txt"
			targetName := "target.txt"
			sourceBucketName := "bucket-source"
			var targetBucketName string
			if testCase.sameBucket {
				targetBucketName = sourceBucketName
			} else {
				targetBucketName = "bucket-target"
			}

			content := []byte("content")
			fakeObjects := Objects{
				fakestorage.Object{
					ObjectAttrs: fakestorage.ObjectAttrs{
						BucketName:      sourceBucketName,
						Name:            sourceName,
						ContentType:     "text/plain",
						ContentEncoding: "utf8",
					},
					Content: content,
				}}
			fakeObjects = append(fakeObjects, fakestorage.Object{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName:      targetBucketName,
					Name:            "place.holder",
					ContentType:     "text/plain",
					ContentEncoding: "utf8",
				},
				Content: []byte{},
			})
			server := fakestorage.NewServer(fakeObjects)
			defer server.Stop()
			client := server.Client()
			opts := Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize}
			fs := NewFileSystem().WithOptions(opts).WithClient(client)
			sourceBucket := client.Bucket(sourceBucketName)
			targetBucket := client.Bucket(targetBucketName)

			ts.True(objectExists(sourceBucket, sourceName), "source should exist")
			ts.True(fsFileNameExists(fs, sourceBucketName, sourceName), "source should exist")
			ts.Equal(content, mustReadObject(sourceBucket, sourceName))
			ts.Equal(content, fsMustReadFileName(fs, sourceBucketName, sourceName))

			ts.False(objectExists(targetBucket, targetName), "target should not exist")
			ts.False(fsFileNameExists(fs, sourceBucketName, targetName), "target should not exist")

			sourceFile, err := fs.NewFile(sourceBucketName, "/"+sourceName)
			ts.NoError(err)
			targetFile, err := fs.NewFile(targetBucketName, "/"+targetName)
			ts.NoError(err)

			if testCase.readFirst {
				_, err := io.ReadAll(sourceFile)
				ts.NoError(err)
			}

			if testCase.move {
				err = sourceFile.MoveToFile(targetFile)
			} else {
				err = sourceFile.CopyToFile(targetFile)
			}

			if testCase.readFirst {
				ts.Error(err, "Error should be returned for operation on file that has been read (i.e. has non 0 cursor position)")
			} else {
				ts.NoError(err, "Error shouldn't be returned from successful operation")

				if testCase.move {
					ts.False(objectExists(sourceBucket, sourceName), "source should not exist")
					ts.False(fsFileNameExists(fs, sourceBucketName, sourceName), "source should not exist")
				} else {
					ts.True(objectExists(sourceBucket, sourceName), "source should exist")
					ts.True(fsFileNameExists(fs, sourceBucketName, sourceName), "source should exist")
					ts.Equal(content, mustReadObject(sourceBucket, sourceName))
					ts.Equal(content, fsMustReadFileName(fs, sourceBucketName, sourceName))
				}

				ts.True(objectExists(targetBucket, targetName), "target should exist")
				ts.True(fsFileNameExists(fs, targetBucketName, targetName), "target should exist")
				ts.Equal(content, mustReadObject(targetBucket, targetName))
				ts.Equal(content, fsMustReadFileName(fs, targetBucketName, targetName))
			}
		})
	}
}

func TestFile(t *testing.T) {
	suite.Run(t, new(fileTestSuite))
}
