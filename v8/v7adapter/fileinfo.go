package v7adapter

import (
	"io/fs"
	"time"

	vfsv7 "github.com/c2fo/vfs/v7"
)

type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (i fileInfo) Name() string       { return i.name }
func (i fileInfo) Size() int64        { return i.size }
func (i fileInfo) Mode() fs.FileMode  { return i.mode }
func (i fileInfo) ModTime() time.Time { return i.modTime }
func (i fileInfo) IsDir() bool        { return false }
func (i fileInfo) Sys() any           { return nil }

func statV7(f vfsv7.File) (fs.FileInfo, error) {
	ok, err := f.Exists()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fs.ErrNotExist
	}

	size, err := f.Size()
	if err != nil {
		return nil, err
	}

	var mod time.Time
	if t, err := f.LastModified(); err == nil && t != nil {
		mod = *t
	}

	return fileInfo{
		name:    f.Name(),
		size:    int64(size),
		mode:    0644,
		modTime: mod,
	}, nil
}
