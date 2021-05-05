package azure

import (
	"os"

	"github.com/c2fo/vfs/v5"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

// Options contains options necessary for the azure vfs implementation
type Options struct {
	// AccountName holds the Azure Blob Storage account name for authentication
	AccountName string

	// AccountKey holds the Azure Blob Storage account key for authentication
	AccountKey string

	// TenantID holds the Azure Service Account tenant id for authentication
	TenantID string

	// ClientID holds the Azure Service Account client id for authentication
	ClientID string

	// ClientSecret holds the Azure Service Account client secret for authentication
	ClientSecret string

	// AzureEnvName holds the name for the Azure environment
	AzureEnvName string

	// RetryFunc holds the retry function
	RetryFunc vfs.Retry

	tokenCredentialFactory TokenCredentialFactory
}

func NewOptions() *Options {
	return &Options{
		AccountName:            os.Getenv("VFS_AZURE_STORAGE_ACCOUNT"),
		AccountKey:             os.Getenv("VFS_AZURE_STORAGE_ACCESS_KEY"),
		TenantID:               os.Getenv("VFS_AZURE_TENANT_ID"),
		ClientID:               os.Getenv("VFS_AZURE_CLIENT_ID"),
		ClientSecret:           os.Getenv("VFS_AZURE_CLIENT_SECRET"),
		AzureEnvName:           os.Getenv("VFS_AZURE_ENV_NAME"),
		tokenCredentialFactory: &DefaultTokenCredentialFactory{},
	}
}

func (o *Options) Credential() (azblob.Credential, error) {
	// Check to see if we have service account credentials
	if o.TenantID != "" && o.ClientID != "" && o.ClientSecret != "" {
		return o.tokenCredentialFactory.New(o.TenantID, o.ClientID, o.ClientSecret, o.AzureEnvName)
	}

	// Check to see if we have storage account credentials
	if o.AccountName != "" && o.AccountKey != "" {
		return azblob.NewSharedKeyCredential(o.AccountName, o.AccountKey)
	}

	// 3. Return an anonymous credential
	return azblob.NewAnonymousCredential(), nil
}
