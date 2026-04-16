package v7adapter

import (
	vfsv7 "github.com/c2fo/vfs/v7"
	vfsv8 "github.com/c2fo/vfs/v8"
)

// AsV7File returns the underlying v7 [vfsv7.File] when f was produced by [FileFromV7].
func AsV7File(f vfsv8.File) (vfsv7.File, bool) {
	if f == nil {
		return nil, false
	}
	w, ok := f.(*File)
	if !ok || w.inner == nil {
		return nil, false
	}
	return w.inner, true
}

// AsV7Location returns the underlying v7 [vfsv7.Location] when loc was produced by [LocationFromV7].
func AsV7Location(loc vfsv8.Location) (vfsv7.Location, bool) {
	if loc == nil {
		return nil, false
	}
	w, ok := loc.(*Location)
	if !ok || w.inner == nil {
		return nil, false
	}
	return w.inner, true
}

// AsV7FileSystem returns the underlying v7 [vfsv7.FileSystem] when fs was produced by [FileSystemFromV7].
func AsV7FileSystem(fs vfsv8.FileSystem) (vfsv7.FileSystem, bool) {
	if fs == nil {
		return nil, false
	}
	w, ok := fs.(*FileSystem)
	if !ok || w.inner == nil {
		return nil, false
	}
	return w.inner, true
}
