package vfs

// Error is a type that allows for error constants below
type Error string

// Error returns a string representation of the error
func (e Error) Error() string { return string(e) }

const (
	// CopyToNotPossible - CopyTo/MoveTo operations are only possible when seek position is 0,0
	CopyToNotPossible = Error("current cursor offset is not 0 as required for this operation")

	// ErrNotExist - File does not exist
	ErrNotExist = Error("file does not exist")

	// ErrSeekInvalidOffset - Offset is invalid. Must be greater than or equal to 0
	ErrSeekInvalidOffset = Error("seek: invalid offset")

	// ErrSeekInvalidWhence - Whence is invalid.  Must be one of the following: 0 (io.SeekStart), 1 (io.SeekCurrent), or 2 (io.SeekEnd)
	ErrSeekInvalidWhence = Error("seek: invalid whence")
)
