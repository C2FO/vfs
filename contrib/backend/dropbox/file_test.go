package dropbox

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/dropbox/mocks"
)

type FileTestSuite struct {
	suite.Suite
	mockClient *mocks.Client
	fs         *FileSystem
	location   *Location
	file       *File
}

func (s *FileTestSuite) SetupTest() {
	s.mockClient = mocks.NewClient(s.T())
	s.fs = &FileSystem{
		client:  s.mockClient,
		options: NewOptions(),
	}
	s.location = &Location{
		fileSystem: s.fs,
		path:       "/test/path/",
	}
	s.file = &File{
		location: s.location,
		path:     "/test/path/file.txt",
	}
}

func (s *FileTestSuite) TestName() {
	s.Equal("file.txt", s.file.Name())
}

func (s *FileTestSuite) TestPath() {
	s.Equal("/test/path/file.txt", s.file.Path())
}

func (s *FileTestSuite) TestURI() {
	uri := s.file.URI()
	s.Contains(uri, "dbx://")
	s.Contains(uri, "/test/path/file.txt")
}

func (s *FileTestSuite) TestLocation() {
	loc := s.file.Location()
	s.Equal(s.location, loc)
}

func (s *FileTestSuite) TestExists() {
	s.Run("Success - file exists", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.MatchedBy(func(arg *files.GetMetadataArg) bool {
				return arg.Path == "/test/path/file.txt"
			})).
			Return(&files.FileMetadata{
				Metadata: files.Metadata{Name: "file.txt"},
			}, nil).
			Once()

		exists, err := s.file.Exists()
		s.Require().NoError(err)
		s.True(exists)
	})

	s.Run("Success - file does not exist", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		exists, err := s.file.Exists()
		s.Require().NoError(err)
		s.False(exists)
	})
}

func (s *FileTestSuite) TestSize() {
	s.Run("Success - returns file size", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{
				Metadata: files.Metadata{Name: "file.txt"},
				Size:     1024,
			}, nil).
			Once()

		size, err := s.file.Size()
		s.Require().NoError(err)
		s.Equal(uint64(1024), size)
	})

	s.Run("Error - not a file", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FolderMetadata{
				Metadata: files.Metadata{Name: "folder"},
			}, nil).
			Once()

		size, err := s.file.Size()
		s.Require().Error(err)
		s.Equal(uint64(0), size)
	})
}

func (s *FileTestSuite) TestLastModified() {
	s.Run("Success - returns last modified time", func() {
		now := time.Now()
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{
				Metadata:       files.Metadata{Name: "file.txt"},
				ServerModified: now,
			}, nil).
			Once()

		lastMod, err := s.file.LastModified()
		s.Require().NoError(err)
		s.Equal(&now, lastMod)
	})
}

func (s *FileTestSuite) TestRead() {
	s.Run("Success - reads file content", func() {
		content := "test file content"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.MatchedBy(func(arg *files.DownloadArg) bool {
				return arg.Path == "/test/path/file.txt"
			})).
			Return(&files.FileMetadata{
				Metadata: files.Metadata{Name: "file.txt"},
				Size:     uint64(len(content)),
			}, reader, nil).
			Once()

		buf := make([]byte, 100)
		n, err := s.file.Read(buf)
		s.Require().NoError(err)
		s.Equal(len(content), n)
		s.Equal(content, string(buf[:n]))

		// Cleanup
		_ = s.file.Close()
	})

	s.Run("Success - handles EOF", func() {
		content := "test"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{}, reader, nil).
			Once()

		buf := make([]byte, 4)
		n, err := s.file.Read(buf)
		s.Require().NoError(err)
		s.Equal(4, n)

		// Second read should return EOF
		n, err = s.file.Read(buf)
		s.Equal(io.EOF, err)
		s.Equal(0, n)

		_ = s.file.Close()
	})
}

