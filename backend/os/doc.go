/*
Package os - built-in os lib VFS implementation.

# Usage

The OS backend interacts directly with the local operating system's file system.

It can be initialized using the backend registry:

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    // Blank import registers the OS backend
	    _ "github.com/c2fo/vfs/v7/backend/os"
	)

	func UseFs() error {
	    fs, err := backend.NewFileSystem(os.Scheme) // os.Scheme is "file"
	    if err != nil {
	        return err
	    }
	    // Use fs
	    ...
	}

Or by calling directly:

	import _os "github.com/c2fo/vfs/v7/backend/os"

	func DoSomething() {
	    // Initialize with default settings
	    fs := _os.NewFileSystem()

	    // Initialize with a custom temporary directory
	    fsWithOpts := _os.NewFileSystem(
	        _os.WithTempDir{TempDir: "/path/to/my/temp"},
	    )
	    // Use fs or fsWithOpts
	    ...
	}

# Options

The OS backend supports the following options for NewFileSystem:

  - WithTempDir: Specifies a custom directory path for temporary files used
    during certain write operations. If not provided, the backend attempts
    to use the system's temporary directory or the target file's directory.

# See Also

See: https://golang.org/pkg/os/
*/
package os
