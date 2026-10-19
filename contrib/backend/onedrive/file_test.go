package onedrive

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"

	"github.com/c2fo/vfs/contrib/backend/onedrive/mocks"
)

type FileTestSuite struct {
	suite.Suite
	client  *mocks.Client
	fs      *FileSystem
	file    *File
	tempDir string
}

func TestFileTestSuite(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}

func (s *FileTestSuite) SetupTest() {
	s.client = mocks.NewClient(s.T())
	s.tempDir = s.T().TempDir()
	s.fs = NewFileSystem(WithClient(s.client), WithTempDir(s.tempDir))
	s.file = s.newFile("/dir/file.txt")
}

func (s *FileTestSuite) TearDownTest() {
	entries, err := os.ReadDir(s.tempDir)
	s.Require().NoError(err)
	s.Empty(entries, "temp files should be cleaned up")
}

func (s *FileTestSuite) newFile(p string) *File {
	f, err := s.fs.NewFile("", p)
	s.Require().NoError(err)
	return f.(*File)
}

func body(content string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(content))
}

// withContent matches an upload body whose full content equals expected.
func withContent(expected string) any {
	return mock.MatchedBy(func(r io.ReadSeeker) bool {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return false
		}
		data, err := io.ReadAll(r)
		return err == nil && string(data) == expected
	})
}

func (s *FileTestSuite) expectDownload(content string) {
	s.client.EXPECT().Download(mock.Anything, "dir/file.txt").Return(body(content), nil).Once()
}

func (s *FileTestSuite) expectUpload(p, content string) {
	s.client.EXPECT().Upload(mock.Anything, p, withContent(content)).Return(&DriveItem{}, nil).Once()
}

func (s *FileTestSuite) TestPathsAndURIs() {
	s.Equal("file.txt", s.file.Name())
	s.Equal("/dir/file.txt", s.file.Path())
	s.Equal("onedrive:///dir/file.txt", s.file.URI())
	s.Equal("onedrive:///dir/file.txt", s.file.String())
	s.Equal("/dir/", s.file.Location().Path())
	s.Equal("dir/file.txt", s.file.oneDrivePath())
	s.Equal("file.txt", s.newFile("/file.txt").oneDrivePath())
}

func (s *FileTestSuite) TestMetadata() {
	modified := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").
		Return(&DriveItem{Name: "file.txt", Size: 12, LastModified: modified}, nil)

	lm, err := s.file.LastModified()
	s.Require().NoError(err)
	s.Equal(modified, *lm)

	size, err := s.file.Size()
	s.Require().NoError(err)
	s.Equal(uint64(12), size)
}

func (s *FileTestSuite) TestMetadataErrors() {
	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(nil, errors.New("boom"))

	_, err := s.file.LastModified()
	s.Require().ErrorContains(err, "boom")
	_, err = s.file.Size()
	s.Require().ErrorContains(err, "boom")
}

func (s *FileTestSuite) TestExists() {
	tests := []struct {
		name          string
		item          *DriveItem
		err           error
		expected      bool
		expectedError string
	}{
		{name: "file exists", item: &DriveItem{}, expected: true},
		{name: "folder at path is not a file", item: &DriveItem{IsFolder: true}, expected: false},
		{name: "not found", err: &apiError{StatusCode: 404}, expected: false},
		{name: "other error", err: errors.New("boom"), expectedError: "boom"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(tt.item, tt.err).Once()

			exists, err := s.file.Exists()
			if tt.expectedError != "" {
				s.Require().ErrorContains(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, exists)
		})
	}
}

