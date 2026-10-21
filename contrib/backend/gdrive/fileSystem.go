package gdrive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the file system type.
const Scheme = "gdrive"

const name = "Google Drive"

var (
	errFileSystemRequired       = errors.New("non-nil gdrive.FileSystem pointer is required")
	errAuthorityAndNameRequired = errors.New("non-empty string for name is required")
	errCredentialsRequired      = errors.New("credentials are required for Google Drive authentication")
)

// FileSystem implements vfs.FileSystem for Google Drive.
//
// A location's authority is a drive ID: empty selects the user's My Drive (or the drive set with
// WithDriveID), anything else is a shared drive ID.
type FileSystem struct {
	client  Client // optional override used for every drive
	clients map[string]Client
	mu      sync.Mutex // guards clients
	authMu  sync.Mutex // guards ts
	ts      oauth2.TokenSource
	options Options
}

// NewFileSystem initializer for FileSystem struct.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		options: NewOptions(),
		clients: map[string]Client{},
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

// NewFile function returns the Google Drive implementation of vfs.File.
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

	absLocPath := utils.EnsureTrailingSlash(path.Dir(name))
	loc, err := fs.NewLocation(authorityStr, absLocPath)
	if err != nil {
		return nil, err
	}

	return loc.NewFile(path.Base(name), opts...)
}

// NewLocation function returns the Google Drive implementation of vfs.Location.
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

// Name returns "Google Drive"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme returns "gdrive" as the initial part of a file URI ie: gdrive://
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Client returns the Client for the given drive ID ("" for the default drive), creating it on first use.
func (fs *FileSystem) Client(driveID string) (Client, error) {
	if fs.client != nil {
		return fs.client, nil
	}

	if driveID == "" {
		driveID = fs.options.DriveID
	}

	if c, ok := fs.cachedClient(driveID); ok {
		return c, nil
	}

	// Obtaining credentials may block on a user completing the device flow; that must not hold fs.mu
	// and stall callers whose drive client already exists.
	ts, err := fs.sharedTokenSource()
	if err != nil {
		return nil, err
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if c, ok := fs.clients[driveID]; ok {
		return c, nil
	}

	c := newDriveClient(fs.httpClient(ts), &fs.options, driveID)
	fs.clients[driveID] = c

	return c, nil
}

func (fs *FileSystem) cachedClient(driveID string) (Client, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	c, ok := fs.clients[driveID]
	return c, ok
}

// sharedTokenSource returns the token source every drive client uses, creating it once.
func (fs *FileSystem) sharedTokenSource() (oauth2.TokenSource, error) {
	fs.authMu.Lock()
	defer fs.authMu.Unlock()

	if fs.ts == nil {
		ts, err := fs.tokenSource()
		if err != nil {
			return nil, err
		}
		fs.ts = ts
	}
	return fs.ts, nil
}

// httpClient builds the authenticated HTTP client shared by every drive: a copy of the configured
// (or default) client with OAuth2 credentials layered over its transport.
func (fs *FileSystem) httpClient(ts oauth2.TokenSource) *http.Client {
	base := fs.options.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}

	c := *base
	c.Transport = &oauth2.Transport{Source: ts, Base: base.Transport}
	return &c
}

// tokenSource selects the OAuth2 token source from the options, falling back to the
// VFS_GDRIVE_ACCESS_TOKEN environment variable.
func (fs *FileSystem) tokenSource() (oauth2.TokenSource, error) {
	opts := &fs.options
	ctx := context.Background()

	switch {
	case opts.TokenSource != nil:
		return opts.TokenSource, nil
	case opts.AccessToken != "":
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: opts.AccessToken}), nil
	case len(opts.ServiceAccountJSON) > 0:
		cfg, err := google.JWTConfigFromJSON(opts.ServiceAccountJSON, opts.Scope)
		if err != nil {
			return nil, fmt.Errorf("gdrive: parsing service account key: %w", err)
		}
		return cfg.TokenSource(ctx), nil
	case opts.UseDefaultCredentials:
		creds, err := google.FindDefaultCredentials(ctx, opts.Scope)
		if err != nil {
			return nil, fmt.Errorf("gdrive: finding default credentials: %w", err)
		}
		return creds.TokenSource, nil
	case opts.DeviceCode:
		return deviceCodeTokenSource(ctx, opts)
	}

	if token := os.Getenv("VFS_GDRIVE_ACCESS_TOKEN"); token != "" {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}), nil
	}

	return nil, errCredentialsRequired
}

// deviceCodeTokenSource runs the OAuth2 device authorization flow, blocking until the user approves.
func deviceCodeTokenSource(ctx context.Context, opts *Options) (oauth2.TokenSource, error) {
	cfg := &oauth2.Config{
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
		Scopes:       []string{opts.Scope},
		Endpoint:     google.Endpoint,
	}

	da, err := cfg.DeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("gdrive: starting device authorization: %w", err)
	}

	if opts.DeviceCodeCallback != nil {
		err = opts.DeviceCodeCallback(ctx, da.VerificationURI, da.UserCode)
	} else {
		_, err = fmt.Printf("To sign in, visit %s and enter code %s\n", da.VerificationURI, da.UserCode)
	}
	if err != nil {
		return nil, fmt.Errorf("gdrive: displaying device code: %w", err)
	}

	tok, err := cfg.DeviceAccessToken(ctx, da)
	if err != nil {
		return nil, fmt.Errorf("gdrive: completing device authorization: %w", err)
	}

	return cfg.TokenSource(ctx, tok), nil
}

func init() {
	backend.Register(Scheme, NewFileSystem())
}
