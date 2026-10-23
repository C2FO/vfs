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

# Service Account Impersonation

Set ImpersonateServiceAccount to have Client() build the storage client with short-lived tokens for that service
account, requested by a base identity, instead of using the base identity's own access:

	fs := gs.NewFileSystem(
	    gs.WithOptions(
	        gs.Options{
	            ImpersonateServiceAccount: "target-sa@my-project.iam.gserviceaccount.com",
	        },
	    ),
	)

The same option decoded from JSON:

	{"impersonateServiceAccount": "target-sa@my-project.iam.gserviceaccount.com"}

The base identity is CredentialFile if set, otherwise Application Default Credentials resolved as described above. ADC
can be any supported credential type, including an external_account (Workload Identity Federation) config referenced by
GOOGLE_APPLICATION_CREDENTIALS, so impersonation also works outside GKE and GCE. The base identity needs the Service
Account Token Creator role (roles/iam.serviceAccountTokenCreator) on the target service account.

Impersonated tokens use the storage client's default scopes (devstorage.full_control and cloud-platform). Endpoint and
Scopes are ignored when ImpersonateServiceAccount is set. Combining it with APIKey makes Client() return an error,
because an API key can't impersonate a service account.

Copies between gs filesystems with the same ImpersonateServiceAccount and the same CredentialFile (or neither has one)
are done server-side.

Anyone who can set ImpersonateServiceAccount can act as any service account the base identity is allowed to
impersonate. If your application builds Options from user-supplied input, restrict which service accounts may be named.

# See Also

See: https://github.com/googleapis/google-cloud-go/tree/master/storage
*/
package gs
