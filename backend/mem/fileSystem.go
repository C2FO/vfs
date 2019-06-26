package mem

import (
	"path"
	"time"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/backend"
	"github.com/c2fo/vfs/v5/utils"
)

//Scheme defines the FileSystem type's underlying implementation.
const Scheme = "mem"
const name = "In-Memory Filesystem"

type obj struct {
	isFile bool
	i      interface{}
}
type objMap map[string]*obj

// FileSystem implements vfs.FileSystem for an in-memory file system.
type FileSystem struct {
	fsMap map[string]objMap
}

// Retry will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

/*
NewFile function returns the in-memory implementation of vfs.File.
Since this is inside FileSystem, we assume that the caller knows that the CWD is the root.
If a non-absolute path is given, an error is thrown. Additionally, a file does not
technically exist until a call to "Touch()" is made on it. The "Touch" call links the
file with FileSystem's map and brings it into existence.
If a file is written to before a touch call, Write() will take care of that call.  This is
true for other functions as well and existence only poses a problem in the context of deletion
or copying FROM a non-existent file.
*/
func (fs *FileSystem) NewFile(volume string, absFilePath string) (vfs.File, error) {

	err := utils.ValidateAbsoluteFilePath(absFilePath)
	if err != nil {
		return nil, err
	}

	file, err := newFile(path.Base(absFilePath)) //validateAbsFile path will throw an error if there was a trailing slash, hence not calling path.Clean()
	if err != nil {
		return nil, err
	}
	tmp, err := fs.NewLocation(volume, utils.EnsureTrailingSlash(path.Dir(absFilePath)))
	if err != nil {
		return nil, err
	}

	file.location = tmp
	return file, nil
}

/*
NewLocation function returns the in-memory implementation of vfs.Location.
A location always exists. If a file is created on a location that has not yet
been made in the fsMap, then the location will be created with the file
*/
func (fs *FileSystem) NewLocation(volume string, absLocPath string) (vfs.Location, error) {

	err := utils.ValidateAbsoluteLocationPath(absLocPath)
	if err != nil {
		return nil, err
	}
	str := utils.EnsureTrailingSlash(path.Clean(absLocPath))
	return &Location{
		fileSystem: fs,
		name:       str,
		exists:     false,
		volume:     volume,
	}, nil

}

// Name returns the name of the underlying FileSystem
func (fs *FileSystem) Name() string {
	return name
}

// Scheme returns the scheme of the underlying FileSystem
func (fs *FileSystem) Scheme() string {
	return Scheme
}

/*
Initialize is used to initialize the fsMap for an in-memory FileSystem.
DISCLAIMER: nothing will work until this call is made
*/
func (fs *FileSystem) Initialize() {

	fs.fsMap = make(map[string]objMap)

}

func init() {
	backend.Register(Scheme, &FileSystem{make(map[string]objMap)}) //even though the map is being made here, a call
	// to Initialize needs to be made to properly use FileSystem

}

//getKeys is used to get a list of absolute paths on a specified volume. These paths are a mixture of files and locations
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

//fileNamesHere returns a list of base names of files found at the absolute location path provided.  If none are there, returns an empty slice
func (o objMap) fileNamesHere(absLocPath string) []string {

	paths := o.getKeys()
	fileList := make([]string, 0)
	for i := range paths {

		object := o[paths[i]]               //retrieve the object
		if object != nil && object.isFile { //if the object is a file, cast its interface, i, to a file and append the name to the slice
			file := object.i.(*File)
			if utils.EnsureTrailingSlash(path.Dir(file.Path())) == absLocPath {
				fileList = append(fileList, file.Name())
			}
		}
	}
	return fileList
}

//Touch takes a in-memory vfs.File, makes it existent, and updates the lastModified
func (f *File) Touch() {
	if f == nil {
		return
	}

	f.exists = true
	f.lastModified = time.Now()
	//files and locations are contained in objects of type "obj".
	// An obj has a blank interface and a boolean that indicates whether or not it is a file
	var locObject obj
	var fileObject obj
	fileObject.i = f
	fileObject.isFile = true
	loc := f.Location().(*Location)
	volume := loc.Volume()
	locObject.i = f.Location()
	locObject.isFile = false

	mapRef := &loc.fileSystem.fsMap      //just a less clunky way of accessing the fsMap
	if _, ok := (*mapRef)[volume]; !ok { //if the objMap map does not exist for the volume yet, then we go ahead and create it.
		(*mapRef)[volume] = make(objMap)
	}

	(*mapRef)[volume][f.Path()] = &fileObject //setting the map at Volume volume and path of f to this fileObject
	locationPath := utils.EnsureTrailingSlash(path.Clean(path.Dir(f.Path())))
	if _, ok := (*mapRef)[volume][locationPath]; !ok { //checking for that locations existence to avoid redundancy
		(*mapRef)[volume][locationPath] = &locObject
	}
}
