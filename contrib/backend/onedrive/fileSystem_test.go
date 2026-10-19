package onedrive

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/stretchr/testify/suite"

	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"

	"github.com/c2fo/vfs/contrib/backend/onedrive/mocks"
)

type FileSystemTestSuite struct {
	suite.Suite
}

func TestFileSystemTestSuite(t *testing.T) {
	suite.Run(t, new(FileSystemTestSuite))
}

func (s *FileSystemTestSuite) SetupTest() {
	for _, env := range []string{
		"VFS_ONEDRIVE_ACCESS_TOKEN", "VFS_ONEDRIVE_TENANT_ID", "VFS_ONEDRIVE_CLIENT_ID", "VFS_ONEDRIVE_CLIENT_SECRET",
	} {
		s.T().Setenv(env, "")
	}
}

func (s *FileSystemTestSuite) TestIdentity() {
	fs := NewFileSystem()
	s.Equal("onedrive", fs.Scheme())
	s.Equal("OneDrive", fs.Name())
	s.NotNil(fs.Retry())
}

func (s *FileSystemTestSuite) TestRegistered() {
	fs := backend.Backend(Scheme)
	s.IsType(&FileSystem{}, fs)
}

func (s *FileSystemTestSuite) TestNewFileSystemOptions() {
	httpClient := &http.Client{}
	callback := func(context.Context, string) error { return nil }

	tests := []struct {
		name   string
		opt    options.NewFileSystemOption[FileSystem]
		optKey string
		check  func(o Options)
	}{
		{name: "access token", opt: WithAccessToken("tok"), optKey: optionNameAccessToken,
			check: func(o Options) { s.Equal("tok", o.AccessToken) }},
		{name: "client credentials", opt: WithClientCredentials("t", "c", "x"), optKey: optionNameClientCredentials,
			check: func(o Options) {
				s.Equal([]string{"t", "c", "x"}, []string{o.TenantID, o.ClientID, o.ClientSecret})
			}},
		{name: "default credential", opt: WithDefaultCredential(), optKey: optionNameDefaultCredential,
			check: func(o Options) { s.True(o.UseDefaultCredential) }},
		{name: "device code", opt: WithDeviceCode("c", "consumers", callback), optKey: optionNameDeviceCode,
			check: func(o Options) {
				s.True(o.UseDeviceCode)
				s.Equal("consumers", o.TenantID)
				s.NotNil(o.DeviceCodeCallback)
			}},
		{name: "device code defaults tenant", opt: WithDeviceCode("c", "", nil), optKey: optionNameDeviceCode,
			check: func(o Options) { s.Equal("common", o.TenantID) }},
		{name: "interactive browser", opt: WithInteractiveBrowser("c", "org", "http://localhost:9999"), optKey: optionNameInteractiveBrowser,
			check: func(o Options) {
				s.True(o.UseInteractiveBrowser)
				s.Equal("org", o.TenantID)
				s.Equal("http://localhost:9999", o.RedirectURL)
			}},
		{name: "interactive browser defaults", opt: WithInteractiveBrowser("c", "", ""), optKey: optionNameInteractiveBrowser,
			check: func(o Options) {
				s.Equal("common", o.TenantID)
				s.Equal("http://localhost", o.RedirectURL)
			}},
		{name: "user drive", opt: WithUserDrive("u@example.com"), optKey: optionNameUserDrive,
			check: func(o Options) { s.Equal("u@example.com", o.UserID) }},
		{name: "site drive", opt: WithSiteDrive("site", "drive"), optKey: optionNameSiteDrive,
			check: func(o Options) { s.Equal([]string{"site", "drive"}, []string{o.SiteID, o.DriveID}) }},
		{name: "drive ID", opt: WithDriveID("drive"), optKey: optionNameDriveID,
			check: func(o Options) { s.Equal("drive", o.DriveID) }},
		{name: "chunk size", opt: WithChunkSize(10 << 20), optKey: optionNameChunkSize,
			check: func(o Options) { s.Equal(int64(10<<20), o.ChunkSize) }},
		{name: "temp dir", opt: WithTempDir("/tmp/custom"), optKey: optionNameTempDir,
			check: func(o Options) { s.Equal("/tmp/custom", o.TempDir) }},
		{name: "retry count", opt: WithRetryCount(5), optKey: optionNameRetryCount,
			check: func(o Options) { s.Equal(5, o.RetryCount) }},
		{name: "base URL", opt: WithBaseURL("http://localhost:8080/v1.0"), optKey: optionNameBaseURL,
			check: func(o Options) { s.Equal("http://localhost:8080/v1.0", o.BaseURL) }},
		{name: "HTTP client", opt: WithHTTPClient(httpClient), optKey: optionNameHTTPClient,
			check: func(o Options) { s.Same(httpClient, o.HTTPClient) }},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.optKey, tt.opt.NewFileSystemOptionName())
			tt.check(NewFileSystem(tt.opt).options)
		})
	}
}

func (s *FileSystemTestSuite) TestDefaults() {
	o := NewFileSystem().options
	s.Equal(int64(5<<20), o.ChunkSize)
	s.Equal(3, o.RetryCount)
	s.Equal(DefaultBaseURL, o.BaseURL)
	s.NotEmpty(o.TempDir)
}

func (s *FileSystemTestSuite) TestWithClient() {
	client := mocks.NewClient(s.T())
	opt := WithClient(client)
	s.Equal(optionNameClient, opt.NewFileSystemOptionName())

	got, err := NewFileSystem(opt).Client()
	s.Require().NoError(err)
	s.Same(client, got)
}

