# azure

---
    import "github.com/c2fo/vfs/backend/azure"

Package azure Microsoft Azure Blob Storage VFS Implementation

### Usage

Rely on github.com/c2fo/vfs/backend

```go
    import(
        "github.com/c2fo/vfs/v6/backend"
        "github.com/c2fo/vfs/v6/backend/azure"
    )

    func UseFs() error {
        fs := backend.Backend(azure.Scheme)
        ...
    }
```

Or call directly:

```go
    import "github.com/c2fo/vfs/v6/backend/azure"

    func DoSomething() {
        fs := azure.NewFileSystem()
        ...
    }
```

azure can be augmented with the following implementation-specific methods.
[Backend](backend.md) returns [vfs.FileSystem](../README.md#type-filesystem) interface so it would have to be cast as
azure.FileSystem to use the following:

```go
    func DoSomething() {
        ...

        // cast if fs was created using backend.Backend().  Not necessary if created directly from azure.NewFileSystem().
        fs = fs.(azure.FileSystem)

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
```

### Authentication

Authentication, by default, occurs automatically when Client() is called. It
looks for credentials in the following places, preferring the first location
found:

1. When the ENV vars `VFS_AZURE_ENV_NAME`, `VFS_AZURE_STORAGE_ACCOUNT`, `VFS_AZURE_TENANT_ID`, `VFS_AZURE_CLIENT_ID`, and
       `VFS_AZURE_CLIENT_SECRET`, authentication is performed using an OAuth Token Authenticator.  This will allow access
       to containers from multiple storage accounts.
1. The ENV vars `VFS_AZURE_STORAGE_ACCOUNT` and `VFS_AZURE_STORAGE_KEY`, a shared key authenticator is used.  This will
       allow access to any containers owned by the designated storage account.
1. If none of the above are present, then an anonymous authenticator is created and only publicly accessible blobs
       will be available

## Usage

```go
const Name = "azure"
```
Name defines the name for the azure implementation

```go
const Scheme = "https"
```
Scheme defines the scheme for the azure implementation

#### func  IsValidURI

```go
func IsValidURI(u *url.URL) bool
```
IsValidURI us a utility function used by vfssimple to determine if the given URI
is a valid Azure URI

#### func  ParsePath

```go
func ParsePath(p string) (host, pth string, err error)
```
ParsePath is a utility function used by vfssimple to separate the host from the
path. The first parameter returned is the host and the second parameter is the
path.

### type BlobProperties

```go
type BlobProperties struct {
	// Size holds the size of the blob.
	Size uint64

	// LastModified holds the last modified time.Time
	LastModified *time.Time

	// Metadata holds the Azure metadata
	Metadata map[string]string
}
```

BlobProperties holds a subset of information returned by Blob.GetProperties(..)

#### func  NewBlobProperties

```go
func NewBlobProperties(azureProps *azblob.BlobGetPropertiesResponse) *BlobProperties
```
NewBlobProperties creates a new BlobProperties from an
azblob.BlobGetPropertiesResponse

### type Client

```go
type Client interface {
	// Properties should return a BlobProperties struct for the blob specified by locationURI, and filePath.  If the
	// blob is not found an error should be returned.
	Properties(locationURI, filePath string) (*BlobProperties, error)

	// SetMetadata should add the metadata specified by the parameter metadata for the blob specified by the parameter
	// file.
	SetMetadata(file vfs.File, metadata map[string]string) error

	// Upload should create or update the blob specified by the file parameter with the contents of the content
	// parameter
	Upload(file vfs.File, content io.ReadSeeker) error

	// Download should return a reader for the blob specified by the file parameter
	Download(file vfs.File) (io.ReadCloser, error)

	// Copy should copy the file specified by srcFile to the file specified by tgtFile
	Copy(srcFile vfs.File, tgtFile vfs.File) error

	// List should return a listing for the specified location. Listings should include the full path for the file.
	List(l vfs.Location) ([]string, error)

	// Delete should delete the file specified by the parameter file.
	Delete(file vfs.File) error
}
```

The Client interface contains methods that perform specific operations to Azure
Blob Storage. This interface is here so we can write mocks over the actual
functionality.

### type DefaultClient

```go
type DefaultClient struct {
}
```

DefaultClient is the main implementation that actually makes the calls to Azure
Blob Storage

#### func  NewClient

```go
func NewClient(options *Options) (*DefaultClient, error)
```
NewClient initializes a new DefaultClient

#### func (*DefaultClient) Copy

```go
func (a *DefaultClient) Copy(srcFile, tgtFile vfs.File) error
```
Copy copies srcFile to the destination tgtFile within Azure Blob Storage. Note
that in the case where we get encoded spaces in the file name (i.e. %20) the '%'
must be encoded or the copy command will return a not found error.

#### func (*DefaultClient) Delete

```go
func (a *DefaultClient) Delete(file vfs.File) error
```
Delete deletes the given file from Azure Blob Storage.

#### func (*DefaultClient) DeleteAllVersions

```go
func (a *DefaultClient) Delete(file vfs.File) error
```

Deletes the file blob using Azure's delete blob api, then each version of the blob is deleted using Azure's delete api. NOTE that if soft deletion is enabled for the blobs in the storage account, each version will be marked as deleted and will get permanently deleted by Azure as per the soft deletion policy. Returns any error returned by the API.

#### func (*DefaultClient) Download

```go
func (a *DefaultClient) Download(file vfs.File) (io.ReadCloser, error)
```
Download returns an io.ReadCloser for the given vfs.File

#### func (*DefaultClient) List

```go
func (a *DefaultClient) List(l vfs.Location) ([]string, error)
```
List will return a listing of the contents of the given location. Each item in
the list will contain the full key as specified by the azure blob (including the
virtual 'path').

#### func (*DefaultClient) Properties

```go
func (a *DefaultClient) Properties(containerURI, filePath string) (*BlobProperties, error)
```
Properties fetches the properties for the blob specified by the parameters
containerURI and filePath

#### func (*DefaultClient) SetMetadata

```go
func (a *DefaultClient) SetMetadata(file vfs.File, metadata map[string]string) error
```
SetMetadata sets the given metadata for the blob

#### func (*DefaultClient) Upload

```go
func (a *DefaultClient) Upload(file vfs.File, content io.ReadSeeker) error
```
Upload uploads a new file to Azure Blob Storage

### func DefaultTokenCredentialFactory

```go
func DefaultTokenCredentialFactory(tenantID, clientID, clientSecret, azureEnvName string) (azblob.TokenCredential, error)
```

DefaultTokenCredentialFactory knows how to make azblob.TokenCredential structs
for OAuth authentication

### type File

```go
type File struct {
}
```

File implements the vfs.File interface for Azure Blob Storage

#### func (*File) Close

```go
func (f *File) Close() error
```
Close cleans up all of the backing data structures used for reading/writing
files. This includes, closing the temp file, uploading the contents of the temp
file to Azure Blob Storage (if necessary), and calling Seek(0, 0).

#### func (*File) CopyToFile

```go
func (f *File) CopyToFile(file vfs.File) error
```
CopyToFile puts the contents of the receiver (f *File) into the passed vfs.File
parameter.

#### func (*File) CopyToLocation

```go
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error)
```
CopyToLocation creates a copy of *File, using the file's current name as the new
file's name at the given location. If the given location is also azure, the
azure API for copying files will be utilized, otherwise, standard io.Copy will
be done to the new file.

#### func (*File) Delete

```go
func (f *File) Delete(opts ...options.DeleteOption) error
```
Deletes the file using Azure's delete blob api. If opts is of type delete.AllVersions, after deleting the blob, each version of the blob is deleted using Azure's delete api. NOTE that if soft deletion is enabled for the blobs, each version will be marked as deleted and will get permanently deleted by Azure as per the soft deletion policy. Returns any error returned by the API.

#### func (*File) Exists

```go
func (f *File) Exists() (bool, error)
```
Exists returns true/false if the file exists/does not exist on Azure

#### func (*File) LastModified

```go
func (f *File) LastModified() (*time.Time, error)
```
LastModified returns the last modified time as a time.Time

#### func (*File) Location

```go
func (f *File) Location() vfs.Location
```
Location returns a Location instance for the files current location

#### func (*File) MoveToFile

```go
func (f *File) MoveToFile(file vfs.File) error
```
MoveToFile copies the receiver to the specified file and deletes the original
file.

#### func (*File) MoveToLocation

```go
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error)
```
MoveToLocation copies the receiver to the passed location. After the copy
succeeds, the original is deleted.

#### func (*File) Name

```go
func (f *File) Name() string
```
Name returns the name of the file

#### func (*File) Path

```go
func (f *File) Path() string
```
Path returns full path with leading slash.

#### func (*File) Read

```go
func (f *File) Read(p []byte) (n int, err error)
```
Read implements the io.Reader interface. For this to work with Azure Blob
Storage, a temporary local copy of the file is created and read operations are
performed against that. The temp file is closed and flushed to Azure when
f.Close() is called.

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek implements the io.Seeker interface. For this to work with Azure Blob
Storage, a temporary local copy of the file is created and operations are
performed against that. The temp file is closed and flushed to Azure when
f.Close() is called.

#### func (*File) Size

```go
func (f *File) Size() (uint64, error)
```
Size returns the size of the blob

#### func (*File) String

```go
func (f *File) String() string
```
String returns the file URI

#### func (*File) Touch

```go
func (f *File) Touch() error
```
Touch creates a zero-length file on the vfs.File if no File exists. If the file
exists, Touch updates the file's last modified parameter.

#### func (*File) URI

```go
func (f *File) URI() string
```
URI returns a full Azure URI for the file

#### func (*File) Write

```go
func (f *File) Write(p []byte) (int, error)
```
Write implements the io.Writer interface. Writes are performed against a
temporary local file. The temp file is closed and flushed to Azure with
f.Close() is called.

### type FileSystem

```go
type FileSystem struct {
}
```

FileSystem implements the vfs.FileSystem interface for Azure Blob Storage

#### func  NewFileSystem

```go
func NewFileSystem() *FileSystem
```
NewFileSystem creates a new default FileSystem. This will set the options
options.AccountName and options.AccountKey with the env variables
AZURE_STORAGE_ACCOUNT and AZURE_STORAGE_ACCESS_KEY respectively.

#### func (*FileSystem) Client

```go
func (fs *FileSystem) Client() (Client, error)
```
Client returns a Client to perform operations against Azure Blob Storage

#### func (*FileSystem) Host

```go
func (fs *FileSystem) Host() string
```
Host returns the host portion of the URI. For azure this consists of
<account_name>.blob.core.windows.net.

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns "azure"

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(volume, absFilePath string, opts ...options.NewFileOption) (vfs.File, error)
```
NewFile returns the azure implementation of vfs.File

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(volume, absLocPath string) (vfs.Location, error)
```
NewLocation returns the azure implementation of vfs.Location

#### func (*FileSystem) Retry

```go
func (fs *FileSystem) Retry() vfs.Retry
```
Retry returns the default retry function. This is overridable via the
WithOptions function.

#### func (*FileSystem) Scheme

```go
func (fs *FileSystem) Scheme() string
```
Scheme returns "https" as the initial part of the URI i.e. https://..

#### func (*FileSystem) WithClient

```go
func (fs *FileSystem) WithClient(client Client) *FileSystem
```
WithClient allows the caller to specify a specific client to be used

#### func (*FileSystem) WithOptions

```go
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem
```
WithOptions allows the caller to override the default options

### type Location

```go
type Location struct {
}
```

Location is the azure implementation of vfs.Location

#### func (*Location) ChangeDir

```go
func (l *Location) ChangeDir(relLocPath string) error
```
ChangeDir changes the current location's path to the new, relative path.

#### func (*Location) ContainerURL

```go
func (l *Location) ContainerURL() string
```
ContainerURL returns the URL for the Azure Blob Storage container.

#### func (*Location) DeleteFile

```go
func (l *Location) DeleteFile(relFilePath string) error
```
DeleteFile deletes the file at the given path, relative to the current location.

#### func (*Location) Exists

```go
func (l *Location) Exists() (bool, error)
```
Exists returns true if the file exists and false. In the case of errors false is
always returned along with the error

#### func (*Location) FileSystem

```go
func (l *Location) FileSystem() vfs.FileSystem
```
FileSystem returns the azure FileSystem instance

#### func (*Location) List

```go
func (l *Location) List() ([]string, error)
```
List returns a list of base names for the given location.

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(prefix string) ([]string, error)
```
ListByPrefix returns a list of base names that contain the given prefix

#### func (*Location) ListByRegex

```go
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error)
```
ListByRegex returns a list of base names that match the given regular expression

#### func (*Location) NewFile

```go
func (l *Location) NewFile(relFilePath string, opts ...options.NewFileOption) (vfs.File, error)
```
NewFile returns a new file instance at the given path, relative to the current
location.

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error)
```
NewLocation creates a new location instance relative to the current location's
path.

