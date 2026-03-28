package azure

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/azure/mocks"
	"github.com/c2fo/vfs/v7/options/delete"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/utils"
)

var errBlobNotFound = &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}

type FileTestSuite struct {
	suite.Suite
}

func (s *FileTestSuite) TestVFSFileImplementor() {
	f := File{}
	s.Implements((*vfs.File)(nil), &f, "Does not implement the vfs.File interface")
}

func (s *FileTestSuite) TestClose() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	f, _ := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(f.Close())
}

func (s *FileTestSuite) TestClose_FlushTempFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	f, _ := fs.NewFile("test-container", "/foo.txt")

	client.EXPECT().Properties("test-container", "/foo.txt").Return(nil, errBlobNotFound)
	client.EXPECT().Upload(mock.Anything, mock.Anything, "").Return(nil)
	_, err := f.Write([]byte("Hello, World!"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())
}

func (s *FileTestSuite) TestRead() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The file should exist so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("Hello World!")), nil)
	contents := make([]byte, 12)
	n, err := f.Read(contents)
	s.Require().NoError(err)
	s.Equal(12, n)
	s.Equal("Hello World!", string(contents))
}

func (s *FileTestSuite) TestSeek() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The file should exist so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("Hello World!")), nil)
	newOffset, err := f.Seek(6, io.SeekStart)
	s.Require().NoError(err)
	s.Equal(int64(6), newOffset)
	contents := make([]byte, 6)
	n, err := f.Read(contents)
	s.Require().NoError(err)
	s.Equal(6, n)
	s.Equal("World!", string(contents))
}

func (s *FileTestSuite) TestWrite() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NotNil(f)
	s.Require().NoError(err)
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	n, err := f.Write([]byte(" Aaaaand, Goodbye!"))
	s.Require().NoError(err)
	s.Equal(18, n)
}

func (s *FileTestSuite) TestString() {
	fs := NewFileSystem()
	l, _ := fs.NewLocation("temp", "/foo/bar/")
	f, _ := l.NewFile("blah.txt")
	s.Equal("az://temp/foo/bar/blah.txt", f.String())

	fs = NewFileSystem()
	l, _ = fs.NewLocation("folder", "/blah/")
	f, _ = l.NewFile("file.txt")
	s.Equal("az://folder/blah/file.txt", f.String())
}

func (s *FileTestSuite) TestExists() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The file should exist so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	exists, err := f.Exists()
	s.Require().NoError(err)
	s.True(exists)
}

func (s *FileTestSuite) TestExists_NonExistentFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(nil, errBlobNotFound)
	exists, err := f.Exists()
	s.Require().NoError(err, "no error is returned when the file does not exist")
	s.False(exists)
}

func (s *FileTestSuite) TestCloseWithContentType() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	f, _ := fs.NewFile("test-container", "/foo.txt", newfile.WithContentType("text/plain"))
	client.EXPECT().Properties("test-container", "/foo.txt").Return(nil, errBlobNotFound)
	client.EXPECT().Upload(mock.Anything, mock.Anything, "text/plain").Return(nil)
	_, _ = f.Write([]byte("Hello, World!"))
	s.Require().NoError(f.Close())
}

func (s *FileTestSuite) TestLocation() {
	fs := NewFileSystem()
	f, _ := fs.NewFile("test-container", "/file.txt")
	l := f.Location()
	s.NotNil(l)
	s.Equal("az://test-container/", l.URI())
}

func (s *FileTestSuite) TestCopyToLocation() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	source, _ := fs.NewFile("test-container", "/foo.txt")
	targetLoc, _ := fs.NewLocation("test-container", "/new/folder/")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("blah")), nil)
	client.EXPECT().Copy(mock.Anything, mock.Anything).Return(nil)
	copiedFile, err := source.CopyToLocation(targetLoc)
	s.Require().NoError(err)
	s.NotNil(copiedFile)
	s.Equal("/new/folder/foo.txt", copiedFile.Path())
}

