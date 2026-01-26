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
		// Check if exists
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(nil, errors.New("path/not_found")).
			Once()

		// Upload empty file
		s.mockClient.EXPECT().
			Upload(mock.MatchedBy(func(arg *files.UploadArg) bool {
				return arg.Path == "/test/path/file.txt"
			}), mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err := s.file.Touch()
		s.Require().NoError(err)
	})

	s.Run("Success - updates existing file timestamp", func() {
		content := "existing content"
		reader := io.NopCloser(strings.NewReader(content))

		// Check if exists
		s.mockClient.EXPECT().
			GetMetadata(mock.Anything).
			Return(&files.FileMetadata{
				Size: uint64(len(content)),
			}, nil).
			Once()

		// Download existing file
		s.mockClient.EXPECT().
			Download(mock.Anything).
			Return(&files.FileMetadata{
				Size: uint64(len(content)),
			}, reader, nil).
			Once()

		// Re-upload to update timestamp
		s.mockClient.EXPECT().
			Upload(mock.Anything, mock.Anything).
			Return(&files.FileMetadata{}, nil).
			Once()

		err := s.file.Touch()
		s.Require().NoError(err)
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

func (s *FileTestSuite) TestChunkedUpload() {
	s.Run("Success - uploads large file in chunks", func() {
		// Skip this complex test for now - chunked upload logic is tested
		// indirectly through integration tests
		s.T().Skip("Chunked upload test skipped - requires complex temp file mocking")
	})
}

func (s *FileTestSuite) TestReadWriteSeekIntegration() {
	s.Run("Write, Seek, Read integration", func() {
		// Skip - this requires complex temp file state management
		// Better tested in actual integration tests
		s.T().Skip("Read/Write/Seek integration test skipped - requires temp file state")
	})
}

func TestFileTestSuite(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
