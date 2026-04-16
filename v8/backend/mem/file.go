package mem

import (
	"errors"
	"io"
	"io/fs"
	"math"
	"path"
	"time"

	vfsv7 "github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
)

type mode int

const (
	none mode = iota
	truncate
	edit
)

// File implements [vfs.File] for the in-memory backend.
type File struct {
	memFile         *memFile
	readWriteSeeker *readWriteSeeker
	name            string
	cursor          int
	writeMode       mode
	isOpen          bool
	readCalled      bool
	seekCalled      bool
}

func deepCopyFile(mf *memFile) *File {
	return &File{
		name:            mf.name,
		memFile:         mf,
		readWriteSeeker: newReadWriteSeekerWithData(mf.contents),
	}
}

// Read implements [io.Reader].
func (f *File) Read(p []byte) (int, error) {
	var existsOnFS bool
	var err error
	if existsOnFS, err = f.Exists(); !existsOnFS && !f.isOpen {
		if err != nil {
			return 0, utils.WrapReadError(err)
		}
		return 0, utils.WrapReadError(fs.ErrNotExist)
	}

	if existsOnFS && f.writeMode == none {
		fsMap := f.memFile.location.fileSystem.fsMap
		if objMap, ok := fsMap[f.Location().Authority().String()]; ok {
			if obj, ok := objMap[f.Path()]; ok {
				if obj.isFile {
					f.memFile = obj.i.(*memFile)
				}
			}
		}
		f.readWriteSeeker = newReadWriteSeekerWithData(f.memFile.contents)
		_, err = f.readWriteSeeker.Seek(int64(f.cursor), 0)
		if err != nil {
			return 0, utils.WrapReadError(err)
		}
	}

	read, err := f.readWriteSeeker.Read(p)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	f.readCalled = true
	f.isOpen = true
	f.cursor += read

	return read, nil
}

// Seek implements [io.Seeker].
func (f *File) Seek(offset int64, whence int) (int64, error) {
	var existsOnFS bool
	var err error
	if existsOnFS, err = f.Exists(); !existsOnFS && !f.isOpen {
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		return 0, utils.WrapSeekError(fs.ErrNotExist)
	}

	if existsOnFS && f.writeMode == none {
		fsMap := f.memFile.location.fileSystem.fsMap
		if objMap, ok := fsMap[f.Location().Authority().String()]; ok {
			if obj, ok := objMap[f.Path()]; ok {
				if obj.isFile {
					f.memFile = obj.i.(*memFile)
				}
			}
		}
		f.readWriteSeeker = newReadWriteSeekerWithData(f.memFile.contents)
		_, err := f.readWriteSeeker.Seek(int64(f.cursor), 0)
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	pos, err := f.readWriteSeeker.Seek(offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	if pos > math.MaxInt {
		return 0, utils.WrapSeekError(vfsv7.ErrSeekInvalidOffset)
	}

	f.isOpen = true
	f.seekCalled = true
	f.cursor = int(pos)

	return pos, nil
}

// Write implements [io.Writer].
func (f *File) Write(p []byte) (int, error) {
	if f.writeMode == none {
		if f.readCalled || f.seekCalled {
			f.writeMode = edit
			f.readWriteSeeker = newReadWriteSeekerWithData(f.memFile.contents)
			_, err := f.readWriteSeeker.Seek(int64(f.cursor), 0)
			if err != nil {
				return 0, utils.WrapWriteError(err)
			}
		} else {
			f.readWriteSeeker = newReadWriteSeeker()
			f.writeMode = truncate
		}
		f.isOpen = true
	}

	written, err := f.readWriteSeeker.Write(p)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	f.cursor += written

	return written, nil
}

// Close persists buffer contents into the shared [memFile] and updates the map.
func (f *File) Close() error {
	if f.isOpen {
		f.memFile.Lock()
		defer f.memFile.Unlock()
		f.memFile.contents = f.readWriteSeeker.Bytes()
		f.memFile.lastModified = time.Now()
		f.memFile.exists = true
		f.memFile.location.exists = true

		f.memFile.location.fileSystem.mu.Lock()
		defer f.memFile.location.fileSystem.mu.Unlock()
		mapRef := f.memFile.location.fileSystem.fsMap
		vol := f.Location().Authority().String()
		if _, ok := mapRef[vol]; ok {
			if _, ok := mapRef[vol][f.Path()]; ok {
				mapRef[vol][f.Path()].i = f.memFile
			} else {
				mapRef[vol][f.Path()] = &fsObject{true, f.memFile}
			}
		} else {
			mapRef[vol] = make(objMap)
			mapRef[vol][f.Path()] = &fsObject{true, f.memFile}
		}
	}

	f.seekCalled = false
	f.readCalled = false
	f.readWriteSeeker = nil
	f.writeMode = none
	f.isOpen = false
	f.cursor = 0

	return nil
}

// String returns the file URI.
func (f *File) String() string {
	return f.URI()
}

// Exists reports whether the file is stored in the map.
func (f *File) Exists() (bool, error) {
	vol := f.Location().Authority().String()
	fullPath := f.Path()
	mapRef := f.memFile.location.fileSystem.fsMap
	if _, ok := mapRef[vol]; ok {
		if object, ok2 := mapRef[vol][fullPath]; ok2 {
			if object != nil && object.i.(*memFile).exists {
				return true, nil
			}
		}
	}
	return false, nil
}

// Location returns a copy of the parent [Location].
func (f *File) Location() vfs.Location {
	return utils.Ptr(*f.memFile.location)
}

// Stat implements [vfs.FileMeta.Stat].
func (f *File) Stat() (fs.FileInfo, error) {
	return f.stat()
}

// LastModified returns the last modification time when the file exists.
func (f *File) LastModified() (*time.Time, error) {
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return nil, utils.WrapLastModifiedError(err)
		}
		return nil, fs.ErrNotExist
	}
	return &f.memFile.lastModified, nil
}

