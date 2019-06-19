package mem

import (
	"bytes"
	"errors"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/utils"
	"io"
	"path"
	"time"
)

//File implements vfs.File interface for os fs.
type File struct {
	exists       bool
	timeStamp    time.Time
	isOpen       bool
	fileContents []byte       //the file contents
	name         string       //the base name of the file
	cursor       int          //the index that the buffer (fileContents) is at
	location     vfs.Location //the location that the file exists on

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
	length := len(p)
	if length == 0 { //length of byte slice is zero, just return 0 and nil
		return 0, nil
	}

	if f.cursor == len(f.fileContents) {
		return 0, io.EOF
	}
	for i := f.cursor; i < length; i++ {
		f.cursor++
		if i >= length || i >= len(f.fileContents) {
			break
		}
		if i == length {
			break
		}
		p[i] = f.fileContents[i]
	}

	return length, nil

}

//Seek implements the io.Seeker interface.  Returns the current position of the cursor and errors if any
func (f *File) Seek(offset int64, whence int) (int64, error) {

	length := len(f.fileContents)
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
	if ex,_:=f.Exists();!ex{
		Touch(f)
	}
	length := len(p)

	if length == 0 {

		return 0, nil
	}
	for length > len(f.fileContents)-f.cursor || length > len(f.fileContents) {
		f.fileContents = append(f.fileContents, make([]byte, 1)...)
	}
	for i := 0; i < length; i++ {
		if i >= length || i >= len(f.fileContents) {
			break
		}
		if i == length {
			break
		}
		f.fileContents[f.cursor] = p[i]
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



	if f!=nil {
		//does it exist on the map?
		vol := f.Location().Volume()
		fullPath := f.Path()
		loc := f.Location().(*Location)
		mapRef := &loc.fileSystem.fsMap
		if _, ok := (*mapRef)[vol]; ok {
			if object, ok2 := (*mapRef)[vol][fullPath]; ok2 {

				if object !=nil && object.i.(*File).exists {
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

	newLoc, err := f.location.NewLocation("")
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

	Touch(newFile)

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

	if ex,_:=f.Exists();!ex{
		return doesNotExist()
	}

	if target == nil {
		return copyFailNil()
	}

	if ex, eerr := target.Exists(); !ex {
		if eerr == nil {
			Touch(target)
		} else {
			return eerr
		}
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

	if ex,eerr:=f.Exists();!ex{
		if eerr==nil{
			Touch(f)
		}else{
			return nil, doesNotExist()
		}

	}



	testPath := path.Join(location.Path(), f.Name()) //this is a potential path to a file that can be fed into the objMap portion of fsMap
	loc := location.(*Location)
	mapRef := &loc.fileSystem.fsMap //mapRef just makes it easier to refer to "loc.fileSystem.fsMap"
	vol := loc.Volume()
	if _, ok := (*mapRef)[vol]; ok { //this checks if the specified volume has any keys
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

	Touch(newFile)

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
			return derr
		}

		newFile, nerr := loc.NewFile(f.Name())
		if nerr != nil {
			return nerr
		}

		Touch(newFile)
		copyErr := f.CopyToFile(newFile)
		if copyErr != nil {
			return copyErr
		}

		return f.Delete()
	}

	newFile, nerr2 := file.Location().NewFile(f.Name())
	if nerr2 != nil {
		return nerr2
	}
	Touch(newFile)
	copyErr := f.CopyToFile(newFile)
	if copyErr != nil {
		return copyErr
	}

	return f.Delete()

}

/*
Delete removes the file from the fs. Sets it path in the fsMap to nil,
 and also nils the file's members
*/
func (f *File) Delete() error {

	if ex,_:=f.Exists();!ex{
		return doesNotExist()
	}

	if f==nil{
		return errors.New("Cannot operate on a nil file reference")
	}


	loc := f.Location().(*Location)
	mapRef := &loc.fileSystem.fsMap
	if _, ok := (*mapRef)[loc.Volume()]; ok { //if there are keys at this volume
		if thisObj, ok2 := (*mapRef)[loc.Volume()][f.Path()]; ok2 { //checking for the object that should contain the file at this key
			str := f.Path()
			file := thisObj.i.(*File) //casting a file to the object's "i" interface
			file.exists = false
			file = nil
			thisObj.i = nil
			thisObj = nil
			(*mapRef)[loc.Volume()][str] = nil //setting that key to nil so it truly no longer lives on this system
		}
	}

	return nil
}

//newFile creates an in-mem vfs file given the name then returns it
func newFile(name string) (*File, error) {
	return &File{
		timeStamp: time.Now(), name: name, cursor: 0,
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

	return uint64(len(f.fileContents)), nil

}

//Path returns the absolute path to the file
func (f *File) Path() string {

	return path.Join(f.Location().Path(), f.name)

}

//Name returns the file's base name
func (f *File) Name() string {
	//if file exists

	return f.name
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

func Touch(file vfs.File){
	if file==nil{
		return
	}
	name:=file.Location().FileSystem().Name()
	if name=="os"{
		_,_=file.Write(make([]byte,0))
		return

	}
	f:=file.(*File)
	f.exists=true
	var locObject obj
	var fileObject obj
	fileObject.i = f
	fileObject.isFile = true
	loc:=f.Location().(*Location)
	volume:=loc.Volume()
	locObject.i = f.Location()
	locObject.isFile = false
	mapRef := &loc.fileSystem.fsMap
	if _, ok := (*mapRef)[volume]; !ok { //if the objMap map does not exist for the volume yet, then we go ahead and create it.
		(*mapRef)[volume] = make(objMap)
	}
	(*mapRef)[volume][f.Path()] = &fileObject
	locationPath:=utils.AddTrailingSlash(path.Clean(path.Dir(f.Path())))
	if _,ok := (*mapRef)[volume][locationPath];!ok{
		(*mapRef)[volume][locationPath] = &locObject
	}
}
