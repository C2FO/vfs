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
	exists 		bool
	timeStamp 	time.Time
	isOpen		bool
	isZB		bool
	isRef		bool
	privSlice 	[] byte
	byteBuf  *bytes.Buffer
	Filename string
	cursor 		int
	location 	vfs.Location
}

func DoesNotExist() error {
	return errors.New("This file does not exist!")
}
func CopyFail() error {
	return errors.New("This file was not successfully copied")
}

func CopyFailNil() error{
	return errors.New("The target file passed in was nil")
}

func DeleteError() error {
	return errors.New("Deletion was unsuccessful")
}

func MoveToFileError() error{
	return errors.New("Move to file unexpectedly failed")
}
func MoveToLocationError() error{
	return errors.New("Move to location unexpectedly failed")
}
func WriteError() error{
	return errors.New("Unexpected Write Error")
}


func (f *File) Close() error {  //NOT DONE
	if !f.isRef {
		// Do nothing on files that were never referenced
		return nil
	}
	if f.byteBuf.Len() > 0{
		f.privSlice = append(f.privSlice,f.byteBuf.Next(200)...) //TODO: maybe change the number for "Next" arg
	}

	f.isOpen =false
	f.cursor = 0
	f.byteBuf.Reset()

	return nil
}


func (f *File) Read(p []byte) (n int, err error) {
	//if file exists:
	if f.isOpen == false{
		f.isOpen = true
	}
	existence, eerr := f.Exists()
	if !existence{
		return 0 ,eerr
	}
	f.isRef = true
	length := len(p)
	if length == 0{  //length of byte slice is zero, just return 0 and nil
		return 0,nil
	}

	if f.cursor == length{
		return 0, io.EOF
	}
	for i:=f.cursor;i<length;i++{
		f.cursor++
		if i >= length || i>= len(f.privSlice){
			break
		}
		if i == length{
			break
		}
		p[i]=f.privSlice[i]
	}
	f.timeStamp = time.Now()


	return length, nil


}

func (File) Seek(offset int64, whence int) (int64, error) {
	panic("implement me")
}

func (f *File) Write(p []byte) (n int, err error) {
	if f.isOpen == false{
		f.isOpen = true
	}
	f.exists=true
	f.isRef = true
	length := len(p)
	if length == 0{

		return 0, nil
	}
	num, err := f.byteBuf.Write(p)
	if(err!=nil){
		return num,WriteError()
	}
	f.timeStamp = time.Now()
	return num, err
}

func (f *File) String() string {
	return f.URI()
}

func (f *File) Exists() (bool, error) {
	if !f.exists {
		return false, DoesNotExist()
	}else{
		return true,nil
	}
}

func (f *File) Location() vfs.Location {
	return f.location
}

/*
CopyToLocation copies the current file to the given location.  If file exists
at given location contents are simply overwritten using "CopyToFile", otherwise
a newFile is made, takes the contents of the current file, and ends up at
the given location
 */
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {

	testPath := path.Join(path.Clean(location.Path()),f.Name())
	if systemMap[testPath]!=nil{	//if file w/name exists @ loc, simply copy contents over
		if tmp := systemMap[testPath]; tmp !=nil{
			cerr := f.CopyToFile(systemMap[testPath])
			if(cerr!=nil){
				return nil, cerr
			}
			return systemMap[testPath], nil
		}else{
			return nil,DoesNotExist()
		}
	}

	newFile,_:= location.NewFile(f.Name())
	_,_ = newFile.Write(make([]byte,0))
	cerr:=f.CopyToFile(newFile)
	return systemMap[testPath],cerr
}

func (f *File) CopyToFile(target vfs.File) error {

	if(target == nil){
		return CopyFailNil()
	}
	if ex,_:=target.Exists(); !ex {
		return DoesNotExist()
	}
	//if target exists, its contents will be overwritten, otherwise it will be created...i'm assuming it exists
	//targetFile := target.(*File)
	name := target.Name()
	loc :=target.Location()
	derr:=target.Delete()
	if(derr!=nil){
		return derr
	}
	newFile,_ := loc.NewFile(name)

	_, err := newFile.Write(f.privSlice)
	_ =newFile.Close()
	return err

}

