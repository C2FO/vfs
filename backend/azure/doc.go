/*
Package azure Microsoft Azure Blob Storage VFS Implementation

Usage

Rely on github.com/c2fo/vfs/backend

  import(
      "github.com/c2fo/vfs/v6/backend"
      "github.com/c2fo/vfs/v6/backend/azure"
  )

  func UseFs() error {
      fs := backend.Backend(azure.Scheme)
      ...
  }

Or call directly:

  import "github.com/c2fo/vfs/v6/backend/azure"

  func DoSomething() {
      fs := azure.NewFilesystem()
      ...
  }

azure can be augmented with the following implementation-specific methods.  Backend returns vfs.Filesystem interface so it
would have to be cast as azure.Filesystem to use the following:

  func DoSomething() {

      ...

      // cast if fs was created using backend.Backend().  Not necessary if created directly from azure.NewFilesystem().
      fs = fs.(azure.Filesystem)

      // to pass in client options
      fs = fs.WithOptions(
          azure.Options{
              AccountName: "...",
              AccountKey: "...
          },
      )

      // to pass specific client, for instance mock client
      client, _ := azure.NewClient(MockAzureClient{...})
      fs = fs.WithClient(client)
  }

Authentication

Authentication, by default, occurs automatically when Client() is called. It looks for credentials in the following places,
preferring the first location found:

  1. When the ENV vars VFS_AZURE_ENV_NAME, VFS_AZURE_STORAGE_ACCOUNT, VFS_AZURE_TENANT_ID, VFS_AZURE_CLIENT_ID, and
     VFS_AZURE_CLIENT_SECRET, authentication is performed using an OAuth Token Authenticator.  This will allow access
     to containers from multiple storage accounts.
  2. The ENV vars VFS_AZURE_STORAGE_ACCOUNT and VFS_AZURE_STORAGE_KEY, a shared key authenticator is used.  This will
     allow access to any containers owned by the designated storage account.
  3. If none of the above are present, then an anonymous authenticator is created and only publicly accessible blobs
     will be available
*/
package azure
