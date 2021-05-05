package azure

import (
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

type MockTokenCredentialFactory struct{}

func (f *MockTokenCredentialFactory) New(tenantID, clientID, clientSecret, azureEnvName string) (azblob.TokenCredential, error) {
	return azblob.NewTokenCredential("aaa", func(credential azblob.TokenCredential) time.Duration { return time.Second * 1 }), nil
}
