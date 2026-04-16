package vfs

// EntryKind classifies a single name returned while listing a [Location].
// Backends that only expose flat object keys should use [EntryUnknown] or [EntryBlob].
type EntryKind int

const (
	// EntryUnknown indicates the backend could not classify the entry.
	EntryUnknown EntryKind = iota
	// EntryFile is a non-directory object (regular file or blob).
	EntryFile
	// EntryLocation is a directory-like prefix or folder marker when distinguishable.
	EntryLocation
	// EntryBlob is a flat name without file vs directory distinction (common for object stores).
	EntryBlob
)

// Entry is one element yielded by [Lister.List]. Name is the base name or list
// key segment as defined by the backend. File and Loc are optional handles when
// the implementation can resolve them without extra cost.
type Entry struct {
	Kind EntryKind
	Name string

	// File is set when Kind is [EntryFile] (or [EntryBlob]) and the backend
	// materializes a [File] handle.
	File File
	// Loc is set when Kind is [EntryLocation] and the backend materializes a [Location].
	Loc Location
}

// BaseName returns the entry name for logging or filtering. It is identical to Name.
func (e Entry) BaseName() string {
	return e.Name
}
