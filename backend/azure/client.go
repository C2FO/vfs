package azure

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

// The Client interface contains methods that perform specific operations to Azure Blob Storage.  This interface is
// here so we can write mocks over the actual functionality.
type Client interface {
	// Properties should return a BlobProperties struct for the blob specified by locationURI, and filePath.  If the
	// blob is not found an error should be returned.
	Properties(locationURI, filePath string) (*BlobProperties, error)

	// SetMetadata should add the metadata specified by the parameter metadata for the blob specified by the parameter
	// file.
	SetMetadata(file vfs.File, metadata map[string]*string) error

	// Upload should create or update the blob specified by the file parameter with the contents of the content
	// parameter
	Upload(file vfs.File, content io.ReadSeeker) error

	// Download should return a reader for the blob specified by the file parameter
	Download(file vfs.File) (io.ReadCloser, error)

	// Copy should copy the file specified by srcFile to the file specified by tgtFile
	Copy(srcFile vfs.File, tgtFile vfs.File) error

	// List should return a listing for the specified location. Listings should include the full path for the file.
	List(l vfs.Location) ([]string, error)

	// Delete should delete the file specified by the parameter file.
	Delete(file vfs.File) error

	// DeleteAllVersions should delete all versions of the file specified by the parameter file.
	DeleteAllVersions(file vfs.File) error
}

// DefaultClient is the main implementation that actually makes the calls to Azure Blob Storage
type DefaultClient struct {
	credential any
}

// NewClient initializes a new DefaultClient
func NewClient(options *Options) (*DefaultClient, error) {
	credential, err := options.Credential()
	if err != nil {
		return nil, err
	}

	return &DefaultClient{credential}, nil
}

func (a *DefaultClient) newContainerClient(containerURL string) (*container.Client, error) {
	switch cred := a.credential.(type) {
	case azcore.TokenCredential:
		return container.NewClient(containerURL, cred, nil)
	case *container.SharedKeyCredential:
		return container.NewClientWithSharedKeyCredential(containerURL, cred, nil)
	default:
		return container.NewClientWithNoCredential(containerURL, nil)
	}
}

// Properties fetches the properties for the blob specified by the parameters containerURI and filePath
func (a *DefaultClient) Properties(containerURI, filePath string) (*BlobProperties, error) {
	cli, err := a.newContainerClient(containerURI)
	if err != nil {
		return nil, err
	}

	if filePath == "" {
		// this is only used to check for the existence of a container so we don't care about anything but the
		// error
		_, err := cli.GetProperties(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(filePath))
	resp, err := blobURL.GetProperties(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return NewBlobProperties(resp), nil
}

// Upload uploads a new file to Azure Blob Storage
func (a *DefaultClient) Upload(file vfs.File, content io.ReadSeeker) error {
	cli, err := a.newContainerClient(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}
	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(file.Path()))
	body, ok := content.(io.ReadSeekCloser)
	if !ok {
		body = streaming.NopCloser(content)
	}
	_, err = blobURL.Upload(context.Background(), body, nil)
	return err
}

// SetMetadata sets the given metadata for the blob
func (a *DefaultClient) SetMetadata(file vfs.File, metadata map[string]*string) error {
	cli, err := a.newContainerClient(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}
	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(file.Path()))
	_, err = blobURL.SetMetadata(context.Background(), metadata, nil)
	return err
}

// Download returns an io.ReadCloser for the given vfs.File
func (a *DefaultClient) Download(file vfs.File) (io.ReadCloser, error) {
	cli, err := a.newContainerClient(file.Location().(*Location).ContainerURL())
	if err != nil {
		return nil, err
	}
	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(file.Path()))
	get, err := blobURL.DownloadStream(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return get.Body, nil
}

// Copy copies srcFile to the destination tgtFile within Azure Blob Storage.  Note that in the case where we get
// encoded spaces in the file name (i.e. %20) the '%' must be encoded or the copy command will return a not found
// error.
func (a *DefaultClient) Copy(srcFile, tgtFile vfs.File) error {
	// Can't use url.PathEscape here since that will escape everything (even the directory separators)
	srcURL := strings.Replace(srcFile.URI(), "%", "%25", -1)

	tgtURL := tgtFile.Location().(*Location).ContainerURL()

	cli, err := a.newContainerClient(tgtURL)
	if err != nil {
		return err
	}
	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(tgtFile.Path()))
	ctx := context.Background()
	resp, err := blobURL.StartCopyFromURL(ctx, srcURL, nil)
	if err != nil {
		return err
	}

	for *resp.CopyStatus == blob.CopyStatusTypePending {
		time.Sleep(2 * time.Second)
	}

	if *resp.CopyStatus == blob.CopyStatusTypeSuccess {
		return nil
	}

	return errors.New("copy failed")
}

// List will return a listing of the contents of the given location.  Each item in the list will contain the full key
// as specified by the azure blob (including the virtual 'path').
func (a *DefaultClient) List(l vfs.Location) ([]string, error) {
	cli, err := a.newContainerClient(l.(*Location).ContainerURL())
	if err != nil {
		return []string{}, err
	}

	pager := cli.NewListBlobsHierarchyPager("/", &container.ListBlobsHierarchyOptions{
		Prefix:  to.Ptr(utils.RemoveLeadingSlash(l.Path())),
		Include: container.ListBlobsInclude{Metadata: true, Tags: true},
	})
	ctx := context.Background()
	var list []string
	for pager.More() {
		listBlob, err := pager.NextPage(ctx)
		if err != nil {
			return []string{}, err
		}

		for i := range listBlob.ListBlobsHierarchySegmentResponse.Segment.BlobItems {
			list = append(list, *listBlob.ListBlobsHierarchySegmentResponse.Segment.BlobItems[i].Name)
		}
	}
	return list, nil
}

// Delete deletes the given file from Azure Blob Storage.
func (a *DefaultClient) Delete(file vfs.File) error {
	cli, err := a.newContainerClient(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}
	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(file.Path()))
	_, err = blobURL.Delete(context.Background(), nil)
	return err
}

// DeleteAllVersions deletes all the versions of the given file from Azure Blob Storage.
// First the file blob is deleted, then each version of the blob is deleted.
// If soft deletion is enabled for blobs in the storage account, each version will be marked for deletion and will be
// permanently deleted by Azure as per the soft deletion policy.
func (a *DefaultClient) DeleteAllVersions(file vfs.File) error {
	cli, err := a.newContainerClient(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}
	blobURL := cli.NewBlockBlobClient(utils.RemoveLeadingSlash(file.Path()))

	versions, err := a.getBlobVersions(cli, utils.RemoveLeadingSlash(file.Path()))
	if err != nil {
		return err
	}

	for _, version := range versions {
		// Delete a specific version
		cli, err := blobURL.WithVersionID(*version)
		if err != nil {
			return err
		}
		_, err = cli.Delete(context.Background(), nil)
		if err != nil {
			return err
		}
	}

	return err
}

func (a *DefaultClient) getBlobVersions(cli *container.Client, blobName string) ([]*string, error) {
	ctx := context.Background()
	pager := cli.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix:  &blobName,
		Include: container.ListBlobsInclude{Versions: true},
	})
	var versions []*string
	for pager.More() {
		listBlob, err := pager.NextPage(ctx)
		if err != nil {
			return []*string{}, err
		}

		for i := range listBlob.ListBlobsFlatSegmentResponse.Segment.BlobItems {
			versions = append(versions, listBlob.ListBlobsFlatSegmentResponse.Segment.BlobItems[i].VersionID)
		}
	}
	return versions, nil
}
