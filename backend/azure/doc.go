/*
Package azure Microsoft Azure Blob Storage VFS Implementation

Usage

Rely on github.com/c2fo/vfs/backend

  import(
      "github.com/c2fo/vfs/v5/backend"
      "github.com/c2fo/vfs/v5/backend/azure"
  )

  func UseFs() error {
      fs := backend.Backend(azure.Scheme)
      ...
  }

Or call directly:

  import "github.com/c2fo/vfs/v5/backend/azure"

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

      // to use your own "context"
      ctx := context.Background()
      fs = fs.WithContext(ctx)

      // to pass in client options
      fs = fs.WithOptions(
          azure.Options{
              AccountName: "...",
              AccountKey: "...
          },
      )

      // to pass specific client, for instance mock client
      ctx := context.Background()
      client, _ := storage.NewClient(ctx, MockAzureClient{...})
      fs = fs.WithClient(client)
  }

Authentication

Authentication, by default, occurs automatically when Client() is called. It looks for credentials in the following places,
preferring the first location found:

  1. The ENV vars AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_KEY
  2. More to come...
*/
package azure
