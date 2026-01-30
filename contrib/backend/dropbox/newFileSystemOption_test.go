package dropbox

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/options"

	"github.com/c2fo/vfs/contrib/backend/dropbox/mocks"
)

type NewFileSystemOptionTestSuite struct {
	suite.Suite
}

func (s *NewFileSystemOptionTestSuite) TestOptions() {
	mockClient := mocks.NewClient(s.T())

	tests := []struct {
		name         string
		opt          options.NewFileSystemOption[FileSystem]
		expectedName string
		validate     func(*FileSystem)
	}{
		{
			name:         "WithAccessToken",
			opt:          WithAccessToken("test-token"),
			expectedName: optionNameAccessToken,
			validate: func(fs *FileSystem) {
				s.Equal("test-token", fs.options.AccessToken)
			},
		},
		{
			name:         "WithChunkSize",
			opt:          WithChunkSize(8 * 1024 * 1024),
			expectedName: optionNameChunkSize,
			validate: func(fs *FileSystem) {
				s.Equal(int64(8*1024*1024), fs.options.ChunkSize)
			},
		},
		{
			name:         "WithTempDir",
			opt:          WithTempDir("/custom/temp"),
			expectedName: optionNameTempDir,
			validate: func(fs *FileSystem) {
				s.Equal("/custom/temp", fs.options.TempDir)
			},
		},
		{
			name:         "WithClient",
			opt:          WithClient(mockClient),
			expectedName: optionNameClient,
			validate: func(fs *FileSystem) {
				s.Equal(mockClient, fs.client)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			fs := &FileSystem{options: NewOptions()}

			tt.opt.Apply(fs)
			tt.validate(fs)

			s.Equal(tt.expectedName, tt.opt.NewFileSystemOptionName())
		})
	}
}

func TestNewFileSystemOptionTestSuite(t *testing.T) {
	suite.Run(t, new(NewFileSystemOptionTestSuite))
}
