package mem

import (
	"path"
	"sync"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

// Scheme defines the FileSystem type's underlying implementation.
const Scheme = "mem"
const name = "In-Memory FileSystem"

type fsObject struct {
	isFile bool
	i      interface{}
}
type objMap map[string]*fsObject

// FileSystem implements vfs.FileSystem for an in-memory file system.
type FileSystem struct {
	mu    sync.Mutex
	fsMap map[string]objMap
}

// NewFileSystem is used to initialize the file system struct for an in-memory FileSystem.
func NewFileSystem(opts ...options.NewFileSystemOption[FileSystem]) *FileSystem {
	fs := &FileSystem{
		sync.Mutex{},
		make(map[string]objMap),
	}

	options.ApplyOptions(fs, opts...)

	return fs
}

// Retry will return a retrier provided via options, or a no-op if none is provided.
//
// Deprecated: This method is deprecated and will be removed in a future release.
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
func (fs *FileSystem) NewFile(volume, absFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
	err := utils.ValidateAbsoluteFilePath(absFilePath)
	if err != nil {
		return nil, err
	}

	location, err := fs.NewLocation(volume, utils.EnsureTrailingSlash(path.Dir(absFilePath)))
	if err != nil {
		return nil, err
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, ok := fs.fsMap[volume]; ok {
		for _, obj := range fs.fsMap[volume] {
			if obj.isFile && obj.i.(*memFile).location.Path() == location.Path() {
				if obj.i.(*memFile).name == path.Base(absFilePath) {
					vfsFile := &File{
						name:            obj.i.(*memFile).name,
						memFile:         obj.i.(*memFile),
						readWriteSeeker: NewReadWriteSeekerWithData(obj.i.(*memFile).contents),
						opts:            opts,
					}
					return vfsFile, nil
				}
			}
		}
	}
	// validateAbsFile path will throw an error if there was a trailing slash, hence not calling path.Clean()
	file := &File{
		name: path.Base(absFilePath),
		opts: opts,
	}

	memFile := newMemFile(file, location.(*Location))
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

	auth, err := authority.NewAuthority(volume)
	if err != nil {
		return nil, err
	}
	return &Location{
		fileSystem: fs,
		name:       str,
		exists:     false,
		authority:  auth,
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
		name:            srcFile.name,
		memFile:         srcFile,
		readWriteSeeker: NewReadWriteSeekerWithData(srcFile.contents),
	}

	destination.memFile = srcFile
	destination.readWriteSeeker = NewReadWriteSeekerWithData(srcFile.contents)
	return destination
}
