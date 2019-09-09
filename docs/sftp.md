## sftp

---


Package sftp SFTP VFS implementation.


### Usage

Rely on github.com/c2fo/vfs/backend

```go
      import(
    	  "github.com/c2fo/vfs/v5/backend"
    	  "github.com/c2fo/vfs/v5/backend/sftp"
      )

      func UseFs() error {
    	  fs, err := backend.Backend(sftp.Scheme)
    	  ...
      }
```

Or call directly:

```go
      import "github.com/c2fo/vfs/v5/backend/sftp"

      func DoSomething() {
    	  fs := sftp.NewFilesystem()

    	  location, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
    	  if err != nil {
    		 #handle error
    	  }
    	  ...
      }
```

sftp can be augmented with some implementation-specific methods. Backend returns
vfs.Filesystem interface so it would have to be cast as sftp.Filesystem to use
them.

These methods are chainable: 

* `(*FileSystem) WithClient(client interface{})*FileSystem`
* `(*FileSystem) WithOptions(opts vfs.Options) *FileSystem`

```go
      func DoSomething() {

    	  // cast if fs was created using backend.Backend().  Not necessary if created directly from sftp.NewFilesystem().
    	  fs, err := backend.Backend(sftp.Scheme)
    	  fs = fs.(*sftp.Filesystem)

    	  // to pass specific client
    	  sshClient, err := ssh.Dial("tcp", "myuser@server.com:22", &ssh.ClientConfig{
    		  User:            "someuser",
    		  Auth:            []ssh.AuthMethod{ssh.Password("mypassword")},
    		  HostKeyCallback: ssh.InsecureIgnoreHostKey,
    	  })
    	  #handle error
    	  client, err := _sftp.NewClient(sshClient)
    	  #handle error

    	  fs = fs.WithClient(client)

    	  // to pass in client options. See Options for more info.  Note that changes to Options will make nil any client.
    	  // This behavior ensures that changes to settings will get applied to a newly created client.
    	  fs = fs.WithOptions(
    		  sftp.Options{
    			  KeyFilePath:   "/home/Bob/.ssh/id_rsa",
    			  KeyPassphrase: "s3cr3t",
    			  KnownHostsCallback: ssh.InsecureIgnoreHostKey,
    		  },
    	  )

    	  location, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
    	  #handle error

    	  file := location.NewFile("myfile.txt")
    	  #handle error

    	  _, err := file.Write([]bytes("some text")
    	  #handle error

    	  err := file.Close()
    	  #handle error

      }
```


### Authentication

Authentication, by default, occurs automatically when Client() is called. Since
user is part of the URI authority section (Volume), auth is handled slightly
differently than other vfs backends.

A client is initialized lazily, meaning we only make a connection to the server
at the last moment so we are free to modify options until then. The
authenticated session is closed any time WithOption(), WithClient(), or Close()
occurs. Currently, that means that closing a file belonging to an fs will break
the connection of any other open file on the same fs.


#### USERNAME

User may only be set in the URI authority section (Volume in vfs parlance).

     scheme             host
     __/             ___/____  port
    /  \            /        \ /\
    sftp://someuser@server.com:22/path/to/file.txt
    	   \____________________/ \______________/
    	   \______/       \               \
    		  /     authority section    path
    	 username       (Volume)

sftp vfs backend accepts either a password or an ssh key, with or without a
passphrase.

#### PASSWORD/PASSPHRASE

Passwords may be passed via Options.Password or via the environmental variable
VFS_SFTP_PASSWORD.

SSH keys may be passed via Options.KeyFilePath and (optionally)
Options.KeyPassphrase. They can also be passed via environmental variables
VFS_SFTP_KEYFILE and VFS_SFTP_KEYFILE_PASSPHRASE, respectively.

_Note that as of Go 1.12, OPENSSH private key format is not supported when encrypted
(with passphrase). See https://github.com/golang/go/issues/18692 To force
creation of PEM format(instead of OPENSSH format), use `ssh-keygen -m PEM`_


### KNOWN HOSTS

Known hosts ensures that the server you're connecting to hasn't been somehow
redirected to another server, collecting your info (man-in-the-middle attack).
Handling for this can be accomplished via: 

1. Options.KnownHostsString which accepts a string. 
2. Options.KnownHostsFile or environmental variable VFS_SFTP_KNOWN_HOSTS_FILE which accepts a path to a known_hosts file.
3. Options.KnownHostsCallback which allows you to specify any of the ssh.AuthMethod
functions. Environmental variable VFS_SFTP_INSECURE_KNOWN_HOSTS will set this callback function to ssh.InsecureIgnoreHostKey
 which may be helpful for testing but should not be used in production.
 which may be helpful for testing but should not be used in production.

## Usage

```go
const Scheme = "sftp"
```
Scheme defines the filesystem type.

#### type Client

```go
type Client interface {
	Chtimes(path string, atime, mtime time.Time) error
	Create(path string) (*_sftp.File, error)
	MkdirAll(path string) error
	OpenFile(path string, f int) (*_sftp.File, error)
	ReadDir(p string) ([]os.FileInfo, error)
	Remove(path string) error
	Rename(oldname, newname string) error
	Stat(p string) (os.FileInfo, error)
}
```

Client is an interface to make it easier to test

#### type File

```go
type File struct {
	Authority utils.Authority
}
```

File implements vfs.File interface for SFTP fs.

#### func (*File) Close

