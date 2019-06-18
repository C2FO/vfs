package mem

import (
	"errors"
	"github.com/c2fo/vfs/v4"
	"github.com/c2fo/vfs/v4/backend"
	"github.com/c2fo/vfs/v4/utils"
	"path"
)

//Scheme defines the filesystem type.
const Scheme = "mem"
const name = "In-Memory Filesystem"

// FileSystem implements vfs.Filesystem for the mem filesystem.
type obj struct {
	isFile bool
	i      interface{}
}
type objMap map[string]*obj
type FileSystem struct {
	fsMap    map[string]objMap
}

// FileSystem will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

/*
NewFile function returns the mem implementation of vfs.File.
Since this is inside "fileSystem" we assume that the path given for name is absolute.
If a non-absolute path is given, a leading slashed is tagged onto the path to make it so.
Additionally, a file does not technically exist until it is written to (common to write zero bytes
as a way of "initializing" the file). Although it may exist on the fsMap, its member "exists" won't
become true until something is written
*/
func (fs *FileSystem) NewFile(volume string, absFilePath string) (vfs.File, error) {

	if !path.IsAbs(absFilePath) {

		return nil, errors.New("Creation failed, provide an absolute path for file creation in the FS")
	}

	file, nerr := newFile(path.Base(path.Clean(absFilePath)))
	if nerr != nil {
		return nil, nerr
	}
	file.fileSystem = fs
	tmp, err := fs.NewLocation(volume, path.Dir(path.Clean(absFilePath)))
	if err != nil {
		return nil, err
	}
	var locObject obj
	var fileObject obj
	fileObject.i = file
	fileObject.isFile = true
	locObject.i = tmp
	locObject.isFile = false
	file.location = tmp
	if _, ok := fs.fsMap[volume]; !ok { //if the objMap map does not exist for the volume yet, then we go ahead and create it.
		fs.fsMap[volume] = make(objMap)
	}
	fs.fsMap[volume][absFilePath] = &fileObject
	locationPath:=utils.AddTrailingSlash(path.Clean(path.Dir(absFilePath)))
	if _,ok := fs.fsMap[volume][locationPath];!ok{
		fs.fsMap[volume][locationPath] = &locObject
	}

	return file, nil
}

/*
NewLocation function returns the mem implementation of vfs.Location.
A location does not exist unless at least one file lives on it. If a file
is created on a non-existent location, then it will be created
*/
func (fs *FileSystem) NewLocation(volume string, absLocPath string) (vfs.Location, error) {

	if !path.IsAbs(absLocPath) {
		return nil, errors.New("Location creation failed, provide an absolute path for location creation in the FS")

	}
	str := utils.AddTrailingSlash(path.Clean(absLocPath))
		return &Location{
			fileSystem: fs,
			name:       str,
			exists:     false,
			volume:     volume,
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
func (fs *FileSystem) Initialize() {

	fs.fsMap = make(map[string]objMap)

}

func init() {
	backend.Register(Scheme, &FileSystem{})

}



func (o objMap) getKeys() []string {
	keyList := make([]string, 0)
	for i := range o {
		keyList = append(keyList, i)
	}
	return keyList
}

//fileHere returns a list of file pointers found at the absolute location path provided.  If none are there, returns an empty slice
func (o objMap) filesHere(absLocPath string) []*File {

	paths := o.getKeys()
	fileList := make([]*File, 0)
	for i := range paths {

		object := o[paths[i]] //retrieve the object
		if object.isFile {    //if the object is a file, cast its interface, i, to a file and append to the slice
			file := object.i.(*File)
			if file.Location().Path() == absLocPath {
				fileList = append(fileList, file)
			}
		}
	}
	return fileList
}

//fileHere returns a list of file pointers found at the absolute location path provided.  If none are there, returns an empty slice
func (o objMap) fileNamesHere(absLocPath string) []string {

	paths := o.getKeys()
	fileList := make([]string, 0)
	for i := range paths {

		object := o[paths[i]] //retrieve the object
		if object.isFile {    //if the object is a file, cast its interface, i, to a file and append to the slice
			file := object.i.(*File)
			if utils.AddTrailingSlash(path.Dir(file.Path())) == absLocPath {
				fileList = append(fileList, file.Name())
			}
		}
	}
	return fileList
}