func (s *FileTestSuite) TestWrite() {
	s.Run("Success - writes content", func() {
		content := "test content"

		s.mockClient.EXPECT().
			Upload(mock.MatchedBy(func(arg *files.UploadArg) bool {
				return arg.Path == "/test/path/file.txt"
			}), mock.Anything).
			Return(&files.FileMetadata{
				Metadata: files.Metadata{Name: "file.txt"},
				Size:     uint64(len(content)),
			}, nil).
			Once()

		n, err := s.file.Write([]byte(content))
		s.Require().NoError(err)
		s.Equal(len(content), n)

		// Close to trigger upload
		err = s.file.Close()
		s.Require().NoError(err)
	})

	s.Run("Success - multiple writes", func() {
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		n1, err := s.file.Write([]byte("first "))
		s.Require().NoError(err)
		s.Equal(6, n1)

		n2, err := s.file.Write([]byte("second"))
		s.Require().NoError(err)
		s.Equal(6, n2)

		err = s.file.Close()
		s.Require().NoError(err)
	})
}

func (s *FileTestSuite) TestSeek() {
	s.Run("Success - seeks to position", func() {
		content := "test file content"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{
				Size: uint64(len(content)),
			}, reader, nil).
			Once()

		// Read first
		buf := make([]byte, 4)
		_, _ = s.file.Read(buf)

		// Seek back to start
		pos, err := s.file.Seek(0, io.SeekStart)
		s.Require().NoError(err)
		s.Equal(int64(0), pos)

		_ = s.file.Close()
	})

	s.Run("Success - seeks without download when only writing", func() {
		// Write first to mark file as being in write mode
		_, _ = s.file.Write([]byte("test"))

		// Now seek should work without trying to download
		pos, err := s.file.Seek(10, io.SeekStart)
		s.Require().NoError(err)
		s.Equal(int64(10), pos)
	})
}

