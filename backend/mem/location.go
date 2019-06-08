package mem

import (
	"bytes"
	"fmt"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/utils"
	"path"
	"regexp"
	"strings"
	"time"
)

//Location implements the vfs.Location interface specific to OS fs.
type Location struct {
	exists		bool
	firstTime	bool
	name       string
	fileSystem vfs.FileSystem
}



func (Location) String() string {
	//panic("implement me")
	return""
}


func (Location) List() ([]string, error) {
	//panic("implement me")
	return nil,nil
}

func (l *Location) ListByPrefix(prefix string) ([]string, error) {

	list := make([]string,1)
	 str := path.Join(path.Dir(l.Path()),prefix)
	 //fmt.Println(l.Path())
	 for _, v:=range fileList{
	 	if v!=nil{
			fmt.Println(v.Location().Path(),str)
	 		if strings.Contains(v.Location().Path(),str){


	 			list = append(list,v.Name())

			}
		}
	 }

/*
	fmt.Println(l.Path())
	fmt.Println(l.name)
	list := make([]string,1)
	str := path.Dir(l.Path())
	fmt.Println(str)
	for i, v:= range fileList{
		if v != nil{
			fmt.Println(v.Location().Path(),i)
			if strings.Contains(path.Dir(v.Path()),str){
				if path.Ext(v.Path()) != " " {
					fmt.Println(v.Path())
					list = append(list, v.Name())
				}
			}
		}


	}
	return list, nil

 */
return list,nil
}

func (Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	return nil,nil
}

func (Location) Volume() string {
	return ""
}

func (l *Location) Path() string {

	if path.IsAbs(l.name){
		return utils.AddTrailingSlash(l.name)
	}
	return l.name
}

func (l *Location) Exists() (bool, error) {
	if l.exists {
		return true,nil
	}
	return false, DoesNotExist()
}

func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {

	str := path.Join(path.Dir(path.Clean(l.Path())),relativePath)
	return &Location{
		fileSystem: l.fileSystem,
		name:       str,
		exists: 	true,
	}, nil



}

func (Location) ChangeDir(relativePath string) error {
	panic("implement me")
}

func (Location) FileSystem() vfs.FileSystem {
	panic("implement me")
}

func (l *Location) NewFile(fileName string) (vfs.File, error) {

	pref := path.Dir(l.Path())
	var buf bytes.Buffer
	buf.WriteString(pref)
	str:=fileName
	buf.WriteString(str)
	nameStr := buf.String()
	//l.name = nameStr
	file := File{timeStamp: time.Now(), isRef: false, Filename: nameStr, byteBuf: new(bytes.Buffer), cursor: 0,
		isOpen: false, isZB: false, exists: true,}

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
	return buf.String()

}


