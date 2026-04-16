// Package options defines option marker interfaces for v8 file system operations.
package options

// NewFileOption marks configuration for [github.com/c2fo/vfs/v8.FileSystem.NewFile] and
// [github.com/c2fo/vfs/v8.Location.NewFile]. Backends extend this with typed options.
type NewFileOption interface {
	NewFileOptionName() string
}

// DeleteOption marks configuration for delete operations on files and objects.
type DeleteOption interface {
	DeleteOptionName() string
}
