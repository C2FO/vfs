package gdrive

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/mem"

	"github.com/c2fo/vfs/contrib/backend/gdrive/mocks"
	"github.com/c2fo/vfs/contrib/backend/gdrive/types"
)

type FileTestSuite struct {
	suite.Suite
	client  *mocks.Client
	fs      *FileSystem
	tempDir string
}

func TestFileTestSuite(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}

func (s *FileTestSuite) SetupTest() {
	s.client = mocks.NewClient(s.T())
	s.tempDir = s.T().TempDir()
	s.fs = NewFileSystem(WithClient(s.client), WithTempDir(s.tempDir), WithChunkSize(uploadChunkMultiple))
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

// readAll reads an upload body from the start, as the real client does regardless of the position
// the caller left the reader at.
func readAll(s *FileTestSuite, r io.ReadSeeker) []byte {
	_, err := r.Seek(0, io.SeekStart)
	s.Require().NoError(err)
	data, err := io.ReadAll(r)
	s.Require().NoError(err)
	return data
}

func download(content string) io.ReadCloser { return io.NopCloser(strings.NewReader(content)) }

func (s *FileTestSuite) TestMetadata() {
	mod := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := s.newFile("/dir/file.txt")

	s.Equal("file.txt", f.Name())
	s.Equal("/dir/file.txt", f.Path())
	s.Equal("gdrive:///dir/file.txt", f.URI())
	s.Equal(f.URI(), f.String())
	s.Equal("/dir/", f.Location().Path())

	s.client.EXPECT().GetItem(mock.Anything, "dir/file.txt").Return(&types.Item{Size: 42, ModTime: mod}, nil)

	size, err := f.Size()
	s.Require().NoError(err)
	s.EqualValues(42, size)

	lm, err := f.LastModified()
	s.Require().NoError(err)
	s.Equal(mod, *lm)

	exists, err := f.Exists()
	s.Require().NoError(err)
	s.True(exists)
}

func (s *FileTestSuite) TestExistsCases() {
	tests := []struct {
		name        string
		item        *types.Item
		err         error
		expected    bool
		expectedErr bool
	}{
		{name: "folder at path is not a file", item: &types.Item{IsFolder: true}},
		{name: "missing", err: ErrNotFound},
		{name: "ambiguous is an error", err: ErrAmbiguous, expectedErr: true},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().GetItem(mock.Anything, "f").Return(tt.item, tt.err).Once()
			got, err := s.newFile("/f").Exists()
			if tt.expectedErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, got)
		})
	}
}

func (s *FileTestSuite) TestMetadataErrors() {
	s.client.EXPECT().GetItem(mock.Anything, "f").Return(nil, errBoom)
	f := s.newFile("/f")

	_, err := f.Size()
	s.Require().Error(err)
	_, err = f.LastModified()
	s.Require().Error(err)
	_, err = f.Exists()
	s.Require().Error(err)
	_, err = f.Seek(0, io.SeekStart)
	s.Require().Error(err)
}

