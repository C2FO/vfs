// Package writebuffer lets copy helpers tell a backend writer that it is about to receive one sequential write
// stream, so the backend can skip its local temporary write buffer. It exists so utils and backend packages can
// cooperate without utils importing the backends.
package writebuffer

import (
	"io"
	"sync"
)

// Handler inspects a writer. When the writer is one the handler understands and it is safe to stream, the handler
// enables ephemeral streaming on it and returns a reset function and true. Otherwise it returns false.
type Handler func(w io.Writer) (reset func(), ok bool)

var (
	mu       sync.RWMutex
	handlers []Handler
)

// Register adds a handler. Backends call it from init.
func Register(h Handler) {
	mu.Lock()
	defer mu.Unlock()
	handlers = append(handlers, h)
}

// EnableEphemeralStreaming asks every registered handler whether it owns w. The returned reset function is always
// non-nil and must be called, typically with defer, once the sequential write is done. It is a no-op when no handler
// claims the writer.
func EnableEphemeralStreaming(w io.Writer) (reset func()) {
	mu.RLock()
	defer mu.RUnlock()
	for _, h := range handlers {
		if r, ok := h(w); ok {
			return r
		}
	}
	return func() {}
}
