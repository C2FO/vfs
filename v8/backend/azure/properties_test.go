package azure

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/stretchr/testify/assert"

	"github.com/c2fo/vfs/v7/utils"
)

func TestNewBlobProperties(t *testing.T) {
	// Prepare test data
	lastModified := time.Now()
	contentLength := int64(1024)
	metadata := map[string]*string{
		"key1": utils.Ptr("value1"),
		"key2": utils.Ptr("value2"),
	}

	azureProps := blob.GetPropertiesResponse{
		LastModified:  &lastModified,
		ContentLength: &contentLength,
		Metadata:      metadata,
	}

	// Call the function
	props := NewBlobProperties(azureProps)

	// Assertions
	assert.NotNil(t, props)
	assert.Equal(t, &contentLength, props.Size)
	assert.Equal(t, &lastModified, props.LastModified)
	assert.Equal(t, metadata, props.Metadata)
}