func (s *FileTestSuite) TestWriteCloseUploadsSmallFile() {
	var got []byte
	s.client.EXPECT().Upload(mock.Anything, "dir/file.txt", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, r io.ReadSeeker) (*types.Item, error) {
			got = readAll(s, r)
			return &types.Item{}, nil
		}).Once()

	f := s.newFile("/dir/file.txt")
	n, err := f.Write([]byte("hello "))
	s.Require().NoError(err)
	s.Equal(6, n)
	_, err = f.Write([]byte("world"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.Equal("hello world", string(got))
}

func (s *FileTestSuite) TestCloseWithoutWriteDoesNotUpload() {
	s.client.EXPECT().Download(mock.Anything, "f").Return(download("abc"), nil).Once()

	f := s.newFile("/f")
	buf := make([]byte, 10)
	n, err := f.Read(buf)
	s.Require().NoError(err)
	s.Equal("abc", string(buf[:n]))
	_, err = f.Read(buf)
	s.Require().ErrorIs(err, io.EOF)
	s.Require().NoError(f.Close())
}

func (s *FileTestSuite) TestReadErrors() {
	s.client.EXPECT().Download(mock.Anything, "f").Return(nil, ErrNotFound).Once()
	_, err := s.newFile("/f").Read(make([]byte, 1))
	s.Require().ErrorIs(err, ErrNotFound)
}

func (s *FileTestSuite) TestWriteExistingAfterReadAndSeek() {
	var got []byte
	s.client.EXPECT().GetItem(mock.Anything, "f").Return(&types.Item{Size: 10}, nil).Once()
	s.client.EXPECT().Download(mock.Anything, "f").Return(download("0123456789"), nil).Once()
	s.client.EXPECT().Upload(mock.Anything, "f", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, r io.ReadSeeker) (*types.Item, error) {
			got = readAll(s, r)
			return &types.Item{}, nil
		}).Once()

	f := s.newFile("/f")
	pos, err := f.Seek(5, io.SeekStart)
	s.Require().NoError(err)
	s.EqualValues(5, pos)
	_, err = f.Write([]byte("XY"))
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.Equal("01234XY789", string(got))
}

func (s *FileTestSuite) TestSeekUsesRemoteSizeThenTempFile() {
	s.client.EXPECT().GetItem(mock.Anything, "f").Return(&types.Item{Size: 10}, nil).Once()
	s.client.EXPECT().Download(mock.Anything, "f").Return(download("0123456789"), nil).Once()

	f := s.newFile("/f")
	pos, err := f.Seek(-3, io.SeekEnd)
	s.Require().NoError(err)
	s.EqualValues(7, pos)

	buf := make([]byte, 10)
	n, err := f.Read(buf)
	s.Require().NoError(err)
	s.Equal("789", string(buf[:n]))

	pos, err = f.Seek(2, io.SeekStart)
	s.Require().NoError(err)
	s.EqualValues(2, pos)

	_, err = f.Seek(-100, io.SeekStart)
	s.Require().Error(err)
	s.Require().NoError(f.Close())
}

func (s *FileTestSuite) TestLargeFileUsesResumableUpload() {
	data := bytes.Repeat([]byte("z"), maxSimpleUploadSize+uploadChunkMultiple*2+10)
	total := int64(len(data))

	session := mocks.NewUploadSession(s.T())
	s.client.EXPECT().CreateUploadSession(mock.Anything, "big", total).Return(session, nil).Once()

	var offsets []int64
	var received int
	session.EXPECT().UploadChunk(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, chunk []byte, offset int64) (*types.Item, error) {
			offsets = append(offsets, offset)
			received += len(chunk)
			if int64(received) == total {
				return &types.Item{}, nil
			}
			s.Zero(len(chunk)%uploadChunkMultiple, "all but the last chunk are 256 KiB multiples")
			return nil, nil
		})

	f := s.newFile("/big")
	_, err := f.Write(data)
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	s.EqualValues(total, received)
	s.Equal(int64(0), offsets[0])
	s.Greater(len(offsets), 1)
}

func (s *FileTestSuite) TestLargeFileUploadFailureCancelsSession() {
	data := bytes.Repeat([]byte("z"), maxSimpleUploadSize+1)

	session := mocks.NewUploadSession(s.T())
	s.client.EXPECT().CreateUploadSession(mock.Anything, "big", mock.Anything).Return(session, nil).Once()
	session.EXPECT().UploadChunk(mock.Anything, mock.Anything, mock.Anything).Return(nil, errBoom).Once()
	session.EXPECT().Cancel(mock.Anything).Return(nil).Once()

	f := s.newFile("/big")
	_, err := f.Write(data)
	s.Require().NoError(err)
	s.Require().Error(f.Close())
}

func (s *FileTestSuite) TestLargeFileSessionCreationFailure() {
	data := bytes.Repeat([]byte("z"), maxSimpleUploadSize+1)
	s.client.EXPECT().CreateUploadSession(mock.Anything, "big", mock.Anything).Return(nil, errBoom).Once()

	f := s.newFile("/big")
	_, err := f.Write(data)
	s.Require().NoError(err)
	s.Require().Error(f.Close())
}

