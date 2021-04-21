package azure

import (
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/c2fo/vfs/v5"
	"github.com/stretchr/testify/suite"
)

type FileTestSuite struct {
	suite.Suite
}

func (suite *FileTestSuite) TestVFSFileImplementor() {
	f := File{}
	suite.Implements((*vfs.File)(nil), &f, "Does not implement the vfs.File interface")
}

func (suite *FileTestSuite) TestClose() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	f, _ := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(f.Close())
}

func (suite *FileTestSuite) TestClose_FlushTempFile() {
	client := MockAzureClient{PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)
	f, _ := fs.NewFile("test-container", "/foo.txt")

	_, _ = f.Write([]byte("Hello, World!"))
	suite.NoError(f.Close())
}

func (suite *FileTestSuite) TestRead() {
	client := MockAzureClient{ExpectedResult: ioutil.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The file should exist so no error should be returned")
	contents := make([]byte, 12)
	n, err := f.Read(contents)
	suite.Equal(12, n)
	suite.Equal("Hello World!", string(contents))
}

func (suite *FileTestSuite) TestSeek() {
	client := MockAzureClient{ExpectedResult: ioutil.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The file should exist so no error should be returned")
	newOffset, err := f.Seek(6, io.SeekStart)
	suite.NoError(err)
	suite.Equal(int64(6), newOffset)
	contents := make([]byte, 6)
	n, err := f.Read(contents)
	suite.Equal(6, n)
	suite.Equal("World!", string(contents))
}

func (suite *FileTestSuite) TestWrite() {
	client := MockAzureClient{ExpectedResult: ioutil.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NotNil(f)
	suite.NoError(err)
	n, err := f.Write([]byte(" Aaaaand, Goodbye!"))
	suite.NoError(err)
	suite.Equal(18, n)
}

func (suite *FileTestSuite) TestString() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("temp", "/foo/bar/")
	f, _ := l.NewFile("blah.txt")
	suite.Equal("https://test-account.blob.core.windows.net/temp/foo/bar/blah.txt", f.String())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ = fs.NewLocation("folder", "/blah/")
	f, _ = l.NewFile("file.txt")
	suite.Equal("https://test-account.blob.core.windows.net/folder/blah/file.txt", f.String())
}

func (suite *FileTestSuite) TestExists() {
	client := MockAzureClient{PropertiesResult: &BlobProperties{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The file should exist so no error should be returned")
	exists, err := f.Exists()
	suite.NoError(err)
	suite.True(exists)
}

func (suite *FileTestSuite) TestExists_NonExistantFile() {
	client := MockAzureClient{PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	exists, err := f.Exists()
	suite.NoError(err, "no error is returned when  the file does not exist")
	suite.False(exists)
}

func (suite *FileTestSuite) TestLocation() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	f, _ := fs.NewFile("test-container", "/file.txt")
	l := f.Location()
	suite.NotNil(l)
	suite.Equal("https://test-account.blob.core.windows.net/test-container/", l.URI())
}

func (suite *FileTestSuite) TestCopyToLocation() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	targetLoc, _ := fs.NewLocation("test-container", "/new/folder/")
	copiedFile, err := source.CopyToLocation(targetLoc)
	suite.NoError(err)
	suite.NotNil(copiedFile)
	suite.Equal("/new/folder/foo.txt", copiedFile.Path())
}

func (suite *FileTestSuite) TestCopyToFile() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	err := source.CopyToFile(target)
	suite.NoError(err)
}

func (suite *FileTestSuite) TestMoveToLocation() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewLocation("test-container", "/new/folder/")

	movedFile, err := source.MoveToLocation(target)
	suite.NoError(err)
	suite.NotNil(movedFile)
	suite.Equal("/new/folder/foo.txt", movedFile.Path())
}

func (suite *FileTestSuite) TestMoveToFile() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")
	err := source.MoveToFile(target)
	suite.NoError(err)
}

func (suite *FileTestSuite) TestDelete() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	suite.NoError(f.Delete(), "The delete should succeed so there should be no error")
}

