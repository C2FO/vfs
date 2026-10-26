/*
Package azure Microsoft Azure Blob Storage VFS Implementation

# Usage

Rely on github.com/c2fo/vfs/v7/backend

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    "github.com/c2fo/vfs/v7/backend/azure"
	)

	func UseFs() error {
	    fs := backend.Backend(azure.Scheme)
	    ...
	}

Or call directly:

	import "github.com/c2fo/vfs/v7/backend/azure"

	func DoSomething() {
	    fs := azure.NewFileSystem()
	    ...
	}

azure can be configured with functional options passed to NewFileSystem:

	func DoSomething() {
	    ...

	    // to pass in client options
	    fs := azure.NewFileSystem(
	        azure.WithOptions(
	            azure.Options{
	                AccountName: "...",
	                AccountKey:  "...",
	            },
	        ),
	    )

	    // to pass specific client, for instance mock client
	    mockClient := mocks.NewClient(t)
	    fs = azure.NewFileSystem(azure.WithClient(mockClient))
	}

# Authentication

Authentication, by default, occurs automatically when Client() is called. NewFileSystem populates Options from the
following environment variables, and any values passed with WithOptions replace them:

  - VFS_AZURE_SERVICE_URL (Options.ServiceURL)
  - VFS_AZURE_STORAGE_ACCOUNT (Options.AccountName)
  - VFS_AZURE_STORAGE_ACCESS_KEY (Options.AccountKey)
  - VFS_AZURE_TENANT_ID (Options.TenantID)
  - VFS_AZURE_CLIENT_ID (Options.ClientID)
  - VFS_AZURE_CLIENT_SECRET (Options.ClientSecret)

Credentials are then chosen in the following order, preferring the first match:

 1. If TenantID, ClientID, and ClientSecret are set, a service principal (client secret) token credential is used.
    This allows access to containers across multiple storage accounts.
 2. If AccountName and AccountKey are set, a shared key credential is used.  This allows access to any containers
    owned by that storage account.
 3. Otherwise no credentials are used and only publicly accessible blobs will be available.

If ServiceURL is empty, it defaults to https://<AccountName>.blob.core.windows.net (or https://blob.core.windows.net
when AccountName is also empty).
*/
package azure
