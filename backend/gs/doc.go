/*
Package gs - Google Cloud Storage VFS implementation.

# Usage

Rely on github.com/c2fo/vfs/backend

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    "github.com/c2fo/vfs/v7/backend/gs"
	)

	func UseFs() error {
	    fs := backend.Backend(gs.Scheme)
	    ...
	}

Or call directly:

	import "github.com/c2fo/vfs/v7/backend/gs"

	func DoSomething() {
	    fs := gs.NewFileSystem()
	    ...
	}

gs can be augmented with the following implementation-specific methods.  Backend returns vfs.FileSystem interface so it
would have to be cast as gs.FileSystem to use the following:

		func DoSomething() {
		    ...

		    // cast if fs was created using backend.Backend().  Not necessary if created directly from gs.NewFileSystem().
		    fs = fs.(gs.FileSystem)

		    // to use your own "context"
		    ctx := context.Background()
		    fs = gs.NewFileSystem(gs.WithContext(ctx))

		    // to pass in client options
		    fs = gs.NewFileSystem(
	            gs.WithOptions(
		            gs.Options{
		                CredentialFile: "/root/.gcloud/account.json",
		                Scopes:         []string{"ScopeReadOnly"},
		                //default scope is "ScopeFullControl"
		            },
	            ),
		    )

		    // to pass specific client, for instance no-auth client
		    ctx := context.Background()
		    client, _ := storage.NewClient(ctx, option.WithoutAuthentication())
		    fs = gs.NewFileSystem(gs.WithClient(client))
		}

# Authentication

Authentication, by default, occurs automatically when Client() is called. It looks for credentials in the following places,
preferring the first location found:

 1. A JSON file whose path is specified by the GOOGLE_APPLICATION_CREDENTIALS environment variable
 2. A JSON file in a location known to the gcloud command-line tool.
    On Windows, this is %APPDATA%/gcloud/application_default_credentials.json.
    On other systems, $HOME/.config/gcloud/application_default_credentials.json.
 3. On Google App Engine it uses the appengine.AccessToken function.
 4. On Google Compute Engine and Google App Engine Managed VMs, it fetches credentials from the metadata server.

See https://cloud.google.com/docs/authentication/production for more auth info

# See Also

See: https://github.com/googleapis/google-cloud-go/tree/master/storage
*/
package gs
