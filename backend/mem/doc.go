/*
Package mem built-in mem lib VFS implementation.
Usage
Rely on github.com/c2fo/vfs/backend
  import(
      "github.com/c2fo/vfs/backend"
      "github.com/c2fo/vfs/backend/mem"
  )
  func UseFs() error {
      fs, err := backend.Backend(mem.Scheme)
      ...
  }
Or call directly:
  import _mem "github.com/c2fo/vfs/backend/mem"
  func DoSomething() {
	fs := &_mem.FileSystem{}
	fs.Initialize()
      ...
  }
*/
package mem
