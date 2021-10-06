package mem

import (
	"bytes"
	"errors"
	"io"
	"path"
	"sync"
	"time"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

//
//	memFile with name "filename" is the single representation of the structure that
//	all calls to "filename" will reference. The point of doing it this way is
//	to allow multiple read threads, and locking up writing and deletion.
//	Files referencing filename will check their own 'contents' slice against
//	"filename's" and is updated accordingly. This allows multiple threads to have the most
//	up-to-date file contents while also preserving the state of their own cursor that is
//	used in Read() and Seek() calls. Calls to Write() all reference
// 	"filename's" buffer, which is why it is locked. Data written into the writeBuffer
//	does not appear in "filename's" 'contents' slice until a call to Close() is made
//
type memFile struct {
	sync.Mutex
	writeBuffer  *bytes.Buffer
	exists       bool
	contents     []byte
	location     vfs.Location
	lastModified time.Time
	name         string
	isOpen       bool
	filepath     string
}

// File implements vfs.File interface for the in-memory implementation of FileSystem.
// A file struct holds a pointer to a single memFile.  Multiple threads will refer to the same
// memFile. Simultaneous reading is allowed, but writing and closing are protected by locks.
type File struct {
	memFile  *memFile
	exists   bool
	isOpen   bool
	contents []byte // the file contents
	name     string // the base name of the file
	cursor   int    // the index that the buffer (contents) is at

}

//		////// Error Functions ///////		//
func doesNotExist() error {
	return errors.New("this file does not exist")
}

func nilReference() error {
	return errors.New("the target file passed in was nil")
}

func seekError() error {
	return errors.New("seek could not complete the desired call")
}

// /////////////////////////////////////////////////////////

// Close imitates io.Closer by resetting the cursor and setting a boolean
func (f *File) Close() error {
	if f == nil {
		return nilReference()
	}
	f.memFile.Lock()
	if f.memFile.writeBuffer.Len() > 0 {
		bufferContents := f.memFile.writeBuffer.Bytes()
		for i := range bufferContents {
			f.memFile.contents = append(f.memFile.contents, bufferContents[i])
		}
		f.memFile.lastModified = time.Now()
		f.memFile.writeBuffer = new(bytes.Buffer)
	}

	f.memFile.isOpen = false
	f.cursor = 0
	f.memFile.Unlock()
	return nil
}

// Read implements the io.Reader interface.  Returns number of bytes read and potential errors
func (f *File) Read(p []byte) (n int, err error) {
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return 0, err
		}
		return 0, doesNotExist()
	}
	// if file exists:
	if !f.isOpen {
		f.isOpen = true
	}

	readBufferLength := len(p)
	// readBufferLength of byte slice is zero, just return 0 and nil
	if readBufferLength == 0 {
		return 0, nil
	}
	// in case the file contents have changed
	f.synchronize()

	fileContentLength := len(f.contents)
	// if the cursor is at the end of the file
	if f.cursor == fileContentLength {
		return 0, io.EOF
	}
	// j is the incrementer for the readBuffer. It always starts at 0, but the cursor may not
	j := 0
	for i := range p {
		if !f.isOpen {
			return i, errors.New("file is closed")
		}
		// if "i" is greater than the readBufferLength of p or readBufferLength of the contents
		if i == readBufferLength || f.cursor == fileContentLength {
			return i, io.EOF
		}
		// otherwise simply copy each index to p
		p[i] = f.contents[f.cursor]
		j++
		f.cursor++

	}
	if f.cursor > len(f.contents) {
		f.cursor = len(f.contents)
	}
	return readBufferLength, nil

}

// Seek implements the io.Seeker interface.  Returns the current position of the cursor and errors if any
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return 0, err
		}
		return 0, doesNotExist()
	}

	length := len(f.contents)

	if num := int64(length) + offset + int64(whence); num == 0 {
		return 0, nil
	}
	switch whence {

	case 0:
		if int(offset) < length && offset >= 0 {
			f.cursor = int(offset)
			return offset, nil
		}
	case 1:
		pos := f.cursor + int(offset)
		if pos <= length && pos >= 0 {
			f.cursor = pos
			return int64(pos), nil
		}
	case 2:
		pos := length + int(offset)
		if pos < length && pos >= 0 {
			f.cursor = pos
			return int64(f.cursor), nil
		}
	}
	return int64(f.cursor), seekError()
}

// Write implements the io.Writer interface. Returns number of bytes written and any errors
func (f *File) Write(p []byte) (int, error) {
	if !f.isOpen {
		f.isOpen = true
	}

	if ex, err := f.Exists(); !ex {
		if err != nil {
			return 0, err
		}
		err = f.Touch()
		if err != nil {
			return 0, err
		}
	}
	f.memFile.Lock()
	num, err := f.memFile.writeBuffer.Write(p)
	f.memFile.lastModified = time.Now()
	f.memFile.Unlock()
	return num, err

}

// String implements the io.Stringer interface. It returns a string representation of the file's URI
func (f *File) String() string {
	return f.URI()
}