func (s *FileTestSuite) TestDelete() {
	s.Run("Success - deletes file", func() {
		s.mockClient.EXPECT().
			DeleteV2(mock.MatchedBy(func(arg *files.DeleteArg) bool {
				return arg.Path == "/test/path/file.txt"
			})).
			Return(&files.DeleteResult{}, nil).
			Once()

		err := s.file.Delete()
		s.Require().NoError(err)
	})

	s.Run("Error - API error", func() {
		s.mockClient.EXPECT().
			DeleteV2(mock.Anything).
			Return(nil, errors.New("api error")).
			Once()

		err := s.file.Delete()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestTouch() {
	s.Run("Success - creates new empty file", func() {
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err := s.file.Touch()
		s.Require().NoError(err)
	})

	s.Run("Success - updates existing file timestamp", func() {
		content := "existing content"
		reader := io.NopCloser(strings.NewReader(content))
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, nil).
			Once()
		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err := s.file.Touch()
		s.Require().NoError(err)
	})

	s.Run("Error - exists check fails", func() {
		file := &File{location: s.location, path: "/test/path/newfile.txt"}
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("api error")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
	})

	s.Run("Error - download fails for existing file", func() {
		file := &File{location: s.location, path: "/test/path/existingfile.txt"}
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: 100}, nil).
			Once()
		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(nil, nil, errors.New("download failed")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestTouchLargeFile() {
	s.Run("Success - touch existing large file uses chunked upload", func() {
		// Create filesystem with low threshold to trigger chunked upload
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/largefile.txt"}

		// Content larger than threshold
		content := "this is large content for touch"
		reader := io.NopCloser(strings.NewReader(content))

		// Mock exists check - file exists
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, nil).
			Once()

		// Mock download
		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		// Mock chunked upload sequence for Touch
		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(&files.UploadSessionStartResult{SessionId: "touch-session"}, nil).
			Once()

		// Content is 31 bytes, chunk size 5: ceil(31/5) = 7 append calls
		s.mockClient.EXPECT().
			UploadSessionAppendV2(mock.Anything, mock.Anything).
			Return(nil).
			Times(7)

		s.mockClient.EXPECT().
			UploadSessionFinish(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err := file.Touch()
		s.Require().NoError(err)
	})

	s.Run("Error - session start fails for large file touch", func() {
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/largefail.txt"}

		content := "content exceeding threshold"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, nil).
			Once()

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(nil, errors.New("touch session start failed")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
		s.Contains(err.Error(), "touch session start failed")
	})

	s.Run("Error - session append fails for large file touch", func() {
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/appendfail.txt"}

		content := "content exceeding threshold"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, nil).
			Once()

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(&files.UploadSessionStartResult{SessionId: "touch-session"}, nil).
			Once()

		s.mockClient.EXPECT().
			UploadSessionAppendV2(mock.Anything, mock.Anything).
			Return(errors.New("touch append failed")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
		s.Contains(err.Error(), "touch append failed")
	})

	s.Run("Error - session finish fails for large file touch", func() {
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/finishfail.txt"}

		// Use exactly 15 bytes: 3 append calls (5+5+5)
		content := "123456789012345"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, nil).
			Once()

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(&files.UploadSessionStartResult{SessionId: "touch-session"}, nil).
			Once()

		// 15 bytes, 5 byte chunks = 3 append calls
		s.mockClient.EXPECT().
			UploadSessionAppendV2(mock.Anything, mock.Anything).
			Return(nil).
			Times(3)

		s.mockClient.EXPECT().
			UploadSessionFinish(mock.Anything, mock.Anything).
			Return(nil, errors.New("touch finish failed")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
		s.Contains(err.Error(), "touch finish failed")
	})
}

func (s *FileTestSuite) TestCopyToFile() {
	s.Run("Success - native copy within same filesystem", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		// Mock check for target existence
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		s.mockClient.EXPECT().
			CopyV2(mock.MatchedBy(func(arg *files.RelocationArg) bool {
				return arg.RelocationPath.FromPath == "/test/path/file.txt" &&
					arg.RelocationPath.ToPath == "/test/path/target.txt"
			})).
			Return(&files.RelocationResult{}, nil).
			Once()

		err := s.file.CopyToFile(targetFile)
		s.Require().NoError(err)
	})

	s.Run("Error - cursor not at start", func() {
		s.file.cursorPos = 10

		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		err := s.file.CopyToFile(targetFile)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestMoveToFile() {
	s.Run("Success - native move within same filesystem", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		s.mockClient.EXPECT().
			MoveV2(mock.MatchedBy(func(arg *files.RelocationArg) bool {
				return arg.RelocationPath.FromPath == "/test/path/file.txt" &&
					arg.RelocationPath.ToPath == "/test/path/target.txt"
			})).
			Return(&files.RelocationResult{}, nil).
			Once()

		err := s.file.MoveToFile(targetFile)
		s.Require().NoError(err)
	})
}

func (s *FileTestSuite) TestClose() {
	s.Run("Success - closes without pending operations", func() {
		err := s.file.Close()
		s.Require().NoError(err)
	})

	s.Run("Success - uploads on close after write", func() {
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		_, _ = s.file.Write([]byte("test"))

		err := s.file.Close()
		s.Require().NoError(err)
	})

	s.Run("Success - cleans up temp files", func() {
		content := "test content"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{}, reader, nil).
			Once()

		// Trigger temp file creation
		buf := make([]byte, 4)
		_, _ = s.file.Read(buf)

		// Verify temp file exists
		s.NotNil(s.file.tempFileRead)
		tempFileName := s.file.tempFileRead.Name()

		err := s.file.Close()
		s.Require().NoError(err)

		// Verify temp file is cleaned up
		s.Nil(s.file.tempFileRead)
		_, err = os.Stat(tempFileName)
		s.True(os.IsNotExist(err))
	})
}

func (s *FileTestSuite) TestString() {
	str := s.file.String()
	s.Contains(str, "dbx://")
	s.Contains(str, "/test/path/file.txt")
}

func (s *FileTestSuite) TestReadMultipleReads() {
	s.Run("Success - multiple sequential reads", func() {
		content := "hello world test content"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		// First read
		buf1 := make([]byte, 5)
		n1, err := s.file.Read(buf1)
		s.Require().NoError(err)
		s.Equal(5, n1)
		s.Equal("hello", string(buf1))

		// Second read continues from where we left off
		buf2 := make([]byte, 6)
		n2, err := s.file.Read(buf2)
		s.Require().NoError(err)
		s.Equal(6, n2)
		s.Equal(" world", string(buf2))

		_ = s.file.Close()
	})
}

func (s *FileTestSuite) TestReadFromWriteBuffer() {
	s.Run("Success - read from write buffer", func() {
		// Write some content first
		content := "written content"
		n, err := s.file.Write([]byte(content))
		s.Require().NoError(err)
		s.Equal(len(content), n)

		// Seek back to beginning
		_, err = s.file.Seek(0, io.SeekStart)
		s.Require().NoError(err)

		// Read should return the written content (from write buffer)
		buf := make([]byte, len(content))
		n, err = s.file.Read(buf)
		s.Require().NoError(err)
		s.Equal(len(content), n)
		s.Equal(content, string(buf))

		// Read again should get EOF
		buf2 := make([]byte, 10)
		n, err = s.file.Read(buf2)
		s.Equal(io.EOF, err)
		s.Equal(0, n)

		// Cleanup - mock the upload for Close
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()
		_ = s.file.Close()
	})
}

func (s *FileTestSuite) TestSeekAfterRead() {
	s.Run("Success - seek to beginning after read", func() {
		content := "test content for seeking"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		// Read some content
		buf := make([]byte, 4)
		_, err := s.file.Read(buf)
		s.Require().NoError(err)
		s.Equal("test", string(buf))

		// Seek back to beginning
		pos, err := s.file.Seek(0, io.SeekStart)
		s.Require().NoError(err)
		s.Equal(int64(0), pos)

		// Read again from beginning
		buf2 := make([]byte, 4)
		n, err := s.file.Read(buf2)
		s.Require().NoError(err)
		s.Equal(4, n)
		s.Equal("test", string(buf2))

		_ = s.file.Close()
	})

	s.Run("Success - seek from end", func() {
		content := "test content"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		// Read to populate temp file
		buf := make([]byte, len(content))
		_, _ = s.file.Read(buf)

		// Seek from end
		pos, err := s.file.Seek(-4, io.SeekEnd)
		s.Require().NoError(err)
		s.Equal(int64(len(content)-4), pos)

		_ = s.file.Close()
	})

	s.Run("Success - seek from current position", func() {
		content := "test content here"
		reader := io.NopCloser(strings.NewReader(content))

		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()

		// Read some
		buf := make([]byte, 5)
		_, _ = s.file.Read(buf)

		// Seek forward from current
		pos, err := s.file.Seek(3, io.SeekCurrent)
		s.Require().NoError(err)
		s.Equal(int64(8), pos)

		_ = s.file.Close()
	})
}

func (s *FileTestSuite) TestSeekOnExistingFile() {
	s.Run("Success - seek on existing file without read/write", func() {
		file := &File{location: s.location, path: "/test/path/existing.txt"}

		// Mock Exists check
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: 100}, nil).
			Once()

		// Mock Size call
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: 100}, nil).
			Once()

		pos, err := file.Seek(50, io.SeekStart)
		s.Require().NoError(err)
		s.Equal(int64(50), pos)
	})
}

