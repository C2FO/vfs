package mem

import (
	"io"
	"math"

	vfsv7 "github.com/c2fo/vfs/v7"
)

// readWriteSeeker implements [io.ReadWriteSeeker] for in-memory file buffers.
type readWriteSeeker struct {
	data   []byte
	cursor int
}

func newReadWriteSeeker() *readWriteSeeker {
	return &readWriteSeeker{data: []byte{}, cursor: 0}
}

func newReadWriteSeekerWithData(data []byte) *readWriteSeeker {
	return &readWriteSeeker{data: data, cursor: 0}
}

func (rws *readWriteSeeker) Write(p []byte) (n int, err error) {
	position := rws.cursor + len(p)
	if position <= len(rws.data) {
		copy(rws.data[rws.cursor:position], p)
	} else {
		if rws.cursor > len(rws.data) {
			gap := make([]byte, rws.cursor-len(rws.data))
			rws.data = append(rws.data, gap...)
		}
		rws.data = append(rws.data[:rws.cursor], p...)
	}
	rws.cursor = position
	return len(p), nil
}

func (rws *readWriteSeeker) Read(p []byte) (n int, err error) {
	if rws.cursor >= len(rws.data) {
		return 0, io.EOF
	}

	n = copy(p, rws.data[rws.cursor:])
	rws.cursor += n
	return n, nil
}

func (rws *readWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var position int64
	switch whence {
	case io.SeekStart:
		position = offset
	case io.SeekCurrent:
		position = int64(rws.cursor) + offset
	case io.SeekEnd:
		position = int64(len(rws.data)) + offset
	default:
		return 0, vfsv7.ErrSeekInvalidWhence
	}

	if position < 0 || position > math.MaxInt {
		return 0, vfsv7.ErrSeekInvalidOffset
	}

	rws.cursor = int(position)

	return position, nil
}

func (rws *readWriteSeeker) Bytes() []byte {
	return rws.data
}
