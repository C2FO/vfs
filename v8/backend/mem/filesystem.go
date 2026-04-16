package mem

import (
	"path"
	"sync"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

// Scheme is the URI scheme for the in-memory file system.
const Scheme = "mem"

const fsDisplayName = "In-Memory FileSystem"

type fsObject struct {
	isFile bool
	i      any
}

type objMap map[string]*fsObject

// FileSystem is an in-memory [vfs.FileSystem].
type FileSystem struct {
	mu    sync.Mutex
	fsMap map[string]objMap
}

// NewFileSystem creates an empty in-memory file system.
func NewFileSystem() *FileSystem {
	return &FileSystem{
		mu:    sync.Mutex{},
		fsMap: make(map[string]objMap),
	}
}

// NewFile returns a [vfs.File] for the given authority (volume) and absolute file path.
// A file is not present in [vfs.File.Exists] until [vfs.File.Touch] or a successful write path stores it.
func (fs *FileSystem) NewFile(volume, absFilePath string, _ ...vfsopt.NewFileOption) (vfs.File, error) {
	if err := utils.ValidateAbsoluteFilePath(absFilePath); err != nil {
		return nil, err
	}

	location, err := fs.NewLocation(volume, utils.EnsureTrailingSlash(path.Dir(absFilePath)))
	if err != nil {
		return nil, err
	}
	loc := location.(*Location)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, ok := fs.fsMap[volume]; ok {
		for _, obj := range fs.fsMap[volume] {
			if obj.isFile && obj.i.(*memFile).location.Path() == loc.Path() {
				if obj.i.(*memFile).name == path.Base(absFilePath) {
					return &File{
						name:            obj.i.(*memFile).name,
						memFile:         obj.i.(*memFile),
						readWriteSeeker: newReadWriteSeekerWithData(obj.i.(*memFile).contents),
					}, nil
				}
			}
		}
	}

	file := &File{name: path.Base(absFilePath)}
	memf := newMemFile(file, loc)
	file.memFile = memf
	return file, nil
}

// NewLocation returns a [vfs.Location] for the given authority and absolute location path.
func (fs *FileSystem) NewLocation(volume, absLocPath string) (vfs.Location, error) {
	if err := utils.ValidateAbsoluteLocationPath(absLocPath); err != nil {
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

// Name returns the display name of the file system.
func (fs *FileSystem) Name() string {
	return fsDisplayName
}

// Scheme returns [Scheme].
func (fs *FileSystem) Scheme() string {
	return Scheme
}

func (o objMap) getKeys() []string {
	keyList := make([]string, 0, len(o))
	for i := range o {
		keyList = append(keyList, i)
	}
	return keyList
}

func (o objMap) filesHere(absLocPath string) []*memFile {
	paths := o.getKeys()
	fileList := make([]*memFile, 0)
	for i := range paths {
		object := o[paths[i]]
		if object != nil && object.isFile {
			file := object.i.(*memFile)
			if file.location.Path() == absLocPath {
				fileList = append(fileList, file)
			}
		}
	}
	return fileList
}

func (o objMap) fileNamesHere(absLocPath string) []string {
	paths := o.getKeys()
	fileList := make([]string, 0)
	for i := range paths {
		object := o[paths[i]]
		if object != nil && object.isFile {
			file := object.i.(*memFile)
			if utils.EnsureTrailingSlash(file.location.Path()) == absLocPath {
				fileList = append(fileList, file.name)
			}
		}
	}
	return fileList
}

var (
	_ vfs.FileSystem = (*FileSystem)(nil)
)
