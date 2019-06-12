package mem

import (
	"bytes"
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
	Filename	string
	name       string
	fileSystem vfs.FileSystem
}



func (l *Location) String() string {


	return l.URI()
}


func (Location) List() ([]string, error) {
	//panic("implement me")
	return nil,nil
}

func (l *Location) ListByPrefix(prefix string) ([]string, error) {

	list := make([]string,1)
	 str := path.Join(l.Path(),prefix)
	 for _, v:=range fileList{
	 	if v!=nil{
	 		path:=v.Path()
	 		tmp:=strings.Contains(path,str)
	 		if tmp{
	 			list = append(list,v.Name())

			}
		}
	 }
return list,nil
}

func (Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	return nil,nil
}

func (Location) Volume() string {
	return ""
}

func (l *Location) Path() string {

	return l.name

}

func (l *Location) Exists() (bool, error) {

	fullPath:= path.Join(l.name,l.Filename)
	if systemMap[fullPath] != nil{
		if systemMap[fullPath].exists{
			l.exists = true
		}
	}

	if l.exists {
		return true,nil
	}
	return false, DoesNotExist()
}

func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {

	str := path.Join(l.Path(),relativePath)
	str = utils.AddTrailingSlash(path.Clean(str))
	return &Location{
		fileSystem: l.fileSystem,
		name:       str,
		exists: 	false,
	}, nil



}

func (l *Location) ChangeDir(relativePath string) error {
	l.name = path.Join(l.name,relativePath)
	return nil

}

func (l *Location) FileSystem() vfs.FileSystem {

	if systemMap[l.name] != nil{
		if systemMap[l.name].exists{
			l.exists = true
		}
	}
	existence, _:= l.Exists()
	if existence{
		return l.fileSystem
	}
	return nil

}

func (l *Location) NewFile(fileName string) (vfs.File, error) {

	pref := l.Path()
	//var buf bytes.Buffer
	//buf.WriteString(pref)
	str:=fileName
	//buf.WriteString(str)
	//nameStr := buf.String()
	var nameStr string
	if pref == "./"{
		nameStr=path.Join("/",fileName)
	}else{
	nameStr=path.Join(pref,str)
	}

	//l.name = nameStr
	loc,_:=l.fileSystem.NewLocation("",nameStr)
	file := &File{timeStamp: time.Now(), isRef: false, Filename: path.Base(nameStr), byteBuf: new(bytes.Buffer), cursor: 0,
		isOpen: false, isZB: false, exists: false,location:loc}
	systemMap[nameStr]=file
	fileList = append(fileList,file)

	return file, nil


}

func (l *Location) DeleteFile(fileName string) error {


	fullPath := path.Join(l.Path(),fileName)
	if systemMap[fullPath] != nil {
		derr := systemMap[fullPath].Delete()
		return derr
	}
	return DoesNotExist()
}

func (l *Location) URI() string {

	existence, _ := l.Exists()
	if !existence{
		return ""
	}
	var buf bytes.Buffer
	pref := "mem://"
	buf.WriteString(pref)
	str := l.Path()
	buf.WriteString(str)
	retStr := utils.AddTrailingSlash(buf.String())
	return retStr

}