func (suite *FileTestSuite) TestDelete_NonExistantFile() {
	client := MockAzureClient{ExpectedError: errors.New("I always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	err = f.Delete()
	suite.Error(err, "If the file does not exist we get an error")
}

func (suite *FileTestSuite) TestLastModified() {
	now := time.Now()
	client := MockAzureClient{PropertiesResult: &BlobProperties{LastModified: &now}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	t, err := f.LastModified()
	suite.NoError(err)
	suite.NotNil(t)
}

func (suite *FileTestSuite) TestSize() {
	client := MockAzureClient{PropertiesResult: &BlobProperties{Size: 5}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	size, err := f.Size()
	suite.NoError(err)
	suite.Equal(uint64(5), size, "The size should be 5")
}

func (suite *FileTestSuite) TestSize_NonExistantFile() {
	client := MockAzureClient{PropertiesError: errors.New("I always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	size, err := f.Size()
	suite.Error(err, "If the file does not exist we get an error")
	suite.Equal(uint64(0), size, "the file does not exist so the size is 0")
}

func (suite *FileTestSuite) TestPath() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	f, _ := fs.NewFile("test-container", "/foo/bar/blah.txt")
	suite.Equal("/foo/bar/blah.txt", f.Path())

	f, _ = fs.NewFile("test-container", "/test/file.txt")
	suite.Equal("/test/file.txt", f.Path())
}

func (suite *FileTestSuite) TestName() {
	f := File{}
	suite.Equal(".", f.Name())

	f = File{name: "foo.txt"}
	suite.Equal("foo.txt", f.Name())
}

func (suite *FileTestSuite) TestTouch() {
	client := MockAzureClient{ExpectedResult: &BlobProperties{}, PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	suite.NoError(f.Touch())
}

func (suite *FileTestSuite) TestTouch_NonexistantContainer() {
	client := MockAzureClient{ExpectedError: errors.New("I always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("nosuchcontainer", "/foo.txt")
	suite.NoError(err, "The path is valid so no error should be returned")
	suite.Error(f.Touch(), "The container does not exist so creating the new file should error")
}

func (suite *FileTestSuite) TestURI() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	f, _ := fs.NewFile("temp", "/foo/bar/blah.txt")
	suite.Equal("https://test-container.blob.core.windows.net/temp/foo/bar/blah.txt", f.URI())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	f, _ = fs.NewFile("folder", "/blah/file.txt")
	suite.Equal("https://test-container.blob.core.windows.net/folder/blah/file.txt", f.URI())
}

func (suite *FileTestSuite) TestCheckTempFile() {
	client := MockAzureClient{ExpectedResult: ioutil.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	suite.True(ok, "Type assertion should succeed so we expect ok to be true")
	suite.NotNil(azureFile)

	suite.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	err = azureFile.checkTempFile()
	suite.NoError(err, "Check temp file should create a local temp file so no error is expected")
	suite.NotNil(azureFile.tempFile, "After the call to checkTempFile we should have a non-nil tempFile")

	contents, err := ioutil.ReadAll(azureFile.tempFile)
	suite.NoError(err, "No error should occur while reading the tempFile")
	suite.Equal("Hello World!", string(contents))
}

func (suite *FileTestSuite) TestCheckTempFile_FileDoesNotExist() {
	client := MockAzureClient{PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	suite.True(ok, "Type assertion should succeed so we expect ok to be true")
	suite.NotNil(azureFile)

	suite.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	err = azureFile.checkTempFile()
	suite.NoError(err, "Check temp file should create a local temp file so no error is expected")
	suite.NotNil(azureFile.tempFile, "After the call to checkTempFile we should have a non-nil tempFile")

	contents, err := ioutil.ReadAll(azureFile.tempFile)
	suite.NoError(err, "No error should occur while reading the tempFile")
	suite.Equal("", string(contents))
}

func (suite *FileTestSuite) TestCheckTempFile_DownloadError() {
	client := MockAzureClient{ExpectedError: errors.New("I always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	suite.NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	suite.True(ok, "Type assertion should succeed so we expect ok to be true")
	suite.NotNil(azureFile)

	suite.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	err = azureFile.checkTempFile()
	suite.Error(err, "The call to client.Download() errors so we expect to get an error")
}

func (suite *FileTestSuite) TestIsSameAuth_SameAcctKey() {
	fs := NewFileSystem().WithOptions(Options{
		AccountName: "foo",
		AccountKey:  "bar",
	})

	f1, _ := fs.NewFile("test-container", "/foo.txt")
	f2, _ := fs.NewFile("test-container", "/bar.txt")
	sourceFile := f1.(*File)
	targetFile := f2.(*File)

	suite.True(sourceFile.isSameAuth(targetFile), "Files were created with the same options so same auth should be true")
}

func (suite *FileTestSuite) TestIsSameAuth_DifferentAcctKey() {
	sourceFs := NewFileSystem().WithOptions(Options{
		AccountName: "foo",
		AccountKey:  "bar",
	})

	targetFs := NewFileSystem().WithOptions(Options{
		AccountName: "blah",
		AccountKey:  "blahblah",
	})

	s, _ := sourceFs.NewFile("test-container", "/foo.txt")
	t, _ := targetFs.NewFile("test-container", "/bar.txt")
	sourceFile := s.(*File)
	targetFile := t.(*File)

	suite.False(sourceFile.isSameAuth(targetFile), "Files were created with different account keys so same auth should be false")
}

func TestAzureFile(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
