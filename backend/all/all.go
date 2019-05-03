// Package all imports all VFS implementations.
package all

import (
	"github.com/c2fo/vfs/v3"
	_ "github.com/c2fo/vfs/v3/backend/gs" // register gs backend
	_ "github.com/c2fo/vfs/v3/backend/os" // register os backend
	_ "github.com/c2fo/vfs/v3/backend/s3" // register s3 backend
)

// DefaultRetrier returns a no-op retryer which simply calls the wrapped command without looping.
func DefaultRetrier() vfs.Retry {
	return func(c func() error) error { return c() }
}