func (s *FileSystemTestSuite) TestCredentialSelection() {
	tests := []struct {
		name          string
		opts          []options.NewFileSystemOption[FileSystem]
		env           map[string]string
		expectedType  azcore.TokenCredential
		expectedError error
	}{
		{name: "no credentials", expectedError: errCredentialsRequired},
		{name: "access token", opts: []options.NewFileSystemOption[FileSystem]{WithAccessToken("tok")},
			expectedType: staticTokenCredential("")},
		{name: "client credentials", opts: []options.NewFileSystemOption[FileSystem]{WithClientCredentials("tenant", "client", "secret")},
			expectedType: &azidentity.ClientSecretCredential{}},
		{name: "device code", opts: []options.NewFileSystemOption[FileSystem]{
			WithDeviceCode("client", "", func(context.Context, string) error { return nil }),
		}, expectedType: &azidentity.DeviceCodeCredential{}},
		{name: "interactive browser", opts: []options.NewFileSystemOption[FileSystem]{WithInteractiveBrowser("client", "", "")},
			expectedType: &azidentity.InteractiveBrowserCredential{}},
		{name: "access token wins over other options", opts: []options.NewFileSystemOption[FileSystem]{
			WithDeviceCode("client", "", nil), WithAccessToken("tok"),
		}, expectedType: staticTokenCredential("")},
		{name: "access token from environment", env: map[string]string{"VFS_ONEDRIVE_ACCESS_TOKEN": "env-tok"},
			expectedType: staticTokenCredential("")},
		{name: "client credentials from environment", env: map[string]string{
			"VFS_ONEDRIVE_TENANT_ID": "t", "VFS_ONEDRIVE_CLIENT_ID": "c", "VFS_ONEDRIVE_CLIENT_SECRET": "x",
		}, expectedType: &azidentity.ClientSecretCredential{}},
		{name: "partial environment is not enough", env: map[string]string{"VFS_ONEDRIVE_CLIENT_ID": "c"},
			expectedError: errCredentialsRequired},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			for k, v := range tt.env {
				s.T().Setenv(k, v)
			}

			cred, err := NewFileSystem(tt.opts...).credential()
			if tt.expectedError != nil {
				s.Require().ErrorIs(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.IsType(tt.expectedType, cred)
		})
	}
}

func (s *FileSystemTestSuite) TestCredentialErrors() {
	_, err := NewFileSystem(WithClientCredentials("not a tenant!", "c", "x")).credential()
	s.Require().ErrorContains(err, "client secret credential")

	_, err = NewFileSystem(WithDeviceCode("c", "not a tenant!", nil)).credential()
	s.Require().ErrorContains(err, "device code credential")

	_, err = NewFileSystem(WithInteractiveBrowser("c", "not a tenant!", "")).credential()
	s.Require().ErrorContains(err, "interactive browser credential")
}

func (s *FileSystemTestSuite) TestDefaultCredential() {
	cred, err := NewFileSystem(WithDefaultCredential()).credential()
	if err != nil {
		s.Require().ErrorContains(err, "default credential")
		return
	}
	s.IsType(&azidentity.DefaultAzureCredential{}, cred)
}

func (s *FileSystemTestSuite) TestStaticTokenCredential() {
	token, err := staticTokenCredential("abc").GetToken(context.Background(), policy.TokenRequestOptions{})
	s.Require().NoError(err)
	s.Equal("abc", token.Token)
	s.False(token.ExpiresOn.IsZero())
}

func (s *FileSystemTestSuite) TestClientIsCreatedOnceConcurrently() {
	fs := NewFileSystem(WithAccessToken("tok"))

	clients := make([]Client, 10)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() {
			c, err := fs.Client()
			s.NoError(err)
			clients[i] = c
		})
	}
	wg.Wait()

	for _, c := range clients[1:] {
		s.Same(clients[0], c)
	}
}

func (s *FileSystemTestSuite) TestClientWithoutCredentials() {
	_, err := NewFileSystem().Client()
	s.Require().ErrorIs(err, errCredentialsRequired)
}

func (s *FileSystemTestSuite) TestNewLocation() {
	fs := NewFileSystem()

	tests := []struct {
		name          string
		fs            *FileSystem
		path          string
		expected      string
		expectedError error
	}{
		{name: "valid path", fs: fs, path: "/path/to/folder/", expected: "/path/to/folder/"},
		{name: "root path", fs: fs, path: "/", expected: "/"},
		{name: "empty path", fs: fs, path: "", expectedError: errAuthorityAndNameRequired},
		{name: "nil filesystem", path: "/path/", expectedError: errFileSystemRequired},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			loc, err := tt.fs.NewLocation("", tt.path)
			if tt.expectedError != nil {
				s.Require().ErrorIs(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal(tt.expected, loc.Path())
		})
	}

	s.Run("relative path rejected", func() {
		_, err := fs.NewLocation("", "relative/")
		s.Require().Error(err)
	})
}

func (s *FileSystemTestSuite) TestNewFile() {
	fs := NewFileSystem()

	tests := []struct {
		name          string
		fs            *FileSystem
		path          string
		expectedError error
	}{
		{name: "valid path", fs: fs, path: "/path/to/file.txt"},
		{name: "empty path", fs: fs, path: "", expectedError: errAuthorityAndNameRequired},
		{name: "nil filesystem", path: "/path/to/file.txt", expectedError: errFileSystemRequired},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			file, err := tt.fs.NewFile("", tt.path)
			if tt.expectedError != nil {
				s.Require().ErrorIs(err, tt.expectedError)
				return
			}
			s.Require().NoError(err)
			s.Equal("file.txt", file.Name())
			s.Equal(tt.path, file.Path())
		})
	}

	s.Run("directory path rejected", func() {
		_, err := fs.NewFile("", "/path/to/")
		s.Require().Error(err)
	})
}
