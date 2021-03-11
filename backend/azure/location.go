package azure

import (
	"github.com/c2fo/vfs/v5"
	"regexp"
)

type Location struct{}

func (l *Location) String() string {
	return ""
}

func (l *Location) List() ([]string, error) {
	return []string{}, nil
}

func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	return []string{}, nil
}

func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	return []string{}, nil
}

func (l *Location) Volume() string {
	return ""
}

func (l *Location) Path() string {
	return ""
}

func (l *Location) Exists() (bool, error) {
	return false, nil
}

func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {
	return &Location{}, nil
}

func (l *Location) ChangeDir(relLocPath string) error {
	return nil
}

func (l *Location) FileSystem() vfs.FileSystem {
	return &FileSystem{}
}

func (l *Location) NewFile(relFilePath string) (vfs.File, error) {
	return &File{}, nil
}

func (l *Location) DeleteFile(relFilePath string) error {
	return nil
}

func (l *Location) URI() string {
	return ""
}
