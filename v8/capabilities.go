package vfs

// Capabilities describes optional features a [FileSystem] or backend may expose.
// Implementations may return a zero value when unknown; callers should probe
// concrete types or perform small operations when precise capability detection matters.
type Capabilities struct {
	// Seek indicates arbitrary io.Seeker-style repositioning on opened files.
	Seek bool
	// ReaderAt indicates sparse or ranged reads without full sequential scan.
	ReaderAt bool
	// WriterAt indicates ranged writes where the backend supports them.
	WriterAt bool
	// ServerSideCopy indicates server-side object copy within the same store.
	ServerSideCopy bool
	// ServerSideMove indicates native rename/move within the same namespace.
	ServerSideMove bool
	// RecursiveList indicates depth listing is honored by List options.
	RecursiveList bool
}

// CapabilityProvider is implemented by file systems that advertise feature flags.
type CapabilityProvider interface {
	Capabilities() Capabilities
}
