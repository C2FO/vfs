package azure

import (
	"github.com/c2fo/vfs/v5"
	"time"
)

type File struct{}

func (f *File) Close() error {
	return nil
}

func (f *File) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (f *File) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (f *File) String() string {
	return ""
}

func (f *File) Exists() (bool, error) {
	return false, nil
}

func (f *File) Location() vfs.Location {
	return &Location{}
}

func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	return &File{}, nil
}

func (f *File) CopyToFile(file vfs.File) error {
	return nil
}

func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	return &File{}, nil
}

func (f *File) MoveToFile(file vfs.File) error {
	return nil
}

func (f *File) Delete() error {
	return nil
}

func (f *File) LastModified() (*time.Time, error) {
	return &time.Time{}, nil
}

func (f *File) Size() (uint64, error) {
	return 0, nil
}

func (f *File) Path() string {
	return ""
}

func (f *File) Name() string {
	return ""
}

func (f *File) Touch() error {
	return nil
}

func (f *File) URI() string {
	return ""
}
