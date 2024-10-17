# utils

---


```go
    import "github.com/c2fo/vfs/v6/utils"
```

#### Error Constants

```go
const (
	// ErrBadAbsFilePath constant is returned when a file path is not absolute
	ErrBadAbsFilePath = "absolute file path is invalid - must include leading slash and may not include trailing slash"
	// ErrBadRelFilePath constant is returned when a file path is not relative
	ErrBadRelFilePath = "relative file path is invalid - may not include leading or trailing slashes"
	// ErrBadAbsLocationPath constant is returned when a file path is not absolute
	ErrBadAbsLocationPath = "absolute location path is invalid - must include leading and trailing slashes"
	// ErrBadRelLocationPath constant is returned when a file path is not relative
	ErrBadRelLocationPath = "relative location path is invalid - may not include leading slash but must include trailing slash"
	// TouchCopyMinBufferSize min buffer size used in TouchCopyBuffered in bytes
	ErrBadPrefix = "prefix is invalid - may not include leading or trailing slashes and may not be empty"
	// TouchCopyMinBufferSize min buffer size used in TouchCopyBuffered in bytes
	TouchCopyMinBufferSize = 262144
)
```

#### func  EnsureLeadingSlash

```go
func EnsureLeadingSlash(dir string) string
```
EnsureLeadingSlash is like EnsureTrailingSlash except that it adds the leading
slash if needed.

#### func  EnsureTrailingSlash

```go
func EnsureTrailingSlash(dir string) string
```
EnsureTrailingSlash is like AddTrailingSlash but will only ever use / since it's
use for web uri's, never a Windows OS path.

#### func  GetFileURI

```go
func GetFileURI(f vfs.File) string
```
GetFileURI returns a File URI

#### func  GetLocationURI

```go
func GetLocationURI(l vfs.Location) string
```
GetLocationURI returns a Location URI

#### func  PathToURI

```go
func PathToURI(p string) (string, error)
```
PathToURI takes a relative or absolute path and returns an OS URI. 
* We assume non-scheme path is an OS File or Location. 
* We assume volume(URI authority) is empty. 
* We assume relative paths are relative to the pwd (program's working directory)
* We assume an empty path is equal to the root path: "/"

| original path | becomes URI |
| --------------- | --------------- |
| /absolute/path/to/file.txt | file:///absolute/path/to/file.txt |
| /some/absolute/path/ | file:///absolute/path/ |
| relative/path/to/file.txt | file:///absolute/path/with/relative/path/to/file.txt |
| relative/path/ | file:///absolute/path/with/relative/path/ |

#### func  RemoveLeadingSlash

```go
func RemoveLeadingSlash(path string) string
```
RemoveLeadingSlash removes leading slash, if any

#### func  RemoveTrailingSlash

```go
func RemoveTrailingSlash(path string) string
```
RemoveTrailingSlash removes trailing slash, if any

#### func  TouchCopy

```go
func TouchCopy(writer io.Writer, reader io.Reader) error
```
TouchCopy is a wrapper around [io.Copy](https://godoc.org/io#Copy) which ensures that even empty source files
(reader) will get written as an empty file. It guarantees a Write() call on the
target file. Deprecated: Use TouchCopyBuffer Instead

#### func  TouchCopyBuffered

```go
func TouchCopyBuffered(writer io.Writer, reader io.Reader, bufferSize int) error
```
TouchCopyBuffered is a wrapper around [io.CopyBuffer](https://godoc.org/io#CopyBuffer) which ensures that even
empty source files (reader) will get written as an empty file. It guarantees a
Write() call on the target file. bufferSize is in bytes and if is less than
TouchCopyMinBufferSize will result in a buffer of size TouchCopyMinBufferSize
bytes. If bufferSize is > TouchCopyMinBufferSize it will result in a buffer of
size bufferSize bytes

#### func  UpdateLastModifiedByMoving

```go
func UpdateLastModifiedByMoving(file vfs.File) error
```
UpdateLastModifiedByMoving is used by some backends' Touch() method when a file
already exists.

#### func  ValidateAbsoluteFilePath

```go
func ValidateAbsoluteFilePath(name string) error
```
ValidateAbsoluteFilePath ensures that a file path has a leading slash but not a
trailing slash

#### func  ValidateAbsoluteLocationPath

```go
func ValidateAbsoluteLocationPath(name string) error
```
ValidateAbsoluteLocationPath ensure that a file path has both leading and
trailing slashes

#### func  ValidatePrefix

```go
func ValidatePrefix(prefix string) error
```
ValidatePrefix ensures that a prefix path has neither leading nor trailing
slashes may not be empty but unlike relative file path, *may* be simply "."

#### func  ValidateRelativeFilePath

```go
func ValidateRelativeFilePath(name string) error
```
ValidateRelativeFilePath ensures that a file path has neither leading nor
trailing slashes

#### func  ValidateRelativeLocationPath

```go
func ValidateRelativeLocationPath(name string) error
```
ValidateRelativeLocationPath ensure that a file path has no leading slash but
has a trailing slash

#### type Authority

```go
type Authority struct {
}
```

Authority represents host, port and userinfo (user/pass) in a URI

#### func  NewAuthority

```go
func NewAuthority(authority string) (Authority, error)
```
NewAuthority initializes Authority struct by parsing authority string.

#### func (Authority) Host

```go
func (a Authority) Host() string
```
Host returns the host portion of an authority

#### func (Authority) HostPortStr

```go
func (a Authority) HostPortStr() string
```
HostPortStr returns a concatenated string of host and port from authority,
separated by a colon, ie "host.com:1234"

#### func (Authority) Port

```go
func (a Authority) Port() uint16
```
Port returns the port portion of an authority

#### func (Authority) String

```go
func (a Authority) String() string
```
String() returns a string representation of authority. It does not include
password per https://tools.ietf.org/html/rfc3986#section-3.2.1

    Applications should not render as clear text any data after the first colon (":") character found within a userinfo
    subcomponent unless the data after the colon is the empty string (indicating no password).

#### func (Authority) UserInfo

```go
func (a Authority) UserInfo() UserInfo
```
UserInfo returns the userinfo section of authority. userinfo is username and
password(deprecated).

#### type UserInfo

```go
type UserInfo struct {
}
```

UserInfo represents user/pass portion of a URI

#### func (UserInfo) Password

```go
func (u UserInfo) Password() string
```
Password returns the password of a URI UserInfo. May be an empty string.

#### func (UserInfo) Username

```go
func (u UserInfo) Username() string
```
Username returns the username of a URI UserInfo. May be an empty string.
