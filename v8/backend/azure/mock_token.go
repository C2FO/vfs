package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
)

// MockTokenCredentialFactory creates a new azcore.TokenCredential struct
func MockTokenCredentialFactory(_, _, _ string) (azcore.TokenCredential, error) {
	return &fake.TokenCredential{}, nil
}
