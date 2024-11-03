# mem

---

Package mem - in-memory backend VFS implementation.

### Usage

Rely on github.com/c2fo/vfs/v6/backend

```go
    import(
        "github.com/c2fo/vfs/v6/backend"
        "github.com/c2fo/vfs/v6/backend/mem"
    )
    func UseFs() error {
        fs := backend.Backend(mem.Scheme)
        ...
    }
```

Or call directly:

```go
      import _mem "github.com/c2fo/vfs/v6/backend/mem"

      func DoSomething() {
    	fs := _mem.NewFileSystem()
          ...
      }
```

## Usage

```go
const Scheme = "mem"
```
Scheme defines the FileSystem type's underlying implementation.

### type File

```go
type File struct {
}
```

File implements vfs.File interface for the in-memory implementation of
FileSystem. A file struct holds a pointer to a single memFile. Multiple threads
will refer to the same memFile. Simultaneous reading is allowed, but writing and
closing are protected by locks.

#### func (*File) Close

```go
func (f *File) Close() error
```
Close imitates io.Closer by resetting the cursor and setting a boolean

#### func (*File) CopyToFile

```go
func (f *File) CopyToFile(target vfs.File) error
```
CopyToFile copies the receiver file into the target file. The target file is
deleted, so any references to it will be nil. In order to access the target
after calling CopyToFile use its previous path to call it using the fsMap.
Additionally, after this is called, f's cursor will reset as if it had been
closed.

#### func (*File) CopyToLocation

```go
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error)
```
CopyToLocation copies the current file to the given location. If file exists at
given location contents are simply overwritten using "CopyToFile", otherwise a
newFile is made, takes the contents of the current file, and ends up at the
given location

#### func (*File) Delete

```go
func (f *File) Delete() error
```
Delete removes the file from the FileSystem. Sets it path in the fsMap to nil,
and also nils the file's members

#### func (*File) Exists

```go
func (f *File) Exists() (bool, error)
```
Exists returns whether or not a file exists. Creating a file does not guarantee
its existence, but creating one and writing to it does

#### func (*File) LastModified

```go
func (f *File) LastModified() (*time.Time, error)
```
LastModified simply returns the file's lastModified, if the file exists

#### func (*File) Location

```go
func (f *File) Location() vfs.Location
```
Location simply returns the file's underlying location struct pointer

#### func (*File) MoveToFile

