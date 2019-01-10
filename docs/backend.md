# backend

--


Package backend provides a means of allowing backend filesystems to
self-register on load via an init() call to backend.Register("some scheme",
vfs.Filesystem)

In this way, a caller of vfs backends can simply load the backend filesystem
(and ONLY those needed) and begin using it:

    package main

    // import backend and each backend you intend to use
    import(
        "github.com/c2fo/vfs/backend"
        _ "github.com/c2fo/vfs/backend/os"
        _ "github.com/c2fo/vfs/backend/s3"
    )

    func main() {
       var err error
       var osfile, s3file vfs.File

        // THEN begin using the filesystems
        osfile, err = backend.Backend(os.Scheme).NewFile("", "/path/to/file.txt")
        if err != nil {
            panic(err)
        }

        s3file, err = backend.Backend(os.Scheme).NewFile("", "/some/file.txt")
        if err != nil {
            panic(err)
        }

        err = osfile.CopyTo(s3file)
        if err != nil {
            panic(err)
        }
    }


### Development

To create your own backend, you must create a package that implements both
[vfs.FileSystem](../README.md#type-filesystem), [vfs.Location](../README.md#type-location), and 
[vfs.File](../README.md#type-file) interfaces. Then ensure it registers itself on load:

    pacakge myexoticfilesystem

    import(
        ...
        "github.com/c2fo/vfs"
        "github.com/c2fo/vfs/backend"
    )

    // IMPLEMENT vfs interfaces
    ...

    // register backend
    func init() {
        backend.Register(
            "My Exotic Filesystem",
            &MyExoticFilesystem{},
        )
    }

Then do use it in some other package do

    pacakge MyExoticFilesystem

    import(
        "github.com/c2fo/vfs/backend"
        _ "github.com/acme/myexoticfilesystem"
    )

    ...

    func useNewBackend() error {
        myExoticFs, err = backend.Backend(myexoticfilesystem.Scheme)
        ...
    }

Thats it. Simple.

## Usage

#### func  Backend

```go
func Backend(name string) vfs.FileSystem
```
Backend returns the backend filesystem by name

#### func  Register

```go
func Register(name string, v vfs.FileSystem)
```
Register a new filesystem in backend map

#### func  RegisteredBackends

```go
func RegisteredBackends() []string
```
RegisteredBackends returns an array of backend names

#### func  Unregister

```go
func Unregister(name string)
```
Unregister unregisters a filesystem from backend map

#### func  UnregisterAll

```go
func UnregisterAll()
```
UnregisterAll unregisters all filesystems from backend map
