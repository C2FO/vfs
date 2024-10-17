package azure

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/options/delete"
	"github.com/c2fo/vfs/v6/utils"
)

type FileTestSuite struct {
	suite.Suite
}

func (s *FileTestSuite) TestVFSFileImplementor() {
	f := File{}
	s.Implements((*vfs.File)(nil), &f, "Does not implement the vfs.File interface")
}

func (s *FileTestSuite) TestClose() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)
	f, _ := fs.NewFile("test-container", "/foo.txt")
	s.NoError(f.Close())
}

func (s *FileTestSuite) TestClose_FlushTempFile() {
	client := MockAzureClient{PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)
	f, _ := fs.NewFile("test-container", "/foo.txt")

	_, _ = f.Write([]byte("Hello, World!"))
	s.NoError(f.Close())
}

func (s *FileTestSuite) TestRead() {
	client := MockAzureClient{ExpectedResult: io.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")
	contents := make([]byte, 12)
	n, err := f.Read(contents)
	s.NoError(err)
	s.Equal(12, n)
	s.Equal("Hello World!", string(contents))
}

func (s *FileTestSuite) TestSeek() {
	client := MockAzureClient{ExpectedResult: io.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")
	newOffset, err := f.Seek(6, io.SeekStart)
	s.NoError(err)
	s.Equal(int64(6), newOffset)
	contents := make([]byte, 6)
	n, err := f.Read(contents)
	s.NoError(err)
	s.Equal(6, n)
	s.Equal("World!", string(contents))
}

func (s *FileTestSuite) TestWrite() {
	client := MockAzureClient{ExpectedResult: io.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NotNil(f)
	s.NoError(err)
	n, err := f.Write([]byte(" Aaaaand, Goodbye!"))
	s.NoError(err)
	s.Equal(18, n)
}

func (s *FileTestSuite) TestString() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ := fs.NewLocation("temp", "/foo/bar/")
	f, _ := l.NewFile("blah.txt")
	s.Equal("https://test-account.blob.core.windows.net/temp/foo/bar/blah.txt", f.String())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	l, _ = fs.NewLocation("folder", "/blah/")
	f, _ = l.NewFile("file.txt")
	s.Equal("https://test-account.blob.core.windows.net/folder/blah/file.txt", f.String())
}

func (s *FileTestSuite) TestExists() {
	client := MockAzureClient{PropertiesResult: &BlobProperties{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")
	exists, err := f.Exists()
	s.NoError(err)
	s.True(exists)
}

func (s *FileTestSuite) TestExists_NonExistentFile() {
	client := MockAzureClient{PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	exists, err := f.Exists()
	s.NoError(err, "no error is returned when the file does not exist")
	s.False(exists)
}

func (s *FileTestSuite) TestLocation() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	f, _ := fs.NewFile("test-container", "/file.txt")
	l := f.Location()
	s.NotNil(l)
	s.Equal("https://test-account.blob.core.windows.net/test-container/", l.URI())
}

func (s *FileTestSuite) TestCopyToLocation() {
	fooReader := io.NopCloser(strings.NewReader("blah"))
	client := MockAzureClient{ExpectedResult: fooReader}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	targetLoc, _ := fs.NewLocation("test-container", "/new/folder/")
	copiedFile, err := source.CopyToLocation(targetLoc)
	s.NoError(err)
	s.NotNil(copiedFile)
	s.Equal("/new/folder/foo.txt", copiedFile.Path())
}

func (s *FileTestSuite) TestCopyToFile() {
	fooReader := io.NopCloser(strings.NewReader("blah"))
	client := MockAzureClient{ExpectedResult: fooReader}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	err := source.CopyToFile(target)
	s.NoError(err)
}

func (s *FileTestSuite) TestCopyToFileBuffered() {
	fooReader := io.NopCloser(strings.NewReader("blah"))
	client := MockAzureClient{ExpectedResult: fooReader}
	opts := Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize}
	fs := NewFileSystem().WithOptions(opts).WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	err := source.CopyToFile(target)
	s.NoError(err)
}

func (s *FileTestSuite) TestMoveToLocation() {
	fooReader := io.NopCloser(strings.NewReader("blah"))
	client := MockAzureClient{ExpectedResult: fooReader}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewLocation("test-container", "/new/folder/")

	movedFile, err := source.MoveToLocation(target)
	s.NoError(err)
	s.NotNil(movedFile)
	s.Equal("/new/folder/foo.txt", movedFile.Path())
}

func (s *FileTestSuite) TestMoveToFile() {
	fooReader := io.NopCloser(strings.NewReader("blah"))
	client := MockAzureClient{ExpectedResult: fooReader}
	fs := NewFileSystem().WithClient(&client)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")
	err := source.MoveToFile(target)
	s.NoError(err)
}

func (s *FileTestSuite) TestDelete() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Delete(), "The delete should succeed so there should be no error")
}

func (s *FileTestSuite) TestDeleteWithAllVersionsOption() {
	client := MockAzureClient{}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Delete(delete.WithAllVersions()), "The delete should succeed so there should be no error")
}

