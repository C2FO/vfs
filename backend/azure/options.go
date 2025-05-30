package azure

import (
	"os"

	"github.com/c2fo/vfs/v7"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// Options contains options necessary for the azure vfs implementation
type Options struct {
	// ServiceURL holds the base URL used for all service requests.
	ServiceURL string

	// AccountName holds the Azure Blob Storage account name for authentication.  This field is required for all
	// authentication types.
	AccountName string

	// AccountKey holds the Azure Blob Storage account key for authentication.  This field is used for shared key
	// authentication.
	AccountKey string

	// TenantID holds the Azure Service Account tenant id for authentication.  This field is used for OAuth token
	// based authentication.
	TenantID string

	// ClientID holds the Azure Service Account client id for authentication.  This field is used for OAuth token
	// based authentication.
	ClientID string

	// ClientSecret holds the Azure Service Account client secret for authentication.  This field is used for OAuth token
	// based authentication.
	ClientSecret string

	// RetryFunc holds the retry function
	//
	// Deprecated: This field is deprecated and will be removed in a future release.
	RetryFunc vfs.Retry //nolint:staticcheck // Deprecated: This field is deprecated and will be removed in a future release.

	// Buffer Size In Bytes Used with utils.TouchCopyBuffered
	FileBufferSize int

	tokenCredentialFactory TokenCredentialFactory
}

// NewOptions creates a new Options struct by populating values from environment variables.
//
//	Env Vars:
//	  *VFS_AZURE_STORAGE_ACCOUNT
//	  *VFS_AZURE_STORAGE_ACCESS_KEY
//	  *VFS_AZURE_TENANT_ID
//	  *VFS_AZURE_CLIENT_ID
//	  *VFS_AZURE_CLIENT_SECRET
//	  *VFS_AZURE_ENV_NAME
func NewOptions() *Options {
	return &Options{
		ServiceURL:             os.Getenv("VFS_AZURE_SERVICE_URL"),
		AccountName:            os.Getenv("VFS_AZURE_STORAGE_ACCOUNT"),
		AccountKey:             os.Getenv("VFS_AZURE_STORAGE_ACCESS_KEY"),
		TenantID:               os.Getenv("VFS_AZURE_TENANT_ID"),
		ClientID:               os.Getenv("VFS_AZURE_CLIENT_ID"),
		ClientSecret:           os.Getenv("VFS_AZURE_CLIENT_SECRET"),
		tokenCredentialFactory: DefaultTokenCredentialFactory,
	}
}

// Credential returns a credential based on how options are configured.  Options are checked
// and evaluated in the following order:
//  1. If TenantID, ClientID, and ClientSecret are non-empty, return azcore.TokenCredential.  This form of authentication
//     is used with service accounts and can be used to access containers across multiple storage accounts.
//  2. If AccountName, and AccountKey are non-empty, return azblob.SharedKeyCredential.  This form or authentication
//     is used with storage accounts and only provides access to a single storage account.
//  3. Returns a nil credential.  This allows access only to public blobs.
func (o *Options) Credential() (any, error) {
	if o.tokenCredentialFactory == nil {
		o.tokenCredentialFactory = DefaultTokenCredentialFactory
	}

	// Check to see if we have service account credentials
	if o.TenantID != "" && o.ClientID != "" && o.ClientSecret != "" {
		return o.tokenCredentialFactory(o.TenantID, o.ClientID, o.ClientSecret)
	}

	// Check to see if we have storage account credentials
	if o.AccountName != "" && o.AccountKey != "" {
		return azblob.NewSharedKeyCredential(o.AccountName, o.AccountKey)
	}

	// 3. Return a nil credential
	return nil, nil
}
