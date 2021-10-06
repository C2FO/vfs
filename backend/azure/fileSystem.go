package azure

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the scheme for the azure implementation
const Scheme = "https"

// Name defines the name for the azure implementation
const Name = "azure"

const errNilFileSystemReceiver = "azure.FileSystem receiver pointer must be non-nil"

// FileSystem implements the vfs.FileSystem interface for Azure Blob Storage
type FileSystem struct {
	options *Options
	client  Client
}

// NewFileSystem creates a new default FileSystem.  This will set the options options.AccountName and
// options.AccountKey with the env variables AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_ACCESS_KEY respectively.
func NewFileSystem() *FileSystem {
	return &FileSystem{options: NewOptions()}
}

// WithOptions allows the caller to override the default options
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem {
	azureOpts, _ := opts.(Options)
	fs.options = &azureOpts
	return fs
}

// WithClient allows the caller to specify a specific client to be used
func (fs *FileSystem) WithClient(client Client) *FileSystem {
	fs.client = client
	return fs
}

// Client returns a Client to perform operations against Azure Blob Storage
func (fs *FileSystem) Client() (Client, error) {
	if fs.client == nil {
		client, err := NewClient(fs.options)
		fs.client = client
		return fs.client, err
	}
	return fs.client, nil
}

// NewFile returns the azure implementation of vfs.File
func (fs *FileSystem) NewFile(volume, absFilePath string) (vfs.File, error) {
	if fs == nil {
		return nil, errors.New(errNilFileSystemReceiver)
	}

	if volume == "" || absFilePath == "" {
		return nil, errors.New("non-empty strings for container and path are required")
	}

	if err := utils.ValidateAbsoluteFilePath(absFilePath); err != nil {
		return nil, err
	}

	return &File{
		fileSystem: fs,
		container:  volume,
		name:       path.Clean(absFilePath),
	}, nil
}

// NewLocation returns the azure implementation of vfs.Location
func (fs *FileSystem) NewLocation(volume, absLocPath string) (vfs.Location, error) {
	if fs == nil {
		return nil, errors.New(errNilFileSystemReceiver)
	}

	if volume == "" || absLocPath == "" {
		return nil, errors.New("non-empty strings for container and path are required")
	}

	if err := utils.ValidateAbsoluteLocationPath(absLocPath); err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: fs,
		container:  volume,
		path:       path.Clean(absLocPath),
	}, nil
}

// Name returns "azure"
func (fs *FileSystem) Name() string {
	return Name
}

// Scheme returns "https" as the initial part of the URI i.e. https://..
func (fs *FileSystem) Scheme() string {
	return Scheme
}

// Host returns the host portion of the URI.  For azure this consists of <account_name>.blob.core.windows.net.
func (fs *FileSystem) Host() string {
	return fmt.Sprintf("%s.blob.core.windows.net", fs.options.AccountName)
}

// Retry returns the default retry function.  This is overridable via the WithOptions function.
func (fs *FileSystem) Retry() vfs.Retry {
	if fs.options.RetryFunc != nil {
		return fs.options.RetryFunc
	}
	return vfs.DefaultRetryer()
}

func init() {
	// registers a default FileSystem
	backend.Register(Scheme, NewFileSystem())
}

// ParsePath is a utility function used by vfssiple to separate the host from the path.  The first parameter returned
// is the host and the second parameter is the path.
func ParsePath(p string) (host, pth string, err error) {
	if p == "/" {
		return "", "", errors.New("no container specified for Azure path")
	}
	l := strings.Split(p, string(os.PathSeparator))
	return l[1], utils.EnsureTrailingSlash(utils.EnsureLeadingSlash(path.Join(l[2:]...))), nil
}

// IsValidURI us a utility function used by vfssimple to determine if the given URI is a valid Azure URI
func IsValidURI(u *url.URL) bool {
	r := regexp.MustCompile(`.*\.blob\.core\.windows\.net`)

	if u.Scheme == Scheme && r.MatchString(u.Host) {
		return true
	}
	return false
}
