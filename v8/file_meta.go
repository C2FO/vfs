package vfs

import (
	"io/fs"
	"time"
)

// FileMeta exposes metadata operations for a [File]. Implementations may map
// to POSIX stat, object HEAD, or synthetic values for ephemeral backends.
type FileMeta interface {
	// Exists reports whether the object is present.
	Exists() (bool, error)
	// Stat returns a [fs.FileInfo] view when the backend can provide one.
	Stat() (fs.FileInfo, error)
	// LastModified returns modification time when available.
	LastModified() (*time.Time, error)
	// Size returns logical size in bytes.
	Size() (uint64, error)
	// Touch creates an empty object or bumps modification time.
	Touch() error
}
