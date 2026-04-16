package mem

import (
	"path"
	"sync"
	"time"
)

// memFile holds at-rest file state shared by [File] handles.
type memFile struct {
	sync.Mutex
	exists       bool
	contents     []byte
	location     *Location
	lastModified time.Time
	name         string
	filepath     string
}

func newMemFile(file *File, location *Location) *memFile {
	return &memFile{
		contents: make([]byte, 0),
		location: location,
		name:     file.name,
		filepath: path.Join(location.Path(), file.Name()),
	}
}
