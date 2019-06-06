package mem

import (
	"bytes"
	"fmt"
	"github.com/c2fo/vfs/v4"
	"regexp"
	"time"
)

//Location implements the vfs.Location interface specific to OS fs.
type Location struct {
	exists		bool
	firstTime	bool
	name       string
	fileSystem vfs.FileSystem
}


func getDirFromPath(path string) string{


	length := len(path)
	if(length == 0){
		return path
	}
	fmt.Println(length)
	index := 0
	for i:=length-1; i>=0 ; i--{

		if string(path[i]) == "/"{
			index = i
			break
		}
	}
	fmt.Println(index)
	diff:=length - (length-index)
	newStr := make([]uint8, diff +1 )
	newStr[index] = path[index]			//adding the trailing slash before constructing the string that prefaces it
	for i:=0;i<diff;i++{
		newStr[i] = path[i]
	}
	return string(newStr)
}

func (Location) String() string {
	panic("implement me")
}


func (Location) List() ([]string, error) {
	panic("implement me")
}

func (Location) ListByPrefix(prefix string) ([]string, error) {
	panic("implement me")
}

func (Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	panic("implement me")
}

func (Location) Volume() string {
	return ""
}

func (l *Location) Path() string {

	return l.name
}

func (Location) Exists() (bool, error) {
	panic("implement me")
}

func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	panic("implement me")

}

func (Location) ChangeDir(relativePath string) error {
	panic("implement me")
}

func (Location) FileSystem() vfs.FileSystem {
	panic("implement me")
}

func (l *Location) NewFile(fileName string) (vfs.File, error) {

	l.name = fileName
	file := File{timeStamp: time.Now(), isRef: false, Filename: fileName, byteBuf: new(bytes.Buffer), cursor: 0,
		isOpen: false, isZB: false, exists: true, location: l}

	return &file, nil




}

func (Location) DeleteFile(fileName string) error {
	panic("implement me")
}

func (l *Location) URI() string {

	//existence, _ := f.Exists()
	//if !existence{
	//	return ""
	//}
	var buf bytes.Buffer
	pref := "file://"
	buf.WriteString(pref)
	str := l.name
	buf.WriteString(str)
	fmt.Println(getDirFromPath("some/path/foo.txt"))
	return buf.String()

}


