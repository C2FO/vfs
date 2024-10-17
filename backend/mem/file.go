package mem

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"sync"
	"time"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

type mode int

const (
	none mode = iota
	truncate
	edit
)

// memFile represents the file's at-rest state on the file system. including its contents, location, and lastModified time
type memFile struct {
	sync.Mutex
	exists       bool
	contents     []byte // the file's contents at rest
	location     *Location
	lastModified time.Time
	name         string
	filepath     string
}

// File implements vfs.File interface for the in-memory implementation of FileSystem.
// A file struct holds a pointer to a single memFile.  Multiple threads will refer to the same
// memFile. Simultaneous reading is allowed, but writing and closing are protected by locks.
type File struct {
	memFile         *memFile
	readWriteSeeker *ReadWriteSeeker
	name            string // the base name of the file
	cursor          int
	writeMode       mode
	isOpen          bool
	readCalled      bool
	seekCalled      bool
}

func nilReference() error {
	return errors.New("the target file passed in was nil")
}

// Close imitates io.Closer by resetting the cursor and setting a boolean
func (f *File) Close() error {
	if f.isOpen {
		// update the contents of the memFile
		f.memFile.Lock()
		defer f.memFile.Unlock()
		f.memFile.contents = f.readWriteSeeker.Bytes()
		f.memFile.lastModified = time.Now()
		f.memFile.exists = true
		f.memFile.location.exists = true

		// update the fsMap
		f.Location().FileSystem().(*FileSystem).mu.Lock()
		defer f.Location().FileSystem().(*FileSystem).mu.Unlock()
		mapRef := f.Location().FileSystem().(*FileSystem).fsMap
		if _, ok := mapRef[f.Location().Volume()]; ok {
			if _, ok := mapRef[f.Location().Volume()][f.Path()]; ok {
				// memfile exists, so we update it
				mapRef[f.Location().Volume()][f.Path()].i = f.memFile
			} else {
				// memfile does not exist, so we create it
				mapRef[f.Location().Volume()][f.Path()] = &fsObject{true, f.memFile}
			}
		} else {
			// volume does not exist, so we create it with the memfile
			mapRef[f.Location().Volume()] = make(objMap)
			mapRef[f.Location().Volume()][f.Path()] = &fsObject{true, f.memFile}
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

// Read implements the io.Reader interface.  Returns number of bytes read and potential errors
func (f *File) Read(p []byte) (n int, err error) {
	var existsOnFS bool
	if existsOnFS, err = f.Exists(); !existsOnFS && !f.isOpen {
		if err != nil {
			return 0, utils.WrapReadError(err)
		}
		return 0, utils.WrapReadError(fs.ErrNotExist)
	}

	// in case the file contents have changed
	if existsOnFS && f.writeMode == none {
		// get the file's contents from fsMap
		fsMap := f.Location().FileSystem().(*FileSystem).fsMap
		if objMap, ok := fsMap[f.Location().Volume()]; ok {
			if obj, ok := objMap[f.Path()]; ok {
				if obj.isFile {
					// update the file's memFile
					f.memFile = obj.i.(*memFile)
				}
			}
		}

		// update the file's readWriteSeeker contents and set the cursor to the current position
		f.readWriteSeeker = NewReadWriteSeekerWithData(f.memFile.contents)
		_, err = f.readWriteSeeker.Seek(int64(f.cursor), 0)
		if err != nil {
			return 0, utils.WrapReadError(err)
		}
	}

	// read file
	read, err := f.readWriteSeeker.Read(p)
	if err != nil {
		// if we got io.EOF, we'll return the read and the EOF error
		// because io.Copy looks for EOF to determine if it's done
		// and doesn't support error wrapping
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	// update open file's state
	f.readCalled = true
	f.isOpen = true
	f.cursor += read

	return read, nil
}

// Seek implements the io.Seeker interface.  Returns the current position of the cursor and errors if any
func (f *File) Seek(offset int64, whence int) (int64, error) {
	var existsOnFS bool
	var err error
	if existsOnFS, err = f.Exists(); !existsOnFS && !f.isOpen {
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
		return 0, utils.WrapSeekError(fs.ErrNotExist)
	}

	// in case the file contents have changed
	if existsOnFS && f.writeMode == none {
		// update the file's memFile
		fsMap := f.Location().FileSystem().(*FileSystem).fsMap
		if objMap, ok := fsMap[f.Location().Volume()]; ok {
			if obj, ok := objMap[f.Path()]; ok {
				if obj.isFile {
					// update the file's memFile
					f.memFile = obj.i.(*memFile)
				}
			}
		}

		// update the file's readWriteSeeker contents and set the cursor to the current position
		f.readWriteSeeker = NewReadWriteSeekerWithData(f.memFile.contents)
		_, err := f.readWriteSeeker.Seek(int64(f.cursor), 0)
		if err != nil {
			return 0, utils.WrapSeekError(err)
		}
	}

	// seek file
	pos, err := f.readWriteSeeker.Seek(offset, whence)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	// update open file's state
	f.isOpen = true
	f.seekCalled = true
	f.cursor = int(pos)

	return pos, nil
}

// Write implements the io.Writer interface. Returns number of bytes written and any errors.
// Unless Seek or Read is called first, Write's should overwrite any existing file.  Otherwise, it should edit the file
// in place.
func (f *File) Write(p []byte) (int, error) {
	// if the file has not yet been opened for writing, set the writeMode and readWriteSeeker
	if f.writeMode == none {
		if f.readCalled || f.seekCalled {
			// file has been read or seeked first, so we are in edit mode
			f.writeMode = edit
			f.readWriteSeeker = NewReadWriteSeekerWithData(f.memFile.contents)
			_, err := f.readWriteSeeker.Seek(int64(f.cursor), 0)
			if err != nil {
				return 0, utils.WrapWriteError(err)
			}
		} else {
			// file has not been read or seeked first, so we are in truncate(overwrite) mode
			f.readWriteSeeker = NewReadWriteSeeker()
			f.writeMode = truncate
		}
		f.isOpen = true
	}

	// write to file buffer (writes aren't committed to the filesystem file until Close is called)
	written, err := f.readWriteSeeker.Write(p)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	// update the file's cursor
	f.cursor += written

	return written, nil
}

// String implements the io.Stringer interface. It returns a string representation of the file's URI
func (f *File) String() string {
	return f.URI()
}

// Exists returns whether a file exists.  Creating a file does not guarantee its existence, but creating one and writing
// to it does.
func (f *File) Exists() (bool, error) {
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

// Location simply returns the file's underlying location struct pointer
func (f *File) Location() vfs.Location {
	// copy the location
	location := *f.memFile.location
	return &location
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
		return nil, fs.ErrNotExist
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

	if err := f.CopyToFile(newFile); err != nil {
		return nil, err
	}

	return newFile, nil
}

// CopyToFile copies the receiver file into the target file. Additionally,
// after this is called, f's cursor will reset as if it had been closed.
func (f *File) CopyToFile(target vfs.File) (err error) {
	if f == nil || target == nil {
		return nilReference()
	}
	// Close file (f) reader regardless of an error
	defer func() {
		// close writer
		wErr := target.Close()
		// close reader
		rErr := f.Close()
		//
		if err == nil {
			if wErr != nil {
				err = wErr
			} else if rErr != nil {
				err = rErr
			}
		}
	}()

	if exists, err := f.Exists(); !exists {
		if err != nil {
			return err
		}
		return fs.ErrNotExist
	}

	// validate seek is at 0,0 before doing copy
	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}

	if target.Location().FileSystem().Scheme() == "mem" {
		target.(*File).memFile.contents = make([]byte, 0)
	}

	if _, err := target.Write(f.memFile.contents); err != nil {
		return err
	}
	cerr := target.Close()
	if cerr != nil {
		return cerr
	}
	return err
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
		return nil, fs.ErrNotExist
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
		f.memFile.location.FileSystem().(*FileSystem).mu.Lock()
		// this checks if the specified volume has any keys
		if _, ok := mapRef[vol]; ok {
			// this block checks if the file already exists at location, if it does, deletes it and inserts the file we have
			if _, ok2 := mapRef[vol][testPath]; ok2 {
				memFile := mapRef[vol][testPath].i.(*memFile)
				f.memFile.location.FileSystem().(*FileSystem).mu.Unlock()
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
		f.memFile.location.FileSystem().(*FileSystem).mu.Unlock()
	}
	// if the file doesn't yet exist at the location, create it there
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}
	// copying over the data
	err = f.MoveToFile(newFile)
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
		return fs.ErrNotExist
	}
	if err := f.CopyToFile(file); err != nil {
		return err
	}

	return f.Delete()
}

// Delete removes the file from the FileSystem. Sets it path in the fsMap to nil,
// and also nils the file's members
func (f *File) Delete(_ ...options.DeleteOption) error {

	if ex, _ := f.Exists(); !ex {
		return fs.ErrNotExist
	}
	f.memFile.Lock()
	defer f.memFile.Unlock()
	loc := f.Location().(*Location)
	mapRef := loc.fileSystem.fsMap
	f.memFile.location.FileSystem().(*FileSystem).mu.Lock()
	defer f.memFile.location.FileSystem().(*FileSystem).mu.Unlock()
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
			delete(mapRef[loc.Volume()], str)
		}
	}

	return nil
}

func newMemFile(file *File, location *Location) *memFile {
	return &memFile{
		contents: make([]byte, 0),
		location: location,
		name:     file.name,
		filepath: path.Join(location.Path(), file.Name()),
	}
}

// LastModified simply returns the file's lastModified, if the file exists
func (f *File) LastModified() (*time.Time, error) {
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return nil, err
		}
		return nil, fs.ErrNotExist
	}
	return &f.memFile.lastModified, nil
}

// Size returns the size of the file contents.  In our case, the length of the file's byte slice
func (f *File) Size() (uint64, error) {
	if exists, err := f.Exists(); !exists {
		if err != nil {
			return 0, err
		}
		return 0, fs.ErrNotExist
	}

	// in case the file contents have changed
	f.readWriteSeeker = NewReadWriteSeekerWithData(f.memFile.contents)

	return uint64(len(f.readWriteSeeker.Bytes())), nil

}

// Touch takes an in-memory vfs.File, makes it existent, and updates the lastModified
func (f *File) Touch() error {
	if f.memFile.exists {
		f.memFile.lastModified = time.Now()

		return nil
	}
	f.memFile.exists = true

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

	f.memFile.location.FileSystem().(*FileSystem).mu.Lock()
	defer f.memFile.location.FileSystem().(*FileSystem).mu.Unlock()
	// just a less clunky way of accessing the fsMap
	mapRef := f.Location().FileSystem().(*FileSystem).fsMap
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
	return path.Join(f.Location().Path(), f.name)
}

// Name returns the basename of the file
func (f *File) Name() string {
	return f.name
}

// URI returns the file's URI, if it exists
func (f *File) URI() string {
	return utils.GetFileURI(f)
}