func (s *FileTestSuite) TestCopyToFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("blah")), nil)
	client.EXPECT().Copy(mock.Anything, mock.Anything).Return(nil)
	err := source.CopyToFile(target)
	s.Require().NoError(err)
}

func (s *FileTestSuite) TestCopyToFileBuffered() {
	client := mocks.NewClient(s.T())
	opts := Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize}
	fs := NewFileSystem(WithOptions(opts), WithClient(client))
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("blah")), nil)
	client.EXPECT().Copy(mock.Anything, mock.Anything).Return(nil)
	err := source.CopyToFile(target)
	s.Require().NoError(err)
}

func (s *FileTestSuite) TestMoveToLocation() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewLocation("test-container", "/new/folder/")

	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("blah")), nil)
	client.EXPECT().Copy(mock.Anything, mock.Anything).Return(nil)
	client.EXPECT().Delete(mock.Anything).Return(nil)
	movedFile, err := source.MoveToLocation(target)
	s.Require().NoError(err)
	s.NotNil(movedFile)
	s.Equal("/new/folder/foo.txt", movedFile.Path())
}

func (s *FileTestSuite) TestMoveToFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("blah")), nil)
	client.EXPECT().Copy(mock.Anything, mock.Anything).Return(nil)
	client.EXPECT().Delete(mock.Anything).Return(nil)
	err := source.MoveToFile(target)
	s.Require().NoError(err)
}

func (s *FileTestSuite) TestDelete() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Delete(mock.Anything).Return(nil)
	s.Require().NoError(f.Delete(), "The delete should succeed so there should be no error")
}

func (s *FileTestSuite) TestDeleteWithAllVersionsOption() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Delete(mock.Anything).Return(nil)
	client.EXPECT().DeleteAllVersions(mock.Anything).Return(nil)
	s.Require().NoError(f.Delete(delete.WithAllVersions()), "The delete should succeed so there should be no error")
}

func (s *FileTestSuite) TestDeleteWithAllVersionsOption_Error() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Delete(mock.Anything).Return(errors.New("i always error"))
	err = f.Delete(delete.WithAllVersions())
	s.Require().Error(err, "If the file does not exist we get an error")
}

func (s *FileTestSuite) TestDelete_NonExistentFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Delete(mock.Anything).Return(errors.New("i always error"))
	err = f.Delete()
	s.Require().Error(err, "If the file does not exist we get an error")
}

func (s *FileTestSuite) TestLastModified() {
	now := time.Now()
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{LastModified: &now}, nil)
	t, err := f.LastModified()
	s.Require().NoError(err)
	s.NotNil(t)
}

func (s *FileTestSuite) TestSize() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{Size: to.Ptr[int64](5)}, nil)
	size, err := f.Size()
	s.Require().NoError(err)
	s.Equal(uint64(5), size, "The size should be 5")
}

func (s *FileTestSuite) TestSize_NonExistentFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(nil, errors.New("i always error"))
	size, err := f.Size()
	s.Require().Error(err, "If the file does not exist we get an error")
	s.Zero(size, "the file does not exist so the size is 0")
}

func (s *FileTestSuite) TestPath() {
	fs := NewFileSystem()
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
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().SetMetadata(mock.Anything, mock.Anything).Return(nil)
	s.Require().NoError(f.Touch())
}

func (s *FileTestSuite) TestTouch_NonExistentContainer() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("nosuchcontainer", "/foo.txt")
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("nosuchcontainer", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().SetMetadata(mock.Anything, mock.Anything).Return(errors.New("i always error"))
	s.Require().Error(f.Touch(), "The container does not exist so creating the new file should error")
}

func (s *FileTestSuite) TestTouchWithContentType() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt", newfile.WithContentType("text/plain"))
	s.Require().NoError(err, "The path is valid so no error should be returned")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(nil, errBlobNotFound)
	client.EXPECT().Upload(mock.Anything, mock.Anything, "text/plain").Return(nil)
	s.Require().NoError(f.Touch())
}

