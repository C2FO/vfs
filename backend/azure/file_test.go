package azure

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/azure/mocks"
	"github.com/c2fo/vfs/v6/options/delete"
	"github.com/c2fo/vfs/v6/options/newfile"
	"github.com/c2fo/vfs/v6/utils"
)

type FileTestSuite struct {
	suite.Suite
	containerCli *mocks.ContainerClient
	blockBlobCli *mocks.BlockBlobClient
}

func (s *FileTestSuite) SetupTest() {
	s.containerCli = mocks.NewContainerClient(s.T())
	containerClientFactory = func(_ *FileSystem, containerName string) (ContainerClient, error) {
		s.Require().Equal("test-container", containerName)
		return s.containerCli, nil
	}
	s.blockBlobCli = mocks.NewBlockBlobClient(s.T())
	blockBlobClientFactory = func(_ *FileSystem, containerName, path string, versionID *string) (BlockBlobClient, error) {
		s.Require().Equal("test-container", containerName)
		s.Require().NotEmpty(path)
		return s.blockBlobCli, nil
	}
}

func (s *FileTestSuite) TestVFSFileImplementor() {
	f := File{}
	s.Implements((*vfs.File)(nil), &f, "Does not implement the vfs.File interface")
}

func (s *FileTestSuite) TestClose() {
	fs := NewFileSystem()
	f, _ := fs.NewFile("test-container", "/foo.txt")
	s.NoError(f.Close())
}

func (s *FileTestSuite) TestClose_FlushTempFile() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.expectUpload("Hello, World!", "")
	fs := NewFileSystem()
	f, _ := fs.NewFile("test-container", "/foo.txt")

	_, err := f.Write([]byte("Hello, World!"))
	s.Require().NoError(err)
	s.NoError(f.Close())
}

func (s *FileTestSuite) TestRead() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, nil).
		Once()
	s.blockBlobCli.EXPECT().DownloadStream(context.Background(), (*blob.DownloadStreamOptions)(nil)).
		Return(blob.DownloadStreamResponse{DownloadResponse: blob.DownloadResponse{Body: io.NopCloser(strings.NewReader("Hello World!"))}}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")
	contents := make([]byte, 12)
	n, err := f.Read(contents)
	s.NoError(err)
	s.Equal(12, n)
	s.Equal("Hello World!", string(contents))
}

func (s *FileTestSuite) TestSeek() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, nil).
		Once()
	s.blockBlobCli.EXPECT().DownloadStream(context.Background(), (*blob.DownloadStreamOptions)(nil)).
		Return(blob.DownloadStreamResponse{DownloadResponse: blob.DownloadResponse{Body: io.NopCloser(strings.NewReader("Hello World!"))}}, nil).
		Once()
	fs := NewFileSystem()

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
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, nil).
		Once()
	s.blockBlobCli.EXPECT().DownloadStream(context.Background(), (*blob.DownloadStreamOptions)(nil)).
		Return(blob.DownloadStreamResponse{DownloadResponse: blob.DownloadResponse{Body: io.NopCloser(strings.NewReader("Hello World!"))}}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NotNil(f)
	s.NoError(err)
	n, err := f.Write([]byte(" Aaaaand, Goodbye!"))
	s.NoError(err)
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
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The file should exist so no error should be returned")
	exists, err := f.Exists()
	s.NoError(err)
	s.True(exists)
}

func (s *FileTestSuite) TestExists_NonExistentFile() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	exists, err := f.Exists()
	s.NoError(err, "no error is returned when the file does not exist")
	s.False(exists)
}

func (s *FileTestSuite) TestCloseWithContentType() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.expectUpload("Hello, World!", "text/plain")
	fs := NewFileSystem()
	f, _ := fs.NewFile("test-container", "/foo.txt", newfile.WithContentType("text/plain"))
	_, _ = f.Write([]byte("Hello, World!"))
	s.NoError(f.Close())
}

func (s *FileTestSuite) TestLocation() {
	fs := NewFileSystem()
	f, _ := fs.NewFile("test-container", "/file.txt")
	l := f.Location()
	s.NotNil(l)
	s.Equal("az://test-container/", l.URI())
}