func (s *FileTestSuite) TestReadDownloadsOnce() {
	s.expectDownload("hello world")

	data, err := io.ReadAll(s.file)
	s.Require().NoError(err)
	s.Equal("hello world", string(data))

	n, err := s.file.Read(make([]byte, 4))
	s.Zero(n)
	s.Require().ErrorIs(err, io.EOF)

	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestReadMissingFile() {
	s.client.EXPECT().Download(mock.Anything, "dir/file.txt").Return(nil, &apiError{StatusCode: 404})

	_, err := s.file.Read(make([]byte, 1))
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *FileTestSuite) TestReadDownloadFailsMidStream() {
	s.client.EXPECT().Download(mock.Anything, "dir/file.txt").
		Return(io.NopCloser(io.MultiReader(strings.NewReader("par"), iotestErrReader{})), nil)

	_, err := s.file.Read(make([]byte, 1))
	s.Require().ErrorContains(err, "downloading /dir/file.txt")
}

type iotestErrReader struct{}

func (iotestErrReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func (s *FileTestSuite) TestWriteFirstOverwrites() {
	s.expectUpload("dir/file.txt", "new content")

	_, err := s.file.Write([]byte("new "))
	s.Require().NoError(err)
	_, err = s.file.Write([]byte("content"))
	s.Require().NoError(err)

	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestSeekZeroThenWriteKeepsRemainingContent() {
	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{Size: 6}, nil).Once()
	s.expectDownload("hello!")
	s.expectUpload("dir/file.txt", "Jello!")

	pos, err := s.file.Seek(0, io.SeekStart)
	s.Require().NoError(err)
	s.Zero(pos)

	_, err = s.file.Write([]byte("J"))
	s.Require().NoError(err)
	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestReadThenWriteModifiesInPlace() {
	s.expectDownload("hello!")
	s.expectUpload("dir/file.txt", "heXYo!")

	buf := make([]byte, 2)
	_, err := s.file.Read(buf)
	s.Require().NoError(err)
	s.Equal("he", string(buf))

	_, err = s.file.Write([]byte("XY"))
	s.Require().NoError(err)
	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestWriteAfterSeekOnDeletedFileStartsEmpty() {
	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{Size: 3}, nil).Once()
	s.client.EXPECT().Download(mock.Anything, "dir/file.txt").Return(nil, &apiError{StatusCode: 404}).Once()
	s.expectUpload("dir/file.txt", "abc")

	_, err := s.file.Seek(0, io.SeekStart)
	s.Require().NoError(err)
	_, err = s.file.Write([]byte("abc"))
	s.Require().NoError(err)
	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestWriteThenReadBack() {
	s.expectUpload("dir/file.txt", "abcdef")

	_, err := s.file.Write([]byte("abcdef"))
	s.Require().NoError(err)

	pos, err := s.file.Seek(-2, io.SeekEnd)
	s.Require().NoError(err)
	s.Equal(int64(4), pos)

	data, err := io.ReadAll(s.file)
	s.Require().NoError(err)
	s.Equal("ef", string(data))

	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestSeek() {
	tests := []struct {
		name          string
		offset        int64
		whence        int
		expected      int64
		expectedError string
	}{
		{name: "from start", offset: 3, whence: io.SeekStart, expected: 3},
		{name: "from end", offset: -1, whence: io.SeekEnd, expected: 9},
		{name: "past end is allowed", offset: 20, whence: io.SeekStart, expected: 20},
		{name: "negative position", offset: -1, whence: io.SeekStart, expectedError: "seek"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{Size: 10}, nil).Once()
			f := s.newFile("/dir/file.txt")

			pos, err := f.Seek(tt.offset, tt.whence)
			if tt.expectedError != "" {
				s.Require().ErrorContains(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, pos)
		})
	}
}

func (s *FileTestSuite) TestSeekPastEndThenRead() {
	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{Size: 3}, nil).Once()
	s.expectDownload("abc")

	_, err := s.file.Seek(5, io.SeekStart)
	s.Require().NoError(err)

	n, err := s.file.Read(make([]byte, 4))
	s.Zero(n)
	s.Require().ErrorIs(err, io.EOF)
	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestSeekMissingFile() {
	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(nil, &apiError{StatusCode: 404})

	_, err := s.file.Seek(0, io.SeekStart)
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *FileTestSuite) TestCloseWithoutWriteDoesNotUpload() {
	s.expectDownload("abc")

	_, err := s.file.Read(make([]byte, 1))
	s.Require().NoError(err)
	s.Require().NoError(s.file.Close())
	s.Require().NoError(s.file.Close(), "Close is idempotent")
}

func (s *FileTestSuite) TestCloseUploadFailure() {
	s.client.EXPECT().Upload(mock.Anything, "dir/file.txt", mock.Anything).Return(nil, errors.New("quota"))

	_, err := s.file.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().ErrorContains(s.file.Close(), "quota")
}

func (s *FileTestSuite) TestLargeFileUsesUploadSession() {
	const size = maxSimpleUploadSize + 1
	s.fs.options.ChunkSize = uploadChunkMultiple
	session := mocks.NewUploadSession(s.T())

	s.client.EXPECT().CreateUploadSession(mock.Anything, "dir/file.txt").Return(session, nil)
	var uploaded int64
	session.EXPECT().UploadChunk(mock.Anything, mock.Anything, mock.Anything, int64(size)).
		RunAndReturn(func(_ context.Context, data []byte, offset, _ int64) (*DriveItem, error) {
			s.Require().Equal(uploaded, offset, "chunks are contiguous")
			s.Require().LessOrEqual(len(data), uploadChunkMultiple)
			uploaded += int64(len(data))
			return nil, nil
		})

	_, err := s.file.Write(make([]byte, size))
	s.Require().NoError(err)
	s.Require().NoError(s.file.Close())
	s.Equal(int64(size), uploaded)
}

func (s *FileTestSuite) TestLargeFileUploadErrors() {
	s.Run("session creation fails", func() {
		s.client.EXPECT().CreateUploadSession(mock.Anything, "dir/file.txt").Return(nil, errors.New("no session")).Once()

		f := s.newFile("/dir/file.txt")
		_, err := f.Write(make([]byte, maxSimpleUploadSize+1))
		s.Require().NoError(err)
		s.Require().ErrorContains(f.Close(), "no session")
	})

	s.Run("chunk failure cancels session", func() {
		session := mocks.NewUploadSession(s.T())
		s.client.EXPECT().CreateUploadSession(mock.Anything, "dir/file.txt").Return(session, nil).Once()
		session.EXPECT().UploadChunk(mock.Anything, mock.Anything, int64(0), mock.Anything).Return(nil, errors.New("chunk failed"))
		session.EXPECT().Cancel(mock.Anything).Return(nil)

		f := s.newFile("/dir/file.txt")
		_, err := f.Write(make([]byte, maxSimpleUploadSize+1))
		s.Require().NoError(err)
		s.Require().ErrorContains(f.Close(), "chunk failed")
	})
}

func (s *FileTestSuite) TestChunkSize() {
	tests := []struct {
		name       string
		configured int64
		expected   int64
	}{
		{name: "exact multiple kept", configured: 16 * uploadChunkMultiple, expected: 16 * uploadChunkMultiple},
		{name: "rounded down", configured: 4 * 1024 * 1024, expected: 12 * uploadChunkMultiple},
		{name: "minimum one unit", configured: 1, expected: uploadChunkMultiple},
		{name: "zero uses minimum", configured: 0, expected: uploadChunkMultiple},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expected, chunkSize(tt.configured))
		})
	}
}

func (s *FileTestSuite) TestDeleteDiscardsPendingWrites() {
	s.client.EXPECT().Delete(mock.Anything, "dir/file.txt").Return(nil)

	_, err := s.file.Write([]byte("never uploaded"))
	s.Require().NoError(err)
	s.Require().NoError(s.file.Delete())
}

func (s *FileTestSuite) TestDeleteError() {
	s.client.EXPECT().Delete(mock.Anything, "dir/file.txt").Return(&apiError{StatusCode: 404})

	s.Require().ErrorIs(s.file.Delete(), ErrNotFound)
}

func (s *FileTestSuite) TestTouch() {
	s.Run("creates missing file", func() {
		s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(nil, &apiError{StatusCode: 404}).Once()
		s.expectUpload("dir/file.txt", "")

		s.Require().NoError(s.file.Touch())
	})

	s.Run("re-uploads existing content", func() {
		s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{Size: 3}, nil).Once()
		s.expectDownload("abc")
		s.expectUpload("dir/file.txt", "abc")

		s.Require().NoError(s.file.Touch())
	})

	s.Run("then reading returns the content", func() {
		s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{Size: 3}, nil).Once()
		s.expectDownload("abc")
		s.expectUpload("dir/file.txt", "abc")
		s.expectDownload("abc")

		s.Require().NoError(s.file.Touch())
		data, err := io.ReadAll(s.file)
		s.Require().NoError(err)
		s.Equal("abc", string(data))
		s.Require().NoError(s.file.Close())
	})

	s.Run("exists check fails", func() {
		s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(nil, errors.New("boom")).Once()

		s.Require().ErrorContains(s.file.Touch(), "boom")
	})

	s.Run("download fails", func() {
		s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&DriveItem{}, nil).Once()
		s.client.EXPECT().Download(mock.Anything, "dir/file.txt").Return(nil, errors.New("down")).Once()

		s.Require().ErrorContains(s.file.Touch(), "down")
	})
}

