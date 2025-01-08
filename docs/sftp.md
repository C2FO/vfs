## sftp

---

Package sftp - SFTP VFS implementation.

In this backend, any new FileSystem instance is a new connection to the remote server.  It may be impolite
to take up a large number of a server's available connections, so better to use the same filesystem.

### Usage

Rely on github.com/c2fo/vfs/v6/backend

```go
      import(
    	  "github.com/c2fo/vfs/v6/backend"
    	  "github.com/c2fo/vfs/v6/backend/sftp"
      )

      func UseFs() error {
    	  fs := backend.Backend(sftp.Scheme)
    	  ...
      }
```

Or call directly:

```go
      import "github.com/c2fo/vfs/v6/backend/sftp"

      func DoSomething() {
    	  fs := sftp.NewFileSystem()

    	  location, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
    	  if err != nil {
    		 #handle error
    	  }
    	  ...
      }
```

sftp can be augmented with some implementation-specific methods. Backend returns
[vfs.FileSystem](../README.md#type-filesystem) interface so it would have to be cast as [sftp.FileSystem](#type-filesystem) to use
them.

These methods are chainable:

* `(*FileSystem) WithClient(client interface{})*FileSystem`
* `(*FileSystem) WithOptions(opts vfs.Options) *FileSystem`

```go
      func DoSomething() {
    	  // cast if fs was created using backend.Backend().  Not necessary if created directly from sftp.NewFileSystem().
    	  fs := backend.Backend(sftp.Scheme)
    	  fs = fs.(*sftp.FileSystem)

    	  // to pass specific client
    	  sshClient, err := ssh.Dial("tcp", "myuser@server.com:22", &ssh.ClientConfig{
    		  User:            "someuser",
    		  Auth:            []ssh.AuthMethod{ssh.Password("mypassword")},
    		  HostKeyCallback: ssh.InsecureIgnoreHostKey,
    	  })
    	  // handle error
    	  client, err := _sftp.NewClient(sshClient)
    	  // handle error

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
    	  // handle error

    	  file := location.NewFile("myfile.txt")
    	  // handle error

    	  _, err := file.Write([]bytes("some text")
    	  // handle error

    	  err := file.Close()
    	  // handle error

      }
```

### Authentication

Authentication, by default, occurs automatically when [Client()](#func-filesystem-client) is called. Since
user is part of the URI authority section (Volume), auth is handled slightly
differently than other vfs [backends](backend.md).

A client is initialized lazily, meaning we only make a connection to the server
at the last moment so we are free to modify options until then. The
authenticated session is closed any time [WithOption()](#func-filesystem-withoptions), [WithClient()](#func-filesystem-withclient),
or [Close()](#func-file-close) occurs. Currently, that means that closing a file belonging to an fs will break
the connection of any other open file on the same fs.

#### USERNAME

User may only be set in the [URI authority](https://tools.ietf.org/html/rfc3986#section-3.2) section (Volume in vfs parlance).

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

Passwords may be passed via [Options](#type-options).Password or via the environmental variable
`VFS_SFTP_PASSWORD`.

SSH keys may be passed via [Options](#type-options).KeyFilePath and (optionally)
[Options](#type-options).KeyPassphrase. They can also be passed via environmental variables
`VFS_SFTP_KEYFILE` and `VFS_SFTP_KEYFILE_PASSPHRASE`, respectively.

### Known Hosts

Known hosts ensures that the server you're connecting to hasn't been somehow
redirected to another server, collecting your info (man-in-the-middle attack).
Handling for this can be accomplished via:

1. [Options](#type-options).KnownHostsString which accepts a string.
2. [Options](#type-options).KnownHostsFile or environmental variable `VFS_SFTP_KNOWN_HOSTS_FILE` which accepts a path to a known_hosts file.
3. [Options](#type-options).KnownHostsCallback which allows you to specify any of the [ssh.AuthMethod](https://godoc.org/golang.org/x/crypto/ssh#AuthMethod)
functions. Environmental variable `VFS_SFTP_INSECURE_KNOWN_HOSTS` will set this callback function to
[ssh.InsecureIgnoreHostKey](https://godoc.org/golang.org/x/crypto/ssh#InsecureIgnoreHostKey) which may be helpful for
testing but should not be used in production.
4. Defaults to trying to find and use <homedir>/.ssh/known_hosts.  For unix, system-wide location /etc/ssh/.ssh/known hosts is also checked. SSH doesn't exist natively on Windows and each third-party implementation has a different location for known_hosts. Because of this, no attempt is made to find a system-wide file for Windows.  It's better to specify in KnownHostsFile in that case.

### Other Options

Passing in multiple host key algorithms, key exchange algorithms is supported -
these are specified as string slices. Example:
```go
    fs = fs.WithOptions(
    	sftp.Options{
    		KeyExchanges: []string{ "diffie-hellman-group-a256", "ecdh-sha2-nistp256" },
    		Ciphers: []string{ "aes256-ctr", "aes192-ctr", "aes128-ctr" },
    		MACs: []string{ "hmac-sha2-256", "hmac-sha2-512" },
    		HostKeyAlgorithms: []string{ "ssh-rsa", "ssh-ed25519" },
    		// other settings
    	},
      )
```

### AutoDisconnect

When dialing a TCP connection, Go doesn't disconnect for you.  This is true even when the connection falls out of scope, and even when
garbage collection is forced.  The connection must be explicitly closed.  Unfortunately, VFS.FileSystem has no explicit close mechanism.

Instead, the SFTP backend will automatically disconnect 10 seconds (default) after connection.  This disconnect timer is
canceled anytime a server-side request (like list, read, etc) is made.  Once the request has completed, a new timer will begin.
If the timer expires (because it is not interrupted by any request), the server connection will be closed.  Any subsequent server
request will first reconnect, perform the request, and start a new disconnect timer.

[Options](#type-options).AutoDisconnect accepts an integer representing the number seconds before disconnecting after being idle.
Default value is 10 seconds.

Any server request action using the same underlying FileSystem (and therefore sftp client), will reset the timer.  This
should be the most desirable behavior.

```go
    func doSFTPStuff() {
    	fs := sftp.NewFileSystem()
    	loc, err := fs.NewLocation("myuser@server.com:22", "/some/path/")
    	file1, _ := loc.NewFile("file1.txt")
    	file2, _ := loc.NewFile("file2.txt")
    	file1.Touch()                               // "touches" file and starts disconnect timer (default: 10sec)
    	_, _ := loc.List()                          // stops timer, does location listing, resets timer to 10 seconds
    	file2.Touch()                               // stops timer, "touches" file2, resets timer to 10 seconds
    	time.Sleep(time.Duration(15) * time.Second) // pause for 15 seconds, disconnects for server after 10 seconds
    	_, _ := loc.List()                          // reconnects, does location listing, starts new disconnect timer
    	return
    }

    func main {
    	// call our sftp function
    	doSFTPStuff()
    	// even though the vfs sftp objects have fallen out of scope, our connection remains UNTIL the timer counts down

    	// do more work (that take longer than 10 seconds
    	doOtherTimeConsumingStuff()

    	// at some point during the above, the sftp connection will have closed
    }
```

NOTE: AutoDisconnect has nothing to do with "keep alive".  Here we're only concerned with releasing resources, not keeping
the server from disconnecting us.  If that is something you want, you'd have to implement yourself, injecting your own
client using WithClient().

## Usage

```go
const Scheme = "sftp"
```
Scheme defines the filesystem type.

### type Client

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
	Close() error
}
```

Client is an interface to make it easier to test

### type File

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
Close calls the underlying [sftp.File](https://godoc.org/github.com/pkg/sftp#File) Close, if opened, and clears the internal
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
CopyToLocation creates a copy of [*File](#type-file), using the file's current path as the new
file's path at the given location.

#### func (*File) Delete

```go
func (f *File) Delete(opts ...options.DeleteOption) error
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
LastModified returns the LastModified property of [sftp.File](https://godoc.org/github.com/pkg/sftp#File).

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
[File.CopyToFile](#func-file-copytofile). If the copy succeeds, the source file is deleted. Any errors
from the copy or delete are returned. If the given location is also sftp AND for
the same user and host, the [sftp.Rename](https://godoc.org/github.com/pkg/sftp#Client.Rename) method is used, otherwise we'll do an
[io.Copy](https://godoc.org/io#Copy) to the destination file then delete source file.

#### func (*File) MoveToLocation

```go
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error)
```
MoveToLocation works by creating a new file on the target location then calling
[MoveToFile()](#func-file-movetofile) on it.

#### func (*File) Name

```go
func (f *File) Name() string
```
Name returns the path portion of the file's path property. IE: "file.txt" of
"sftp://someuser@host.com/some/path/to/file.txt"

#### func (*File) Path

```go
func (f *File) Path() string
```
Path return the directory portion of the file's path. IE: "path/to" of
"sftp://someuser@host.com/some/path/to/file.txt"

#### func (*File) Read

```go
func (f *File) Read(p []byte) (n int, err error)
```
Read calls the underlying [sftp.File Read](https://godoc.org/github.com/pkg/sftp#File.Read).

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek calls the underlying [sftp.File Seek](https://godoc.org/github.com/pkg/sftp#File.Seek).

#### func (*File) Size

```go
func (f *File) Size() (uint64, error)
```
Size returns the size of the remote file.

#### func (*File) String

```go
func (f *File) String() string
```
String implement [fmt.Stringer](https://golang.org/pkg/fmt/#Stringer), returning the file's URI as the default string.

#### func (*File) Touch

```go
func (f *File) Touch() error
```
Touch creates a zero-length file on the [vfs.File](../README.md#type-file) if no File exists. Update
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
Write calls the underlying [sftp.File Write](https://godoc.org/github.com/pkg/sftp#File.Write).

### type FileSystem

```go
type FileSystem struct {
}
```

FileSystem implements vfs.FileSystem for the SFTP filesystem.

#### func  NewFileSystem

```go
func NewFileSystem() *FileSystem
```
NewFileSystem initializer for fileSystem struct.

#### func (*FileSystem) Client

```go
func (fs *FileSystem) Client(authority utils.Authority) (Client, error)
```
Client returns the underlying [sftp.Client](https://godoc.org/github.com/pkg/sftp#Client), creating it, if necessary See
[Overview](#authentication) for authentication resolution

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns "Secure File Transfer Protocol"

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(authority, filePath string, opts ...options.NewFileOption) (vfs.File, error)
```
NewFile function returns the SFTP implementation of vfs.File.

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(authority, locPath string) (vfs.Location, error)
```
NewLocation function returns the SFTP implementation of [vfs.Location](../README.md#type-location).

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

### type Location

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
func (l *Location) DeleteFile(fileName string, opts ...options.DeleteOption) error
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
FileSystem returns a [vfs.FileSystem](../README.md#type-filesystem) interface of the location's underlying
fileSystem.

#### func (*Location) List

```go
func (l *Location) List() ([]string, error)
```
List calls [sftp.ReadDir](https://godoc.org/github.com/pkg/sftp#Client.ReadDir) to list all files in the location's path. If you have
many thousands of files at the given location, this could become quite
expensive.

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(prefix string) ([]string, error)
```
ListByPrefix calls [sftp.ReadDir](https://godoc.org/github.com/pkg/sftp#Client.ReadDir) with the location's path modified relatively by
the prefix arg passed to the function.

#### func (*Location) ListByRegex

```go
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error)
```
ListByRegex retrieves the filenames of all the files at the location's current
path, then filters out all those that don't match the given regex. The resource
considerations of [List()](#func-location-list) apply here as well.

#### func (*Location) NewFile

```go
func (l *Location) NewFile(filePath string, opts ...options.NewFileOption) (vfs.File, error)
```
NewFile uses the properties of the calling location to generate a [vfs.File](../README.md#type-file)
(backed by an sftp.File). The filePath argument is expected to be a relative
path to the location's current path.

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relativePath string) (vfs.Location, error)
```
NewLocation makes a copy of the underlying [Location](#type-location), then modifies its path by
calling [ChangeDir](#func-location-changedir) with the relativePath argument, returning the resulting
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
Volume returns the [Authority](utils.md#type-authority) the location is contained in.

### type Options

```go
type Options struct {
	Password           string              `json:"password,omitempty"`       // env var VFS_SFTP_PASSWORD
	KeyFilePath        string              `json:"keyFilePath,omitempty"`    // env var VFS_SFTP_KEYFILE
	KeyPassphrase      string              `json:"keyPassphrase,omitempty"`  // env var VFS_SFTP_KEYFILE_PASSPHRASE
	KnownHostsFile     string              `json:"knownHostsFile,omitempty"` // env var VFS_SFTP_KNOWN_HOSTS_FILE
	KnownHostsString   string              `json:"knownHostsString,omitempty"`
	KeyExchanges       []string            `json:"keyExchanges,omitempty"`
	Ciphers            []string            `json:"ciphers,omitempty"`
	MACs               []string            `json:"macs,omitempty"`
	HostKeyAlgorithms  []string            `json:"hostKeyAlgorithms,omitempty"`
	AutoDisconnect     int                 `json:"autoDisconnect,omitempty"` // seconds before disconnecting. default: 10
	KnownHostsCallback ssh.HostKeyCallback // env var VFS_SFTP_INSECURE_KNOWN_HOSTS
	FileBufferSize     int                 // Buffer Size In Bytes Used with utils.TouchCopyBuffered
}
```

Options holds sftp-specific options. Currently only client options are used.

### type ReadWriteSeekCloser

```go
type ReadWriteSeekCloser interface {
	io.ReadWriteSeeker
	io.Closer
}
```

ReadWriteSeekCloser is a read write seek closer interface representing capabilities needed from std libs sftp File struct.
