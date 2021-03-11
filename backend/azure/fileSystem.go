package azure

import "github.com/c2fo/vfs/v5"

const Scheme = "https"
const Name = "azure"

type FileSystem struct{}

func (fs *FileSystem) NewFile(volume string, absFilePath string) (vfs.File, error) {
	return &File{}, nil
}

func (fs *FileSystem) NewLocation(volume string, absLocPath string) (vfs.Location, error) {
	return &Location{}, nil
}

func (fs *FileSystem) Name() string {
	return Name
}

func (fs *FileSystem) Scheme() string {
	return Scheme
}

func (fs *FileSystem) Retry() vfs.Retry {
	return func(wrapped func() error) error { return nil }
}
