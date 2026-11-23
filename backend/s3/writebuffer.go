package s3

import (
	"fmt"

	"github.com/c2fo/vfs/v7/options/newfile"
)

// BeginSequentialWrite implements utils.SequentialWriter. When the file has not been written, seeked or read and was
// not explicitly opened with newfile.WithWriteBuffer, writes until the returned function is called skip the local
// temporary file. Such a file cannot be sought or read until it is closed.
func (f *File) BeginSequentialWrite() (end func()) {
	if f.writeCalled || f.seekCalled || f.readCalled || f.tempFileWriter != nil || f.streaming ||
		newfile.UseWriteBuffer(false, f.opts) {
		return func() {}
	}
	f.ephemeralStream = true

	return func() { f.ephemeralStream = false }
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
