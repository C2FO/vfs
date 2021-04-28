package azure

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-pipeline-go/pipeline"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/utils"
)

// The Client interface contains methods that perform specific operations to Azure Blob Storage.  This interface is
// here so we can write mocks over the actual functionality.
type Client interface {
	// Properties should return a BlobProperties struct for the blob specified by locationURI, and filePath.  If the
	// blob is not found an error should be returned.
	Properties(locationURI, filePath string) (*BlobProperties, error)

	// SetMetadata should add the metadata specified by the parameter metadata for the blob specified by the parameter
	// file.
	SetMetadata(file vfs.File, metadata map[string]string) error

	// Upload should create or update the blob specified by the file parameter with the contents of the content
	// parameter
	Upload(file vfs.File, content io.ReadSeeker) error

	// Download should return a reader for the blob specified by the file parameter
	Download(file vfs.File) (io.ReadCloser, error)

	// Copy should copy the file specified by srcFile to the file specified by tgtFile
	Copy(srcFile vfs.File, tgtFile vfs.File) error

	// List should return a listing for the specified location.  Listings should include the full path for the file.
	List(l vfs.Location) ([]string, error)

	// Delete should delete the file specified by the parameter file.
	Delete(file vfs.File) error
}

// ClientImpl is the main implementation that actually makes the calls to Azure Blob Storage
type ClientImpl struct {
	pipeline pipeline.Pipeline
}

// NewClient initializes a new ClientImpl
func NewClient(accountName, accountKey string) (*ClientImpl, error) {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, err
	}

	// This configures the client to use the default retry policy.  The default policy uses exponential backoff with
	// maxRetries = 4.  If this behavior needs to be changed, add the Retry member to azblob.PipelineOptions.  For
	// more information on azure retry policies see https://pkg.go.dev/github.com/Azure/azure-storage-blob-go/azblob#RetryOptions
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	return &ClientImpl{pipeline: p}, nil
}

// Properties fetches the properties for the blob specified by the parameters containerURI and filePath
func (a *ClientImpl) Properties(containerURI, filePath string) (*BlobProperties, error) {
	URL, err := url.Parse(containerURI)
	if err != nil {
		return nil, err
	}
	containerURL := azblob.NewContainerURL(*URL, a.pipeline)

	if filePath == "" {
		// this is only used to check for the existence of a container so we don't care about anything but the
		// error
		_, err := containerURL.GetProperties(context.Background(), azblob.LeaseAccessConditions{})
		if err != nil {
			return nil, err
		}
		return nil, nil
	}

	blobURL := containerURL.NewBlockBlobURL(utils.RemoveLeadingSlash(filePath))
	resp, err := blobURL.GetProperties(context.Background(), azblob.BlobAccessConditions{}, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return nil, err
	}
	return NewBlobProperties(resp), nil
}

// Upload uploads a new file to Azure Blob Storage
func (a *ClientImpl) Upload(file vfs.File, content io.ReadSeeker) error {
	URL, err := url.Parse(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}

	containerURL := azblob.NewContainerURL(*URL, a.pipeline)
	blobURL := containerURL.NewBlockBlobURL(utils.RemoveLeadingSlash(file.Path()))
	_, err = blobURL.Upload(context.Background(), content, azblob.BlobHTTPHeaders{}, azblob.Metadata{}, azblob.BlobAccessConditions{}, azblob.DefaultAccessTier, nil, azblob.ClientProvidedKeyOptions{})
	return err
}

// SetMetadata sets the given metadata for the blob
func (a *ClientImpl) SetMetadata(file vfs.File, metadata map[string]string) error {
	URL, err := url.Parse(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}

	containerURL := azblob.NewContainerURL(*URL, a.pipeline)
	blobURL := containerURL.NewBlockBlobURL(utils.RemoveLeadingSlash(file.Path()))
	_, err = blobURL.SetMetadata(context.Background(), metadata, azblob.BlobAccessConditions{}, azblob.ClientProvidedKeyOptions{})
	return err
}

// Download returns an io.ReadCloser for the given vfs.File
func (a *ClientImpl) Download(file vfs.File) (io.ReadCloser, error) {
	URL, err := url.Parse(file.Location().(*Location).ContainerURL())
	if err != nil {
		return nil, err
	}

	containerURL := azblob.NewContainerURL(*URL, a.pipeline)
	blobURL := containerURL.NewBlockBlobURL(utils.RemoveLeadingSlash(file.Path()))
	get, err := blobURL.Download(context.Background(), 0, 0, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		return nil, err
	}
	return get.Body(azblob.RetryReaderOptions{}), nil
}

// Copy copies srcFile to the destination tgtFile within Azure Blob Storage.  Note that in the case where we get
// encoded spaces in the file name (i.e. %20) the '%' must be encoded or the copy command will return a not found
// error.
func (a *ClientImpl) Copy(srcFile vfs.File, tgtFile vfs.File) error {
	// Can't use url.PathEscape here since that will escape everything (even the directory separators)
	srcURL, err := url.Parse(strings.Replace(srcFile.URI(), "%", "%25", -1))
	if err != nil {
		return err
	}

	tgtURL, err := url.Parse(tgtFile.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}

	containerURL := azblob.NewContainerURL(*tgtURL, a.pipeline)
	blobURL := containerURL.NewBlockBlobURL(utils.RemoveLeadingSlash(tgtFile.Path()))
	ctx := context.Background()
	resp, err := blobURL.StartCopyFromURL(ctx, *srcURL, azblob.Metadata{}, azblob.ModifiedAccessConditions{}, azblob.BlobAccessConditions{}, azblob.DefaultAccessTier, nil)
	if err != nil {
		return err
	}

	for resp.CopyStatus() == azblob.CopyStatusPending {
		time.Sleep(2 * time.Second)
	}

	if resp.CopyStatus() == azblob.CopyStatusSuccess {
		return nil
	}

	return fmt.Errorf("Copy failed.  ERROR[%s]", resp.ErrorCode())
}

// List will return a listing of the contents of the given location.  Each item in the list will contain the full key
// as specified by the azure blob (incliding the virtual 'path').
func (a *ClientImpl) List(l vfs.Location) ([]string, error) {
	URL, err := url.Parse(l.(*Location).ContainerURL())
	if err != nil {
		return []string{}, err
	}

	containerURL := azblob.NewContainerURL(*URL, a.pipeline)
	ctx := context.Background()
	var list []string
	for marker := (azblob.Marker{}); marker.NotDone(); {
		listBlob, err := containerURL.ListBlobsHierarchySegment(ctx, marker, "/", azblob.ListBlobsSegmentOptions{Prefix: utils.RemoveLeadingSlash(l.Path())})
		if err != nil {
			log.Fatal("Unable to list blobs.  error: " + err.Error())
		}

		marker = listBlob.NextMarker

		for _, blobInfo := range listBlob.Segment.BlobItems {
			list = append(list, blobInfo.Name)
		}
	}
	return list, nil
}

// Delete deletes the given file from Azure Blob Storage.
func (a *ClientImpl) Delete(file vfs.File) error {
	URL, err := url.Parse(file.Location().(*Location).ContainerURL())
	if err != nil {
		return err
	}

	containerURL := azblob.NewContainerURL(*URL, a.pipeline)
	blobURL := containerURL.NewBlockBlobURL(utils.RemoveLeadingSlash(file.Path()))
	_, err = blobURL.Delete(context.Background(), azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
	return err
}
