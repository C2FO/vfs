package gdrive

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"

	"github.com/c2fo/vfs/contrib/backend/gdrive/mocks"
)

type FileSystemTestSuite struct {
	suite.Suite
}

func TestFileSystemTestSuite(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}

func (s *FileSystemTestSuite) SetupTest() {
	s.T().Setenv("VFS_GDRIVE_ACCESS_TOKEN", "")
	s.T().Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/credentials.json")
}

func (s *FileSystemTestSuite) TestIdentity() {
	fs := NewFileSystem()
	s.Equal("gdrive", fs.Scheme())
	s.Equal("Google Drive", fs.Name())
	s.NotNil(fs.Retry())
}

func (s *FileSystemTestSuite) TestRegistered() {
	s.IsType(&FileSystem{}, backend.Backend(Scheme))
}

func (s *FileSystemTestSuite) TestOptions() {
	httpClient := &http.Client{}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"})
	callback := func(context.Context, string, string) error { return nil }
	mock := mocks.NewClient(s.T())

	tests := []struct {
		name   string
		opt    options.NewFileSystemOption[FileSystem]
		optKey string
		check  func(fs *FileSystem)
	}{
		{name: "access token", opt: WithAccessToken("tok"), optKey: optionNameAccessToken,
			check: func(fs *FileSystem) { s.Equal("tok", fs.options.AccessToken) }},
		{name: "token source", opt: WithTokenSource(ts), optKey: optionNameTokenSource,
			check: func(fs *FileSystem) { s.Equal(ts, fs.options.TokenSource) }},
		{name: "service account", opt: WithServiceAccountJSON([]byte("{}")), optKey: optionNameServiceAccountJSON,
			check: func(fs *FileSystem) { s.Equal([]byte("{}"), fs.options.ServiceAccountJSON) }},
		{name: "default credentials", opt: WithDefaultCredentials(), optKey: optionNameDefaultCredentials,
			check: func(fs *FileSystem) { s.True(fs.options.UseDefaultCredentials) }},
		{name: "device code", opt: WithDeviceCode("id", "secret", callback), optKey: optionNameDeviceCode,
			check: func(fs *FileSystem) {
				s.True(fs.options.DeviceCode)
				s.Equal("id", fs.options.ClientID)
				s.Equal("secret", fs.options.ClientSecret)
				s.NotNil(fs.options.DeviceCodeCallback)
			}},
		{name: "scope", opt: WithScope(ScopeDriveFile), optKey: optionNameScope,
			check: func(fs *FileSystem) { s.Equal(ScopeDriveFile, fs.options.Scope) }},
		{name: "drive id", opt: WithDriveID("d1"), optKey: optionNameDriveID,
			check: func(fs *FileSystem) { s.Equal("d1", fs.options.DriveID) }},
		{name: "chunk size", opt: WithChunkSize(1), optKey: optionNameChunkSize,
			check: func(fs *FileSystem) { s.EqualValues(1, fs.options.ChunkSize) }},
		{name: "temp dir", opt: WithTempDir("/tmp/x"), optKey: optionNameTempDir,
			check: func(fs *FileSystem) { s.Equal("/tmp/x", fs.options.TempDir) }},
		{name: "retry count", opt: WithRetryCount(9), optKey: optionNameRetryCount,
			check: func(fs *FileSystem) { s.Equal(9, fs.options.RetryCount) }},
		{name: "base url", opt: WithBaseURL("http://x"), optKey: optionNameBaseURL,
			check: func(fs *FileSystem) { s.Equal("http://x", fs.options.BaseURL) }},
		{name: "http client", opt: WithHTTPClient(httpClient), optKey: optionNameHTTPClient,
			check: func(fs *FileSystem) { s.Same(httpClient, fs.options.HTTPClient) }},
		{name: "trash on delete", opt: WithTrashOnDelete(), optKey: optionNameTrashOnDelete,
			check: func(fs *FileSystem) { s.True(fs.options.TrashOnDelete) }},
		{name: "export mime type", opt: WithExportMimeType("application/pdf"), optKey: optionNameExportMimeType,
			check: func(fs *FileSystem) { s.Equal("application/pdf", fs.options.ExportMimeType) }},
		{name: "client", opt: WithClient(mock), optKey: optionNameClient,
			check: func(fs *FileSystem) { s.Same(mock, fs.client) }},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.optKey, tt.opt.NewFileSystemOptionName())
			fs := NewFileSystem(tt.opt)
			tt.check(fs)
		})
	}
}

