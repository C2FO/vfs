package mem

import (
	"bytes"
	"errors"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/utils"
	"path"
	"regexp"
	"strings"
	"time"
)

//Location implements the vfs.Location interface specific to in-memory fs.
type Location struct {
	exists     bool
	firstTime  bool
	name       string //the path that this location exists on
	fileSystem *FileSystem
	volume     string
}

//String implements io.Stringer by returning the location's URI as a string
func (l *Location) String() string {

	return l.URI()
}

/*
List finds all of the files living at the current location and returns them in a slice of strings.
If there are no files at location, then an empty slice will be returned
*/

func (l *Location) List() ([]string, error) {

	str := l.Path()                         //full path of this location
	mapRef := &l.fileSystem.fsMap           //setting mapRef to this value for code readability
	if _, ok := (*mapRef)[l.Volume()]; ok { //are there paths on this volume?
		list := (*mapRef)[l.Volume()].fileNamesHere(str) //getting a list of the file names on this location
		return list, nil                                 // "fileNamesHere" returns an empty list if no files were found
	}
	return make([]string, 0), nil //if the volume has nothing on it, return an empty list as well
}

/*
ListByPrefix tags a prefix onto the current path and in a slice,
 returns all file base names whose full paths contain that substring
Returns empty slice if nothing found
*/

func (l *Location) ListByPrefix(prefix string) ([]string, error) {

	list := make([]string, 0)
	str := path.Join(l.Path(), prefix)
	mapRef := &l.fileSystem.fsMap
	if _, ok := (*mapRef)[l.volume]; ok {
		paths := (*mapRef)[l.volume].getKeys()
		for i := range paths {
			if strings.Contains(paths[i], str) {
				list = append(list, path.Base(paths[i]))
			}
		}
	}

	return list, nil
}

/*
ListByRegex takes a regular expression and returns
a slice of strings containing the base names of files
found that matched the regular expression.  Returns an
empty slice upon nothing found
*/

func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {

	list := make([]string, 0)
	str := l.Path()
	mapRef := &l.fileSystem.fsMap
	if _, ok := (*mapRef)[l.Volume()]; ok {
		namesHere := (*mapRef)[l.Volume()].fileNamesHere(str)
		for i := range namesHere {
			if regex.MatchString(namesHere[i]) {
				list = append(list, namesHere[i])
			}
		}
	}
	return list, nil
}

//Volume returns the volume of the current fs. In-Memory-Fs has no volume
func (l *Location) Volume() string {
	return l.volume
}

//Path returns the full, absolute path of the location with leading and trailing slashes
func (l *Location) Path() string {

	return utils.AddTrailingSlash(path.Clean(l.name))

}

//Exists reports whether or not a location exists. Creating a location does not guarantee its existence
func (l *Location) Exists() (bool, error) {

	data, _ := l.List()
	if len(data) == 0 {
		return false, nil
	}
	mapRef := &l.fileSystem.fsMap
	if _, ok := (*mapRef)[l.volume]; ok {
		fileList := (*mapRef)[l.volume].filesHere(l.Path())
		for i := range fileList {
			if fileList[i].exists {
				return true, nil
			}
		}
	}
	l.exists = false
	return false, nil

}

/*
NewLocation creates a new location at the
given relative path, which is tagged onto the current locations absolute path
*/
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {

	str := path.Join(l.Path(), relLocPath)
	str = utils.AddTrailingSlash(path.Clean(str))
	return &Location{
		fileSystem: l.fileSystem,
		name:       str,
		exists:     false,
		volume:     l.Volume(),
	}, nil

}

//ChangeDir simply changes the directory of the location
func (l *Location) ChangeDir(relLocPath string) error {
	l.name = path.Join(l.name, relLocPath)
	return nil

}

//FileSystem returns the type of filesystem location exists on, if it exists at all
func (l *Location) FileSystem() vfs.FileSystem {

	existence, _ := l.Exists()
	if existence {

		return l.fileSystem
	}
	return nil

}

//NewFile creates a vfs file given its relative path and tags it onto "l's" path
func (l *Location) NewFile(relFilePath string) (vfs.File, error) {

	if path.IsAbs(relFilePath){
		return nil, errors.New("Expected relative path, got an absolute")
	}
	pref := l.Path()
	str := relFilePath
	var nameStr string

	nameStr = path.Join(pref, str)

	loc, lerr := l.fileSystem.NewLocation("", path.Dir(nameStr))
	if lerr != nil {
		return nil, lerr
	}

	file := &File{timeStamp: time.Now(), isRef: false, name: path.Base(nameStr), cursor: 0,
		isOpen: false, exists: false, location: loc, fileSystem: l.fileSystem}
	l.fileSystem.fsMap[l.volume][nameStr] = &obj{true, file}
	l.fileSystem.fsMap[l.volume][path.Dir(nameStr)] = &obj{false, loc}

	return file, nil

}

//DeleteFile locates the file given the fileName and calls delete on it
func (l *Location) DeleteFile(relFilePath string) error {
	if path.IsAbs(relFilePath){
		return errors.New("Expected relative path, got an absolute")
	}
	vol := l.Volume()
	fullPath := path.Join(l.Path(), relFilePath)

	if _, ok := l.fileSystem.fsMap[vol]; ok {
		if _, ok2 := l.fileSystem.fsMap[vol][fullPath]; ok2 {
			file := l.fileSystem.fsMap[vol][fullPath].i.(*File)
			derr := file.Delete()
			return derr
		}
	}
	return errors.New("This file does not exist")
}

//URI returns the URI of the location if the location exists
func (l *Location) URI() string {

	var buf bytes.Buffer
	pref := "mem://"
	buf.WriteString(pref)
	buf.WriteString(l.Volume())
	str := l.Path()
	buf.WriteString(str)
	retStr := utils.AddTrailingSlash(buf.String())
	return retStr
}
