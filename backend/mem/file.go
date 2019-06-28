package mem

import (
	"bytes"
	"errors"
	"io"
	"path"
	"sync"
	"time"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/utils"
)

//File implements vfs.File interface for the in-memory implementation of FileSystem.
//TODO: this is NOT thread safe!
type File struct {
	exists       bool
	lastModified time.Time
	isOpen       bool
	writeBuffer  *bytes.Buffer
	contents     []byte       //the file contents
	name         string       //the base name of the file
	cursor       int          //the index that the buffer (contents) is at
	location     vfs.Location //the location that the file exists on
	mutex        sync.Mutex
}

/*		******* Error Functions *******		*/
func doesNotExist() error {
	return errors.New("This file does not exist!")
}

func copyFailNil() error {
	return errors.New("The target file passed in was nil")
}

func seekError() error {
	return errors.New("Seek could not complete the desired call")
}

/*		******************		*/

//Close imitates io.Closer by resetting the cursor and setting a boolean
func (f *File) Close() error {

	if f == nil {
		return errors.New("Cannot close a nil file")
	}
	if f.writeBuffer.Len() > 0{
		bufferContents := f.writeBuffer.Bytes()
		for i ,_ := range bufferContents{
			f.contents = append(f.contents,bufferContents[i])

		}
		f.writeBuffer = new(bytes.Buffer)
	}

	f.isOpen = false
	f.cursor = 0
	return nil
}

//Read implements the io.Reader interface.  Returns number of bytes read and potential errors
func (f *File) Read(p []byte) (n int, err error) {

	//if file exists:
	offset := f.cursor
	if f.isOpen == false {
		f.isOpen = true
	}
	existence, err := f.Exists()
	if !existence {
		return 0, err
	}
	readBufLen := len(p)
	if readBufLen == 0 { //readBufLen of byte slice is zero, just return 0 and nil
		return 0, nil
	}
	fileContentLength := len(f.contents)
	if f.cursor == len(f.contents) { //if the cursor is at the end of the file
		return 0, io.EOF
	}
	j := 0        //j is the incrementer for the readBuffer. It always starts at 0, but the cursor may not
	i := f.cursor //i takes the position of the cursor
	for i = range p {
		if !f.isOpen{
			return i,errors.New("file is closed")
		}

		if i == readBufLen || f.cursor == fileContentLength { //if "i" is greater than the readBufLen of p or readBufLen of the contents
			return i, io.EOF
		}

		p[i] = f.contents[f.cursor] //otherwise simply copy each index to p
		j++
		f.cursor++

	}
	if f.cursor > len(f.contents) {
		f.cursor = len(f.contents)
	}
	return readBufLen - offset, nil

}

