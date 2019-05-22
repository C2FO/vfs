// Package all imports all VFS implementations.
package all

import (
	_ "github.com/c2fo/vfs/v4/backend/gs" // register gs backend
	_ "github.com/c2fo/vfs/v4/backend/os" // register os backend
	_ "github.com/c2fo/vfs/v4/backend/s3" // register s3 backend
)
