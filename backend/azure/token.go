package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// TokenCredentialFactory creates azure.TokenCredentials.  This function is provided to allow for mocking in unit tests.
type TokenCredentialFactory func(tenantID, clientID, clientSecret string) (azcore.TokenCredential, error)

// DefaultTokenCredentialFactory knows how to make azblob.TokenCredential structs for OAuth authentication
func DefaultTokenCredentialFactory(tenantID, clientID, clientSecret string) (azcore.TokenCredential, error) {
	if clientID != "" || clientSecret != "" {
		return azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	}
	return azidentity.NewEnvironmentCredential(nil)
}
