package mem

import (
	"bytes"
	"errors"
	"github.com/c2fo/vfs/v4"
	"io"
	"path"
	"time"
)

//File implements vfs.File interface for os fs.
type File struct {
	exists     bool
	timeStamp  time.Time
	isOpen     bool
	isRef      bool         //has it been referenced before?
	privSlice  []byte       //the file contents
	Filename   string       //the base name of the file
	cursor     int          //the index that the buffer (privSlice) is at
	location   vfs.Location //the location that the file exists on
	fileSystem *FileSystem
}

/*		******* Error Functions *******		*/
func doesNotExist() error {
	return errors.New("This file does not exist!")
}
func copyFail() error {
	return errors.New("This file was not successfully copied")
}

func copyFailNil() error {
	return errors.New("The target file passed in was nil")
}

func deleteError() error {
	return errors.New("Deletion was unsuccessful")
}

func moveToLocationError() error {
	return errors.New("Move to location unexpectedly failed")
}

func writeError() error {
	return errors.New("Unexpected Write Error")
}

func seekError() error {
	return errors.New("Seek could not complete the desired call")
}

/*		******************		*/

//Close imitates io.Closer by resetting the cursor and setting a boolean
func (f *File) Close() error {
	if !f.isRef {
		// Do nothing on files that were never referenced
		return nil
	}
	if f == nil {
		return errors.New("Cannot close a nil file")
	}

	f.isOpen = false
	f.cursor = 0
	return nil
}

//Read implements the io.Reader interface.  Returns number of bytes read and potential errors
func (f *File) Read(p []byte) (n int, err error) {
	//if file exists:
	if f.isOpen == false {
		f.isOpen = true
	}
	existence, eerr := f.Exists()
	if !existence {
		return 0, eerr
	}
	f.isRef = true
	length := len(p)
	if length == 0 { //length of byte slice is zero, just return 0 and nil
		return 0, nil
	}

	if f.cursor == len(f.privSlice) {
		return 0, io.EOF
	}
	for i := f.cursor; i < length; i++ {
		f.cursor++
		if i >= length || i >= len(f.privSlice) {
			break
		}
		if i == length {
			break
		}
		p[i] = f.privSlice[i]
	}

	return length, nil

}

//Seek implements the io.Seeker interface.  Returns the current position of the cursor and errors if any
func (f *File) Seek(offset int64, whence int) (int64, error) {

	length := len(f.privSlice)
	if length == 0 && offset == 0 && whence == 0 {
		return 0, nil
	}

	switch whence {

	case 0:
		if int(offset) < length && offset >= 0 {
			f.cursor = int(offset)
			return offset, nil
		} else {
			return int64(f.cursor), seekError()
		}

	case 1:
		pos := f.cursor + int(offset)
		if pos < length && pos >= 0 {
			f.cursor = pos
			return int64(pos), nil
		} else {
			return int64(f.cursor), seekError()
		}

	case 2:
		pos := length + int(offset)
		if pos < length && pos >= 0 {
			f.cursor = pos
			return int64(f.cursor), nil
		} else {
			return int64(f.cursor), seekError()
		}

	default:
		return 0, seekError()

	}

}

//Write implements the io.Writer interface. Returns number of bytes written and any errors
func (f *File) Write(p []byte) (n int, err error) {
	if f.isOpen == false {
		f.isOpen = true
	}
	f.exists = true
	f.isRef = true
	length := len(p)

	if length == 0 {

		return 0, nil
	}
	for length > len(f.privSlice)-f.cursor || length > len(f.privSlice) {
		f.privSlice = append(f.privSlice, make([]byte, 1)...)
	}
	for i := 0; i < length; i++ {
		if i >= length || i >= len(f.privSlice) {
			break
		}
		if i == length {
			break
		}
		f.privSlice[f.cursor] = p[i]
		f.cursor++

	}

	f.timeStamp = time.Now()
	return length - 1, err
}

//String implements the io.Stringer interface. It returns a string representation of the file's URI
func (f *File) String() string {
	return f.URI()
}