func (s *FileTestSuite) TestURI() {
	fs := NewFileSystem()
	f, _ := fs.NewFile("temp", "/foo/bar/blah.txt")
	s.Equal("az://temp/foo/bar/blah.txt", f.URI())

	fs = NewFileSystem()
	f, _ = fs.NewFile("folder", "/blah/file.txt")
	s.Equal("az://folder/blah/file.txt", f.URI())
}

func (s *FileTestSuite) TestCheckTempFile() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	s.True(ok, "Type assertion should succeed so we expect ok to be true")
	s.NotNil(azureFile)

	s.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(io.NopCloser(strings.NewReader("Hello World!")), nil)
	err = azureFile.checkTempFile(false)
	s.Require().NoError(err, "Check temp file should create a local temp file so no error is expected")
	s.NotNil(azureFile.tempFile, "After the call to checkTempFile we should have a non-nil tempFile")

	contents, err := io.ReadAll(azureFile.tempFile)
	s.Require().NoError(err, "No error should occur while reading the tempFile")
	s.Equal("Hello World!", string(contents))
}

func (s *FileTestSuite) TestCheckTempFile_FileDoesNotExist() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	s.True(ok, "Type assertion should succeed so we expect ok to be true")
	s.NotNil(azureFile)

	s.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(nil, errBlobNotFound)
	err = azureFile.checkTempFile(true)
	s.Require().NoError(err, "Check temp file should create a local temp file so no error is expected")
	s.NotNil(azureFile.tempFile, "After the call to checkTempFile we should have a non-nil tempFile")

	contents, err := io.ReadAll(azureFile.tempFile)
	s.Require().NoError(err, "No error should occur while reading the tempFile")
	s.Empty(contents)
}

func (s *FileTestSuite) TestCheckTempFile_DownloadError() {
	client := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(client))

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.Require().NoError(err, "The file should exist so no error should be returned")

	azureFile, ok := f.(*File)
	s.True(ok, "Type assertion should succeed so we expect ok to be true")
	s.NotNil(azureFile)

	s.Nil(azureFile.tempFile, "No calls to checkTempFile have occurred so we expect tempFile to be nil")
	client.EXPECT().Properties("test-container", "/foo.txt").Return(&BlobProperties{}, nil)
	client.EXPECT().Download(mock.Anything).Return(nil, errors.New("i always error"))
	err = azureFile.checkTempFile(false)
	s.Require().Error(err, "The call to client.Download() errors so we expect to get an error")
}

func (s *FileTestSuite) TestIsSameAuth_SameAcctKey() {
	fs := NewFileSystem(WithOptions(Options{
		AccountName: "foo",
		AccountKey:  "bar",
	}))

	f1, _ := fs.NewFile("test-container", "/foo.txt")
	f2, _ := fs.NewFile("test-container", "/bar.txt")
	sourceFile := f1.(*File)
	targetFile := f2.(*File)

	s.True(sourceFile.isSameAuth(targetFile), "Files were created with the same options so same auth should be true")
}

func (s *FileTestSuite) TestIsSameAuth_DifferentAcctKey() {
	sourceFs := NewFileSystem(WithOptions(Options{
		AccountName: "foo",
		AccountKey:  "bar",
	}))

	targetFs := NewFileSystem(WithOptions(Options{
		AccountName: "blah",
		AccountKey:  "blahblah",
	}))

	src, _ := sourceFs.NewFile("test-container", "/foo.txt")
	tgt, _ := targetFs.NewFile("test-container", "/bar.txt")
	sourceFile := src.(*File)
	targetFile := tgt.(*File)

	s.False(sourceFile.isSameAuth(targetFile), "Files were created with different account keys so same auth should be false")
}

func TestAzureFile(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
