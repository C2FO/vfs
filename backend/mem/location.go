package mem

import (
	"github.com/c2fo/vfs/v4"
	"regexp"
)

//Location implements the vfs.Location interface specific to OS fs.
type Location struct {
	name       string
	fileSystem vfs.FileSystem
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
	panic("implement me")
}

func (l *Location) Path() string {

	return l.name
}

func (Location) Exists() (bool, error) {
	panic("implement me")
}

func (Location) NewLocation(relativePath string) (vfs.Location, error) {
	panic("implement me")
}

func (Location) ChangeDir(relativePath string) error {
	panic("implement me")
}

func (Location) FileSystem() vfs.FileSystem {
	panic("implement me")
}

func (Location) NewFile(fileName string) (vfs.File, error) {
	panic("implement me")
}

func (Location) DeleteFile(fileName string) error {
	panic("implement me")
}

func (Location) URI() string {
	panic("implement me")
}