//Seek implements the io.Seeker interface.  Returns the current position of the cursor and errors if any
func (f *File) Seek(offset int64, whence int) (int64, error) {

	length := len(f.contents)
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
func (f *File) Write(p []byte) (int,  error) {
	if f.isOpen == false {
		f.isOpen = true
	}

	if ex, _ := f.Exists(); !ex {
		f.Touch()
	}
	num,err :=f.writeBuffer.Write(p)
	return num, err

	/*

	if writeBufferLength == 0 {

		return 0, nil
	}
	if len(f.contents)-f.cursor <= writeBufferLength {
		f.contents = append(f.contents, make([]byte, writeBufferLength)...) //extend the filecontent slice by as much as we need
	}
	for i := 0; i < writeBufferLength; i++ {
		if i >= writeBufferLength || i >= len(f.contents) {
			break
		}
		if i == writeBufferLength {
			break
		}
		f.contents[f.cursor] = p[i]
		f.cursor++

	}

	f.lastModified = time.Now()
	return writeBufferLength, err

	 */
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

	if f != nil {
		//does it exist on the map?
		vol := f.Location().Volume()
		fullPath := f.Path()
		loc := f.Location().(*Location)
		mapRef := loc.fileSystem.fsMap
		if _, ok := mapRef[vol]; ok {
			if object, ok2 := mapRef[vol][fullPath]; ok2 {

				if object != nil && object.i.(*File).exists {
					return true, nil
				}
			}
		}
		return false, nil
	}
	return false, errors.New("Receiver is a nil value")
}

//Location simply returns the file's underlying location struct pointer
func (f *File) Location() vfs.Location {

	newLoc, err := f.location.FileSystem().NewLocation(f.location.Volume(), f.location.Path())
	if err != nil {
		panic(err)
	}
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
	mapRef := thisLoc.fileSystem.fsMap
	vol := thisLoc.Volume()
	if _, ok := mapRef[vol]; ok { //making sure that this volume has keys at all
		if _, ok2 := mapRef[vol][testPath]; ok2 { //if file w/name exists @ loc, simply copy contents over
			file := mapRef[vol][testPath].i.(*File) //casting obj to a file
			cerr := f.CopyToFile(file)
			if cerr != nil {
				return nil, cerr
			}
			return file, nil
		}
	} //end outer-if

	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}
	_, err = newFile.Write(make([]byte, 0)) //writing to existence. Whatever the underlying vfs implementation is will take care of this
	if err != nil {
		return nil, err
	}
	err = f.CopyToFile(newFile)
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

/*
 CopyToFile copies the receiver file into the target file.
The target file is deleted, so any references to it will
be nil.  In order to access the target after calling CopyToFile
use its previous path to call it using the fsMap.  Additionally,
after this is called, f's cursor will reset as if it had been closed.
*/
func (f *File) CopyToFile(target vfs.File) error {

	if ex, _ := f.Exists(); !ex {
		return doesNotExist()
	}

	if target == nil {
		return copyFailNil()
	}
	if target.Location().FileSystem().Scheme() == "mem"{
		target.(*File).contents = make([]byte,0)
	}

	if ex, err := target.Exists(); !ex {
		if err == nil {

			if _, werr := target.Write(f.contents); err != nil {
				return werr
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
	if _, err := target.Write(f.contents); err != nil {
		return err
	}
	err := target.Close()
	if err != nil {
		return err
	}
	return f.Close()
}

/*
 MoveToLocation moves the receiver file to the passed in location. It does so by
creating a copy of 'f' in "location".  'f' is subsequently  deleted
*/
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {

	if ex, err := f.Exists(); !ex {
		if err == nil {
			_, werr := f.Write(make([]byte, 0))
			if werr != nil {
				return nil, werr
			}
		} else {
			return nil, doesNotExist()
		}

	}
	/*
		 if the underling FileSystem is in-memory, then this is the native way of
		replacing a file with the same name as "f" at the location
	*/
	if location.FileSystem().Scheme() == "mem" {
		testPath := path.Join(location.Path(), f.Name()) //this is a potential path to a file that can be fed into the objMap portion of fsMap
		loc := location.(*Location)
		mapRef := loc.fileSystem.fsMap //mapRef just makes it easier to refer to "loc.fileSystem.fsMap"
		vol := loc.Volume()
		if _, ok := mapRef[vol]; ok { //this checks if the specified volume has any keys
			//this block checks if the file already exists at location, if it does, deletes it and inserts the file we have
			if _, ok2 := mapRef[vol][testPath]; ok2 { //if the file already exists at that location
				file := mapRef[vol][testPath].i.(*File)
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

	// if the file doesn't yet exist at the location, create it there
	newFile, err := location.NewFile(f.Name()) //creating the file in the desired location
	if err != nil {
		return nil, err
	}

	_, err = newFile.Write(make([]byte, 0)) //initialize the file
	if err != nil {
		return nil, err
	}
	err = f.CopyToFile(newFile) //copying over the data
	if err != nil {
		return nil, err
	}

	err = f.Delete() //delete the receiver
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

/*
MoveToFile creates a newFile, and moves it to "file".
The receiver is always deleted (since it's being "moved")
*/
func (f *File) MoveToFile(file vfs.File) error {

	if err := f.CopyToFile(file); err != nil {
		return err
	}

	return f.Delete()
}

/*
Delete removes the file from the FileSystem. Sets it path in the fsMap to nil,
 and also nils the file's members
*/
func (f *File) Delete() error {

	if f == nil {
		return errors.New("Cannot operate on a nil file reference")
	}

	if ex, _ := f.Exists(); !ex {
		return doesNotExist()
	}

	loc := f.Location().(*Location)
	mapRef := loc.fileSystem.fsMap
	if _, ok := mapRef[loc.Volume()]; ok { //if there are keys at this volume
		if thisObj, ok2 := mapRef[loc.Volume()][f.Path()]; ok2 { //checking for the object that should contain the file at this key
			str := f.Path()
			file := thisObj.i.(*File) //casting a file to the object's "i" interface
			file.exists = false
			file = nil
			thisObj.i = nil
			thisObj = nil
			mapRef[loc.Volume()][str] = nil //setting that key to nil so it truly no longer lives on this system
		}
	}

	return nil
}

//newFile creates an in-memory vfs file given the name then returns it
func newFile(name string) (*File, error) {
	return &File{
		lastModified: time.Now(), name: name, cursor: 0,
		isOpen: false, exists: false, writeBuffer: new(bytes.Buffer),
	}, nil

}

//LastModified simply returns the file's lastModified, if the file exists
func (f *File) LastModified() (*time.Time, error) {

	existence, err := f.Exists()

	if existence {
		return &f.lastModified, err
	}
	return nil, err
}

//Size returns the size of the file contents.  In our case, the length of the file's byte slice
func (f *File) Size() (uint64, error) {

	return uint64(len(f.contents)), nil

}

//Path returns the absolute path to the file
func (f *File) Path() string {

	return path.Join(f.Location().Path(), f.name)

}

func (f *File) Name() string {

	return f.name
}

//URI returns the file's URI, if it exists
func (f *File) URI() string {

	return utils.GetFileURI(f)

}
