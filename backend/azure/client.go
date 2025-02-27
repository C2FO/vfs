package azure

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

type ContainerClient interface {
	GetProperties(ctx context.Context, o *container.GetPropertiesOptions) (container.GetPropertiesResponse, error)
	NewListBlobsFlatPager(o *container.ListBlobsFlatOptions) *runtime.Pager[container.ListBlobsFlatResponse]
	NewListBlobsHierarchyPager(delimiter string, o *container.ListBlobsHierarchyOptions) *runtime.Pager[container.ListBlobsHierarchyResponse]
}

type BlockBlobClient interface {
	GetProperties(ctx context.Context, o *blob.GetPropertiesOptions) (blob.GetPropertiesResponse, error)
	SetMetadata(ctx context.Context, metadata map[string]*string, o *blob.SetMetadataOptions) (blob.SetMetadataResponse, error)
	Upload(ctx context.Context, body io.ReadSeekCloser, options *blockblob.UploadOptions) (blockblob.UploadResponse, error)
	DownloadStream(ctx context.Context, o *blob.DownloadStreamOptions) (blob.DownloadStreamResponse, error)
	StartCopyFromURL(ctx context.Context, copySource string, o *blob.StartCopyFromURLOptions) (blob.StartCopyFromURLResponse, error)
	Delete(ctx context.Context, o *blob.DeleteOptions) (blob.DeleteResponse, error)
}
