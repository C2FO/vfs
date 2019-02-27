/*
Package backend provides a means of allowing backend filesystems to self-register on load via an init() call to
backend.Register("some name", vfs.Filesystem)

In this way, a caller of vfs backends can simply load the backend filesystem (and ONLY those needed) and begin using it:

  package main

  // import backend and each backend you intend to use
  import(
      "github.com/c2fo/vfs/backend"
      "github.com/c2fo/vfs/backend/os"
      "github.com/c2fo/vfs/backend/s3"
  )

  func main() {
     var err error
     var osfile, s3file vfs.File

      // THEN begin using the filesystems
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

Development

To create your own backend, you must create a package that implements the interfaces: vfs.Filesystem, vfs.Location, and vfs.File.
Then ensure it registers itself on load:

  package myexoticfilesystem

  import(
      ...
      "github.com/c2fo/vfs"
      "github.com/c2fo/vfs/backend"
  )

  // IMPLEMENT vfs interfaces
  ...

  // register backend
  func init() {
      backend.Register("exfs", &MyExoticFilesystem{})
  }

Then do use it in some other package do
  package MyExoticFilesystem

  import(
      "github.com/c2fo/vfs/backend"
      "github.com/acme/myexoticfilesystem"
  )

  ...

  func useNewBackend() error {
      myExoticFs, err = backend.Backend(myexoticfilesystem.Scheme)
      ...
  }

Thats it.  Simple.
*/
package backend
