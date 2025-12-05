package gs

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/suite"
	"google.golang.org/api/option"

	"github.com/c2fo/vfs/v7/utils"
)

type fileSystemSuite struct {
	suite.Suite
}

func TestFileSystemSuite(t *testing.T) {
	suite.Run(t, new(fileSystemSuite))
}

func (s *fileSystemSuite) TestNewFile() {
	testCases := []struct {
		description string
		volume      string
		filename    string
		expectedErr error
		nilFS       bool
	}{
		{
			description: "nil filesystem",
			volume:      "bucket",
			filename:    "/file.txt",
			expectedErr: errFileSystemRequired,
			nilFS:       true,
		},
		{
			description: "empty volume",
			volume:      "",
			filename:    "/file.txt",
			expectedErr: errAuthorityAndPathRequired,
		},
		{
			description: "empty filename",
			volume:      "bucket",
			filename:    "",
			expectedErr: errAuthorityAndPathRequired,
		},
		{
			description: "invalid filename",
			volume:      "bucket",
			filename:    "/file.txt/",
			expectedErr: utils.ErrBadAbsFilePath,
		},
		{
			description: "valid filename",
			volume:      "bucket",
			filename:    "/file.txt",
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.description, func() {
			fs := &FileSystem{}
			if tc.nilFS {
				fs = nil
			}
			_, err := fs.NewFile(tc.volume, tc.filename)
			if tc.expectedErr == nil {
				s.Require().NoError(err)
				return
			}
			s.Require().ErrorIs(err, tc.expectedErr)
		})
	}
}

func (s *fileSystemSuite) TestNewLocation() {
	testCases := []struct {
		description string
		volume      string
		name        string
		expectedErr error
		nilFS       bool
	}{
		{
			description: "nil filesystem",
			volume:      "bucket",
			name:        "/",
			expectedErr: errFileSystemRequired,
			nilFS:       true,
		},
		{
			description: "empty volume",
			volume:      "",
			name:        "/",
			expectedErr: errAuthorityAndPathRequired,
		},
		{
			description: "empty name",
			volume:      "bucket",
			name:        "",
			expectedErr: errAuthorityAndPathRequired,
		},
		{
			description: "invalid name",
			volume:      "bucket",
			name:        "/path",
			expectedErr: utils.ErrBadAbsLocationPath,
		},
		{
			description: "valid name",
			volume:      "bucket",
			name:        "/path/",
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.description, func() {
			fs := &FileSystem{}
			if tc.nilFS {
				fs = nil
			}
			_, err := fs.NewLocation(tc.volume, tc.name)
			if tc.expectedErr == nil {
				s.Require().NoError(err)
				return
			}
			s.Require().ErrorIs(err, tc.expectedErr)
		})
	}
}

func (s *fileSystemSuite) TestName() {
	fs := &FileSystem{}
	s.Equal(name, fs.Name())
}

func (s *fileSystemSuite) TestRetry() {
	sentinel := errors.New("sentinel")
	fs := &FileSystem{
		retryer: func(wrapped func() error) error {
			return sentinel
		},
	}
	s.Equal(sentinel, fs.Retry()(nil))
}

type mockClientCreatorWithError struct{}

func (c *mockClientCreatorWithError) NewClient(ctx context.Context, opts ...option.ClientOption) (*storage.Client, error) {
	return nil, errors.New("mock error")
}

type mockClientCreator struct{}

func (c *mockClientCreator) NewClient(ctx context.Context, opts ...option.ClientOption) (*storage.Client, error) {
	return &storage.Client{}, nil
}

func (s *fileSystemSuite) TestClient() {
	testCases := []struct {
		name         string
		setup        func() *FileSystem
		expectError  bool
		expectNotNil bool
	}{
		{
			name: "With predefined client",
			setup: func() *FileSystem {
				return &FileSystem{
					client: &storage.Client{},
				}
			},
			expectError:  false,
			expectNotNil: true,
		},
		{
			name: "New FileSystem without predefined client",
			setup: func() *FileSystem {
				return &FileSystem{
					clientCreator: &mockClientCreator{},
				}
			},
			expectError:  false,
			expectNotNil: true,
		},
		{
			name: "New FileSystem with error",
			setup: func() *FileSystem {
				return &FileSystem{
					clientCreator: &mockClientCreatorWithError{},
				}
			},
			expectError:  true,
			expectNotNil: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			fs := tc.setup()

			client, err := fs.Client()

			if tc.expectError {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}

			if tc.expectNotNil {
				s.NotNil(client)
			} else {
				s.Nil(client)
			}
		})
	}
}

func (s *fileSystemSuite) TestWithContext() {
	fs := &FileSystem{}
	ctx := s.T().Context()
	fs = fs.WithContext(ctx)
	s.Equal(ctx, fs.ctx)
}

func (s *fileSystemSuite) TestNewFileSystem() {
	fs := NewFileSystem()
	s.NotNil(fs, "Should return a non-nil pointer to the new file system")

	// test with options
	newFS := NewFileSystem(WithOptions(Options{APIKey: "123"}))
	s.NotNil(newFS, "Should return a new fileSystem for gs")
	s.Equal("123", newFS.options.APIKey, "Should set APIKey name to 123")

	// test with client
	client := &storage.Client{}
	newFS = NewFileSystem(WithClient(client))
	s.NotNil(newFS, "Should return a new fileSystem for gs")
	s.Equal(client, newFS.client, "Should set client to azureMock")
}
