package mem

import (
	"path"
	"sync"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/utils"
)

// Scheme defines the FileSystem type's underlying implementation.
const Scheme = "mem"
const name = "In-Memory Filesystem"

type fsObject struct {
	isFile bool
	i      interface{}
}
type objMap map[string]*fsObject

// FileSystem implements vfs.FileSystem for an in-memory file system.
type FileSystem struct {
	sync.Mutex
	fsMap map[string]objMap
}

// Retry will return a retrier provided via options, or a no-op if none is provided.
func (fs *FileSystem) Retry() vfs.Retry {
	return vfs.DefaultRetryer()
}

// NewFile function returns the in-memory implementation of vfs.File.
// Since this is inside FileSystem, we assume that the caller knows that the CWD is the root.
// If a non-absolute path is given, an error is thrown. Additionally, a file does not
// technically exist until a call to "Touch()" is made on it. The "Touch" call links the
// file with FileSystem's map and brings it into existence.
// If a file is written to before a touch call, Write() will take care of that call.  This is
// true for other functions as well and existence only poses a problem in the context of deletion
// or copying FROM a non-existent file.
func (fs *FileSystem) NewFile(volume, absFilePath string) (vfs.File, error) {

	err := utils.ValidateAbsoluteFilePath(absFilePath)
	if err != nil {
		return nil, err
	}

	location, err := fs.NewLocation(volume, utils.EnsureTrailingSlash(path.Dir(absFilePath)))
	if err != nil {
		return nil, err
	}
	mapRef := location.(*Location).fileSystem.fsMap
	if _, ok := mapRef[volume]; ok {
		fileList := mapRef[volume].filesHere(location.Path())
		for _, file := range fileList {
			if file.name == path.Base(absFilePath) {
				fileCopy := deepCopy(file)
				file.location = location
				fileCopy.(*File).memFile = file
				return fileCopy, nil
			}
		}
	}
	// validateAbsFile path will throw an error if there was a trailing slash, hence not calling path.Clean()
	file := &File{
		name: path.Base(absFilePath),
	}

	memFile := newMemFile(file, location)
	file.memFile = memFile
	return file, nil
}

// NewLocation function returns the in-memory implementation of vfs.Location.
// A location always exists. If a file is created on a location that has not yet
// been made in the fsMap, then the location will be created with the file
func (fs *FileSystem) NewLocation(volume, absLocPath string) (vfs.Location, error) {

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

// NewFileSystem is used to initialize the file system struct for an in-memory FileSystem.
func NewFileSystem() *FileSystem {

	return &FileSystem{
		sync.Mutex{},
		make(map[string]objMap),
	}

}

func init() {
	// Even though the map is being made here, a call to
	backend.Register(Scheme, NewFileSystem())

}

// getKeys is used to get a list of absolute paths on a specified volume. These paths are a mixture of files and locations
func (o objMap) getKeys() []string {
	keyList := make([]string, 0)
	for i := range o {
		keyList = append(keyList, i)
	}
	return keyList
}

// fileHere returns a list of file pointers found at the absolute location path provided.
// If none are there, returns an empty slice
func (o objMap) filesHere(absLocPath string) []*memFile {

	paths := o.getKeys()
	fileList := make([]*memFile, 0)
	for i := range paths {

		object := o[paths[i]]                         // retrieve the object
		if ok := object != nil && object.isFile; ok { // if the object is a file, cast its interface, i, to a file and append to the slice
			file := object.i.(*memFile)
			if file.location.Path() == absLocPath {
				fileList = append(fileList, file)
			}
		}
	}
	return fileList
}

// fileNamesHere returns a list of base names of files found at the absolute location path provided.
// If none are there, returns an empty slice
func (o objMap) fileNamesHere(absLocPath string) []string {

	paths := o.getKeys()
	fileList := make([]string, 0)
	for i := range paths {

		object := o[paths[i]]               // retrieve the object
		if object != nil && object.isFile { // if the object is a file, cast its interface, i, to a file and append the name to the slice
			file := object.i.(*memFile)
			if utils.EnsureTrailingSlash(file.location.Path()) == absLocPath {
				fileList = append(fileList, file.name)
			}
		}
	}
	return fileList
}

func deepCopy(srcFile *memFile) vfs.File {
	destination := &File{
		name: srcFile.name,
	}

	destination.memFile = srcFile
	destination.exists = srcFile.exists
	destination.isOpen = srcFile.isOpen
	destination.contents = srcFile.contents
	return destination
}
