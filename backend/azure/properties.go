package azure

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
)

// BlobProperties holds a subset of information returned by Blob.GetProperties(..)
type BlobProperties struct {
	// Size holds the size of the blob.
	Size *int64

	// LastModified holds the last modified time.Time
	LastModified *time.Time

	// Metadata holds the Azure metadata
	Metadata map[string]*string
}

// NewBlobProperties creates a new BlobProperties from an azblob.BlobGetPropertiesResponse
func NewBlobProperties(azureProps blob.GetPropertiesResponse) *BlobProperties {
	lastModified := azureProps.LastModified
	return &BlobProperties{
		LastModified: lastModified,
		Metadata:     azureProps.Metadata,
		Size:         azureProps.ContentLength,
	}
}