func (s *FileTestSuite) TestWriteAfterSeek() {
	s.Run("Success - write after seek downloads existing content", func() {
		file := &File{location: s.location, path: "/test/path/seekwrite.txt"}

		// First seek (marks seekCalled = true)
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: 20}, nil).
			Once()
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: 20}, nil).
			Once()

		_, err := file.Seek(10, io.SeekStart)
		s.Require().NoError(err)

		// Write should download existing file first (because seekCalled is true)
		existingContent := "existing file content"
		reader := io.NopCloser(strings.NewReader(existingContent))
		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(existingContent))}, reader, nil).
			Once()

		n, err := file.Write([]byte("new"))
		s.Require().NoError(err)
		s.Equal(3, n)

		// Mock upload for close
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		_ = file.Close()
	})
}

func (s *FileTestSuite) TestCopyToLocation() {
	s.Run("Success - copies to location", func() {
		targetLocation := &Location{
			fileSystem: s.fs,
			path:       "/target/path/",
		}

		// Mock for CopyToFile internals - check target exists
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		s.mockClient.EXPECT().
			CopyV2(mock.Anything).
			Return(&files.RelocationResult{}, nil).
			Once()

		newFile, err := s.file.CopyToLocation(targetLocation)
		s.Require().NoError(err)
		s.NotNil(newFile)
		s.Equal("file.txt", newFile.Name())
	})

	s.Run("Error - location NewFile fails", func() {
		var targetLocation *Location

		newFile, err := s.file.CopyToLocation(targetLocation)
		s.Require().Error(err)
		s.Nil(newFile)
	})
}

