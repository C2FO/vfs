package azure

import (
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

// BlobProperties holds a subset of information returned by Blob.GetProperties(..)
type BlobProperties struct {
	// Size holds the size of the blob.
	Size uint64

	// LastModified holds the last modified time.Time
	LastModified *time.Time

	// Metadata holds the Azure metadata
	Metadata map[string]string
}

// NewBlobProperties creates a new BlobProperties from an azblob.BlobGetPropertiesResponse
func NewBlobProperties(azureProps *azblob.BlobGetPropertiesResponse) *BlobProperties {
	lastModified := azureProps.LastModified()
	return &BlobProperties{
		LastModified: &lastModified,
		Metadata:     azureProps.NewMetadata(),
		Size:         uint64(azureProps.ContentLength()),
	}
}
