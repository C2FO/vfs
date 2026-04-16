/*
Package gs implements [github.com/c2fo/vfs/v8.FileSystem] for Google Cloud Storage (gs://).

# Usage

	import (
	    "cloud.google.com/go/storage"
	    "github.com/c2fo/vfs/v8/backend/gs"
	)

	func Example() {
	    fs := gs.NewFileSystem()
	    _ = fs.Scheme() // "gs"
	}

The file system can be configured with [NewFileSystem] and options from this package
([WithClient], [WithOptions], [WithContext], [WithRetryer]):

	fs := gs.NewFileSystem(
	    gs.WithContext(ctx),
	    gs.WithOptions(gs.Options{
	        CredentialFile: "/path/to/account.json",
	    }),
	)

To use an existing client:

	client, _ := storage.NewClient(ctx, option.WithoutAuthentication())
	fs := gs.NewFileSystem(gs.WithClient(client))

# Authentication

By default, authentication runs when [FileSystem.Client] is called. Credentials are resolved like the
Google Cloud client libraries (environment variable, gcloud defaults, GCE metadata, etc.).

See https://cloud.google.com/docs/authentication/production for details.

# See Also

https://pkg.go.dev/cloud.google.com/go/storage
*/
package gs