/*
Exists returns whether or not a file exists.  Creating a file does not
guarantee its existence, but creating one and writing to it does
*/
func (f *File) Exists() (bool, error) {
	if !f.exists {
		return false, nil

	} else {
		return true, nil
	}
}

//Location simply returns the file's underlying location struct pointer
func (f *File) Location() vfs.Location {

	newLoc, _ := f.location.NewLocation("")

	return newLoc
}

/*
CopyToLocation copies the current file to the given location.  If file exists
at given location contents are simply overwritten using "CopyToFile", otherwise
a newFile is made, takes the contents of the current file, and ends up at
the given location
*/

func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {

	testPath := path.Join(path.Clean(location.Path()), f.Name())
	thisLoc := f.Location().(*Location)
	mapRef := &thisLoc.fileSystem.fsMap
	vol := thisLoc.Volume()
	if _, ok := (*mapRef)[vol]; ok { //making sure that this volume has keys at all
		if _, ok2 := (*mapRef)[vol][testPath]; ok2 { //if file w/name exists @ loc, simply copy contents over
			file := (*mapRef)[vol][testPath].i.(*File) //casting obj to a file
			cerr := f.CopyToFile(file)
			if cerr != nil {
				return nil, cerr
			}
			return file, nil
		}
	} //end outer-if

	newFile, nerr := location.NewFile(f.Name())

	if nerr != nil {
		return nil, nerr
	}
	_, werr := newFile.Write(make([]byte, 0))

	if werr != nil {
		return newFile, werr
	}
	cerr := f.CopyToFile(newFile)
	if cerr != nil {
		return nil, cerr
	}
	return newFile, nil
}

/*
 CopyToFile copies the receiver file into the target file.
The target file is deleted, so any references to it will
be nil.  In order to access the target after calling CopyToFile
use its previous path to call it using the systemMap
*/
func (f *File) CopyToFile(target vfs.File) error {

	if target == nil {
		return copyFailNil()
	}
	if ex, _ := target.Exists(); !ex {
		return doesNotExist()
	}
	_, serr1 := target.Seek(0, 0)
	if serr1 != nil {
		return serr1
	}
	_, serr2 := f.Seek(0, 0)
	if serr2 != nil {
		return serr2
	}
	size, sizeErr := f.Size()
	if sizeErr != nil {
		return sizeErr
	}
	oldCursor, serr3 := f.Seek(10, 2)
	if serr3 == nil { //want it to throw error because we just want the current cursor position
		return serr3
	}
	bSlice := make([]byte, int(size))
	_, rerr := f.Read(bSlice)
	if rerr != nil {
		return rerr
	}

	_, werr := target.Write(bSlice)
	if werr != nil {
		return werr
	}

	closeErr := target.Close()
	if closeErr != nil {
		return closeErr
	}

	_, serr4 := f.Seek(oldCursor, 0)
	return serr4
}

/*
 MoveToLocation moves the receiver file to the passed in location. It does so by
creating a copy of 'f' in "location".  'f' is subsequently  deleted
*/
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {

	testPath := path.Join(location.Path(), f.Name())
	loc := location.(*Location)
	mapRef := &loc.fileSystem.fsMap
	vol := loc.Volume()
	if _, ok := (*mapRef)[vol]; ok {
		//this block checks if the file already exists at location, if it does, deletes it and inserts the file we have
		if _, ok2 := (*mapRef)[vol][testPath]; ok2 { //if the file already exists at that location
			file := (*mapRef)[vol][testPath].i.(*File)

			cerr := f.CopyToFile(file)
			if cerr != nil {
				return nil, cerr
			}

			derr := f.Delete()
			if derr != nil {
				return nil, derr
			}
			return file, nil
		}
	}
	newFile, nerr := location.NewFile(f.Name()) //creating the file in the desired location
	if nerr != nil {
		return nil, nerr
	}

	_, werr := newFile.Write(make([]byte, 0)) //writing zero bytes to ensure existence
	if werr != nil {
		return nil, werr
	}

	cerr := f.CopyToFile(newFile) //copying over the data
	if cerr != nil {
		return nil, cerr
	}

	derr := f.Delete()
	if derr != nil {
		return nil, derr
	}

	return newFile, nil
}

