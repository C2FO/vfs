package mem

import (
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	"github.com/c2fo/vfs/v4/utils"
	"path"
)

//Scheme defines the filesystem type.
const Scheme = "mem"
const name = "In-Memory Filesystem"



// FileSystem implements vfs.Filesystem for the mem filesystem.
type FileSystem struct{
	systemMap	map[string]*File

	fileList 	[]*File
}

// FileSystem will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the mem implementation of vfs.File.  NOT DONE
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {

	if !path.IsAbs(name) {
		name = path.Join("/", name)
	}

	file, _ := newFile(name)
	file.fileSystem=fs
	tmp, err := fs.NewLocation(volume, name)
	file.location = tmp
	if fs.systemMap[name] != nil && fs.systemMap[name].getIndex() != -1 {
		_ = fs.systemMap[name].Delete()

	}
	fs.systemMap[name] = file
	fs.fileList = append(fs.fileList, file)
	return file, err
}

// NewLocation function returns the mem implementation of vfs.Location. NOT DONE
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {
	if path.Ext(name) != "" {
		str := path.Dir(path.Clean(name))
		return &Location{
			fileSystem: fs,
			name:       utils.AddTrailingSlash(str),
			exists:     false,
			Filename:   path.Base(name),
		}, nil

	}
	return &Location{
		fileSystem: fs,
		name:       utils.AddTrailingSlash(path.Clean(name)),
		exists:     false,
	}, nil

}

// Name returns "mem"
func (fs *FileSystem) Name() string {
	return name
}

// Scheme return "file" as the initial part of a file URI ie: file://
func (fs *FileSystem) Scheme() string {
	return Scheme
}
func (fs *FileSystem) Initialize(){
	fs.systemMap = make(map[string]*File)
	fs.fileList = make([]*File, 0)
}

func init() {
	backend.Register(Scheme, &FileSystem{})

}
