package vfs

import (
	"github.com/c2fo/vfs/v7/utils/authority"
)

// FileIdentity is stable addressing metadata for a [File] without implying I/O.
type FileIdentity interface {
	// URI returns the fully qualified URI for the file, e.g. s3://bucket/path/obj.txt.
	URI() string
	// Path returns the absolute path including the file name.
	Path() string
	// Name returns the base name of the file.
	Name() string
	// Authority identifies the URI authority segment (bucket, host, namespace, ...).
	Authority() authority.Authority
	// Location returns the parent [Location] for this file.
	Location() Location
}

// LocationIdentity is stable addressing metadata for a [Location].
type LocationIdentity interface {
	// URI returns the fully qualified URI; location URIs end with a slash.
	URI() string
	// Path returns the absolute directory path ending with a slash when applicable.
	Path() string
	// Authority identifies the URI authority segment.
	Authority() authority.Authority
}
