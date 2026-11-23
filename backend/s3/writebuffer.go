package s3

import (
	"fmt"
	"io"

	"github.com/c2fo/vfs/v7/internal/writebuffer"
	"github.com/c2fo/vfs/v7/options/newfile"
)

func init() {
	writebuffer.Register(func(w io.Writer) (func(), bool) {
		f, ok := w.(*File)
		if !ok || f.writeCalled || f.seekCalled || f.readCalled || f.tempFileWriter != nil || f.streaming {
			return nil, false
		}
		// a file explicitly opened with newfile.WithWriteBuffer keeps its local buffer
		if newfile.UseWriteBuffer(false, f.opts) {
			return nil, false
		}
		f.ephemeralStream = true
		return func() { f.ephemeralStream = false }, true
	})
}

// useWriteBuffer reports whether writes to this file are mirrored to a local temporary file. Per-file options take
// precedence over Options.DisableWriteBuffer, and the last of newfile.WithStreamingWrite and newfile.WithWriteBuffer wins.
func (f *File) useWriteBuffer() bool {
	if f.ephemeralStream {
		return false
	}
	return newfile.UseWriteBuffer(!f.location.fileSystem.options.DisableWriteBuffer, f.opts)
}

// errLocalWriteBuffer wraps a failure to use the local temporary write buffer with a hint about streaming.
func errLocalWriteBuffer(err error) error {
	return fmt.Errorf(
		"local write buffer failed (use newfile.WithStreamingWrite() or Options.DisableWriteBuffer to stream without local temp storage): %w",
		err,
	)
}
