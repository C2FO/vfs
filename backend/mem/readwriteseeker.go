package mem

import (
	"io"

	"github.com/c2fo/vfs/v6"
)

// ReadWriteSeeker is a custom type that implements io.ReadWriteSeeker.
type ReadWriteSeeker struct {
	data   []byte
	cursor int
}

// NewReadWriteSeeker creates a new ReadWriteSeeker.
func NewReadWriteSeeker() *ReadWriteSeeker {
	return &ReadWriteSeeker{
		data:   []byte{},
		cursor: 0,
	}
}

// NewReadWriteSeekerWithData creates a new ReadWriteSeeker with the provided data.
func NewReadWriteSeekerWithData(data []byte) *ReadWriteSeeker {
	return &ReadWriteSeeker{
		data:   data,
		cursor: 0,
	}
}

// Write writes data to the current cursor position and advances the cursor.
func (rws *ReadWriteSeeker) Write(p []byte) (n int, err error) {
	position := rws.cursor + len(p)
	if position <= len(rws.data) {
		copy(rws.data[rws.cursor:position], p)
	} else {
		rws.data = append(rws.data[:rws.cursor], p...)
	}
	rws.cursor = position
	return len(p), nil
}

// Read reads data from the current cursor position and advances the cursor.
func (rws *ReadWriteSeeker) Read(p []byte) (n int, err error) {
	if rws.cursor >= len(rws.data) {
		return 0, io.EOF
	}

	n = copy(p, rws.data[rws.cursor:])
	rws.cursor += n
	return n, nil
}

// Seek sets the cursor position.
func (rws *ReadWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var position int64
	switch whence {
	case io.SeekStart: // io.SeekStart
		position = offset
	case io.SeekCurrent: // io.SeekCurrent
		position = int64(rws.cursor) + offset
	case io.SeekEnd: // io.SeekEnd
		position = int64(len(rws.data)) + offset
	default:
		return 0, vfs.ErrSeekInvalidWhence
	}

	if position < 0 || position > int64(len(rws.data)) {
		return 0, vfs.ErrSeekInvalidOffset
	}

	rws.cursor = int(position)

	return position, nil
}

// Bytes returns a byte slice of the data.
func (rws *ReadWriteSeeker) Bytes() []byte {
	return rws.data
}
