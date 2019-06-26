package mem

import (
	"errors"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/utils"
)

//Location implements the vfs.Location interface specific to in-memory FileSystem.
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

		return list, nil // "fileNamesHere" returns an empty list if no files were found
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
				if path.Ext(paths[i]) != "" && strings.Contains(str, utils.EnsureTrailingSlash(path.Dir(paths[i]))) {
					list = append(list, path.Base(paths[i]))
					sort.Strings(list)
				}
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

//Volume returns the volume of the current FileSystem.
func (l *Location) Volume() string {
	return l.volume
}

//Path returns the full, absolute path of the location with leading and trailing slashes
func (l *Location) Path() string {

	//just to be sure that we return a trailing and leading slash
	str := utils.EnsureTrailingSlash(path.Clean(l.name))
	return utils.EnsureLeadingSlash(str)

}

//Exists always returns true on locations
func (l *Location) Exists() (bool, error) {

	l.exists = true
	return true, nil
}

/*
NewLocation creates a new location at the
given relative path, which is tagged onto the current locations absolute path
*/
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {
	err := utils.ValidateRelativeLocationPath(relLocPath)
	if err != nil {
		return nil, err
	}
	str := path.Join(l.Path(), relLocPath)
	str = utils.EnsureTrailingSlash(path.Clean(str))
	return &Location{
		fileSystem: l.fileSystem,
		name:       str,
		exists:     false,
		volume:     l.Volume(),
	}, nil

}

//ChangeDir simply changes the directory of the location
func (l *Location) ChangeDir(relLocPath string) error {
	err := utils.ValidateRelativeLocationPath(relLocPath)
	if err != nil {
		return err
	}
	l.name = path.Join(l.name, relLocPath)
	return nil

}

//FileSystem returns the type of file system location exists on, if it exists at all
func (l *Location) FileSystem() vfs.FileSystem {

	return l.fileSystem

}

//NewFile creates a vfs.File given its relative path and tags it onto "l's" path
func (l *Location) NewFile(relFilePath string) (vfs.File, error) {

	err := utils.ValidateRelativeFilePath(relFilePath)
	if err != nil {
		return nil, err
	}
	pref := l.Path()
	str := relFilePath
	var nameStr string

	nameStr = path.Join(pref, str)

	loc, err := l.fileSystem.NewLocation(l.Volume(), utils.EnsureTrailingSlash(path.Dir(nameStr)))
	if err != nil {
		return nil, err
	}

	file := &File{lastModified: time.Now(), name: path.Base(nameStr), cursor: 0,
		isOpen: false, exists: false, location: loc}
	return file, nil

}

//DeleteFile locates the file given the fileName and calls delete on it
func (l *Location) DeleteFile(relFilePath string) error {
	err := utils.ValidateRelativeFilePath(relFilePath)
	if err != nil {
		return err
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

	return utils.GetLocationURI(l)
}
