# utils

--

## Usage

```go
const (
	// Windows constant represents a target operating system running a version of Microsoft Windows
	Windows = "windows"
	// BadFilePrefix constant is returned when path has leading slash or backslash
	BadFilePrefix = "expecting only a filename prefix, which may not include slashes or backslashes"
)
```

#### func  AddTrailingSlash

```go
func AddTrailingSlash(path string) string
```
AddTrailingSlash is a helper function accepts a path string and returns the path
string with a trailing slash if there wasn't one.

#### func  CleanPrefix

```go
func CleanPrefix(prefix string) string
```
CleanPrefix resolves relative dot pathing, removing any leading . or / and
removes any trailing /

#### func  EnsureTrailingSlash

```go
func EnsureTrailingSlash(dir string) string
```
EnsureTrailingSlash is like AddTrailingSlash but will only ever use / since it's
use for web uri's, never an Windows OS path.

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

#### func  TouchCopy

```go
func TouchCopy(writer, reader vfs.File) error
```
TouchCopy is a wrapper around io.Copy which ensures that even empty source files
(reader) will get written as an empty file. It guarantees a Write() call on the
target file.

#### func  ValidateFilePrefix

```go
func ValidateFilePrefix(filenamePrefix string) error
```
ValidateFilePrefix performs a validation check on a prefix. The prefix should
not include "/" or "\\" characters. An error is returned if either of those
conditions are true.