func (s *FileSystemTestSuite) TestDefaults() {
	o := NewFileSystem().options
	s.Equal(ScopeDrive, o.Scope)
	s.EqualValues(DefaultChunkSize, o.ChunkSize)
	s.Equal(DefaultRetryCount, o.RetryCount)
}

func (s *FileSystemTestSuite) TestNewLocationAndFile() {
	fs := NewFileSystem()

	tests := []struct {
		name        string
		authority   string
		path        string
		file        bool
		expectedErr string
		expectedURI string
	}{
		{name: "location", path: "/a/b/", expectedURI: "gdrive:///a/b/"},
		{name: "shared drive location", authority: "0ABCdrive", path: "/a/", expectedURI: "gdrive://0ABCdrive/a/"},
		{name: "file", path: "/a/b.txt", file: true, expectedURI: "gdrive:///a/b.txt"},
		{name: "empty location", path: "", expectedErr: "non-empty string"},
		{name: "empty file", path: "", file: true, expectedErr: "non-empty string"},
		{name: "relative location", path: "a/b/", expectedErr: "absolute"},
		{name: "relative file", path: "a/b.txt", file: true, expectedErr: "absolute"},
		{name: "file as location", path: "/a/b.txt", expectedErr: "trailing slash"},
		{name: "location as file", path: "/a/b/", file: true, expectedErr: "trailing slash"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var uri string
			var err error
			if tt.file {
				var f interface{ URI() string }
				f, err = fs.NewFile(tt.authority, tt.path)
				if err == nil {
					uri = f.URI()
				}
			} else {
				var l interface{ URI() string }
				l, err = fs.NewLocation(tt.authority, tt.path)
				if err == nil {
					uri = l.URI()
				}
			}

			if tt.expectedErr != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.expectedErr)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expectedURI, uri)
		})
	}
}

func (s *FileSystemTestSuite) TestNilReceiver() {
	var fs *FileSystem
	_, err := fs.NewFile("", "/a")
	s.Require().ErrorIs(err, errFileSystemRequired)
	_, err = fs.NewLocation("", "/a/")
	s.Require().ErrorIs(err, errFileSystemRequired)
}

func (s *FileSystemTestSuite) TestClientPerDriveIsCached() {
	fs := NewFileSystem(WithAccessToken("t"), WithDriveID("default"))

	a, err := fs.Client("")
	s.Require().NoError(err)
	b, err := fs.Client("default")
	s.Require().NoError(err)
	c, err := fs.Client("other")
	s.Require().NoError(err)

	s.Same(a, b, "empty authority maps to the configured default drive")
	s.NotSame(a, c)
	s.Equal("other", c.(*driveClient).rootID)

	again, err := fs.Client("other")
	s.Require().NoError(err)
	s.Same(c, again)
}

func (s *FileSystemTestSuite) TestClientOverride() {
	mock := mocks.NewClient(s.T())
	fs := NewFileSystem(WithClient(mock))

	c, err := fs.Client("anything")
	s.Require().NoError(err)
	s.Same(mock, c)
}

func (s *FileSystemTestSuite) TestCredentialSelection() {
	tests := []struct {
		name        string
		env         string
		opts        []options.NewFileSystemOption[FileSystem]
		expectedErr string
	}{
		{name: "none configured", expectedErr: "credentials are required"},
		{name: "access token", opts: []options.NewFileSystemOption[FileSystem]{WithAccessToken("t")}},
		{name: "environment token", env: "envtoken"},
		{name: "token source", opts: []options.NewFileSystemOption[FileSystem]{
			WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"}))}},
		{name: "bad service account json", opts: []options.NewFileSystemOption[FileSystem]{WithServiceAccountJSON([]byte("nope"))},
			expectedErr: "parsing service account key"},
		{name: "default credentials unavailable", opts: []options.NewFileSystemOption[FileSystem]{WithDefaultCredentials()},
			expectedErr: "finding default credentials"},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.T().Setenv("VFS_GDRIVE_ACCESS_TOKEN", tt.env)
			fs := NewFileSystem(tt.opts...)

			_, err := fs.Client("")
			if tt.expectedErr != "" {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.expectedErr)
				return
			}
			s.Require().NoError(err)
		})
	}
}