#### func (*Location) Path

```go
func (l *Location) Path() string
```
Path returns the absolute path for the Location

#### func (*Location) String

```go
func (l *Location) String() string
```
String returns the URI

#### func (*Location) URI

```go
func (l *Location) URI() string
```
URI returns a URI string for the azure location.

#### func (*Location) Volume

```go
func (l *Location) Volume() string
```
Volume returns the azure container. Azure containers are equivalent to AWS
Buckets

### type MockAzureClient

```go
type MockAzureClient struct {
	PropertiesError  error
	PropertiesResult *BlobProperties
	ExpectedError    error
	ExpectedResult   interface{}
}
```

MockAzureClient is a mock implementation of azure.Client.

#### func (*MockAzureClient) Copy

```go
func (a *MockAzureClient) Copy(srcFile, tgtFile vfs.File) error
```
Copy returns the value of ExpectedError

#### func (*MockAzureClient) Delete

```go
func (a *MockAzureClient) Delete(file vfs.File) error
```
Delete returns the value of ExpectedError

#### func (*MockAzureClient) Download

```go
func (a *MockAzureClient) Download(file vfs.File) (io.ReadCloser, error)
```
Download returns ExpectedResult if it exists, otherwise it returns ExpectedError

#### func (*MockAzureClient) List

