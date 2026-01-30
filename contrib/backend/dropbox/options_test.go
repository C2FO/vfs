package dropbox

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"
)

type OptionsTestSuite struct {
	suite.Suite
}

func (s *OptionsTestSuite) TestNewOptions() {
	s.Run("Returns default options", func() {
		opts := NewOptions()

		s.Equal(int64(4*1024*1024), opts.ChunkSize)
		s.Equal(os.TempDir(), opts.TempDir)
		s.Empty(opts.AccessToken)
	})
}

func (s *OptionsTestSuite) TestOptionsFields() {
	tests := []struct {
		name         string
		setupOptions func() Options
		validate     func(*OptionsTestSuite, Options)
	}{
		{
			name: "Custom access token",
			setupOptions: func() Options {
				opts := NewOptions()
				opts.AccessToken = "test-token"
				return opts
			},
			validate: func(s *OptionsTestSuite, opts Options) {
				s.Equal("test-token", opts.AccessToken)
			},
		},
		{
			name: "Custom chunk size",
			setupOptions: func() Options {
				opts := NewOptions()
				opts.ChunkSize = 8 * 1024 * 1024
				return opts
			},
			validate: func(s *OptionsTestSuite, opts Options) {
				s.Equal(int64(8*1024*1024), opts.ChunkSize)
			},
		},
		{
			name: "Custom temp dir",
			setupOptions: func() Options {
				opts := NewOptions()
				opts.TempDir = "/custom/temp"
				return opts
			},
			validate: func(s *OptionsTestSuite, opts Options) {
				s.Equal("/custom/temp", opts.TempDir)
			},
		},
		{
			name: "All custom values",
			setupOptions: func() Options {
				return Options{
					AccessToken: "custom-token",
					ChunkSize:   10 * 1024 * 1024,
					TempDir:     "/tmp/custom",
				}
			},
			validate: func(s *OptionsTestSuite, opts Options) {
				s.Equal("custom-token", opts.AccessToken)
				s.Equal(int64(10*1024*1024), opts.ChunkSize)
				s.Equal("/tmp/custom", opts.TempDir)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			opts := tt.setupOptions()
			tt.validate(s, opts)
		})
	}
}

func TestOptionsTestSuite(t *testing.T) {
	suite.Run(t, new(OptionsTestSuite))
}