func (s *FileTestSuite) TestUploadFailureOnClose() {
	s.client.EXPECT().Upload(mock.Anything, "f", mock.Anything).Return(nil, errBoom).Once()

	f := s.newFile("/f")
	_, err := f.Write([]byte("x"))
	s.Require().NoError(err)
	s.Require().ErrorContains(f.Close(), "boom")
}

func (s *FileTestSuite) TestDelete() {
	s.client.EXPECT().Delete(mock.Anything, "f").Return(nil).Once()
	s.Require().NoError(s.newFile("/f").Delete())

	s.client.EXPECT().Delete(mock.Anything, "g").Return(ErrIsFolder).Once()
	s.Require().ErrorIs(s.newFile("/g").Delete(), ErrIsFolder)
}

func (s *FileTestSuite) TestDeleteDiscardsPendingWrites() {
	s.client.EXPECT().Delete(mock.Anything, "f").Return(nil).Once()

	f := s.newFile("/f")
	_, err := f.Write([]byte("pending"))
	s.Require().NoError(err)
	s.Require().NoError(f.Delete())
	s.Require().NoError(f.Close(), "nothing left to upload")
}

func (s *FileTestSuite) TestTouch() {
	tests := []struct {
		name  string
		item  *types.Item
		err   error
		setup func()
	}{
		{
			name: "creates empty file when missing", err: ErrNotFound,
			setup: func() {
				s.client.EXPECT().Upload(mock.Anything, "f", mock.Anything).Return(&types.Item{}, nil).Once()
			},
		},
		{
			name: "updates timestamp when present", item: &types.Item{},
			setup: func() { s.client.EXPECT().Touch(mock.Anything, "f").Return(nil).Once() },
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.client.EXPECT().GetItem(mock.Anything, "f").Return(tt.item, tt.err).Once()
			tt.setup()
			s.Require().NoError(s.newFile("/f").Touch())
		})
	}

	s.client.EXPECT().GetItem(mock.Anything, "e").Return(nil, errBoom).Once()
	s.Require().Error(s.newFile("/e").Touch())
}

func (s *FileTestSuite) TestCopyToFileNative() {
	s.client.EXPECT().Copy(mock.Anything, "a/src.txt", "b", "dst.txt").Return(nil).Once()
	s.Require().NoError(s.newFile("/a/src.txt").CopyToFile(s.newFile("/b/dst.txt")))
}

func (s *FileTestSuite) TestCopyToLocationAndMoveToLocation() {
	dest, err := s.fs.NewLocation("", "/b/")
	s.Require().NoError(err)

	s.client.EXPECT().Copy(mock.Anything, "a/src.txt", "b", "src.txt").Return(nil).Once()
	copied, err := s.newFile("/a/src.txt").CopyToLocation(dest)
	s.Require().NoError(err)
	s.Equal("/b/src.txt", copied.Path())

	s.client.EXPECT().Move(mock.Anything, "a/m.txt", "b", "m.txt").Return(&types.Item{}, nil).Once()
	moved, err := s.newFile("/a/m.txt").MoveToLocation(dest)
	s.Require().NoError(err)
	s.Equal("/b/m.txt", moved.Path())
}

func (s *FileTestSuite) TestCopyFallsBackWhenDestinationFolderMissing() {
	s.client.EXPECT().Copy(mock.Anything, "a/src.txt", "new", "dst.txt").Return(ErrNotFound).Once()
	s.client.EXPECT().Download(mock.Anything, "a/src.txt").Return(download("payload"), nil).Once()
	s.client.EXPECT().GetItem(mock.Anything, "new/dst.txt").Return(nil, ErrNotFound).Maybe()

	var got []byte
	s.client.EXPECT().Upload(mock.Anything, "new/dst.txt", mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, r io.ReadSeeker) (*types.Item, error) {
			got = readAll(s, r)
			return &types.Item{}, nil
		}).Once()

	s.Require().NoError(s.newFile("/a/src.txt").CopyToFile(s.newFile("/new/dst.txt")))
	s.Equal("payload", string(got))
}

