package vfs

import "io"

// FileReader is an optional capability beyond the minimal [File] surface.
type FileReader = io.Reader

// FileWriter is an optional capability beyond the minimal [File] surface.
type FileWriter = io.Writer

// FileSeeker is implemented by files that support repositioning (POSIX lseek).
// Callers test with interface assertions: `s, ok := f.(io.Seeker)`.
type FileSeeker interface {
	io.Seeker
}

// FileReaderAt supports ranged reads without a global offset.
type FileReaderAt interface {
	io.ReaderAt
}

// FileWriterAt supports ranged writes when the backend allows them.
type FileWriterAt interface {
	io.WriterAt
}