func (s *FileTestSuite) TestCopyToLocation() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.blockBlobCli.EXPECT().
		StartCopyFromURL(context.Background(), "https://blob.core.windows.net/test-container/foo.txt", (*blob.StartCopyFromURLOptions)(nil)).
		Return(blob.StartCopyFromURLResponse{CopyStatus: to.Ptr(blob.CopyStatusTypeSuccess)}, nil).
		Once()
	fs := NewFileSystem()
	source, _ := fs.NewFile("test-container", "/foo.txt")
	targetLoc, _ := fs.NewLocation("test-container", "/new/folder/")
	copiedFile, err := source.CopyToLocation(targetLoc)
	s.NoError(err)
	s.NotNil(copiedFile)
	s.Equal("/new/folder/foo.txt", copiedFile.Path())
}

func (s *FileTestSuite) TestCopyToFile() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.blockBlobCli.EXPECT().
		StartCopyFromURL(context.Background(), "https://blob.core.windows.net/test-container/foo.txt", (*blob.StartCopyFromURLOptions)(nil)).
		Return(blob.StartCopyFromURLResponse{CopyStatus: to.Ptr(blob.CopyStatusTypeSuccess)}, nil).
		Once()
	fs := NewFileSystem()
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	err := source.CopyToFile(target)
	s.NoError(err)
}

func (s *FileTestSuite) TestCopyToFileBuffered() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.blockBlobCli.EXPECT().
		StartCopyFromURL(context.Background(), "https://blob.core.windows.net/test-container/foo.txt", (*blob.StartCopyFromURLOptions)(nil)).
		Return(blob.StartCopyFromURLResponse{CopyStatus: to.Ptr(blob.CopyStatusTypeSuccess)}, nil).
		Once()
	opts := Options{FileBufferSize: 2 * utils.TouchCopyMinBufferSize}
	fs := NewFileSystem().WithOptions(opts)
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")

	err := source.CopyToFile(target)
	s.NoError(err)
}

func (s *FileTestSuite) TestMoveToLocation() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.blockBlobCli.EXPECT().
		StartCopyFromURL(context.Background(), "https://blob.core.windows.net/test-container/foo.txt", (*blob.StartCopyFromURLOptions)(nil)).
		Return(blob.StartCopyFromURLResponse{CopyStatus: to.Ptr(blob.CopyStatusTypeSuccess)}, nil).
		Once()
	s.blockBlobCli.EXPECT().Delete(context.Background(), (*blob.DeleteOptions)(nil)).
		Return(blob.DeleteResponse{}, nil).
		Once()
	fs := NewFileSystem()
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewLocation("test-container", "/new/folder/")

	movedFile, err := source.MoveToLocation(target)
	s.NoError(err)
	s.NotNil(movedFile)
	s.Equal("/new/folder/foo.txt", movedFile.Path())
}

func (s *FileTestSuite) TestMoveToFile() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.blockBlobCli.EXPECT().
		StartCopyFromURL(context.Background(), "https://blob.core.windows.net/test-container/foo.txt", (*blob.StartCopyFromURLOptions)(nil)).
		Return(blob.StartCopyFromURLResponse{CopyStatus: to.Ptr(blob.CopyStatusTypeSuccess)}, nil).
		Once()
	s.blockBlobCli.EXPECT().Delete(context.Background(), (*blob.DeleteOptions)(nil)).
		Return(blob.DeleteResponse{}, nil).
		Once()
	fs := NewFileSystem()
	source, _ := fs.NewFile("test-container", "/foo.txt")
	target, _ := fs.NewFile("test-container", "/bar.txt")
	err := source.MoveToFile(target)
	s.NoError(err)
}

func (s *FileTestSuite) TestDelete() {
	s.blockBlobCli.EXPECT().Delete(context.Background(), (*blob.DeleteOptions)(nil)).
		Return(blob.DeleteResponse{}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Delete(), "The delete should succeed so there should be no error")
}

func (s *FileTestSuite) TestDeleteWithAllVersionsOption() {
	s.expectNewListBlobsFlatPager()
	s.blockBlobCli.EXPECT().Delete(context.Background(), (*blob.DeleteOptions)(nil)).
		Return(blob.DeleteResponse{}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Delete(delete.WithAllVersions()), "The delete should succeed so there should be no error")
}

func (s *FileTestSuite) TestDeleteWithAllVersionsOption_Error() {
	s.expectNewListBlobsFlatPager()
	s.blockBlobCli.EXPECT().Delete(context.Background(), (*blob.DeleteOptions)(nil)).
		Return(blob.DeleteResponse{}, errors.New("i always error")).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	err = f.Delete(delete.WithAllVersions())
	s.Error(err, "If the file does not exist we get an error")
}

