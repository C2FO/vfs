# os

---

Package os - built-in os lib VFS implementation.


### Usage

Rely on github.com/c2fo/vfs/v6/backend

```go
    import(
        "github.com/c2fo/vfs/v6/backend"
        "github.com/c2fo/vfs/v6/backend/os"
    )

    func UseFs() error {
        fs := backend.Backend(os.Scheme)
        ...
    }
```

Or call directly:

```go
    import _os "github.com/c2fo/vfs/v6/backend/os"

    func DoSomething() {
        fs := &_os.FileSystem{}
        ...
    }
```

### See Also

See: https://golang.org/pkg/os/

## Usage

```go
const Scheme = "file"
```
Scheme defines the file system type.

### type File

```go
type File struct {
}
```

File implements [vfs.File](../README.md#type-file) interface for S3 fs.

#### func (*File) Close

```go
func (f *File) Close() error
```
Close implements the [io.Closer](https://godoc.org/io#Closer) interface, closing the underlying *os.File. its
an error, if any.

#### func (*File) CopyToFile

```go
func (f *File) CopyToFile(target vfs.File) error
```
CopyToFile copies the file to a new File. It accepts a [vfs.File](../README.md#type-file) and returns an
error, if any.

#### func (*File) CopyToLocation

```go
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error)
```
CopyToLocation copies existing File to new Location with the same name. It
accepts a [vfs.Location](../README.md#type-location) and returns a [vfs.File](../README.md#type-file) and error, if any.

#### func (*File) Delete

```go
func (f *File) Delete() error
```
Delete unlinks the file returning any error or nil.

#### func (*File) Exists

```go
func (f *File) Exists() (bool, error)
```
Exists true if the file exists on the file system, otherwise false, and an error,
if any.

#### func (*File) LastModified

```go
func (f *File) LastModified() (*time.Time, error)
```
LastModified returns the timestamp of the file's mtime or error, if any.

#### func (*File) Location

```go
func (f *File) Location() vfs.Location
```
Location returns the underlying [os.Location](#type-location).

#### func (*File) MoveToFile

```go
func (f *File) MoveToFile(target vfs.File) error
```
MoveToFile move a file. It accepts a target vfs.File and returns an error, if
any.
 
__TODO:__ we might consider using os.Rename() for efficiency when
target.Location().FileSystem().Scheme equals f.Location().FileSystem().Scheme()

#### func (*File) MoveToLocation

```go
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error)
```
MoveToLocation moves a file to a new Location. It accepts a target vfs.Location
and returns a vfs.File and an error, if any. 

__TODO:__ we might consider using os.Rename() for efficiency when location.FileSystem().Scheme() equals
f.Location().FileSystem().Scheme()

#### func (*File) Name

```go
func (f *File) Name() string
```
Name returns the full name of the File relative to [Location.Name()](#func-filesystem-name).

#### func (*File) Path

```go
func (f *File) Path() string
```
Path returns the path of the File relative to [Location.Name()](#func-filesystem-name).

#### func (*File) Read

```go
func (f *File) Read(p []byte) (int, error)
```
Read implements the [io.Reader](https://godoc.org/io#Reader) interface. It returns the bytes read and an error,
if any.

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek implements the io.Seeker interface. It accepts an offset and "whence" where
0 means relative to the origin of the file, 1 means relative to the current
offset, and 2 means relative to the end. It returns the new offset and an error,
if any.

#### func (*File) Size

```go
func (f *File) Size() (uint64, error)
```
Size returns the size (in bytes) of the [File](#type-file) or any error.

#### func (*File) String

```go
func (f *File) String() string
```
String implement [fmt.Stringer](https://godoc.org/fmt#Stringer), returning the file's URI as the default string.

#### func (*File) URI

```go
func (f *File) URI() string
```
URI returns the [File](#type-file)'s URI as a string.

#### func (*File) Write

```go
func (f *File) Write(p []byte) (n int, err error)
```
Write implements the [io.Writer](https://godoc.org/io#Writer) interface. It accepts a slice of bytes and
returns the number of bytes written and an error, if any.

### type FileSystem

```go
type FileSystem struct{}
```

FileSystem implements [vfs.FileSystem](../README.md#type-filesystem) for the OS file system.

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns "os"

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error)
```
NewFile function returns the os implementation of [vfs.File](../README.md#type-file).

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error)
```
NewLocation function returns the os implementation of [vfs.Location](../README.md#type-location).

#### func (*FileSystem) Scheme

```go
func (fs *FileSystem) Scheme() string
```
Scheme return "file" as the initial part of a file URI ie: file://

### type Location

```go
type Location struct {
}
```

Location implements the  [vfs.Location](../README.md#type-location) interface specific to OS fs.

#### func (*Location) ChangeDir

```go
func (l *Location) ChangeDir(relativePath string) error
```
ChangeDir takes a relative path, and modifies the underlying [Location](#type-location)'s path.
The caller is modified by this so the only return is any error. For this
implementation there are no errors.

#### func (*Location) DeleteFile

```go
func (l *Location) DeleteFile(fileName string) error
```
DeleteFile deletes the file of the given name at the location. This is meant to
be a short cut for instantiating a new file and calling delete on that with all
the necessary error handling overhead.

#### func (*Location) Exists

```go
func (l *Location) Exists() (bool, error)
```
Exists returns true if the location exists, and the calling user has the
appropriate permissions. Will receive false without an error if the location
simply doesn't exist. Otherwise could receive false and any errors passed back
from the OS.

#### func (*Location) FileSystem

```go
func (l *Location) FileSystem() vfs.FileSystem
```
FileSystem returns a  [vfs.FileSystem](../README.md#type-filesystem) interface of the location's underlying
file system.

#### func (*Location) List

```go
func (l *Location) List() ([]string, error)
```
List returns a slice of all files in the top directory of the location.

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(prefix string) ([]string, error)
```
ListByPrefix returns a slice of all files starting with "prefix" in the top
directory of the location.

#### func (*Location) ListByRegex

```go
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error)
```
ListByRegex returns a slice of all files matching the regex in the top directory
of the location.

#### func (*Location) NewFile

```go
func (l *Location) NewFile(fileName string) (vfs.File, error)
```
NewFile uses the properties of the calling location to generate a  [vfs.File](../README.md#type-file)
(backed by an [os.File](#type-file)). A string argument is expected to be a relative path to
the location's current path.

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relativePath string) (vfs.Location, error)
```
NewLocation makes a copy of the underlying [Location](#type-location), then modifies its path by
calling ChangeDir with the relativePath argument, returning the resulting
location. The only possible errors come from the call to ChangeDir.

#### func (*Location) Path

```go
func (l *Location) Path() string
```
Path returns the location path.

#### func (*Location) String

```go
func (l *Location) String() string
```
String implement [fmt.Stringer](https://godoc.org/fmt#Stringer), returning the location's URI as the default
string.

#### func (*Location) URI

```go
func (l *Location) URI() string
```
URI returns the [Location](#type-location)'s URI as a string.

#### func (*Location) Volume

```go
func (l *Location) Volume() string
```
Volume returns the volume, if any, of the location. Given "C:\foo\bar" it returns "C:" on
Windows. On other platforms it returns "".
