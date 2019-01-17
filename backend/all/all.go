// Package all imports all VFS implementations.
package all

import (
	_ "github.com/c2fo/vfs/backend/gs"
	_ "github.com/c2fo/vfs/backend/os"
	_ "github.com/c2fo/vfs/backend/s3"
)
