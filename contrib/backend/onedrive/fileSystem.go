package onedrive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the file system type.
const Scheme = "onedrive"

const name = "OneDrive"

var (
	errFileSystemRequired       = errors.New("non-nil onedrive.FileSystem pointer is required")
	errAuthorityAndNameRequired = errors.New("non-empty string for name is required")
	errCredentialsRequired      = errors.New("credentials are required for OneDrive authentication")
)

// FileSystem implements vfs.FileSystem for OneDrive.
type FileSystem struct {
	client   Client
	clientMu sync.Mutex
	options  Options
}

// NewFileSystem initializer for FileSystem struct.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: NewOptions(),
	}

	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry returns the default no-op retrier.
//
// Deprecated: This method is deprecated and will be removed in a future release.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the OneDrive implementation of vfs.File.
func (fs *FileSystem) NewFile(authorityStr, name string, opts ...options.NewFileOption) (vfs.File, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if name == "" {
		return nil, errAuthorityAndNameRequired
	}

	if err := utils.ValidateAbsoluteFilePath(name); err != nil {
		return nil, err
	}

	// get location path
	absLocPath := utils.EnsureTrailingSlash(path.Dir(name))
	loc, err := fs.NewLocation(authorityStr, absLocPath)
	if err != nil {
		return nil, err
	}

	filename := path.Base(name)
	return loc.NewFile(filename, opts...)
}

// NewLocation function returns the OneDrive implementation of vfs.Location.
func (fs *FileSystem) NewLocation(authorityStr, name string) (vfs.Location, error) {
	if fs == nil {
		return nil, errFileSystemRequired
	}

	if name == "" {
		return nil, errAuthorityAndNameRequired
	}

	if err := utils.ValidateAbsoluteLocationPath(name); err != nil {
		return nil, err
	}

	// OneDrive doesn't use authority (single namespace per drive)
	// But we still create an authority object for consistency
	auth, err := authority.NewAuthority(utils.RemoveTrailingSlash(authorityStr))
	if err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		path:       utils.EnsureTrailingSlash(path.Clean(name)),
		authority:  auth,
	}, nil
}

// Name returns "OneDrive"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme returns "onedrive" as the initial part of a file URI ie: onedrive://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the underlying OneDrive client, creating it on first use.
func (fs *FileSystem) Client() (Client, error) {
	fs.clientMu.Lock()
	defer fs.clientMu.Unlock()

	if fs.client == nil {
		cred, err := fs.credential()
		if err != nil {
			return nil, err
		}
		fs.client = newGraphClient(cred, &fs.options)
	}

	return fs.client, nil
}

// credential builds the token credential selected by the options, falling back to the
// VFS_ONEDRIVE_ACCESS_TOKEN and VFS_ONEDRIVE_{TENANT_ID,CLIENT_ID,CLIENT_SECRET} environment variables.
func (fs *FileSystem) credential() (azcore.TokenCredential, error) {
	opts := fs.options

	switch {
	case opts.AccessToken != "":
		return staticTokenCredential(opts.AccessToken), nil
	case opts.TenantID != "" && opts.ClientID != "" && opts.ClientSecret != "":
		return newClientSecretCredential(opts.TenantID, opts.ClientID, opts.ClientSecret)
	case opts.UseDeviceCode || opts.UseInteractiveBrowser:
		return interactiveCredential(&opts)
	case opts.UseDefaultCredential:
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("onedrive: creating default credential: %w", err)
		}
		return cred, nil
	}

	if token := os.Getenv("VFS_ONEDRIVE_ACCESS_TOKEN"); token != "" {
		return staticTokenCredential(token), nil
	}

	tenantID := os.Getenv("VFS_ONEDRIVE_TENANT_ID")
	clientID := os.Getenv("VFS_ONEDRIVE_CLIENT_ID")
	clientSecret := os.Getenv("VFS_ONEDRIVE_CLIENT_SECRET")
	if tenantID != "" && clientID != "" && clientSecret != "" {
		return newClientSecretCredential(tenantID, clientID, clientSecret)
	}

	return nil, errCredentialsRequired
}

// interactiveCredential builds a device code or interactive browser credential, which prompt the
// user to sign in on first use.
func interactiveCredential(opts *Options) (azcore.TokenCredential, error) {
	if opts.UseDeviceCode {
		deviceOpts := &azidentity.DeviceCodeCredentialOptions{
			TenantID: tenantOrCommon(opts.TenantID),
			ClientID: opts.ClientID,
		}
		if opts.DeviceCodeCallback != nil {
			callback := opts.DeviceCodeCallback
			deviceOpts.UserPrompt = func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
				return callback(ctx, msg.Message)
			}
		}
		cred, err := azidentity.NewDeviceCodeCredential(deviceOpts)
		if err != nil {
			return nil, fmt.Errorf("onedrive: creating device code credential: %w", err)
		}
		return cred, nil
	}

	cred, err := azidentity.NewInteractiveBrowserCredential(&azidentity.InteractiveBrowserCredentialOptions{
		TenantID:    tenantOrCommon(opts.TenantID),
		ClientID:    opts.ClientID,
		RedirectURL: opts.RedirectURL,
	})
	if err != nil {
		return nil, fmt.Errorf("onedrive: creating interactive browser credential: %w", err)
	}
	return cred, nil
}

func newClientSecretCredential(tenantID, clientID, clientSecret string) (azcore.TokenCredential, error) {
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, fmt.Errorf("onedrive: creating client secret credential: %w", err)
	}
	return cred, nil
}

func tenantOrCommon(tenantID string) string {
	if tenantID == "" {
		return "common"
	}
	return tenantID
}

// staticTokenCredential is an azcore.TokenCredential for a caller-supplied access token.
// The token is never refreshed; callers are responsible for supplying a valid one.
type staticTokenCredential string

// GetToken implements azcore.TokenCredential.
func (s staticTokenCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: string(s), ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func init() {
	// Register a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}