// Size returns the byte length of the file.
func (f *File) Size() (uint64, error) {
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return 0, utils.WrapSizeError(err)
		}
		return 0, fs.ErrNotExist
	}

	f.readWriteSeeker = newReadWriteSeekerWithData(f.memFile.contents)

	return uint64(len(f.readWriteSeeker.Bytes())), nil
}

// Touch creates or registers the file in the map and updates timestamps.
func (f *File) Touch() error {
	if f.memFile.exists {
		f.memFile.lastModified = time.Now()
		return nil
	}
	f.memFile.exists = true

	auth := f.Location().Authority().String()
	f.memFile.lastModified = time.Now()

	fileObject := &fsObject{true, f.memFile}
	locObject := &fsObject{false, utils.Ptr(*f.memFile.location)}

	f.memFile.location.fileSystem.mu.Lock()
	defer f.memFile.location.fileSystem.mu.Unlock()

	mapRef := f.memFile.location.fileSystem.fsMap
	if _, ok := mapRef[auth]; !ok {
		mapRef[auth] = make(objMap)
	}

	mapRef[auth][f.Path()] = fileObject
	f.memFile = mapRef[auth][f.Path()].i.(*memFile)

	locationPath := utils.EnsureTrailingSlash(path.Clean(path.Dir(f.Path())))
	if _, ok := mapRef[auth][locationPath]; !ok {
		mapRef[auth][locationPath] = locObject
	}
	return nil
}

// Path returns the absolute path including the file name.
func (f *File) Path() string {
	return path.Join(f.Location().Path(), f.name)
}

// Name returns the base name of the file.
func (f *File) Name() string {
	return f.name
}

// URI returns the fully qualified file URI.
func (f *File) URI() string {
	return formatFileURI(f)
}

// Authority returns the URI authority for this file.
func (f *File) Authority() authority.Authority {
	return f.memFile.location.Authority()
}

var _ vfs.File = (*File)(nil)