// Exists returns whether or not a file exists.  Creating a file does not
// guarantee its existence, but creating one and writing to it does
func (f *File) Exists() (bool, error) {
	if f != nil {
		// does it exist on the map?
		vol := f.Location().Volume()
		fullPath := f.Path()
		loc := f.Location().(*Location)
		mapRef := loc.fileSystem.fsMap
		if _, ok := mapRef[vol]; ok {
			if object, ok2 := mapRef[vol][fullPath]; ok2 {

				if object != nil && object.i.(*memFile).exists {
					return true, nil
				}
			}
		}
		return false, nil
	}

	return false, nilReference()
}

// Location simply returns the file's underlying location struct pointer
func (f *File) Location() vfs.Location {

	newLoc, err := f.memFile.location.FileSystem().NewLocation(f.memFile.location.Volume(), f.memFile.location.Path())
	if err != nil {
		panic(err)
	}
	return newLoc
}

// CopyToLocation copies the current file to the given location.  If file exists
// at given location contents are simply overwritten using "CopyToFile", otherwise
// a newFile is made, takes the contents of the current file, and ends up at
// the given location
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	if ok, err := f.Exists(); !ok {
		if err != nil {
			return nil, err
		}
		return nil, doesNotExist()
	}
	testPath := path.Join(path.Clean(location.Path()), f.Name())
	thisLoc := f.Location().(*Location)
	mapRef := thisLoc.fileSystem.fsMap
	vol := thisLoc.Volume()
	// making sure that this volume has keys at all
	if _, ok := mapRef[vol]; ok {
		// if file w/name exists @ loc, simply copy contents over
		if _, ok2 := mapRef[vol][testPath]; ok2 {
			// casting fsObject to a file
			memFile := mapRef[vol][testPath].i.(*memFile)
			file := deepCopy(memFile)

			cerr := f.CopyToFile(file)

			if cerr != nil {
				return nil, cerr
			}
			return file, nil
		}
	} // end outer-if

	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}
	// writing to existence. Whatever the underlying vfs implementation is will take care of this
	if _, err := newFile.Write(make([]byte, 0)); err != nil {
		return nil, err
	}

	if err := f.CopyToFile(newFile); err != nil {
		return nil, err
	}

	return newFile, nil
}

// CopyToFile copies the receiver file into the target file.
// The target file is deleted, so any references to it will
// be nil.  In order to access the target after calling CopyToFile
// use its previous path to call it using the fsMap.  Additionally,
// after this is called, f's cursor will reset as if it had been closed.
func (f *File) CopyToFile(target vfs.File) error {
	if f == nil || target == nil {
		return nilReference()
	}

	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}

	if ex, _ := f.Exists(); !ex {
		return doesNotExist()
	}

	if target.Location().FileSystem().Scheme() == "mem" {
		target.(*File).memFile.contents = make([]byte, 0)
	}

	if ex, err := target.Exists(); !ex {
		if err == nil {

			if _, err := target.Write(f.memFile.contents); err != nil {
				return err
			}
			err = f.Close()
			if err != nil {
				return err
			}
			err = target.Close()
			if err != nil {
				return err
			}
		}

	}
	if _, err := target.Write(f.memFile.contents); err != nil {
		return err
	}
	err := target.Close()
	if err != nil {
		return err
	}
	return f.Close()
}

// MoveToLocation moves the receiver file to the passed in location. It does so by
// creating a copy of 'f' in "location".  'f' is subsequently  deleted
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	if f == nil || location == nil {
		return nil, nilReference()
	}

	if exists, err := f.Exists(); !exists {
		if err != nil {
			return nil, err
		}
		return nil, doesNotExist()
	}

	// if the underling FileSystem is in-memory, then this is the native way of
	// replacing a file with the same name as "f" at the location
	if location.FileSystem().Scheme() == "mem" {
		// this is a potential path to a file that can be fed into the objMap portion of fsMap
		testPath := path.Join(location.Path(), f.Name())
		loc := location.(*Location)
		// mapRef just makes it easier to refer to "loc.fileSystem.fsMap"
		mapRef := loc.fileSystem.fsMap
		vol := loc.Volume()
		f.memFile.location.FileSystem().(*FileSystem).Lock()
		// this checks if the specified volume has any keys
		if _, ok := mapRef[vol]; ok {
			// this block checks if the file already exists at location, if it does, deletes it and inserts the file we have
			if _, ok2 := mapRef[vol][testPath]; ok2 {
				memFile := mapRef[vol][testPath].i.(*memFile)
				f.memFile.location.FileSystem().(*FileSystem).Unlock()
				file := deepCopy(memFile)
				err := f.CopyToFile(file)
				if err != nil {
					return nil, err
				}

				err = f.Delete()
				if err != nil {
					return nil, err
				}

				return file, nil
			}
		}
	}
	f.memFile.location.FileSystem().(*FileSystem).Unlock()
	// if the file doesn't yet exist at the location, create it there
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}
	// initialize the file
	_, err = newFile.Write(make([]byte, 0))
	if err != nil {
		return nil, err
	}
	// copying over the data
	err = f.CopyToFile(newFile)
	if err != nil {
		return nil, err
	}
	// delete the receiver
	err = f.Delete()
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

