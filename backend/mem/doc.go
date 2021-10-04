/*
Package mem built-in mem lib VFS implementation.
Usage
Rely on github.com/c2fo/vfs/v6/backend
  import(
      "github.com/c2fo/vfs/v6/backend"
      "github.com/c2fo/vfs/v6/backend/mem"
  )
  func UseFs() error {
      fs := backend.Backend(mem.Scheme)
      ...
  }
Or call directly:
  import _mem "github.com/c2fo/vfs/v6/backend/mem"
  func DoSomething() {
	fs := _mem.NewFileSystem()
      ...
  }
*/
package mem
