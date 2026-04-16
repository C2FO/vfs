package vfs

import "errors"

// Sentinel errors for capability and POSIX-style semantics. Backends should wrap
// these with fmt.Errorf where additional context is useful.
var (
	// ErrNotSupported indicates the backend cannot satisfy the requested operation
	// or option (for example seek on a write-only object handle).
	ErrNotSupported = errors.New("vfs: operation not supported")

	// ErrInvalidSeek is returned when a seek offset is not valid for the handle.
	ErrInvalidSeek = errors.New("vfs: invalid seek")
)
