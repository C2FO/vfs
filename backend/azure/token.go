package azure

import (
	"time"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
)

// TokenCredentialFactory is an interface that provides a single factory method to create azure.TokenCredentials.  This
// interface is provided to allow for mocking in unit tests.
type TokenCredentialFactory interface {
	// New creates a new azblob.TokenCredntial struct
	New(tenantID, clientID, clientSecret, azureEnvName string) (azblob.TokenCredential, error)
}

// DefaultTokenCredentialFactory knows how to make azblob.TokenCredential structs for OAuth authentication
type DefaultTokenCredentialFactory struct{}

// New creates a new azblob.TokenCredntial struct
func (f *DefaultTokenCredentialFactory) New(tenantID, clientID, clientSecret, azureEnvName string) (azblob.TokenCredential, error) {
	env, err := azure.EnvironmentFromName(azureEnvName)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, tenantID)
	if err != nil {
		return nil, err
	}

	spt, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.ResourceIdentifiers.Storage)
	if err != nil {
		return nil, err
	}

	initialToken := spt.Token()

	var tokenRefresher = func(credential azblob.TokenCredential) time.Duration {
		err := spt.Refresh()
		if err != nil {
			// Not sure what else to do here except panic...
			// The documentation suggests returning 0 so the token will never attempt to refresh again
			// and then call cancel() on any context in the pipeline that have this credential object
			panic(err)
		}

		token := spt.Token()
		credential.SetToken(token.AccessToken)
		return token.Expires().Sub(time.Now().Add(2 * time.Minute))
	}

	return azblob.NewTokenCredential(initialToken.AccessToken, tokenRefresher), nil
}
