# backend

---

Package backend provides a means of allowing backend file systems to
self-register on load via an init() call to backend.Register("some scheme",
vfs.FileSystem)

In this way, a caller of vfs backends can simply load the backend file system
(and ONLY those needed) and begin using it:

```go
    package main

    // import backend and each backend you intend to use
    import(
        "github.com/c2fo/vfs/v6/backend"
        "github.com/c2fo/vfs/v6/backend/os"
        "github.com/c2fo/vfs/v6/backend/s3"
    )

    func main() {
       var err error
       var osfile, s3file vfs.File

        // THEN begin using the file systems
        osfile, err = backend.Backend(os.Scheme).NewFile("", "/path/to/file.txt")
        if err != nil {
            panic(err)
        }

        s3file, err = backend.Backend(s3.Scheme).NewFile("mybucket", "/some/file.txt")
        if err != nil {
            panic(err)
        }

        err = osfile.CopyTo(s3file)
        if err != nil {
            panic(err)
        }
    }
```

### Development

To create your own backend, you must create a package that implements the interfaces:
[vfs.FileSystem](../README.md#type-filesystem), [vfs.Location](../README.md#type-location), and
[vfs.File](../README.md#type-file). Then ensure it registers itself on load:

```go
    package myexoticfilesystem

    import(
        ...
        "github.com/c2fo/vfs/v6"
        "github.com/c2fo/vfs/v6/backend"
    )

    // IMPLEMENT vfs interfaces
    ...

    // register backend
    func init() {
        backend.Register("exfs", &MyExoticFileSystem{})
    }
```

Then do use it in some other package do

```go
    package MyExoticFileSystem

    import(
        "github.com/c2fo/vfs/v6/backend"
        "github.com/acme/myexoticfilesystem"
    )

    ...

    func useNewBackend() error {
        myExoticFs, err = backend.Backend(myexoticfilesystem.Scheme)
        ...
    }
```

That's it. Simple.

## Usage

#### func  Backend

```go
func Backend(name string) vfs.FileSystem
```
Backend returns the backend file system by name

#### func  Register

```go
func Register(name string, v vfs.FileSystem)
```
Register a new file system in backend map

#### func  RegisteredBackends

```go
func RegisteredBackends() []string
```
RegisteredBackends returns an array of backend names

#### func  Unregister

```go
func Unregister(name string)
```
Unregister unregisters a file system from backend map

#### func  UnregisterAll

```go
func UnregisterAll()
```
UnregisterAll unregisters all file systems from backend map
