package vfs

import (
	"io/fs"
)

// DirEntry adapts [Entry] to [fs.DirEntry]. Info returns [fs.ErrNotExist] when
// no concrete [fs.FileInfo] is available; callers needing stat data should use
// [File.Stat] on Entry.File when present.
func (e Entry) DirEntry() fs.DirEntry {
	return entryDirEntry{entry: e}
}

type entryDirEntry struct {
	entry Entry
}

func (d entryDirEntry) Name() string {
	return d.entry.Name
}

func (d entryDirEntry) IsDir() bool {
	return d.entry.Kind == EntryLocation
}

func (d entryDirEntry) Type() fs.FileMode {
	switch d.entry.Kind {
	case EntryLocation:
		return fs.ModeDir
	case EntryFile, EntryBlob:
		return 0
	default:
		return fs.ModeIrregular
	}
}

func (d entryDirEntry) Info() (fs.FileInfo, error) {
	if d.entry.File == nil {
		return nil, fs.ErrNotExist
	}
	return d.entry.File.Stat()
}
