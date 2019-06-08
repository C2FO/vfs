package mem

import (
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
)

//Scheme defines the filesystem type.
const Scheme = "file"
const name = "mem"
var systemMap map[string]*File
var fileList []*File




// FileSystem implements vfs.Filesystem for the mem filesystem.
type FileSystem struct{}

// FileSystem will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the mem implementation of vfs.File.  NOT DONE
func (fs *FileSystem) NewFile(volume string, name string) (vfs.File, error) {
	file, _ := newFile(name)
	tmp,err :=fs.NewLocation(volume,name)
	file.location = tmp
	systemMap[name] = file
	fileList = append(fileList,file)
	return file,err
}

// NewLocation function returns the mem implementation of vfs.Location. NOT DONE
func (fs *FileSystem) NewLocation(volume string, name string) (vfs.Location, error) {

	return &Location{
		fileSystem: fs,
		name:       name,
		exists: 	true,
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

func init() {
	backend.Register(Scheme, &FileSystem{})
	systemMap = make(map[string]*File)
	fileList = make([]*File, 0)
}