// MoveToFile creates a newFile, and moves it to "file".
// The receiver is always deleted (since it's being "moved")
func (f *File) MoveToFile(file vfs.File) error {
	if f == nil {
		return nilReference()
	}
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return err
		}
		return doesNotExist()
	}
	if err := f.CopyToFile(file); err != nil {
		return err
	}

	return f.Delete()
}

// Delete removes the file from the FileSystem. Sets it path in the fsMap to nil,
// and also nils the file's members
func (f *File) Delete() error {
	if f == nil {
		return nilReference()
	}

	if ex, _ := f.Exists(); !ex {
		return doesNotExist()
	}
	f.memFile.Lock()
	defer f.memFile.Unlock()
	loc := f.Location().(*Location)
	mapRef := loc.fileSystem.fsMap
	f.memFile.location.FileSystem().(*FileSystem).Lock()
	defer f.memFile.location.FileSystem().(*FileSystem).Unlock()
	// if there are keys at this volume
	if _, ok := mapRef[loc.Volume()]; ok {
		// checking for the object that should contain the file at this key
		if thisObj, ok2 := mapRef[loc.Volume()][f.Path()]; ok2 {
			str := f.Path()
			// casting a file to the object's "i" interface
			file := thisObj.i.(*memFile)
			file.exists = false
			file = nil
			thisObj.i = nil
			thisObj = nil
			// setting that key to nil so it truly no longer lives on this system
			mapRef[loc.Volume()][str] = nil
		}
	}

	return nil
}

func newMemFile(file *File, location vfs.Location) *memFile {
	return &memFile{
		sync.Mutex{},
		new(bytes.Buffer),
		false,
		make([]byte, 0),
		location,
		time.Time{},
		file.name,
		false,
		path.Join(location.Path(), file.Name()),
	}
}

// LastModified simply returns the file's lastModified, if the file exists
func (f *File) LastModified() (*time.Time, error) {
	if f == nil {
		return nil, nilReference()
	}
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return nil, err
		}
		return nil, doesNotExist()
	}
	return &f.memFile.lastModified, nil
}

// Size returns the size of the file contents.  In our case, the length of the file's byte slice
func (f *File) Size() (uint64, error) {
	if f == nil {
		return 0, nilReference()
	}
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return 0, err
		}
		return 0, doesNotExist()
	}
	f.synchronize()
	return uint64(len(f.contents)), nil

}

// Touch takes a in-memory vfs.File, makes it existent, and updates the lastModified
func (f *File) Touch() error {
	if f == nil {
		return nilReference()
	}
	if f.memFile.exists {
		f.exists = true
		f.memFile.lastModified = time.Now()

		return nil
	}
	f.memFile.exists = true
	f.exists = true

	volume := f.Location().Volume()
	f.memFile.lastModified = time.Now()
	// files and locations are contained in objects of type "fsObject".
	// An fsObject has a blank interface and a boolean that indicates whether or not it is a file
	fileObject := &fsObject{
		true,
		f.memFile,
	}
	locObject := &fsObject{
		false,
		f.Location(),
	}

	f.memFile.location.FileSystem().(*FileSystem).Lock()
	defer f.memFile.location.FileSystem().(*FileSystem).Unlock()
	// just a less clunky way of accessing the fsMap
	mapRef := f.memFile.location.(*Location).fileSystem.fsMap
	// if the objMap map does not exist for the volume yet, then we go ahead and create it.
	if _, ok := mapRef[volume]; !ok {
		mapRef[volume] = make(objMap)
	}

	// setting the map at Volume volume and path of f to this fileObject
	mapRef[volume][f.Path()] = fileObject
	f.memFile = mapRef[volume][f.Path()].i.(*memFile)
	locationPath := utils.EnsureTrailingSlash(path.Clean(path.Dir(f.Path())))
	// checking for locations that exist to avoid redundancy
	if _, ok := mapRef[volume][locationPath]; !ok {
		mapRef[volume][locationPath] = locObject
	}
	return nil
}

// Path returns the absolute path to the file
func (f *File) Path() string {
	if f == nil {
		panic(nilReference())
	}
	return path.Join(f.Location().Path(), f.name)
}

// Name returns the basename of the file
func (f *File) Name() string {
	if f == nil {
		panic(nilReference())
	}
	return f.name
}

// URI returns the file's URI, if it exists
func (f *File) URI() string {
	if f == nil {
		panic(nilReference())
	}
	return utils.GetFileURI(f)
}

// synchronize updates a memFile's contents slice and cursor members
func (f *File) synchronize() {
	if f == nil {
		panic(nilReference())
	}
	if ok := string(f.contents) != string(f.memFile.contents); ok {
		f.cursor = 0
		f.contents = f.memFile.contents
	}
}