func (s *FileTestSuite) expectNewListBlobsFlatPager() {
	s.containerCli.EXPECT().NewListBlobsFlatPager(
		&container.ListBlobsFlatOptions{
			Prefix:  to.Ptr("foo.txt"),
			Include: container.ListBlobsInclude{Versions: true},
		}).
		Return(runtime.NewPager[container.ListBlobsFlatResponse](runtime.PagingHandler[container.ListBlobsFlatResponse]{
			More: func(container.ListBlobsFlatResponse) bool { return false },
			Fetcher: func(context.Context, *container.ListBlobsFlatResponse) (container.ListBlobsFlatResponse, error) {
				return container.ListBlobsFlatResponse{
					ListBlobsFlatSegmentResponse: container.ListBlobsFlatSegmentResponse{
						Segment: &container.BlobFlatListSegment{BlobItems: []*container.BlobItem{{VersionID: to.Ptr("abc123")}}},
					},
				}, nil
			},
		})).
		Once()
}

func (s *FileTestSuite) TestDelete_NonExistentFile() {
	s.blockBlobCli.EXPECT().Delete(context.Background(), (*blob.DeleteOptions)(nil)).
		Return(blob.DeleteResponse{}, errors.New("i always error")).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	err = f.Delete()
	s.Error(err, "If the file does not exist we get an error")
}

func (s *FileTestSuite) TestLastModified() {
	now := time.Now()
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{LastModified: to.Ptr(now)}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	t, err := f.LastModified()
	s.NoError(err)
	s.Equal(to.Ptr(now), t)
}

func (s *FileTestSuite) TestSize() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{ContentLength: to.Ptr[int64](5)}, nil).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	size, err := f.Size()
	s.NoError(err)
	s.Equal(uint64(5), size, "The size should be 5")
}

func (s *FileTestSuite) TestSize_NonExistentFile() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, errors.New("i always error")).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	size, err := f.Size()
	s.Error(err, "If the file does not exist we get an error")
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
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.expectUpload("", "")
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Touch())
}

func (s *FileTestSuite) TestTouch_NonexistentContainer() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.blockBlobCli.EXPECT().Upload(context.Background(), mock.Anything, (*blockblob.UploadOptions)(nil)).
		Return(blockblob.UploadResponse{}, errors.New("i always error")).
		Once()
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt")
	s.NoError(err, "The path is valid so no error should be returned")
	s.Error(f.Touch(), "The container does not exist so creating the new file should error")
}

func (s *FileTestSuite) TestTouchWithContentType() {
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	s.expectUpload("", "text/plain")
	fs := NewFileSystem()

	f, err := fs.NewFile("test-container", "/foo.txt", newfile.WithContentType("text/plain"))
	s.NoError(err, "The path is valid so no error should be returned")
	s.NoError(f.Touch())
}

func (s *FileTestSuite) expectUpload(content, contentType string) {
	var opts *blockblob.UploadOptions
	if contentType != "" {
		opts = &blockblob.UploadOptions{HTTPHeaders: &blob.HTTPHeaders{BlobContentType: to.Ptr("text/plain")}}
	}
	s.blockBlobCli.EXPECT().Upload(context.Background(), mock.Anything, opts).
		Run(func(ctx context.Context, body io.ReadSeekCloser, options *blockblob.UploadOptions) {
			b, err := io.ReadAll(body)
			s.Require().NoError(err)
			s.Require().Equal(content, string(b))
		}).
		Return(blockblob.UploadResponse{}, nil).
		Once()
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
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, nil).
		Once()
	s.blockBlobCli.EXPECT().DownloadStream(context.Background(), (*blob.DownloadStreamOptions)(nil)).
		Return(blob.DownloadStreamResponse{DownloadResponse: blob.DownloadResponse{Body: io.NopCloser(strings.NewReader("Hello World!"))}}, nil).
		Once()
	fs := NewFileSystem()

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
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, &azcore.ResponseError{ErrorCode: string(bloberror.BlobNotFound)}).
		Once()
	fs := NewFileSystem()

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
	s.blockBlobCli.EXPECT().GetProperties(context.Background(), (*blob.GetPropertiesOptions)(nil)).
		Return(blob.GetPropertiesResponse{}, nil).
		Once()
	s.blockBlobCli.EXPECT().DownloadStream(context.Background(), (*blob.DownloadStreamOptions)(nil)).
		Return(blob.DownloadStreamResponse{}, errors.New("i always error")).
		Once()
	fs := NewFileSystem()

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