func (s *FileTestSuite) TestCopyToFileNative() {
	target := s.newFile("/other/copy.txt")
	s.client.EXPECT().Copy(mock.Anything, "dir/file.txt", "other", "copy.txt").Return(nil)

	s.Require().NoError(s.file.CopyToFile(target))
}

func (s *FileTestSuite) TestCopyToFileMissingDestinationFolderStreams() {
	target := s.newFile("/new/folder/copy.txt")
	s.client.EXPECT().Copy(mock.Anything, "dir/file.txt", "new/folder", "copy.txt").Return(&apiError{StatusCode: 404})
	s.expectDownload("payload")
	s.expectUpload("new/folder/copy.txt", "payload")

	s.Require().NoError(s.file.CopyToFile(target))
}

func (s *FileTestSuite) TestCopyToFileNativeError() {
	target := s.newFile("/other/copy.txt")
	s.client.EXPECT().Copy(mock.Anything, "dir/file.txt", "other", "copy.txt").Return(errors.New("denied"))

	s.Require().ErrorContains(s.file.CopyToFile(target), "denied")
}

func (s *FileTestSuite) TestCopyToFileWithLocalStateStreams() {
	target := s.newFile("/other/copy.txt")
	s.expectUpload("other/copy.txt", "fresh")
	s.expectUpload("dir/file.txt", "fresh")

	_, err := s.file.Write([]byte("fresh"))
	s.Require().NoError(err)
	_, err = s.file.Seek(0, io.SeekStart)
	s.Require().NoError(err)

	s.Require().NoError(s.file.CopyToFile(target))
}

