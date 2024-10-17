package azure

import (
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

// MockTokenCredentialFactory creates a new azblob.TokenCredential struct
func MockTokenCredentialFactory(string, string, string, string) (azblob.TokenCredential, error) {
	return azblob.NewTokenCredential("aaa", func(credential azblob.TokenCredential) time.Duration { return time.Second * 1 }), nil
}
