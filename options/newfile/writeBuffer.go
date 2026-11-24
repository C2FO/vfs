package newfile

import (
	"github.com/c2fo/vfs/v7/options"
)

const (
	optionNameNewFileStreamingWrite = "newFileStreamingWrite"
	optionNameNewFileWriteBuffer    = "newFileWriteBuffer"
)

// WithStreamingWrite returns the StreamingWrite implementation of NewFileOption.
//
// Backends that stage writes in a local temporary file so that Seek or Read can follow Write (s3, gs, and azure) skip
// that local file and stream bytes directly to the remote object. Calling Seek or Read after Write, or Write after Seek
// or Read, before Close then fails with vfs.ErrWriteBufferDisabled. Backends that do not stage writes ignore the option.
func WithStreamingWrite() options.NewFileOption {
	return StreamingWrite{}
}

// StreamingWrite represents the NewFileOption that disables local write buffering for a file.
type StreamingWrite struct{}

// NewFileOptionName returns the name of the StreamingWrite option.
func (StreamingWrite) NewFileOptionName() string {
	return optionNameNewFileStreamingWrite
}

// WithWriteBuffer returns the WriteBuffer implementation of NewFileOption.
//
// It enables local write buffering for a file even when the backend's filesystem options disable it (for example
// s3.Options.DisableWriteBuffer), so that one file can use Write together with Seek or Read. When both
// WithStreamingWrite and WithWriteBuffer are passed, the last one wins. Backends that do not stage writes ignore the
// option.
func WithWriteBuffer() options.NewFileOption {
	return WriteBuffer{}
}

// WriteBuffer represents the NewFileOption that enables local write buffering for a file.
type WriteBuffer struct{}

// NewFileOptionName returns the name of the WriteBuffer option.
func (WriteBuffer) NewFileOptionName() string {
	return optionNameNewFileWriteBuffer
}

// UseWriteBuffer resolves whether local write buffering applies to a file. defaultBuffered is the backend's default
// for the file's filesystem, and opts are the options passed to NewFile. StreamingWrite and WriteBuffer, as a value or
// a pointer, are applied in order, so the last one wins.
func UseWriteBuffer(defaultBuffered bool, opts []options.NewFileOption) bool {
	buffered := defaultBuffered
	for _, o := range opts {
		switch o.(type) {
		case StreamingWrite, *StreamingWrite:
			buffered = false
		case WriteBuffer, *WriteBuffer:
			buffered = true
		default:
		}
	}

	return buffered
}
