package azure

import (
	"errors"
	"fmt"
	"path"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the scheme for the azure implementation
const Scheme = "az"

// Name defines the name for the azure implementation
const Name = "azure"

const errNilFileSystemReceiver = "azure.FileSystem receiver pointer must be non-nil"

// FileSystem implements the vfs.FileSystem interface for Azure Blob Storage
type FileSystem struct {
	options *Options
	client  *service.Client
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
func (fs *FileSystem) WithClient(client *service.Client) *FileSystem {
	fs.client = client
	return fs
}

// Client returns a Client to perform operations against Azure Blob Storage
func (fs *FileSystem) Client() (*service.Client, error) {
	if fs.client == nil {
		credential, err := fs.options.Credential()
		if err != nil {
			return nil, err
		}

		switch cred := credential.(type) {
		case azcore.TokenCredential:
			fs.client, err = service.NewClient(fs.serviceURL(), cred, nil)
		case *container.SharedKeyCredential:
			fs.client, err = service.NewClientWithSharedKeyCredential(fs.serviceURL(), cred, nil)
		default:
			fs.client, err = service.NewClientWithNoCredential(fs.serviceURL(), nil)
		}
		if err != nil {
			return nil, err
		}
	}
	return fs.client, nil
}

func (fs *FileSystem) serviceURL() string {
	if fs.options.ServiceURL != "" {
		return fs.options.ServiceURL
	}
	if fs.options.AccountName != "" {
		return fmt.Sprintf("https://%s.blob.core.windows.net", fs.options.AccountName)
	}
	return "https://blob.core.windows.net"
}

var (
	containerClientFactory = func(fs *FileSystem, container string) (ContainerClient, error) {
		cli, err := fs.Client()
		if err != nil {
			return nil, err
		}
		return cli.NewContainerClient(container), nil
	}
	blockBlobClientFactory = func(fs *FileSystem, container, path string, versionID *string) (BlockBlobClient, error) {
		cli, err := fs.Client()
		if err != nil {
			return nil, err
		}
		blobCli := cli.NewContainerClient(container).NewBlockBlobClient(path)
		if versionID != nil {
			blobCli, err = blobCli.WithVersionID(*versionID)
			if err != nil {
				return nil, err
			}
		}
		return blobCli, nil
	}
)

// NewFile returns the azure implementation of vfs.File
func (fs *FileSystem) NewFile(volume, absFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
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
		opts:       opts,
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

// Scheme returns "az" as the initial part of the URI i.e. https://..
func (fs *FileSystem) Scheme() string {
	return Scheme
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
