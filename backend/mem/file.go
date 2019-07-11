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



type memFile struct{
	sync.Mutex
	writeBuffer *bytes.Buffer
	exists     bool
	contents []byte
	location vfs.Location
	lastModified time.Time
	name 		string
	isOpen 		bool
	filepath 	string
}
/*

	//File implements vfs.File interface for the in-memory implementation of FileSystem.
	//A file struct holds a pointer to a single memFile.  Multiple threads will refer to the same
	//memFile. Simultaneous reading is allowed, but writing and closing are protected by locks.
*/
type File struct {
	memFile      *memFile
	exists       bool
	lastModified time.Time
	isOpen       bool
	contents     []byte       //the file contents
	name         string       //the base name of the file
	cursor       int          //the index that the buffer (contents) is at
	location     vfs.Location //the location that the file exists on

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
	f.memFile.Lock()
	if f.memFile.writeBuffer.Len() > 0{
		bufferContents := f.memFile.writeBuffer.Bytes()
		for i ,_ := range bufferContents{
			f.memFile.contents = append(f.memFile.contents,bufferContents[i])

		}
		f.memFile.writeBuffer = new(bytes.Buffer)
	}

	f.memFile.isOpen = false
	f.cursor = 0
	f.memFile.Unlock()
	return nil
}

//Read implements the io.Reader interface.  Returns number of bytes read and potential errors
func (f *File) Read(p []byte) (n int, err error) {

	//if file exists:
	offset := f.cursor
	if f.isOpen == false {
		f.isOpen = true
	}
	exists, err := f.Exists()
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	readBufferLength := len(p)
	if readBufferLength == 0 { //readBufferLength of byte slice is zero, just return 0 and nil
		return 0, nil
	}

	f.update()	//in case the file contents have changed

	fileContentLength := len(f.contents)
	if f.cursor == fileContentLength { //if the cursor is at the end of the file
		return 0, io.EOF
	}
	j := 0        //j is the incrementer for the readBuffer. It always starts at 0, but the cursor may not
	for i := range p {
		if !f.isOpen{
			return i,errors.New("file is closed")
		}

		if i == readBufferLength || f.cursor == fileContentLength { //if "i" is greater than the readBufferLength of p or readBufferLength of the contents
			return i, io.EOF
		}

		p[i] = f.contents[f.cursor] //otherwise simply copy each index to p
		j++
		f.cursor++

	}
	if f.cursor > len(f.contents) {
		f.cursor = len(f.contents)
	}
	return readBufferLength - offset, nil

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
	if !f.isOpen {
		f.isOpen = true
	}

	if ex, err := f.Exists(); !ex {
		if err !=nil {
			return 0, err
		}
		err = f.Touch()
		if err != nil {
			return 0,err
		}
	}
	f.memFile.Lock()
	num,err := f.memFile.writeBuffer.Write(p)
	f.memFile.lastModified = time.Now()
	f.memFile.Unlock()
	return num, err

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

				if object != nil && object.i.(*memFile).exists {
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

	if ok, err := f.Exists(); !ok{
		return nil, err
	}
	testPath := path.Join(path.Clean(location.Path()), f.Name())
	thisLoc := f.Location().(*Location)
	mapRef := thisLoc.fileSystem.fsMap
	vol := thisLoc.Volume()

	if _, ok := mapRef[vol]; ok { //making sure that this volume has keys at all
		if _, ok2 := mapRef[vol][testPath]; ok2 { //if file w/name exists @ loc, simply copy contents over
			memFile := mapRef[vol][testPath].i.(*memFile) //casting obj to a file
			file := memToVFs(memFile)

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
		target.(*File).memFile.contents = make([]byte,0)
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
		f.location.FileSystem().(*FileSystem).Lock()

		if _, ok := mapRef[vol]; ok { //this checks if the specified volume has any keys
			//this block checks if the file already exists at location, if it does, deletes it and inserts the file we have
			if _, ok2 := mapRef[vol][testPath]; ok2 { //if the file already exists at that location
				memFile := mapRef[vol][testPath].i.(*memFile)
				f.location.FileSystem().(*FileSystem).Unlock()
				file := memToVFs(memFile)
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
	f.location.FileSystem().(*FileSystem).Unlock()
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
	if ok, err := f.Exists(); !ok{
		return err
	}
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
	f.memFile.Lock()
	defer f.memFile.Unlock()
	loc := f.Location().(*Location)
	mapRef := loc.fileSystem.fsMap
	f.location.FileSystem().(*FileSystem).Lock()
	defer 	f.location.FileSystem().(*FileSystem).Unlock()
	if _, ok := mapRef[loc.Volume()]; ok { //if there are keys at this volume
		if thisObj, ok2 := mapRef[loc.Volume()][f.Path()]; ok2 { //checking for the object that should contain the file at this key
			str := f.Path()
			file := thisObj.i.(*memFile) //casting a file to the object's "i" interface
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
		isOpen: false, exists: false,
	}, nil

}

func newMemFile(file *File) *memFile{

	return &memFile{
		sync.Mutex{},
		new(bytes.Buffer),
		false,
		make([]byte,0),
		file.location,
		file.lastModified,
		file.name,
		false,
		file.Path(),
	}
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
	f.update()
	return uint64(len(f.contents)), nil

}
//Touch takes a in-memory vfs.File, makes it existent, and updates the lastModified
func (f *File) Touch() error {
	if f == nil {
		return errors.New("Receiver is nil")
	}
	if f.memFile.exists{
		f.exists = true
		return nil
	}
	f.memFile.exists = true
	f.exists = true
	f.lastModified = time.Now()
	//files and locations are contained in objects of type "obj".
	// An obj has a blank interface and a boolean that indicates whether or not it is a file
	var locObject obj
	var fileObject obj
	fileObject.i = f.memFile
	fileObject.isFile = true

	loc := f.Location().(*Location)
	volume := loc.Volume()
	locObject.i = f.Location()
	locObject.isFile = false
	f.location.FileSystem().(*FileSystem).Lock()
	defer f.location.FileSystem().(*FileSystem).Unlock()
	mapRef := loc.fileSystem.fsMap      //just a less clunky way of accessing the fsMap
	if _, ok := mapRef[volume]; !ok { //if the objMap map does not exist for the volume yet, then we go ahead and create it.
		mapRef[volume] = make(objMap)
	}

	mapRef[volume][f.Path()] = &fileObject //setting the map at Volume volume and path of f to this fileObject
	f.memFile = mapRef[volume][f.Path()].i.(*memFile)
	locationPath := utils.EnsureTrailingSlash(path.Clean(path.Dir(f.Path())))
	if _, ok := mapRef[volume][locationPath]; !ok { //checking for that locations existence to avoid redundancy
		mapRef[volume][locationPath] = &locObject
	}
	return nil
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
//update updates a memFile's contents slice and cursor members
func (f *File) update(){
	if ok := string(f.contents) != string(f.memFile.contents); ok{
		f.cursor = 0
		f.contents = f.memFile.contents
		f.lastModified = f.memFile.lastModified
		f.location = f.memFile.location
	}
}

func memToVFs(file *memFile) vfs.File{
	return &File{
		file,
		true,
		file.lastModified,
		true,
		file.contents,
		file.name,
		0,
		file.location,
	}
}