func (s *FileTestSuite) TestCopyErrors() {
	s.client.EXPECT().Copy(mock.Anything, "a", "b", "c").Return(errBoom).Once()
	s.Require().Error(s.newFile("/a").CopyToFile(s.newFile("/b/c")))

	src := s.newFile("/a")
	src.cursorPos = 5
	s.Require().ErrorIs(src.CopyToFile(s.newFile("/b/c")), vfs.ErrCopyToNotPossible)
	s.Require().ErrorIs(src.MoveToFile(s.newFile("/b/c")), vfs.ErrCopyToNotPossible)
}

func (s *FileTestSuite) TestCopyToOtherBackendStreams() {
	memFS := mem.NewFileSystem()
	target, err := memFS.NewFile("", "/out.txt")
	s.Require().NoError(err)

	s.client.EXPECT().Download(mock.Anything, "src.txt").Return(download("across"), nil).Once()
	s.Require().NoError(s.newFile("/src.txt").CopyToFile(target))

	rd, err := memFS.NewFile("", "/out.txt")
	s.Require().NoError(err)
	data, err := io.ReadAll(rd)
	s.Require().NoError(err)
	s.Equal("across", string(data))
}

func (s *FileTestSuite) TestMoveToFile() {
	s.client.EXPECT().Move(mock.Anything, "a/m", "b", "n").Return(&types.Item{}, nil).Once()
	s.Require().NoError(s.newFile("/a/m").MoveToFile(s.newFile("/b/n")))

	s.client.EXPECT().Move(mock.Anything, "a/e", "b", "n").Return(nil, errBoom).Once()
	s.Require().Error(s.newFile("/a/e").MoveToFile(s.newFile("/b/n")))
}

func (s *FileTestSuite) TestMoveFallsBackToCopyAndDelete() {
	s.client.EXPECT().Move(mock.Anything, "a/m", "new", "n").Return(nil, ErrNotFound).Once()
	s.client.EXPECT().Copy(mock.Anything, "a/m", "new", "n").Return(ErrNotFound).Once()
	s.client.EXPECT().Download(mock.Anything, "a/m").Return(download("data"), nil).Once()
	s.client.EXPECT().Upload(mock.Anything, "new/n", mock.Anything).Return(&types.Item{}, nil).Once()
	s.client.EXPECT().Delete(mock.Anything, "a/m").Return(nil).Once()

	s.Require().NoError(s.newFile("/a/m").MoveToFile(s.newFile("/new/n")))
}

func (s *FileTestSuite) TestNativeTargetRules() {
	src := s.newFile("/a")

	other := NewFileSystem(WithClient(s.client))
	of, err := other.NewFile("", "/b")
	s.Require().NoError(err)
	_, ok := src.nativeTarget(of)
	s.False(ok, "different FileSystem")

	sharedDrive, err := s.fs.NewFile("0Adrive", "/b")
	s.Require().NoError(err)
	_, ok = src.nativeTarget(sharedDrive)
	s.False(ok, "different drive")

	same := s.newFile("/b")
	_, ok = src.nativeTarget(same)
	s.True(ok)

	s.client.EXPECT().Download(mock.Anything, "b").Return(download("x"), nil).Once()
	dirty := s.newFile("/b")
	_, err = dirty.Read(make([]byte, 1))
	s.Require().NoError(err)
	_, ok = src.nativeTarget(dirty)
	s.False(ok, "target has local state")
	s.Require().NoError(dirty.Close())
}

func (s *FileTestSuite) TestChunkSize() {
	tests := []struct {
		in       int64
		expected int64
	}{
		{in: 0, expected: uploadChunkMultiple},
		{in: 1, expected: uploadChunkMultiple},
		{in: uploadChunkMultiple*3 + 5, expected: uploadChunkMultiple * 3},
	}
	for _, tt := range tests {
		s.Equal(tt.expected, chunkSize(tt.in))
	}
}