func (s *FileTestSuite) TestCopyToFileOtherBackendStreams() {
	memFile, err := mem.NewFileSystem().NewFile("", "/copy.txt")
	s.Require().NoError(err)
	s.expectDownload("to mem")

	s.Require().NoError(s.file.CopyToFile(memFile))

	data, err := io.ReadAll(memFile)
	s.Require().NoError(err)
	s.Equal("to mem", string(data))
}

func (s *FileTestSuite) TestCopyToFileAfterReadNotPossible() {
	s.expectDownload("abc")
	_, err := s.file.Read(make([]byte, 1))
	s.Require().NoError(err)

	s.Require().ErrorIs(s.file.CopyToFile(s.newFile("/x.txt")), vfs.ErrCopyToNotPossible)
	s.Require().ErrorIs(s.file.MoveToFile(s.newFile("/x.txt")), vfs.ErrCopyToNotPossible)
	s.Require().NoError(s.file.Close())
}

func (s *FileTestSuite) TestCopyToLocation() {
	loc, err := s.fs.NewLocation("", "/backup/")
	s.Require().NoError(err)
	s.client.EXPECT().Copy(mock.Anything, "dir/file.txt", "backup", "file.txt").Return(nil)

	newFile, err := s.file.CopyToLocation(loc)
	s.Require().NoError(err)
	s.Equal("/backup/file.txt", newFile.Path())
}

func (s *FileTestSuite) TestMoveToFileNative() {
	target := s.newFile("/other/moved.txt")
	s.client.EXPECT().Move(mock.Anything, "dir/file.txt", "other", "moved.txt").Return(&DriveItem{}, nil)

	s.Require().NoError(s.file.MoveToFile(target))
}

func (s *FileTestSuite) TestMoveToFileMissingDestinationFolderCopiesThenDeletes() {
	target := s.newFile("/new/moved.txt")
	s.client.EXPECT().Move(mock.Anything, "dir/file.txt", "new", "moved.txt").Return(nil, &apiError{StatusCode: 404})
	s.client.EXPECT().Copy(mock.Anything, "dir/file.txt", "new", "moved.txt").Return(&apiError{StatusCode: 404})
	s.expectDownload("payload")
	s.expectUpload("new/moved.txt", "payload")
	s.client.EXPECT().Delete(mock.Anything, "dir/file.txt").Return(nil)

	s.Require().NoError(s.file.MoveToFile(target))
}

func (s *FileTestSuite) TestMoveToFileErrors() {
	s.Run("native move fails", func() {
		s.client.EXPECT().Move(mock.Anything, "dir/file.txt", "other", "m.txt").Return(nil, errors.New("locked")).Once()

		s.Require().ErrorContains(s.file.MoveToFile(s.newFile("/other/m.txt")), "locked")
	})

	s.Run("delete after copy fails", func() {
		memFile, err := mem.NewFileSystem().NewFile("", "/m.txt")
		s.Require().NoError(err)
		s.expectDownload("x")
		s.client.EXPECT().Delete(mock.Anything, "dir/file.txt").Return(errors.New("locked")).Once()

		s.Require().ErrorContains(s.file.MoveToFile(memFile), "locked")
	})
}

func (s *FileTestSuite) TestMoveToLocation() {
	loc, err := s.fs.NewLocation("", "/archive/")
	s.Require().NoError(err)
	s.client.EXPECT().Move(mock.Anything, "dir/file.txt", "archive", "file.txt").Return(&DriveItem{}, nil)

	newFile, err := s.file.MoveToLocation(loc)
	s.Require().NoError(err)
	s.Equal("/archive/file.txt", newFile.Path())
}

func (s *FileTestSuite) TestOperationsWithoutCredentials() {
	s.T().Setenv("VFS_ONEDRIVE_ACCESS_TOKEN", "")
	s.T().Setenv("VFS_ONEDRIVE_TENANT_ID", "")
	f, err := NewFileSystem(WithTempDir(s.tempDir)).NewFile("", "/a.txt")
	s.Require().NoError(err)

	_, err = f.Exists()
	s.Require().ErrorIs(err, errCredentialsRequired)
	_, err = f.Read(make([]byte, 1))
	s.Require().ErrorIs(err, errCredentialsRequired)
	s.Require().ErrorIs(f.Delete(), errCredentialsRequired)
	s.Require().ErrorIs(f.Touch(), errCredentialsRequired)
	_, err = f.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().ErrorIs(f.Close(), errCredentialsRequired)
}
