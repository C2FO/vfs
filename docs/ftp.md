# ftp

---

Package ftp - FTP VFS implementation.

### Usage

Rely on [github.com/c2fo/vfs/v6/backend](backend.md)

```go
      import(
    	  "github.com/c2fo/vfs/v6/backend"
    	  "github.com/c2fo/vfs/v6/backend/ftp"
      )

      func UseFs() error {
    	  fs := backend.Backend(ftp.Scheme)
    	  ...
      }
```

Or call directly:

```go
      import "github.com/c2fo/vfs/v6/backend/ftp"

      func DoSomething() {
    	  fs := ftp.NewFileSystem()

    	  location, err := fs.NewLocation("myuser@server.com:21", "/some/path/")
    	  if err != nil {
    		 #handle error
    	  }
    	  ...
      }
```

ftp can be augmented with some implementation-specific methods. [Backend](backend.md) returns
[vfs.FileSystem](../README.md#type-filesystem) interface, so it would have to be cast as ftp.FileSystem to use
them.

These methods are chainable: (*FileSystem) WithClient(client interface{})
*FileSystem (*FileSystem) WithOptions(opts vfs.Options) *FileSystem

```go
      func DoSomething() {
    	  // cast if fs was created using backend.Backend().  Not necessary if created directly from ftp.NewFileSystem().
    	  fs := backend.Backend(ftp.Scheme)
    	  fs = fs.(*ftp.FileSystem)

    	  // to pass specific client implementing types.Client interface (in this case, _ftp github.com/jlaffaye/ftp)
    	  client, _ := _ftp.Dial("server.com:21")
    	  fs = fs.WithClient(client)

    	  // to pass in client options. See Options for more info.  Note that changes to Options will make nil any client.
    	  // This behavior ensures that changes to settings will get applied to a newly created client.
    	  fs = fs.WithOptions(
    		  ftp.Options{
    			  Password: "s3cr3t",
    			  DisableEPSV: true,
    			  Protocol: ftp.ProtocolFTPES,
    			  DialTimeout: 15 * time.Second,
    			  DebugWriter: os.Stdout,
    		  },
    	  )

    	  location, err := fs.NewLocation("myuser@server.com:21", "/some/path/")
    	  #handle error

    	  file, err := location.NewFile("myfile.txt")
    	  #handle error

    	  _, err := file.Write([]bytes("some text")
    	  #handle error

    	  err := file.Close()
    	  #handle error

      }
```

Note - this vfs implementation can have issues conducting simultaneous reads and writes on files created from the same filesystem. This can
cause issues when attempting to use those files with functions such as io.CopyBuffer.

The provided CopyToFile and CopyToLocation functions should be used instead in these instances.

```go
		func DoSomething() {
		  // cast if fs was created using backend.Backend().  Not necessary if created directly from ftp.NewFileSystem().
		  fs := backend.Backend(ftp.Scheme)
		  fs = fs.(*ftp.FileSystem)

		  // to pass specific client implementing types.Client interface (in this case, _ftp github.com/jlaffaye/ftp)
		  client, _ := _ftp.Dial("server.com:21")
		  fs = fs.WithClient(client)

		  // to pass in client options. See Options for more info.  Note that changes to Options will make nil any client.
		  // This behavior ensures that changes to settings will get applied to a newly created client.
		  fs = fs.WithOptions(
			  ftp.Options{
				  Password: "s3cr3t",
				  DisableEPSV: true,
				  Protocol: ftp.ProtocolFTPES,
				  DialTimeout: 15 * time.Second,
				  DebugWriter: os.Stdout,
			  },
		  )

		  location, err := fs.NewLocation("myuser@server.com:21", "/some/path/")
		  #handle error

		  file, err := location.NewFile("myfile.txt")
		  #handle error

		  _, err = file.Write([]byte("some text"))
		  #handle error

		  err = file.Close()
		  #handle error

	  }
```

### Authentication

Authentication, by default, occurs automatically when [Client()](#func-filesystem-client) is called. Since
user is part of the URI authority section (Volume), auth is handled slightly
differently than other vfs [backends](backend.md) (except [SFTP](sftp.md)).

A client is initialized lazily, meaning we only make a connection to the server
at the last moment, so we are free to modify options until then. The
authenticated session is closed any time [WithOption()](#func-filesystem-withoptions) or [WithClient()](#func-filesystem-withclient) occurs.

#### USERNAME

User may only be set in the [URI authority](https://tools.ietf.org/html/rfc3986#section-3.2) section (Volume in vfs parlance).

     scheme             host
     __/             ___/____  port
    /  \            /        \ /\
    ftp://someuser@server.com:22/path/to/file.txt
           \____________________/ \______________/
           \______/       \               \
               /     authority section    path
         username       (Volume)

ftp vfs backend defaults to "anonymous" if no username is provided in the
authority, ie "ftp://service.com/".

#### PASSWORD

Passwords may be passed via [Options.Password](#type-options) or via the environmental variable
`VFS_FTP_PASSWORD`. If not password is provided, default is "anonymous".
Password precedence is default, env var, Options.Password, such that env var, if
set, overrides default and Options.Password, if set, overrides env var.

### Protocol

The ftp backend supports the following FTP protocols: FTP (unencrypted), FTPS
(implicit TLS), and FTPES (explicit TLS). Protocol can be set by env var
`VFS_FTP_PROTOCOL` or in Options.Protocol. Options values take precedence over
env vars.

By default, FTPS and FTPS will use the following TLS configuration but can be
overridden(recommended) with Options.TLSConfig:

    tlsConfig := &tls.Config{
    	MinVersion:         tls.VersionTLS12,
    	InsecureSkipVerify: true,
    	ClientSessionCache: tls.NewLRUClientSessionCache(0),
    	ServerName:         hostname,
    }

See https://pkg.go.dev/crypto/tls#Config for all TLS configuration options.

### Other Options

DebugWriter *io.Writer* - captures FTP command details to any writer.

DialTimeout *time.Duration - sets timeout for connecting only.

DisableEPSV bool - Extended Passive mode (EPSV) is attempted by default. Set to true to use regular Passive mode (PASV).

## Usage

```go
const (
	// ProtocolFTP signifies plain, unencrypted FTP
	ProtocolFTP = "FTP"
	// ProtocolFTPS signifies FTP over implicit TLS
	ProtocolFTPS = "FTPS"
	// ProtocolFTPES signifies FTP over explicit TLS
	ProtocolFTPES = "FTPES"
)
```

```go
const Scheme = "ftp"
```
Scheme defines the filesystem type.

### type File

```go
type File struct {
}
```

File implements vfs.File interface for FTP fs.

#### func (*File) Close

```go
func (f *File) Close() error
```
Close calls the underlying ftp.Response Close, if opened, and clears the
internal pointer

#### func (*File) CopyToFile

```go
func (f *File) CopyToFile(file vfs.File) error
```
CopyToFile puts the contents of File into the targetFile passed.

#### func (*File) CopyToLocation

```go
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error)
```
CopyToLocation creates a copy of *File, using the file's current path as the new
file's path at the given location.

#### func (*File) Delete

```go
func (f *File) Delete(_ ...options.DeleteOption) error
```
Delete removes the remote file. Error is returned, if any.

#### func (*File) Exists

```go
func (f *File) Exists() (bool, error)
```
Exists returns a boolean of whether or not the file exists on the ftp server

#### func (*File) LastModified

```go
func (f *File) LastModified() (*time.Time, error)
```
LastModified returns the LastModified property of ftp file.

#### func (*File) Location

```go
func (f *File) Location() vfs.Location
```
Location returns a vfs.Location at the location of the file. IE: if file is at
ftp://someuser@host.com/here/is/the/file.txt the location points to
ftp://someuser@host.com/here/is/the/

#### func (*File) MoveToFile

```go
func (f *File) MoveToFile(t vfs.File) error
```
MoveToFile puts the contents of File into the targetFile passed using
File.CopyToFile. If the copy succeeds, the source file is deleted. Any errors
from the copy or delete are returned. If the given location is also ftp AND for
the same user and host, the ftp Rename method is used, otherwise we'll do an
io.Copy to the destination file then delete source file.

#### func (*File) MoveToLocation

```go
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error)
```
MoveToLocation works by creating a new file on the target location then calling
MoveToFile() on it.

#### func (*File) Name

```go
func (f *File) Name() string
```
Name returns the path portion of the file's path property. IE: "file.txt" of
"ftp://someuser@host.com/some/path/to/file.txt

#### func (*File) Path

```go
func (f *File) Path() string
```
Path return the directory portion of the file's path. IE: "path/to" of
"ftp://someuser@host.com/some/path/to/file.txt

#### func (*File) Read

```go
func (f *File) Read(p []byte) (n int, err error)
```
Read calls the underlying ftp.File Read.

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek calls the underlying ftp.File Seek.

#### func (*File) Size

```go
func (f *File) Size() (uint64, error)
```
Size returns the size of the remote file.

#### func (*File) String

```go
func (f *File) String() string
```
String implement fmt.Stringer, returning the file's URI as the default string.

#### func (*File) Touch

```go
func (f *File) Touch() error
```
Touch creates a zero-length file on the vfs.File if no File exists. Update
File's last modified timestamp. Returns error if unable to touch File.

#### func (*File) URI

```go
func (f *File) URI() string
```
URI returns the File's URI as a string.

#### func (*File) Write

```go
func (f *File) Write(data []byte) (res int, err error)
```
Write calls the underlying ftp.File Write.

### type FileSystem

```go
type FileSystem struct {
}
```

FileSystem implements vfs.FileSystem for the FTP filesystem.

#### func  NewFileSystem

```go
func NewFileSystem() *FileSystem
```
NewFileSystem initializer for fileSystem struct.

#### func (*FileSystem) Client

```go
func (fs *FileSystem) Client(ctx context.Context, authority utils.Authority) (types.Client, error)
```
Client returns the underlying ftp client, creating it, if necessary See Overview
for authentication resolution

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns "Secure File Transfer Protocol"

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(authority, filePath string) (vfs.File, error)
```
NewFile function returns the FTP implementation of vfs.File.

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(authority, locPath string) (vfs.Location, error)
```
NewLocation function returns the FTP implementation of vfs.Location.

#### func (*FileSystem) Retry

```go
func (fs *FileSystem) Retry() vfs.Retry
```
Retry will return the default no-op retrier. The FTP client provides its own
retryer interface, and is available to override via the ftp.FileSystem Options
type.

#### func (*FileSystem) Scheme

```go
func (fs *FileSystem) Scheme() string
```
Scheme return "ftp" as the initial part of a file URI ie: ftp://

#### func (*FileSystem) WithClient

```go
func (fs *FileSystem) WithClient(client types.Client) *FileSystem
```
WithClient passes in an ftp client and returns the filesystem (chainable)

#### func (*FileSystem) WithOptions

```go
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem
```
WithOptions sets options for client and returns the filesystem (chainable)

### type Location

```go
type Location struct {
	Authority utils.Authority
}
```

Location implements the vfs.Location interface specific to ftp fs.

#### func (*Location) ChangeDir

```go
func (l *Location) ChangeDir(relativePath string) error
```
ChangeDir takes a relative path, and modifies the underlying Location's path.
The caller is modified by this so the only return is any error. For this
implementation there are no errors.

#### func (*Location) DeleteFile

```go
func (l *Location) DeleteFile(fileName string, _ ...options.DeleteOption) error
```
DeleteFile removes the file at fileName path.

#### func (*Location) Exists

```go
func (l *Location) Exists() (bool, error)
```
Exists returns true if the remote FTP file exists.

#### func (*Location) FileSystem

```go
func (l *Location) FileSystem() vfs.FileSystem
```
FileSystem returns a vfs.fileSystem interface of the location's underlying
fileSystem.

#### func (*Location) List

```go
func (l *Location) List() ([]string, error)
```
List calls FTP ReadDir to list all files in the location's path. If you have
many thousands of files at the given location, this could become quite
expensive.

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(prefix string) ([]string, error)
```
ListByPrefix calls FTP ReadDir with the location's path modified relatively by
the prefix arg passed to the function.

    - Returns ([]string{}, nil) in the case of a non-existent directory/prefix/location.
    - "relative" prefixes are allowed, ie, listByPrefix from "/some/path/" with prefix "to/somepattern" is the same as
      location "/some/path/to/" with prefix of "somepattern"
    - If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
      be checked first.

#### func (*Location) ListByRegex

```go
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error)
```
ListByRegex retrieves the filenames of all the files at the location's current
path, then filters out all those that don't match the given regex. The resource
considerations of List() apply here as well.

#### func (*Location) NewFile

```go
func (l *Location) NewFile(filePath string) (vfs.File, error)
```
NewFile uses the properties of the calling location to generate a vfs.File
(backed by an ftp.File). The filePath argument is expected to be a relative path
to the location's current path.

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relativePath string) (vfs.Location, error)
```
NewLocation makes a copy of the underlying Location, then modifies its path by
calling ChangeDir with the relativePath argument, returning the resulting
location. The only possible errors come from the call to ChangeDir, which, for
the FTP implementation doesn't ever result in an error.

#### func (*Location) Path

```go
func (l *Location) Path() string
```
Path returns the path the location references in most FTP calls.

#### func (*Location) String

```go
func (l *Location) String() string
```
String implement fmt.Stringer, returning the location's URI as the default
string.

#### func (*Location) URI

```go
func (l *Location) URI() string
```
URI returns the Location's URI as a string.

#### func (*Location) Volume

```go
func (l *Location) Volume() string
```
Volume returns the Authority the location is contained in.

### type Options

```go
type Options struct {
	Password    string // env var VFS_FTP_PASSWORD
	Protocol    string // env var VFS_FTP_PROTOCOL
	DisableEPSV *bool  // env var VFS_DISABLE_EPSV
	DebugWriter io.Writer
	TLSConfig   *tls.Config
	DialTimeout time.Duration
}
```

Options struct implements the vfs.Options interface, providing optional
parameters for creating and ftp filesystem.
