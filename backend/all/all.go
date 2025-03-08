// Package all imports all VFS implementations.
package all

import (
	_ "github.com/c2fo/vfs/v7/backend/azure" // register azure backend
	_ "github.com/c2fo/vfs/v7/backend/ftp"   // register sftp backend
	_ "github.com/c2fo/vfs/v7/backend/gs"    // register gs backend
	_ "github.com/c2fo/vfs/v7/backend/mem"   // register mem backend
	_ "github.com/c2fo/vfs/v7/backend/os"    // register os backend
	_ "github.com/c2fo/vfs/v7/backend/s3"    // register s3 backend
	_ "github.com/c2fo/vfs/v7/backend/sftp"  // register sftp backend
)