func (s *FileTestSuite) TestDeleteWithAllVersionsOption_Error() {
	client := MockAzureClient{ExpectedError: errors.New("i always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	err = f.Delete(delete.WithAllVersions())
	s.Error(err, "If the file does not exist we get an error")
}

func (s *FileTestSuite) TestDelete_NonExistentFile() {
	client := MockAzureClient{ExpectedError: errors.New("i always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	err = f.Delete()
	s.Error(err, "If the file does not exist we get an error")
}

func (s *FileTestSuite) TestLastModified() {
	now := time.Now()
	client := MockAzureClient{PropertiesResult: &BlobProperties{LastModified: &now}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	t, err := f.LastModified()
	s.NoError(err)
	s.NotNil(t)
}

func (s *FileTestSuite) TestSize() {
	client := MockAzureClient{PropertiesResult: &BlobProperties{Size: 5}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	size, err := f.Size()
	s.NoError(err)
	s.Equal(uint64(5), size, "The size should be 5")
}

func (s *FileTestSuite) TestSize_NonExistentFile() {
	client := MockAzureClient{PropertiesError: errors.New("i always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	size, err := f.Size()
	s.Error(err, "If the file does not exist we get an error")
	s.Zero(size, "the file does not exist so the size is 0")
}

func (s *FileTestSuite) TestPath() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-account"})
	f, _ := fs.NewFile("test-container", "/foo/bar/blah.txt")
	s.Equal("/foo/bar/blah.txt", f.Path())

	f, _ = fs.NewFile("test-container", "/test/file.txt")
	s.Equal("/test/file.txt", f.Path())
}

func (s *FileTestSuite) TestName() {
	f := File{}
	s.Equal(".", f.Name())

	f = File{name: "foo.txt"}
	s.Equal("foo.txt", f.Name())
}

func (s *FileTestSuite) TestTouch() {
	client := MockAzureClient{ExpectedResult: &BlobProperties{}, PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Touch())
}

func (s *FileTestSuite) TestTouch_NonexistentContainer() {
	client := MockAzureClient{ExpectedError: errors.New("i always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("nosuchcontainer", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.Error(f.Touch(), "The container does not exist so creating the new file should error")
}

func (s *FileTestSuite) TestURI() {
	fs := NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	f, _ := fs.NewFile("temp", "/foo/bar/blah.txt")
	s.Equal("https://test-container.blob.core.windows.net/temp/foo/bar/blah.txt", f.URI())

	fs = NewFileSystem().WithOptions(Options{AccountName: "test-container"})
	f, _ = fs.NewFile("folder", "/blah/file.txt")
	s.Equal("https://test-container.blob.core.windows.net/folder/blah/file.txt", f.URI())
}

func (s *FileTestSuite) TestCheckTempFile() {
	client := MockAzureClient{ExpectedResult: io.NopCloser(strings.NewReader("Hello World!"))}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	s.True(ok, "Type assertion should succeed so we expect ok to be true")
	s.NotNil(azureFile)

	s.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	err = azureFile.checkTempFile()
	s.NoError(err, "Check temp file should create a local temp file so no error is expected")
	s.NotNil(azureFile.tempFile, "After the call to checkTempFile we should have a non-nil tempFile")

	contents, err := io.ReadAll(azureFile.tempFile)
	s.NoError(err, "No error should occur while reading the tempFile")
	s.Equal("Hello World!", string(contents))
}

func (s *FileTestSuite) TestCheckTempFile_FileDoesNotExist() {
	client := MockAzureClient{PropertiesError: MockStorageError{}}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	s.True(ok, "Type assertion should succeed so we expect ok to be true")
	s.NotNil(azureFile)

	s.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	err = azureFile.checkTempFile()
	s.NoError(err, "Check temp file should create a local temp file so no error is expected")
	s.NotNil(azureFile.tempFile, "After the call to checkTempFile we should have a non-nil tempFile")

	contents, err := io.ReadAll(azureFile.tempFile)
	s.NoError(err, "No error should occur while reading the tempFile")
	s.Empty(contents)
}

func (s *FileTestSuite) TestCheckTempFile_DownloadError() {
	client := MockAzureClient{ExpectedError: errors.New("i always error")}
	fs := NewFileSystem().WithClient(&client)

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	s.True(ok, "Type assertion should succeed so we expect ok to be true")
	s.NotNil(azureFile)

	s.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	err = azureFile.checkTempFile()
	s.Error(err, "The call to client.Download() errors so we expect to get an error")
}

func (s *FileTestSuite) TestIsSameAuth_SameAcctKey() {
	fs := NewFileSystem().WithOptions(Options{
		AccountName: "foo",
		AccountKey:  "bar",
	})

	f1, _ := fs.NewFile("test-container", "/foo.txt")
	f2, _ := fs.NewFile("test-container", "/bar.txt")
	sourceFile := f1.(*File)
	targetFile := f2.(*File)

	s.True(sourceFile.isSameAuth(targetFile), "Files were created with the same options so same auth should be true")
}

func (s *FileTestSuite) TestIsSameAuth_DifferentAcctKey() {
	sourceFs := NewFileSystem().WithOptions(Options{
		AccountName: "foo",
		AccountKey:  "bar",
	})

	targetFs := NewFileSystem().WithOptions(Options{
		AccountName: "blah",
		AccountKey:  "blahblah",
	})

	src, _ := sourceFs.NewFile("test-container", "/foo.txt")
	tgt, _ := targetFs.NewFile("test-container", "/bar.txt")
	sourceFile := src.(*File)
	targetFile := tgt.(*File)

	s.False(sourceFile.isSameAuth(targetFile), "Files were created with different account keys so same auth should be false")
}

func TestAzureFile(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
