package gs

import (
	"fmt"

	"github.com/c2fo/vfs/v7/options/newfile"
)

// BeginSequentialWrite implements utils.SequentialWriter. When the file has not been written, seeked or read and was
// not explicitly opened with newfile.WithWriteBuffer, writes until the returned function is called skip the local
// temporary file. Such a file cannot be sought or read until it is closed.
func (f *File) BeginSequentialWrite() (end func()) {
	if f.ephemeralStream || f.writeCalled || f.seekCalled || f.readCalled ||
		f.tempFileWriter != nil || f.streaming || f.writeErr != nil ||
		newfile.UseWriteBuffer(false, f.opts) {
		return func() {}
	}
	f.ephemeralStream = true

	return func() { f.ephemeralStream = false }
}

// AbortWrite implements utils.WriteAborter. When err is non-nil and a write has started on f (and has not already
// failed), it cancels the in-flight GCS upload, removes any local temporary file, and latches err so subsequent
// operations and Close report it without committing a partial object.
func (f *File) AbortWrite(err error) {
	if err == nil || f.writeErr != nil {
		return
	}
	if f.writeCalled || f.gcsWriter != nil || f.tempFileWriter != nil || f.streaming {
		_ = f.abortWrite(err)
	}
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
