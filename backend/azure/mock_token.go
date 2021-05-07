package azure

import (
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

// MockTokenCredentialFactory knows how to create a "do-nothing" credential used for unit testing
type MockTokenCredentialFactory struct{}

// New creates a new azblob.TokenCredntial struct
func (f *MockTokenCredentialFactory) New(tenantID, clientID, clientSecret, azureEnvName string) (azblob.TokenCredential, error) {
	return azblob.NewTokenCredential("aaa", func(credential azblob.TokenCredential) time.Duration { return time.Second * 1 }), nil
}
