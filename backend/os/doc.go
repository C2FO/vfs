/*
Package os - built-in os lib VFS implementation.

# Usage

Rely on github.com/c2fo/vfs/v7/backend

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    "github.com/c2fo/vfs/v7/backend/os"
	)

	func UseFs() error {
	    fs := backend.Backend(os.Scheme)
	    ...
	}

Or call directly:

	import _os "github.com/c2fo/vfs/v7/backend/os"

	func DoSomething() {
	    fs := &_os.NewFileSystem()
	    ...
	}

# See Also

See: https://golang.org/pkg/os/
*/
package os
