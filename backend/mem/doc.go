/*
Package os built-in os lib VFS implementation.

Usage

Rely on github.com/c2fo/vfs/backend

  import(
      "github.com/c2fo/vfs/backend"
      "github.com/c2fo/vfs/backend/os"
  )

  func UseFs() error {
      fs, err := backend.Backend(os.Scheme)
      ...
  }

Or call directly:

  import _os "github.com/c2fo/vfs/backend/os"

  func DoSomething() {
      fs := &_os.FileSystem{}
      ...
  }

See Also

See: https://golang.org/pkg/os/
*/
package os