func (s *FileTestSuite) TestMoveToLocation() {
	s.Run("Success - moves to location", func() {
		targetLocation := &Location{
			fileSystem: s.fs,
			path:       "/target/path/",
		}

		// Mock for CopyToFile internals
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		s.mockClient.EXPECT().
			CopyV2(mock.Anything).
			Return(&files.RelocationResult{}, nil).
			Once()

		// Mock for Delete
		s.mockClient.EXPECT().
			DeleteV2(mock.Anything).
			Return(&files.DeleteResult{}, nil).
			Once()

		newFile, err := s.file.MoveToLocation(targetLocation)
		s.Require().NoError(err)
		s.NotNil(newFile)
	})
}

func (s *FileTestSuite) TestMoveToFileErrors() {
	s.Run("Error - cursor not at start", func() {
		file := &File{
			location:  s.location,
			path:      "/test/path/file.txt",
			cursorPos: 10,
		}

		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		err := file.MoveToFile(targetFile)
		s.Require().Error(err)
	})

	s.Run("Error - client error", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		s.mockClient.EXPECT().
			MoveV2(mock.Anything).
			Return(nil, errors.New("move failed")).
			Once()

		err := s.file.MoveToFile(targetFile)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestMoveToLocationErrors() {
	s.Run("Error - cursor not at start", func() {
		file := &File{
			location:  s.location,
			path:      "/test/path/file.txt",
			cursorPos: 10,
		}

		targetLocation := &Location{
			fileSystem: s.fs,
			path:       "/target/path/",
		}

		newFile, err := file.MoveToLocation(targetLocation)
		s.Require().Error(err)
		s.Nil(newFile)
	})
}

func (s *FileTestSuite) TestCopyToFileErrors() {
	s.Run("Error - cursor not at start", func() {
		file := &File{
			location:  s.location,
			path:      "/test/path/file.txt",
			cursorPos: 10,
		}

		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		err := file.CopyToFile(targetFile)
		s.Require().Error(err)
	})

	s.Run("Error - target exists error", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("api error")).
			Once()

		err := s.file.CopyToFile(targetFile)
		s.Require().Error(err)
	})

	s.Run("Error - copy API error", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/target.txt",
		}

		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		s.mockClient.EXPECT().
			CopyV2(mock.Anything).
			Return(nil, errors.New("copy failed")).
			Once()

		err := s.file.CopyToFile(targetFile)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestCopyToLocationErrors() {
	s.Run("Error - cursor not at start", func() {
		file := &File{
			location:  s.location,
			path:      "/test/path/file.txt",
			cursorPos: 10,
		}

		targetLocation := &Location{
			fileSystem: s.fs,
			path:       "/target/path/",
		}

		newFile, err := file.CopyToLocation(targetLocation)
		s.Require().Error(err)
		s.Nil(newFile)
	})
}

func (s *FileTestSuite) TestMoveToFileSuccess() {
	s.Run("Success - native move within same filesystem", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/moved.txt",
		}

		s.mockClient.EXPECT().
			MoveV2(mock.MatchedBy(func(arg *files.RelocationArg) bool {
				return arg.FromPath == "/test/path/file.txt" && arg.ToPath == "/test/path/moved.txt"
			})).
			Return(&files.RelocationResult{}, nil).
			Once()

		err := s.file.MoveToFile(targetFile)
		s.Require().NoError(err)
	})
}