```go
func (f *File) Close() error
```
Close calls the underlying sftp.File Close, if opened, and clears the internal
pointer

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
func (f *File) Delete() error
```
Delete removes the remote file. Error is returned, if any.

#### func (*File) Exists

```go
func (f *File) Exists() (bool, error)
```
Exists returns a boolean of whether or not the file exists on the sftp server

#### func (*File) LastModified

```go
func (f *File) LastModified() (*time.Time, error)
```
LastModified returns the LastModified property of sftp file.

#### func (*File) Location

```go
func (f *File) Location() vfs.Location
```
Location returns a vfs.Location at the location of the file. IE: if file is at
sftp://someuser@host.com/here/is/the/file.txt the location points to
sftp://someuser@host.com/here/is/the/

#### func (*File) MoveToFile

```go
func (f *File) MoveToFile(t vfs.File) error
```
MoveToFile puts the contents of File into the targetFile passed using
File.CopyToFile. If the copy succeeds, the source file is deleted. Any errors
from the copy or delete are returned. If the given location is also sftp AND for
the same user and host, the sftp Rename method is used, otherwise we'll do a an
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
"sftp://someuser@host.com/some/path/to/file.txt

#### func (*File) Path

```go
func (f *File) Path() string
```
Path return the directory portion of the file's path. IE: "path/to" of
"sftp://someuser@host.com/some/path/to/file.txt

#### func (*File) Read

```go
func (f *File) Read(p []byte) (n int, err error)
```
Read calls the underlying sftp.File Read.

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek calls the underlying sftp.File Seek.

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
Write calls the underlying sftp.File Write.

#### type FileSystem

```go
type FileSystem struct {
}
```

FileSystem implements vfs.Filesystem for the SFTP filesystem.

#### func  NewFileSystem

```go
func NewFileSystem() *FileSystem
```
NewFileSystem initializer for fileSystem struct.

#### func (*FileSystem) Client

```go
func (fs *FileSystem) Client(authority utils.Authority) (Client, error)
```
Client returns the underlying sftp client, creating it, if necessary See
Overview for authentication resolution

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns "Secure File Transfer Protocol"

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(authority string, filePath string) (vfs.File, error)
```
NewFile function returns the SFTP implementation of vfs.File.

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(authority string, locPath string) (vfs.Location, error)
```
NewLocation function returns the SFTP implementation of vfs.Location.

#### func (*FileSystem) Retry

```go
func (fs *FileSystem) Retry() vfs.Retry
```
Retry will return the default no-op retrier. The SFTP client provides its own
retryer interface, and is available to override via the sftp.FileSystem Options
type.

#### func (*FileSystem) Scheme

```go
func (fs *FileSystem) Scheme() string
```
Scheme return "sftp" as the initial part of a file URI ie: sftp://

#### func (*FileSystem) WithClient

```go
func (fs *FileSystem) WithClient(client interface{}) *FileSystem
```
WithClient passes in an sftp client and returns the filesystem (chainable)

#### func (*FileSystem) WithOptions

```go
func (fs *FileSystem) WithOptions(opts vfs.Options) *FileSystem
```
WithOptions sets options for client and returns the filesystem (chainable)

#### type Location

```go
type Location struct {
	Authority utils.Authority
}
```

Location implements the vfs.Location interface specific to sftp fs.

#### func (*Location) ChangeDir

```go
func (l *Location) ChangeDir(relativePath string) error
```
ChangeDir takes a relative path, and modifies the underlying Location's path.
The caller is modified by this so the only return is any error. For this
implementation there are no errors.

#### func (*Location) DeleteFile

```go
func (l *Location) DeleteFile(fileName string) error
```
DeleteFile removes the file at fileName path.

#### func (*Location) Exists

```go
func (l *Location) Exists() (bool, error)
```
Exists returns true if the remote SFTP file exists.

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
List calls SFTP ReadDir to list all files in the location's path. If you have
many thousands of files at the given location, this could become quite
expensive.

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(prefix string) ([]string, error)
```
ListByPrefix calls SFTP ReadDir with the location's path modified relatively by
the prefix arg passed to the function.

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
(backed by an sftp.File). The filePath argument is expected to be a relative
path to the location's current path.

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relativePath string) (vfs.Location, error)
```
NewLocation makes a copy of the underlying Location, then modifies its path by
calling ChangeDir with the relativePath argument, returning the resulting
location. The only possible errors come from the call to ChangeDir, which, for
the SFTP implementation doesn't ever result in an error.

#### func (*Location) Path

```go
func (l *Location) Path() string
```
Path returns the path the location references in most SFTP calls.

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

#### type Options

```go
type Options struct {
	Password           string              `json:"accessKeyId,omitempty"`    // env var VFS_SFTP_PASSWORD
	KeyFilePath        string              `json:"keyFilePath,omitempty"`    // env var VFS_SFTP_KEYFILE
	KeyPassphrase      string              `json:"KeyPassphrase,omitempty"`  // env var VFS_SFTP_KEYFILE_PASSPHRASE
	KnownHostsFile     string              `json:"KnownHostsFile,omitempty"` // env var VFS_SFTP_KNOWN_HOSTS_FILE
	KnownHostsString   string              `json:"KnownHostsString,omitempty"`
	KnownHostsCallback ssh.HostKeyCallback //env var VFS_SFTP_INSECURE_KNOWN_HOSTS
	Retry              vfs.Retry
	MaxRetries         int
}
```

Options holds sftp-specific options. Currently only client options are used.

#### type SFTPFile

```go
type SFTPFile interface {
	io.ReadWriteSeeker
	io.Closer
}
```
