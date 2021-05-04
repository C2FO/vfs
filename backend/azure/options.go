package azure

import "github.com/c2fo/vfs/v5"

// Options contains options necessary for the azure vfs implementation
type Options struct {
	// AccountName holds the Azure Blob Storage account name for authentication
	AccountName string

	// AccountKey holds the Azure Blob Storage account key for authentication
	AccountKey string

	// RetryFunc holds the retry function
	RetryFunc vfs.Retry
}