func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {



	testPath := path.Join(location.Path(),f.Name())
	if systemMap[testPath]!=nil{
	err :=	f.CopyToFile(systemMap[path.Clean(location.Path())])
	if err!=nil{
		return nil, MoveToLocationError()
	}
	return f,nil
	}
	fileName := f.Name()
	newPath := path.Join(location.Path(),fileName)
	newFile,_ := location.NewFile(path.Base(newPath))
	_,_ = newFile.Write(make([]byte,0))
	cerr := f.CopyToFile(newFile)
	if(cerr!=nil){
		return nil,MoveToLocationError()
	}
	derr := f.Delete()
	if(derr !=nil){
		return nil, derr
	}


return systemMap[newPath],nil
}

/*MoveToFile creates a newFile, and moves it to "file".
If names are same, "file" is deleted and newFile takes its place.
The receiver is always deleted (since it's being "moved")
*/

func (f *File) MoveToFile(file vfs.File) error {

	if f.Name() == file.Name(){

		derr := file.Delete()
		if(derr!=nil){
			return DeleteError()
		}
		newFile,_:=file.Location().NewFile(f.Name())
		_,_ = newFile.Write(make([]byte,0))
		_=newFile.Close()
		copyErr:=f.CopyToFile(newFile)
		if(copyErr != nil){
			return CopyFail()
		}
		derr1 := f.Delete()
		return derr1
	}

	newFile,_ := file.Location().NewFile(f.Name())
	_,_ = newFile.Write(make([]byte,0))
	_ = newFile.Close()


	copyErr := f.CopyToFile(newFile)
	if copyErr != nil {
		return CopyFail()
	}


	derr:=f.Delete()

	return derr



}

func (f *File) Delete() error {
	existence, err := f.Exists()
	str := f.Filename
	index := f.getIndex()
	if(index == -1){
		return DoesNotExist()
	}
	if existence {
		//do some work to adjust the location (later)
		systemMap[f.Filename] = nil
		f.exists = false
		f.privSlice = nil
		f.byteBuf = nil
		f.timeStamp = time.Now()
		fileList[index] = nil
		copy(fileList[index:], fileList[index+1:])
		fileList[len(fileList)-1] = nil // or the zero value of T
		fileList = fileList[:len(fileList)-1]



	}
	if(systemMap[str] != nil ){
		return DeleteError()
	}

	if(f.getIndex()!=-1){
		return DeleteError()
	}
	return err


}

func newFile(name string) (*File, error){


	//var l Location
	//tmp, err := (*Location).NewFile(&l,name)
	//file := tmp.(*File)


	return &File{
		timeStamp: time.Now(), isRef: false, Filename: name, byteBuf: new(bytes.Buffer), cursor: 0,
		isOpen: false, isZB: false, exists: false,
	}, nil

}

func (f *File) LastModified() (*time.Time, error) {

	existence,err := f.Exists()

	if existence {
		return &f.timeStamp, err
	}
	return nil, err
}

func (f *File) Size() (uint64, error) {


	return uint64(len(f.privSlice)),nil


}

func (f *File) Path() string {
	if !path.IsAbs(f.Filename){
		return path.Join("/",f.Filename)
	}
	return f.Filename
}

func (f *File) Name() string {
	//if file exists

	return path.Base(f.Filename)
}

func (f *File) URI() string {  //works but test says it fails, probably other dependencies

	existence, _ := f.Exists()
	if !existence{
		return ""
	}
	var buf bytes.Buffer
	pref := "mem:///"
	buf.WriteString(pref)
	str := f.Filename
	buf.WriteString(str)
	//retStr := utils.AddTrailingSlash(buf.String())
	return buf.String()


}

func (f *File) getIndex() int{

	if systemMap[f.Filename] == nil{
		return -1
	}
	str := f.Path()
	for i,v:= range fileList{
		existence,_:=v.Exists()
		if v.Path() == str && existence{
			return i
		}
	}
	return -1
}