```go
func (a *MockAzureClient) List(l vfs.Location) ([]string, error)
```
List returns the value of ExpectedResult if it exists, otherwise it returns
ExpectedError.

#### func (*MockAzureClient) Properties

```go
func (a *MockAzureClient) Properties(locationURI, filePath string) (*BlobProperties, error)
```
Properties returns a PropertiesResult if it exists, otherwise it will return the
value of PropertiesError

#### func (*MockAzureClient) SetMetadata

```go
func (a *MockAzureClient) SetMetadata(file vfs.File, metadata map[string]string) error
```
SetMetadata returns the value of ExpectedError

#### func (*MockAzureClient) Upload

```go
func (a *MockAzureClient) Upload(file vfs.File, content io.ReadSeeker) error
```
Upload returns the value of ExpectedError

### type MockStorageError

```go
type MockStorageError struct {
	azblob.ResponseError
}
```

MockStorageError is a mock for the azblob.StorageError interface

#### func (MockStorageError) Error

```go
func (mse MockStorageError) Error() string
```
Error returns empty string

#### func (MockStorageError) Response

```go
func (mse MockStorageError) Response() *http.Response
```
Response returns nil

#### func (MockStorageError) ServiceCode

```go
func (mse MockStorageError) ServiceCode() azblob.ServiceCodeType
```
ServiceCode always returns "BlobNotFound" to simulate the not found condition

