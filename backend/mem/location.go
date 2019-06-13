package mem

import (
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
	Filename   string //the baseName of the file this location belongs to
	name       string //the path that this location exists on
	fileSystem vfs.FileSystem
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
	//panic("implement me")

	list := make([]string, 0)
	str := l.Path()
	for _, v := range fileList {
		if v != nil {
			fullPath := v.Path()
			if utils.AddTrailingSlash(path.Dir(fullPath)) == str {
				if systemMap[fullPath] != nil {
					existence, _ := systemMap[fullPath].Exists()
					if existence {
						list = append(list, v.Name())
					}
				}
			}
		}
	}
	return list, nil
}

/*
ListByPrefix tags a prefix onto the current path and in a slice,
 returns all file base names whose full paths contain that substring
Returns empty slice if nothing found
*/
func (l *Location) ListByPrefix(prefix string) ([]string, error) {

	list := make([]string, 0)
	str := path.Join(l.Path(), prefix)
	for _, v := range fileList {
		if v != nil {
			path := v.Path()
			tmp := strings.Contains(path, str)
			if tmp {
				list = append(list, v.Name())

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
	filesHere, _ := l.List()
	for _, hereList := range filesHere {

		potentialPath := path.Join(str, hereList)

		for _, systemFileList := range fileList {

			if systemFileList != nil && systemFileList.Path() == potentialPath {
				ex, _ := systemFileList.Exists()
				if regex.MatchString(path.Base(potentialPath)) && ex {
					list = append(list, systemFileList.Name())

				}

			}
		}

	}

	return list, nil
}

//Volume returns the volume of the current fs. In-Memory-Fs has no volume
func (l *Location) Volume() string {
	return ""
}

//Path returns the full, absolute path of the location with leading and trailing slashes
func (l *Location) Path() string {

	return l.name

}

//Exists reports whether or not a location exists. Creating a location does not guarantee its existence
func (l *Location) Exists() (bool, error) {

	data, _ := l.List()
	if len(data) == 0 {
		return false, nil
	}
	return true, nil

}

/*
NewLocation creates a new location at the
given relative path, which is tagged onto the current locations absolute path
*/
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {

	str := path.Join(l.Path(), relativePath)
	str = utils.AddTrailingSlash(path.Clean(str))
	return &Location{
		fileSystem: l.fileSystem,
		name:       str,
		exists:     false,
	}, nil

}

//ChangeDir simply changes the directory of the location
func (l *Location) ChangeDir(relativePath string) error {
	l.name = path.Join(l.name, relativePath)
	return nil

}

//FileSystem returns the type of filesystem location exists on, if it exists at all
func (l *Location) FileSystem() vfs.FileSystem {

	filePath := path.Join(path.Clean(l.Path()), l.Filename)
	if systemMap[filePath] != nil {
		if systemMap[filePath].exists {
			l.exists = true
		}
	}
	existence, _ := l.Exists()
	if existence {
		return l.fileSystem
	}
	return nil

}

//NewFile creates a vfs file given its relative path and tags it onto "l's" path
func (l *Location) NewFile(fileName string) (vfs.File, error) {

	pref := l.Path()
	str := fileName
	var nameStr string
	if pref == "./" {
		nameStr = path.Join("/", fileName)
	} else {
		nameStr = path.Join(pref, str)
	}

	loc, _ := l.fileSystem.NewLocation("", nameStr)
	file := &File{timeStamp: time.Now(), isRef: false, Filename: path.Base(nameStr), cursor: 0,
		isOpen: false, exists: false, location: loc}
	systemMap[nameStr] = file
	fileList = append(fileList, file)

	return file, nil

}

//DeleteFile locates the file given the fileName and calls delete on it
func (l *Location) DeleteFile(fileName string) error {

	fullPath := path.Join(l.Path(), fileName)
	if systemMap[fullPath] != nil {
		derr := systemMap[fullPath].Delete()
		return derr
	}
	return errors.New("This file does not exist")
}

//URI returns the URI of the location if the location exists
func (l *Location) URI() string {

	existence, _ := l.Exists()
	if !existence {
		return ""
	}
	return utils.GetLocationURI(l)
}