/*
MoveToFile creates a newFile, and moves it to "file".
If names are same, "file" is deleted and newFile takes its place.
The receiver is always deleted (since it's being "moved")
*/

func (f *File) MoveToFile(file vfs.File) error {

	if f.Name() == file.Name() {

		loc := file.Location()

		derr := file.Delete()
		if derr != nil {
			return deleteError()
		}

		newFile, nerr := loc.NewFile(f.Name())
		if nerr != nil {
			return nerr
		}

		_, werr := newFile.Write(make([]byte, 0))
		if werr != nil {
			return werr
		}
		cerr := newFile.Close()
		if cerr != nil {
			return cerr
		}
		copyErr := f.CopyToFile(newFile)
		if copyErr != nil {
			return copyFail()
		}
		derr1 := f.Delete()
		return derr1
	}

	newFile, nerr2 := file.Location().NewFile(f.Name())
	if nerr2 != nil {
		return nerr2
	}
	_, werr := newFile.Write(make([]byte, 0))
	if werr != nil {
		return werr
	}
	cerr := newFile.Close()
	if cerr != nil {
		return cerr
	}
	copyErr := f.CopyToFile(newFile)
	if copyErr != nil {
		return errors.New("CopyToFile failed unexpectedly and as a result so did MoveToFile")
	}

	derr := f.Delete()

	return derr

}

/*
Delete removes the file from the fs. Sets it path to the systemMap to nil,
 removes it from the filelist, and appropriately shifts the list
*/
func (f *File) Delete() error {

	existence, err := f.Exists()
	if err != nil {
		return err
	}

	loc := f.Location().(*Location)
	mapRef := &loc.fileSystem.fsMap
	if existence {
		if _, ok := (*mapRef)[loc.Volume()]; ok {
			if thisObj, ok2 := (*mapRef)[loc.Volume()][f.Path()]; ok2 {
				str := f.Path()
				file := thisObj.i.(*File)
				file.exists = false
				file.location = nil
				file = nil
				thisObj.i = nil
				thisObj = nil
				(*mapRef)[loc.Volume()][str] = nil //setting that key to nil so it truly no longer lives on this system
			}
		}

		return nil
	}
	return doesNotExist()
}

//newFile creates an in-mem vfs file given the name then returns it
func newFile(name string) (*File, error) {
	return &File{
		timeStamp: time.Now(), isRef: false, Filename: name, cursor: 0,
		isOpen: false, exists: false,
	}, nil

}

//LastModified simply returns the file's timeStamp, if the file exists
func (f *File) LastModified() (*time.Time, error) {

	existence, err := f.Exists()

	if existence {
		return &f.timeStamp, err
	}
	return nil, err
}

//Size returns the size of the file contents.  In our case, the length of the file's byte slice
func (f *File) Size() (uint64, error) {

	return uint64(len(f.privSlice)), nil

}

//Path returns the absolute path to the file
func (f *File) Path() string {

	return path.Join(f.Location().Path(), f.Filename)

}

//Name returns the file's base name
func (f *File) Name() string {
	//if file exists

	return f.Filename
}

//URI returns the file's URI, if it exists
func (f *File) URI() string {

	existence, _ := f.Exists()
	if !existence {
		return ""
	}
	var buf bytes.Buffer
	pref := "mem://"
	buf.WriteString(pref)
	buf.WriteString(f.location.Volume())
	str := f.Path()
	buf.WriteString(str)
	return buf.String()
}

func (o objMap) hasLocation(loc string) bool {
	if _, ok := o[loc]; ok {
		return ok
	}
	return false
}

func (o objMap) remove(toRemove string) error {
	o[toRemove] = nil
	if _, ok := o[toRemove]; !ok {
		return nil
	}
	return errors.New("Could not remove object")
}