func (s *FileTestSuite) TestTouchUploadError() {
	s.Run("Error - upload fails for new file", func() {
		file := &File{location: s.location, path: "/test/path/newtouch.txt"}
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(nil, errors.New("upload failed")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
	})

	s.Run("Error - re-upload fails for existing file", func() {
		content := "existing"
		reader := io.NopCloser(strings.NewReader(content))
		file := &File{location: s.location, path: "/test/path/existingtouch.txt"}

		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, nil).
			Once()
		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{Size: uint64(len(content))}, reader, nil).
			Once()
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(nil, errors.New("upload failed")).
			Once()

		err := file.Touch()
		s.Require().Error(err)
	})

	s.Run("Error - client error for new file", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/noclient.txt"}

		// GetMetadata returns not found - will try to upload
		// But client retrieval will fail
		err := file.Touch()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestMoveToFileClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/source.txt"}

		targetFile := &File{
			location: loc,
			path:     "/test/path/target.txt",
		}

		err := file.MoveToFile(targetFile)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestMoveToLocationDeleteError() {
	s.Run("Error - delete fails after copy", func() {
		targetLocation := &Location{
			fileSystem: s.fs,
			path:       "/target/path/",
		}

		// Mock for CopyToFile
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()
		s.mockClient.EXPECT().
			CopyV2(mock.Anything).
			Return(&files.RelocationResult{}, nil).
			Once()

		// Mock for Delete - fails
		s.mockClient.EXPECT().
			DeleteV2(mock.Anything).
			Return(nil, errors.New("delete failed")).
			Once()

		newFile, err := s.file.MoveToLocation(targetLocation)
		s.Require().Error(err)
		s.NotNil(newFile) // File was copied even though delete failed
	})
}

func (s *FileTestSuite) TestCopyToFileSameFile() {
	s.Run("Success - copy to same dropbox filesystem", func() {
		targetFile := &File{
			location: s.location,
			path:     "/test/path/copy.txt",
		}

		// Mock target exists check (doesn't exist)
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		// Mock copy
		s.mockClient.EXPECT().
			CopyV2(mock.MatchedBy(func(arg *files.RelocationArg) bool {
				return arg.FromPath == "/test/path/file.txt" && arg.ToPath == "/test/path/copy.txt"
			})).
			Return(&files.RelocationResult{}, nil).
			Once()

		err := s.file.CopyToFile(targetFile)
		s.Require().NoError(err)
	})
}

func (s *FileTestSuite) TestDeleteWithTempFiles() {
	s.Run("Success - delete cleans up temp files", func() {
		file := &File{location: s.location, path: "/test/path/deletetemp.txt"}

		// First write to create temp file
		_, err := file.Write([]byte("content"))
		s.Require().NoError(err)

		// Delete calls Close first, which uploads
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		// Then delete
		s.mockClient.EXPECT().
			DeleteV2(mock.Anything).
			Return(&files.DeleteResult{}, nil).
			Once()

		err = file.Delete()
		s.Require().NoError(err)
	})
}

func (s *FileTestSuite) TestCloseWithUploadError() {
	s.Run("Error - upload fails on close", func() {
		file := &File{location: s.location, path: "/test/path/closeerror.txt"}

		_, err := file.Write([]byte("content"))
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(nil, errors.New("upload failed")).
			Once()

		err = file.Close()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestUploadToDropboxSeekError() {
	s.Run("Error - client retrieval fails during upload", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/noclient.txt"}

		_, err := file.Write([]byte("content"))
		s.Require().NoError(err)

		err = file.Close()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestCopyToFileClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/source.txt"}

		targetFile := &File{location: loc, path: "/test/path/target.txt"}

		err := file.CopyToFile(targetFile)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestDeleteClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/file.txt"}

		err := file.Delete()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestEnsureTempFileReadClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/file.txt"}

		buf := make([]byte, 10)
		_, err := file.Read(buf)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestSizeClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/file.txt"}

		_, err := file.Size()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestLastModifiedClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/file.txt"}

		_, err := file.LastModified()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestExistsClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/file.txt"}

		_, err := file.Exists()
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestCopyToLocationClientError() {
	s.Run("Error - client retrieval fails", func() {
		fsNoClient := &FileSystem{options: NewOptions()}
		loc := &Location{fileSystem: fsNoClient, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/file.txt"}

		targetLoc := &Location{fileSystem: fsNoClient, path: "/target/"}

		_, err := file.CopyToLocation(targetLoc)
		s.Require().Error(err)
	})
}

func (s *FileTestSuite) TestWriteMultipleWrites() {
	s.Run("Success - multiple sequential writes", func() {
		file := &File{location: s.location, path: "/test/path/multiwrite.txt"}

		// First write
		n1, err := file.Write([]byte("hello"))
		s.Require().NoError(err)
		s.Equal(5, n1)

		// Second write appends
		n2, err := file.Write([]byte(" world"))
		s.Require().NoError(err)
		s.Equal(6, n2)

		// Mock upload for close
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err = file.Close()
		s.Require().NoError(err)
	})
}

func (s *FileTestSuite) TestSeekAfterWrite() {
	s.Run("Success - seek and read after write", func() {
		file := &File{location: s.location, path: "/test/path/seekwrite.txt"}

		// Write content
		_, err := file.Write([]byte("hello world"))
		s.Require().NoError(err)

		// Seek to middle
		pos, err := file.Seek(6, io.SeekStart)
		s.Require().NoError(err)
		s.Equal(int64(6), pos)

		// Read from current position
		buf := make([]byte, 5)
		n, err := file.Read(buf)
		s.Require().NoError(err)
		s.Equal(5, n)
		s.Equal("world", string(buf))

		// Mock upload for close
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		_ = file.Close()
	})
}

func (s *FileTestSuite) TestChunkedUpload() {
	s.Run("Success - chunked upload for file exceeding threshold", func() {
		// Create a filesystem with a very low threshold to trigger chunked upload
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10 // 10 bytes threshold
		opts.ChunkSize = 5            // 5 byte chunks
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/chunked.txt"}

		// Write content larger than threshold (>10 bytes)
		content := "this is a long content that exceeds the threshold"
		_, err := file.Write([]byte(content))
		s.Require().NoError(err)

		// Mock chunked upload sequence
		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(&files.UploadSessionStartResult{SessionId: "test-session"}, nil).
			Once()

		// Multiple append calls for chunks
		s.mockClient.EXPECT().
			UploadSessionAppendV2(mock.Anything, mock.Anything).
			Return(nil).
			Times(9) // (50 - 5) / 5 = 9 more chunks

		s.mockClient.EXPECT().
			UploadSessionFinish(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err = file.Close()
		s.Require().NoError(err)
	})

	s.Run("Error - upload session start fails", func() {
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/chunkedfail.txt"}

		content := "content exceeding threshold"
		_, err := file.Write([]byte(content))
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(nil, errors.New("session start failed")).
			Once()

		err = file.Close()
		s.Require().Error(err)
		s.Contains(err.Error(), "session start failed")
	})

	s.Run("Error - upload session append fails", func() {
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/appendfail.txt"}

		content := "content exceeding threshold"
		_, err := file.Write([]byte(content))
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(&files.UploadSessionStartResult{SessionId: "test-session"}, nil).
			Once()

		s.mockClient.EXPECT().
			UploadSessionAppendV2(mock.Anything, mock.Anything).
			Return(errors.New("append failed")).
			Once()

		err = file.Close()
		s.Require().Error(err)
		s.Contains(err.Error(), "append failed")
	})

	s.Run("Error - upload session finish fails", func() {
		opts := NewOptions()
		opts.MaxSimpleUploadSize = 10
		opts.ChunkSize = 5
		fs := &FileSystem{
			client:  s.mockClient,
			options: opts,
		}
		loc := &Location{fileSystem: fs, path: "/test/path/"}
		file := &File{location: loc, path: "/test/path/finishfail.txt"}

		// Use exactly 15 bytes: first chunk 5, then 2 appends (5+5), total 15
		content := "123456789012345"
		_, err := file.Write([]byte(content))
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			UploadSessionStart(mock.Anything, mock.Anything).
			Return(&files.UploadSessionStartResult{SessionId: "test-session"}, nil).
			Once()

		// 15 bytes total, 5 byte chunks: start reads 5, then 2 appends (5+5)
		s.mockClient.EXPECT().
			UploadSessionAppendV2(mock.Anything, mock.Anything).
			Return(nil).
			Times(2)

		s.mockClient.EXPECT().
			UploadSessionFinish(mock.Anything, mock.Anything).
			Return(nil, errors.New("finish failed")).
			Once()

		err = file.Close()
		s.Require().Error(err)
		s.Contains(err.Error(), "finish failed")
	})
}

func (s *FileTestSuite) TestClientRetrievalErrors() {
	fsNoClient := &FileSystem{
		options: NewOptions(),
	}
	loc := &Location{
		fileSystem: fsNoClient,
		path:       "/test/path/",
	}
	file := &File{
		location: loc,
		path:     "/test/path/file.txt",
	}

	tests := []struct {
		name      string
		operation func() error
	}{
		{
			name: "Exists fails without client",
			operation: func() error {
				_, err := file.Exists()
				return err
			},
		},
		{
			name: "Touch fails without client",
			operation: func() error {
				return file.Touch()
			},
		},
		{
			name: "Delete fails without client",
			operation: func() error {
				return file.Delete()
			},
		},
		{
			name: "CopyToFile fails without client",
			operation: func() error {
				targetFile := &File{location: loc, path: "/test/path/target.txt"}
				return file.CopyToFile(targetFile)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.operation()
			s.Require().Error(err)
		})
	}
}

func (s *FileTestSuite) TestMetadataErrors() {
	tests := []struct {
		name      string
		setupMock func()
		operation func() error
	}{
		{
			name: "Exists - non-not-found error",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(nil, errors.New("internal server error")).
					Once()
			},
			operation: func() error {
				_, err := s.file.Exists()
				return err
			},
		},
		{
			name: "Size - client error",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(nil, errors.New("api error")).
					Once()
			},
			operation: func() error {
				_, err := s.file.Size()
				return err
			},
		},
		{
			name: "LastModified - client error",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(nil, errors.New("api error")).
					Once()
			},
			operation: func() error {
				_, err := s.file.LastModified()
				return err
			},
		},
		{
			name: "LastModified - not a file",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(&files.FolderMetadata{}, nil).
					Once()
			},
			operation: func() error {
				_, err := s.file.LastModified()
				return err
			},
		},
		{
			name: "Read - download error",
			setupMock: func() {
				s.mockClient.EXPECT().
					Download(mock.Anything).
					Return(nil, nil, errors.New("download failed")).
					Once()
			},
			operation: func() error {
				buf := make([]byte, 100)
				_, err := s.file.Read(buf)
				return err
			},
		},
		{
			name: "Touch - upload fails for new file",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(nil, errors.New("path/not_found")).
					Once()
				s.mockClient.EXPECT().
					Upload(mock.Anything, mock.Anything).
					Return(nil, errors.New("upload failed")).
					Once()
			},
			operation: func() error {
				return s.file.Touch()
			},
		},
		{
			name: "CopyToFile - copy API error",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(nil, errors.New("path/not_found")).
					Once()
				s.mockClient.EXPECT().
					CopyV2(mock.Anything).
					Return(nil, errors.New("copy failed")).
					Once()
			},
			operation: func() error {
				targetFile := &File{location: s.location, path: "/test/path/target.txt"}
				return s.file.CopyToFile(targetFile)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			tt.setupMock()
			err := tt.operation()
			s.Require().Error(err)
		})
	}
}

func (s *FileTestSuite) TestSeekErrors() {
	tests := []struct {
		name      string
		setup     func(*File)
		setupMock func()
		offset    int64
		whence    int
	}{
		{
			name: "Invalid whence",
			setup: func(f *File) {
				_, _ = f.Write([]byte("test"))
			},
			offset: 0,
			whence: 99,
		},
		{
			name: "Negative position",
			setup: func(f *File) {
				_, _ = f.Write([]byte("test"))
			},
			offset: -10,
			whence: io.SeekStart,
		},
		{
			name: "Seek on non-existent file",
			setupMock: func() {
				s.mockClient.EXPECT().
					GetMetadata(mock.Anything).
					Return(nil, errors.New("path/not_found")).
					Once()
			},
			offset: 0,
			whence: io.SeekStart,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			file := &File{
				location: s.location,
				path:     "/test/path/file.txt",
			}
			if tt.setup != nil {
				tt.setup(file)
			}
			if tt.setupMock != nil {
				tt.setupMock()
			}

			pos, err := file.Seek(tt.offset, tt.whence)
			s.Require().Error(err)
			s.Equal(int64(0), pos)
		})
	}
}

func (s *FileTestSuite) TestWriteErrors() {
	s.Run("Error - temp dir creation fails", func() {
		fs := &FileSystem{
			client: s.mockClient,
			options: Options{
				TempDir: "/nonexistent/path/that/does/not/exist",
			},
		}
		loc := &Location{
			fileSystem: fs,
			path:       "/test/path/",
		}
		file := &File{
			location: loc,
			path:     "/test/path/file.txt",
		}

		_, err := file.Write([]byte("test"))
		s.Require().Error(err)
	})
}

func TestFileTestSuite(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