#### func (MockStorageError) Temporary

```go
func (mse MockStorageError) Temporary() bool
```
Temporary returns nil

#### func (MockStorageError) Timeout

```go
func (mse MockStorageError) Timeout() bool
```
Timeout returns nil

### func MockTokenCredentialFactory

```go
func MockTokenCredentialFactory(_, _, _, _ string) (azblob.TokenCredential, error)
```

MockTokenCredentialFactory knows how to create a "do-nothing" credential used
for unit testing

### type Options

```go
type Options struct {
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

	// AzureEnvName holds the name for the Azure environment.  This field is used for OAuth token
	// based authentication.
	AzureEnvName string

	// RetryFunc holds the retry function
	RetryFunc vfs.Retry
}
```

Options contains options necessary for the azure vfs implementation

#### func  NewOptions

```go
func NewOptions() *Options
```
NewOptions creates a new Options struct by populating values from environment
variables.

    Env Vars:
      *VFS_AZURE_STORAGE_ACCOUNT
      *VFS_AZURE_STORAGE_ACCESS_KEY
      *VFS_AZURE_TENANT_ID
      *VFS_AZURE_CLIENT_ID
      *VFS_AZURE_CLIENT_SECRET
      *VFS_AZURE_ENV_NAME

#### func (*Options) Credential

```go
func (o *Options) Credential() (azblob.Credential, error)
```
Credential returns an azblob.Credential struct based on how options are
configured. Options are checked and evaluated in the following order:

    1. If TenantID, ClientID, and ClientSecret are non-empty, return azblob.TokenCredential.  This form of authentication
       is used with service accounts and can be used to access containers across multiple storage accounts.
    2. If AccountName, and AccountKey are non-empty, return azblob.SharedKeyCredential.  This form or authentication
       is used with storage accounts and only provides access to a single storage account.
    3. Returns an anonymous credential.  This allows access only to public blobs.

### func TokenCredentialFactory

```go
type TokenCredentialFactory func(tenantID, clientID, clientSecret, azureEnvName string) (azblob.TokenCredential, error)
```

TokenCredentialFactory creates azure.TokenCredentials. This function is provided to allow for mocking
in unit tests.
