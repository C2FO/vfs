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
	exists       bool
	timeStamp    time.Time
	isOpen       bool
	isZB         bool
	isRef        bool
	privSlice    []byte
	fileContents []byte
	Filename     string
	cursor       int
	location     vfs.Location
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
	f.timeStamp = time.Now()

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
//Exists returns whether or not a file exists.  Creating a file does not guarantee its existence, but creating one and writing to it does
func (f *File) Exists() (bool, error) {
	if !f.exists {
		return false, nil
	} else {
		return true, nil
	}
}

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
	if systemMap[testPath] != nil { //if file w/name exists @ loc, simply copy contents over
		if tmp := systemMap[testPath]; tmp != nil {
			cerr := f.CopyToFile(systemMap[testPath])
			if cerr != nil {
				return nil, cerr
			}
			return systemMap[testPath], nil
		} else {
			return nil, doesNotExist()
		}
	}

	newFile, _ := location.NewFile(f.Name())
	_, _ = newFile.Write(make([]byte, 0))
	cerr := f.CopyToFile(newFile)
	return systemMap[testPath], cerr
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
	//if target exists, its contents will be overwritten, otherwise it will be created...i'm assuming it exists
	//targetFile := target.(*File)
	name := target.Name()
	loc := target.Location()
	derr := target.Delete()
	if derr != nil {
		return derr
	}
	newFile, _ := loc.NewFile(name)

	_, err := newFile.Write(f.privSlice)
	_ = newFile.Close()
	target = systemMap[newFile.Path()]
	return err

}
/*
 MoveToLocation moves the receiver file to the passed in location. It does so by
creating a copy of 'f' in "location".  'f' is subsequently  deleted
*/
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {


	testPath := path.Join(location.Path(), f.Name())
	if systemMap[testPath] != nil {
		err := f.CopyToFile(systemMap[path.Clean(location.Path())])
		if err != nil {
			return nil, moveToLocationError()
		}
		return f, nil
	}
	fileName := f.Name()
	newPath := path.Join(location.Path(), fileName)
	newFile, _ := location.NewFile(path.Base(newPath))
	_, _ = newFile.Write(make([]byte, 0))
	cerr := f.CopyToFile(newFile)
	if cerr != nil {
		return nil, moveToLocationError()
	}
	derr := f.Delete()
	if derr != nil {
		return nil, derr
	}

	return systemMap[newPath], nil
}

/*
MoveToFile creates a newFile, and moves it to "file".
If names are same, "file" is deleted and newFile takes its place.
The receiver is always deleted (since it's being "moved")
*/

func (f *File) MoveToFile(file vfs.File) error {

	if f.Name() == file.Name() {

		derr := file.Delete()
		if derr != nil {
			return deleteError()
		}
		newFile, _ := file.Location().NewFile(f.Name())
		_, _ = newFile.Write(make([]byte, 0))
		_ = newFile.Close()
		copyErr := f.CopyToFile(newFile)
		if copyErr != nil {
			return copyFail()
		}
		derr1 := f.Delete()
		return derr1
	}

	newFile, _ := file.Location().NewFile(f.Name())
	_, _ = newFile.Write(make([]byte, 0))
	_ = newFile.Close()

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
	str := f.Filename
	index := f.getIndex()
	if index == -1 {
		return errors.New("This file does not exist!")
	}
	if existence {
		//do some work to adjust the location (later)
		systemMap[f.Path()] = nil
		f.exists = false
		f.privSlice = nil
		f.timeStamp = time.Now()
		fileList[index] = nil
		copy(fileList[index:], fileList[index+1:])
		fileList[len(fileList)-1] = nil // or the zero value of T
		fileList = fileList[:len(fileList)-1]

	}
	if systemMap[str] != nil {
		return errors.New("This file still exists after calling Delete()")
	}

	if f.getIndex() != -1 {
		return errors.New("This file still exists after calling Delete()")
	}
	return err

}

func newFile(name string) (*File, error) {

	//var l Location
	//tmp, err := (*Location).NewFile(&l,name)
	//file := tmp.(*File)

	return &File{
		timeStamp: time.Now(), isRef: false, Filename: path.Base(name), cursor: 0,
		isOpen: false, isZB: false, exists: false,
	}, nil

}

func (f *File) LastModified() (*time.Time, error) {

	existence, err := f.Exists()

	if existence {
		return &f.timeStamp, err
	}
	return nil, err
}

func (f *File) Size() (uint64, error) {

	return uint64(len(f.privSlice)), nil

}

func (f *File) Path() string {

	return path.Join(f.Location().Path(), f.Filename)

}

func (f *File) Name() string {
	//if file exists

	return f.Filename
}

func (f *File) URI() string {

	existence, _ := f.Exists()
	if !existence {
		return ""
	}
	var buf bytes.Buffer
	pref := "mem://"
	buf.WriteString(pref)
	str := f.Path()
	buf.WriteString(str)
	return buf.String()

}

func (f *File) getIndex() int {

	if systemMap[f.Path()] == nil {
		return -1
	}
	str := f.Path()
	for i, v := range fileList {
		existence, _ := v.Exists()
		if v.Path() == str && existence {
			return i
		}
	}
	return -1
}
