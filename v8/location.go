package vfs

import (
	"context"
	"fmt"
	"iter"

	"github.com/c2fo/vfs/v8/options"
)

// Lister yields directory or prefix entries. Pagination and continuation tokens are
// internal to the implementation; consumers stop early by breaking the range.
type Lister interface {
	List(ctx context.Context, opts ...ListOption) iter.Seq2[Entry, error]
}

// Location is a directory- or prefix-like scope for listing and relative addressing.
type Location interface {
	fmt.Stringer
	LocationIdentity
	Lister

	Exists() (bool, error)
	NewLocation(rel string) (Location, error)
	NewFile(rel string, opts ...options.NewFileOption) (File, error)
	DeleteFile(rel string, opts ...options.DeleteOption) error
	FileSystem() FileSystem
}
