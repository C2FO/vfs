/*
Package os - built-in os lib VFS implementation.

# Usage

The OS backend interacts directly with the local operating system's file system.

It can be initialized using the backend registry:

	import(
	    "github.com/c2fo/vfs/v7/backend"
	    _ "github.com/c2fo/vfs/v7/backend/os"
	)

	func UseFs() error {
	    fs, err := backend.NewFileSystem(os.Scheme)
	    if err != nil {
	        return err
	    }
	    // Use fs
	    ...
	}

Or by calling directly:

	import _os "github.com/c2fo/vfs/v7/backend/os"

	func DoSomething() {
	    fs := _os.NewFileSystem()

	    // Or with a custom temporary directory
	    fsWithOpts := _os.NewFileSystem(
	        _os.WithTempDir{TempDir: "/path/to/my/temp"},
	    )
	    ...
	}

# Options

The OS backend supports the following options for NewFileSystem:

  - WithTempDir: Specifies a custom directory path for temporary files used
    during write operations. If not provided, the backend uses the system
    temp directory when it's on the same device as the target file, or falls
    back to the target file's parent directory.

# Read-only files

Reading does not require write access. Read and Seek open the existing file with os.O_RDONLY (and never create it),
so files the process cannot write to can be read, seeked and copied from. Writes are staged in a temp file and
renamed over the original on Close, so replacing a read-only file requires write permission on its directory rather
than on the file itself.

# See Also

See: https://golang.org/pkg/os/
*/
package os
