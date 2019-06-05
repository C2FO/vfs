package mem

import (
	"bytes"
	"errors"
	"github.com/c2fo/vfs/v4"
	"io"
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
	location vfs.Location
}

func DoesNotExist() error {
	return errors.New("This file does not exist!")
}
func CopyFail() error {
	return errors.New("This file was not successfully copied")
}

func DeleteError() error {
	return errors.New("Deletion was unsuccessful")
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
	f.isRef = true
	length := len(p)
	if length == 0{
		if f.byteBuf.Cap() == 0{
			f.isZB = true
		}
		return 0, nil
	}
	num, err := f.byteBuf.Write(p)
	f.timeStamp = time.Now()
	return num, err
}

func (File) String() string {
	panic("implement me")
}

func (f *File) Exists() (bool, error) {
	if !f.exists {
		return false, DoesNotExist()
	}else{
		return true,nil
	}
}

func (File) Location() vfs.Location {
	panic("implement me")
}

func (File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	panic("implement me")
}

func (f *File) CopyToFile(target vfs.File) error {
	//if target exists, its contents will be overwritten, otherwise it will be created...i'm assuming it exists
	_, err := target.Write(f.privSlice)
	_ =target.Close()
	return err

}

func (File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	panic("implement me")
}

func (File) MoveToFile(vfs.File) error {
	panic("implement me")
}

func (f *File) Delete() error {
	existence, err := f.Exists()
	if existence {
		//do some work to adjust the location (later)
		f.exists = false
		f.privSlice = nil
		f.byteBuf = nil
		f.timeStamp = time.Now()
	}
	return err


}

func newFile(name string) (*File, error){

	file := File{ timeStamp: time.Now(), isRef: false, Filename: name, byteBuf: new(bytes.Buffer), cursor: 0, isOpen: false, isZB: false, exists: true}
	return &file, nil

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

func (File) Path() string {
	panic("implement me")
}

func (f *File) Name() string {
	//if file exists
	return f.Filename
}

func (File) URI() string {
	panic("implement me")
}


