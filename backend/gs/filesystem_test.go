package gs

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/suite"
	"google.golang.org/api/option"
)

type fileysystemSuite struct {
	suite.Suite
}

func TestFilesystemSuite(t *testing.T) {
	suite.Run(t, new(fileysystemSuite))
}

func (s *fileysystemSuite) TestNewFile() {
	testCases := []struct {
		description       string
		volume            string
		filename          string
		expectedErrString string
		nilFS             bool
	}{
		{
			description:       "nil filesystem",
			volume:            "bucket",
			filename:          "/file.txt",
			expectedErrString: "non-nil gs.FileSystem pointer is required",
			nilFS:             true,
		},
		{
			description:       "empty volume",
			volume:            "",
			filename:          "/file.txt",
			expectedErrString: "non-empty strings for Bucket and Key are required",
		},
		{
			description:       "empty filename",
			volume:            "bucket",
			filename:          "",
			expectedErrString: "non-empty strings for Bucket and Key are required",
		},
		{
			description:       "invalid filename",
			volume:            "bucket",
			filename:          "/file.txt/",
			expectedErrString: "absolute file path is invalid - must include leading slash and may not include trailing slash",
		},
		{
			description:       "valid filename",
			volume:            "bucket",
			filename:          "/file.txt",
			expectedErrString: "",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.description, func() {
			fs := &FileSystem{}
			if tc.nilFS {
				fs = nil
			}
			_, err := fs.NewFile(tc.volume, tc.filename)
			if tc.expectedErrString == "" {
				s.NoError(err)
				return
			}
			s.EqualError(err, tc.expectedErrString)
		})
	}
}

func (s *fileysystemSuite) TestNewLocation() {
	testCases := []struct {
		description       string
		volume            string
		name              string
		expectedErrString string
		nilFS             bool
	}{
		{
			description:       "nil filesystem",
			volume:            "bucket",
			name:              "/",
			expectedErrString: "non-nil gs.FileSystem pointer is required",
			nilFS:             true,
		},
		{
			description:       "empty volume",
			volume:            "",
			name:              "/",
			expectedErrString: "non-empty strings for bucket and key are required",
		},
		{
			description:       "empty name",
			volume:            "bucket",
			name:              "",
			expectedErrString: "non-empty strings for bucket and key are required",
		},
		{
			description:       "invalid name",
			volume:            "bucket",
			name:              "/path",
			expectedErrString: "absolute location path is invalid - must include leading and trailing slashes",
		},
		{
			description:       "valid name",
			volume:            "bucket",
			name:              "/path/",
			expectedErrString: "",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.description, func() {
			fs := &FileSystem{}
			if tc.nilFS {
				fs = nil
			}
			_, err := fs.NewLocation(tc.volume, tc.name)
			if tc.expectedErrString == "" {
				s.NoError(err)
				return
			}
			s.EqualError(err, tc.expectedErrString)
		})
	}
}

func (s *fileysystemSuite) TestName() {
	fs := &FileSystem{}
	s.Equal(name, fs.Name())
}

func (s *fileysystemSuite) TestRetry() {
	sentinel := errors.New("sentinel")
	fs := &FileSystem{
		options: Options{
			Retry: func(wrapped func() error) error {
				return sentinel
			},
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

func (s *fileysystemSuite) TestClient() {
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
				s.Error(err)
			} else {
				s.NoError(err)
			}

			if tc.expectNotNil {
				s.NotNil(client)
			} else {
				s.Nil(client)
			}
		})
	}
}

func (s *fileysystemSuite) TestWithContext() {
	fs := &FileSystem{}
	ctx := context.Background()
	fs = fs.WithContext(ctx)
	s.Equal(ctx, fs.ctx)
}