```go
func (f *File) MoveToFile(file vfs.File) error
```
MoveToFile creates a newFile, and moves it to "file". The receiver is always
deleted (since it's being "moved")

#### func (*File) MoveToLocation

```go
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error)
```
MoveToLocation moves the receiver file to the passed in location. It does so by
creating a copy of 'f' in "location". 'f' is subsequently deleted

#### func (*File) Name

```go
func (f *File) Name() string
```
Name returns the basename of the file

#### func (*File) Path

```go
func (f *File) Path() string
```
Path returns the absolute path to the file

#### func (*File) Read

```go
func (f *File) Read(p []byte) (n int, err error)
```
Read implements the io.Reader interface. Returns number of bytes read and
potential errors

#### func (*File) Seek

```go
func (f *File) Seek(offset int64, whence int) (int64, error)
```
Seek implements the io.Seeker interface. Returns the current position of the
cursor and errors if any

#### func (*File) Size

```go
func (f *File) Size() (uint64, error)
```
Size returns the size of the file contents. In our case, the length of the
file's byte slice

#### func (*File) String

```go
func (f *File) String() string
```
String implements the io.Stringer interface. It returns a string representation
of the file's URI

#### func (*File) Touch

```go
func (f *File) Touch() error
```
Touch takes an in-memory vfs.File, makes it existent, and updates the
lastModified

#### func (*File) URI

```go
func (f *File) URI() string
```
URI returns the file's URI, if it exists

#### func (*File) Write

```go
func (f *File) Write(p []byte) (int, error)
```
Write implements the io.Writer interface. Returns number of bytes written and
any errors

### type FileSystem

```go
type FileSystem struct {
	sync.Mutex
}
```

FileSystem implements vfs.FileSystem for an in-memory file system.

#### func  NewFileSystem

```go
func NewFileSystem() *FileSystem
```
NewFileSystem is used to initialize the file system struct for an in-memory FileSystem.

#### func (*FileSystem) Name

```go
func (fs *FileSystem) Name() string
```
Name returns the name of the underlying FileSystem

#### func (*FileSystem) NewFile

```go
func (fs *FileSystem) NewFile(volume string, absFilePath string) (vfs.File, error)
```
NewFile function returns the in-memory implementation of vfs.File. Since this is
inside FileSystem, we assume that the caller knows that the CWD is the root. If
a non-absolute path is given, an error is thrown. Additionally, a file does not
technically exist until a call to "Touch()" is made on it. The "Touch" call
links the file with FileSystem's map and brings it into existence. If a file is
written to before a touch call, Write() will take care of that call. This is
true for other functions as well and existence only poses a problem in the
context of deletion or copying FROM a non-existent file.

#### func (*FileSystem) NewLocation

```go
func (fs *FileSystem) NewLocation(volume string, absLocPath string) (vfs.Location, error)
```
NewLocation function returns the in-memory implementation of vfs.Location. A
location always exists. If a file is created on a location that has not yet been
made in the fsMap, then the location will be created with the file

#### func (*FileSystem) Retry

```go
func (fs *FileSystem) Retry() vfs.Retry
```
Retry will return a retrier provided via options, or a no-op if none is
provided.

#### func (*FileSystem) Scheme

```go
func (fs *FileSystem) Scheme() string
```
Scheme returns the scheme of the underlying FileSystem

### type Location

```go
type Location struct {
}
```

Location implements the vfs.Location interface specific to in-memory FileSystem.

#### func (*Location) ChangeDir

```go
func (l *Location) ChangeDir(relLocPath string) error
```
ChangeDir simply changes the directory of the location

#### func (*Location) DeleteFile

```go
func (l *Location) DeleteFile(relFilePath string) error
```
DeleteFile locates the file given the fileName and calls delete on it

#### func (*Location) Exists

```go
func (l *Location) Exists() (bool, error)
```
Exists always returns true on locations

#### func (*Location) FileSystem

```go
func (l *Location) FileSystem() vfs.FileSystem
```
FileSystem returns the type of file system location exists on, if it exists at
all

#### func (*Location) List

```go
func (l *Location) List() ([]string, error)
```
List finds all of the files living at the current location and returns them in a
slice of strings. If there are no files at location, then an empty slice will be
returned

#### func (*Location) ListByPrefix

```go
func (l *Location) ListByPrefix(prefix string) ([]string, error)
```
ListByPrefix tags a prefix onto the current path and in a slice, returns all
file base names whose full paths contain that substring Returns empty slice if
nothing found

#### func (*Location) ListByRegex

```go
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error)
```
ListByRegex takes a regular expression and returns a slice of strings containing
the base names of files found that matched the regular expression. Returns an
empty slice upon nothing found

#### func (*Location) NewFile

```go
func (l *Location) NewFile(relFilePath string) (vfs.File, error)
```
NewFile creates a vfs.File given its relative path and tags it onto "l's" path

#### func (*Location) NewLocation

```go
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error)
```
NewLocation creates a new location at the given relative path, which is tagged
onto the current locations absolute path

#### func (*Location) Path

```go
func (l *Location) Path() string
```
Path returns the full, absolute path of the location with leading and trailing
slashes

#### func (*Location) String

```go
func (l *Location) String() string
```
String implements io.Stringer by returning the location's URI as a string

#### func (*Location) URI

```go
func (l *Location) URI() string
```
URI returns the URI of the location if the location exists

#### func (*Location) Volume

```go
func (l *Location) Volume() string
```
Volume returns the volume of the current FileSystem.
