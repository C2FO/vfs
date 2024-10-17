# gs

---

Package gs - Google Cloud Storage VFS implementation.

### Usage

Rely on [github.com/c2fo/vfs/v6/backend](backend.md)

```go
    import(
        "github.com/c2fo/vfs/v6/backend"
        "github.com/c2fo/vfs/v6/backend/gs"
    )

    func UseFs() error {
        fs := backend.Backend(gs.Scheme)
        ...
    }
```

Or call directly:

```go
    import "github.com/c2fo/vfs/v6/backend/gs"

    func DoSomething() {
        fs := gs.NewFileSystem()
        ...
    }
```

gs can be augmented with the following implementation-specific methods. [Backend](backend.md)
returns [vfs.FileSystem](../README.md#type-filesystem) interface so it would have to be cast as gs.FileSystem to
use the following:

```go
    func DoSomething() {
        ...

        // cast if fs was created using backend.Backend().  Not necessary if created directly from gs.NewFileSystem().
        fs = fs.(gs.FileSystem)

        // to use your own "context"
        ctx := context.Background()
        fs = fs.WithContext(ctx)

        // to pass in client options
        fs = fs.WithOptions(
            gs.Options{
                CredentialFile: "/root/.gcloud/account.json",
                Scopes:         []string{"ScopeReadOnly"},
                //default scope is "ScopeFullControl"
            },
        )

        // to pass specific client, for instance no-auth client
        ctx := context.Background()
        client, _ := storage.NewClient(ctx, option.WithoutAuthentication())
        fs = fs.WithClient(client)
    }
```

### Authentication

Authentication, by default, occurs automatically when [Client()](#func-filesystem-client) is called. It
looks for credentials in the following places, preferring the first location
found:

1. A JSON file whose path is specified by the `GOOGLE_APPLICATION_CREDENTIALS` environment variable
1. A JSON file in a location known to the gcloud command-line tool.
    * On Windows, this is `%APPDATA%/gcloud/application_default_credentials.json`.
    * On other systems, `$HOME/.config/gcloud/application_default_credentials.json`.
1. On Google App Engine it uses the appengine.AccessToken function.
1. On Google Compute Engine and Google App Engine Managed VMs, it fetches credentials from the metadata server.

See https://cloud.google.com/docs/authentication/production for more auth info

### See Also

See: https://github.com/googleapis/google-cloud-go/tree/master/storage

## Usage

```go
const Scheme = "gs"
```
Scheme defines the file system type.

### type File

```go
type File struct {
}
```

File implements [vfs.File](../README.md#type-file) interface for GS fs.

#### func (*File) Close

```go
func (f *File) Close() error
```
Close cleans up underlying mechanisms for reading from and writing to the file.
Closes and removes the local temp file, and triggers a write to GCS of anything
in the f.writeBuffer if it has been created.

#### func (*File) CopyToFile

```go
func (f *File) CopyToFile(targetFile vfs.File) error
```
CopyToFile puts the contents of File into the targetFile passed. Uses the GCS
CopierFrom method if the target file is also on GCS, otherwise uses [io.Copy](https://godoc.org/io#Copy).

#### func (*File) CopyToLocation

```go
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error)
```
CopyToLocation creates a copy of *File, using the file's current name as the new
file's name at the given location. If the given location is also GCS, the GCS
API for copying files will be utilized, otherwise, standard [io.Copy](https://godoc.org/io#Copy) will be done
to the new file.

#### func (*File) Delete

```go
func (f *File) Delete(opts ...options.DeleteOption) error
```
Delete clears any local temp file, or write buffer from read/writes to the file,
then makes a DeleteObject call to GCS for the file. If opts is of type delete.AllVersions, DeleteObject call is made to
GCS for each version of the file. Returns any error returned by the API.

#### func (*File) Exists

```go
func (f *File) Exists() (bool, error)
```
Exists returns a boolean of whether or not the object exists in GCS.

#### func (*File) LastModified

```go
func (f *File) LastModified() (*time.Time, error)
```
LastModified returns the 'Updated' property from the GCS attributes.

#### func (*File) Location

```go
func (f *File) Location() vfs.Location
```
Location returns a Location instance for the file's current location.

#### func (*File) MoveToFile

```go
func (f *File) MoveToFile(targetFile vfs.File) error
```
MoveToFile puts the contents of File into the targetFile passed using
File.CopyToFile. If the copy succeeds, the source file is deleted. Any errors
from the copy or delete are returned.

#### func (*File) MoveToLocation

```go
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error)
```
MoveToLocation works by first calling File.CopyToLocation(vfs.Location) then, if
that succeeds, it deletes the original file, returning the new file. If the copy
process fails the error is returned, and the Delete isn't called. If the call to
Delete fails, the error and the file generated by the copy are both returned.

#### func (*File) Name

```go
func (f *File) Name() string
```
Name returns the file name.

#### func (*File) Path

```go
func (f *File) Path() string
```
Path returns full path with leading slash of the GCS file key.

#### func (*File) Read

```go
func (f *File) Read(p []byte) (n int, err error)
```
Read implements the standard for [io.Reader](https://godoc.org/io#Reader). For this to work with an GCS file, a
temporary local copy of the file is created, and reads work on that. This file
is closed and removed upon calling f.Close()

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek implements the standard for io.Seeker. A temporary local copy of the GCS
file is created (the same one used for Reads) which Seek() acts on. This file is
closed and removed upon calling f.Close()

#### func (*File) Size

```go
func (f *File) Size() (uint64, error)
```
Size returns the 'Size' property from the GCS attributes.

#### func (*File) String

```go
func (f *File) String() string
```
String returns the file URI string.

#### func (*File) URI

```go
func (f *File) URI() string
```
URI returns a full GCS URI string of the file.

#### func (*File) Write

```go
func (f *File) Write(data []byte) (n int, err error)
```
Write implements the standard for [io.Writer](https://godoc.org/io#Writer). A buffer is added to with each
subsequent write. Calling [Close()](#func-file-close) will write the contents back to GCS.

### type FileSystem

```go
type FileSystem struct {
}
```

FileSystem implements [vfs.FileSystem](../README.md#type-filesystem) for the GCS file system.

#### func  NewFileSystem

```go
func NewFileSystem() *FileSystem
```
NewFileSystem initializer for [FileSystem](#type-filesystem) struct accepts google cloud storage
client and returns FileSystem or error.

#### func (*FileSystem) Client

```go
func (fs *FileSystem) Client() (*storage.Client, error)
```
Client returns the underlying google storage client, creating it, if
necessary See [Authentication](#authentication) section for authentication resolution

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns "Google Cloud Storage"

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error)
```
NewFile function returns the gcs implementation of [vfs.File](../README.md#type-file).

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(volume string, path string) (loc vfs.Location, err error)
```
NewLocation function returns the s3 implementation of [vfs.Location](../README.md#type-location).

#### func (*FileSystem) Scheme

```go
func (fs *FileSystem) Scheme() string
```
Scheme return "gs" as the initial part of a file URI ie: gs://

#### func (*FileSystem) WithClient

```go
func (fs *FileSystem) WithClient(client *storage.Client) *FileSystem
```
WithClient passes in a google storage client and returns the FileSystem
(chainable)

#### func (*FileSystem) WithContext

```go
func (fs *FileSystem) WithContext(ctx context.Context) *FileSystem
```
WithContext passes in user context and returns the FileSystem (chainable)

#### func (*FileSystem) WithOptions

```go
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem
```
WithOptions sets options for client and returns the FileSystem (chainable)

### type Location

```go
type Location struct {
}
```

Location implements [vfs.Location](../README.md#type-location) for gs fs.

#### func (*Location) ChangeDir

```go
func (l *Location) ChangeDir(relativePath string) error
```
ChangeDir changes the current location's path to the new, relative path.

#### func (*Location) DeleteFile

```go
func (l *Location) DeleteFile(fileName string, opts ...options.DeleteOption) error
```
DeleteFile deletes the file at the given path, relative to the current location using given options.

#### func (*Location) Exists

```go
func (l *Location) Exists() (bool, error)
```
Exists returns whether the location exists or not. In the case of an error,
false is returned.

#### func (*Location) FileSystem

```go
func (l *Location) FileSystem() vfs.FileSystem
```
FileSystem returns the GCS file system instance.

#### func (*Location) List

```go
func (l *Location) List() ([]string, error)
```
List returns a list of file name strings for the current location.

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(filenamePrefix string) ([]string, error)
```
ListByPrefix returns a slice of file base names and any error, if any prefix
means filename prefix and therefore should not have slash List functions return
only files [List](#func-location-list) functions return only basenames

#### func (*Location) ListByRegex

```go
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error)
```
ListByRegex returns a list of file names at the location which match the
provided regular expression.

#### func (*Location) NewFile

```go
func (l *Location) NewFile(filePath string) (vfs.File, error)
```
NewFile returns a new file instance at the given path, relative to the current
location.

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relativePath string) (vfs.Location, error)
```
NewLocation creates a new location instance relative to the current location's
path.

#### func (*Location) Path

```go
func (l *Location) Path() string
```
Path returns the path of the file at the current location, starting with a
leading '/'

#### func (*Location) String

```go
func (l *Location) String() string
```
String returns the full URI of the location.

#### func (*Location) URI

```go
func (l *Location) URI() string
```
URI returns a URI string for the GCS location.

#### func (*Location) Volume

```go
func (l *Location) Volume() string
```
Volume returns the GCS bucket name.

### type Options

```go
type Options struct {
	APIKey         string   `json:"apiKey,omitempty"`
	CredentialFile string   `json:"credentialFilePath,omitempty"`
	Endpoint       string   `json:"endpoint,omitempty"`
	Scopes         []string `json:"WithoutAuthentication,omitempty"`
}
```

Options holds Google Cloud Storage -specific options. Currently only client
options are used.
