package vfs

import (
	"context"
	"fmt"
	"io"
)

// File is a composed handle for reading and writing blob content addressed by URI.
// Optional capabilities (seek, ranged I/O) are accessed via type assertions to
// [io.Seeker], [FileReaderAt], and [FileWriterAt].
type File interface {
	io.Reader
	io.Writer
	io.Closer
	fmt.Stringer

	FileIdentity
	FileMeta
}

// FileTransfers captures high-level operations that v7 exposed as methods. v8
// encourages using these with [context.Context]; backends may still implement
// similar methods on concrete types for ergonomics.
type FileTransfers interface {
	CopyToLocation(ctx context.Context, dst Location) (File, error)
	CopyToFile(ctx context.Context, dst File) error
	MoveToLocation(ctx context.Context, dst Location) (File, error)
	MoveToFile(ctx context.Context, dst File) error
}
