package dropbox

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/contrib/backend/dropbox/mocks"
)

type FileSystemTestSuite struct {
	suite.Suite
	mockClient *mocks.Client
	fs         *FileSystem
}

func (s *FileSystemTestSuite) SetupTest() {
	s.mockClient = mocks.NewClient(s.T())
	s.fs = NewFileSystem(
		WithClient(s.mockClient),
		WithAccessToken("test-token"),
	)
}

func (s *FileSystemTestSuite) TestNewFileSystem() {
	tests := []struct {
		name string
		opts []any
	}{
		{
			name: "Default options",
			opts: nil,
		},
		{
			name: "With access token",
			opts: []any{WithAccessToken("test-token")},
		},
		{
			name: "With chunk size",
			opts: []any{WithChunkSize(8 * 1024 * 1024)},
		},
		{
			name: "With multiple options",
			opts: []any{
				WithAccessToken("test-token"),
				WithChunkSize(8 * 1024 * 1024),
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			fs := NewFileSystem()
			s.NotNil(fs)
			s.Equal(Scheme, fs.Scheme())
			s.Equal(name, fs.Name())
		})
	}
}

func (s *FileSystemTestSuite) TestScheme() {
	s.Equal("dbx", s.fs.Scheme())
}

func (s *FileSystemTestSuite) TestName() {
	s.Equal("Dropbox", s.fs.Name())
}

func (s *FileSystemTestSuite) TestClient() {
	s.Run("Returns existing client", func() {
		client, err := s.fs.Client()
		s.Require().NoError(err)
		s.NotNil(client)
		s.Equal(s.mockClient, client)
	})

	s.Run("Returns error when no access token", func() {
		// Temporarily unset env var to test error case
		oldToken := os.Getenv("VFS_DROPBOX_ACCESS_TOKEN")
		_ = os.Unsetenv("VFS_DROPBOX_ACCESS_TOKEN")
		defer func() {
			if oldToken != "" {
				_ = os.Setenv("VFS_DROPBOX_ACCESS_TOKEN", oldToken)
			}
		}()

		fs := NewFileSystem()
		client, err := fs.Client()
		s.Require().Error(err)
		s.Nil(client)
		s.Contains(err.Error(), "access token is required")
	})

	s.Run("Creates client on first call", func() {
		fs := NewFileSystem(WithAccessToken("test-token"))
		client, err := fs.Client()
		s.Require().NoError(err)
		s.NotNil(client)
	})
}

func (s *FileSystemTestSuite) TestNewFile() {
	tests := []struct {
		name          string
		authority     string
		filePath      string
		expectedError string
	}{
		{
			name:          "Valid file path",
			authority:     "",
			filePath:      "/path/to/file.txt",
			expectedError: "",
		},
		{
			name:          "Valid file path with authority",
			authority:     "user",
			filePath:      "/file.txt",
			expectedError: "",
		},
		{
			name:          "Empty file path",
			authority:     "",
			filePath:      "",
			expectedError: "non-empty string for name is required",
		},
		{
			name:          "Relative file path",
			authority:     "",
			filePath:      "relative/path.txt",
			expectedError: "absolute",
		},
		{
			name:          "Nil filesystem",
			authority:     "",
			filePath:      "/file.txt",
			expectedError: "non-nil",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var fs *FileSystem
			if tt.name == "Nil filesystem" {
				fs = nil
			} else {
				fs = s.fs
			}

			file, err := fs.NewFile(tt.authority, tt.filePath)

			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Nil(file)
				s.Contains(err.Error(), tt.expectedError)
			} else {
				s.Require().NoError(err)
				s.NotNil(file)
				s.IsType(&File{}, file)
			}
		})
	}
}

func (s *FileSystemTestSuite) TestNewLocation() {
	tests := []struct {
		name          string
		authority     string
		locPath       string
		expectedError string
	}{
		{
			name:          "Valid location path",
			authority:     "",
			locPath:       "/path/to/folder/",
			expectedError: "",
		},
		{
			name:          "Valid location path with clean",
			authority:     "",
			locPath:       "/path/to/folder/",
			expectedError: "",
		},
		{
			name:          "Root location",
			authority:     "",
			locPath:       "/",
			expectedError: "",
		},
		{
			name:          "Empty location path",
			authority:     "",
			locPath:       "",
			expectedError: "non-empty string for name is required",
		},
		{
			name:          "Relative location path",
			authority:     "",
			locPath:       "relative/path/",
			expectedError: "absolute",
		},
		{
			name:          "Nil filesystem",
			authority:     "",
			locPath:       "/path/",
			expectedError: "non-nil",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var fs *FileSystem
			if tt.name == "Nil filesystem" {
				fs = nil
			} else {
				fs = s.fs
			}

			loc, err := fs.NewLocation(tt.authority, tt.locPath)

			if tt.expectedError != "" {
				s.Require().Error(err)
				s.Nil(loc)
				s.Contains(err.Error(), tt.expectedError)
			} else {
				s.Require().NoError(err)
				s.NotNil(loc)
				s.IsType(&Location{}, loc)
			}
		})
	}
}

func (s *FileSystemTestSuite) TestRetry() {
	retry := s.fs.Retry()
	s.NotNil(retry)

	// Test that retry is a no-op
	called := false
	err := retry(func() error {
		called = true
		return nil
	})
	s.Require().NoError(err)
	s.True(called)
}

func TestFileSystemTestSuite(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}
