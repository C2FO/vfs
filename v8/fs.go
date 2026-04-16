package vfs

import "github.com/c2fo/vfs/v8/options"

// FileSystem constructs [File] and [Location] values for an authority and path.
// Retry and other cross-cutting concerns belong in backend constructors, not here.
type FileSystem interface {
	NewFile(authority string, absFilePath string, opts ...options.NewFileOption) (File, error)
	NewLocation(authority string, absLocPath string) (Location, error)
	Name() string
	Scheme() string
}
