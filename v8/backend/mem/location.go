package mem

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

// Location implements [vfs.Location] for in-memory paths.
type Location struct {
	exists     bool
	name       string
	fileSystem *FileSystem
	authority  authority.Authority
}

// String returns the location URI.
func (l *Location) String() string {
	return l.URI()
}

// List implements [vfs.Lister.List].
func (l *Location) List(ctx context.Context, opts ...vfs.ListOption) iter.Seq2[vfs.Entry, error] {
	cfg := vfs.ApplyListOptions(vfs.ListConfig{}, opts...)

	return func(yield func(vfs.Entry, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(vfs.Entry{}, err)
			return
		}
		if cfg.Recursive {
			yield(vfs.Entry{}, vfs.ErrNotSupported)
			return
		}

		names, err := l.listNames(cfg)
		if err != nil {
			yield(vfs.Entry{}, err)
			return
		}

		for _, name := range names {
			if err := ctx.Err(); err != nil {
				yield(vfs.Entry{}, err)
				return
			}
			if cfg.Matcher != nil && !cfg.Matcher(name) {
				continue
			}
			if !yield(vfs.Entry{Kind: vfs.EntryBlob, Name: name}, nil) {
				return
			}
		}
	}
}

func (l *Location) listNames(cfg vfs.ListConfig) ([]string, error) {
	mapRef := l.fileSystem.fsMap
	if _, ok := mapRef[l.Authority().String()]; !ok {
		return nil, nil
	}

	if cfg.Prefix != "" {
		return l.listByPrefix(cfg.Prefix, mapRef)
	}

	return mapRef[l.Authority().String()].fileNamesHere(l.Path()), nil
}

func (l *Location) listByPrefix(prefix string, mapRef map[string]objMap) ([]string, error) {
	list := make([]string, 0)
	str := path.Join(l.Path(), prefix)
	vol := l.Authority().String()
	if _, ok := mapRef[vol]; !ok {
		return list, nil
	}
	paths := mapRef[vol].getKeys()
	for i := range paths {
		if strings.Contains(paths[i], str) {
			if path.Ext(paths[i]) != "" && strings.Contains(str, utils.EnsureTrailingSlash(path.Dir(paths[i]))) {
				list = append(list, path.Base(paths[i]))
				sort.Strings(list)
			}
		}
	}
	return list, nil
}

// Authority returns the location authority.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the absolute location path with leading and trailing slashes.
func (l *Location) Path() string {
	str := utils.EnsureTrailingSlash(path.Clean(l.name))
	return utils.EnsureLeadingSlash(str)
}

// Exists always reports true for locations (same as v7 mem).
func (l *Location) Exists() (bool, error) {
	l.exists = true
	return true, nil
}

// NewLocation resolves a relative path against this location.
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {
	if err := utils.ValidateRelativeLocationPath(relLocPath); err != nil {
		return nil, err
	}
	str := path.Join(l.Path(), relLocPath)
	str = utils.EnsureTrailingSlash(path.Clean(str))
	mapRef := l.fileSystem.fsMap
	if object, ok := mapRef[l.Authority().String()]; ok {
		paths := object.getKeys()
		for _, potentialPath := range paths {
			if potentialPath != str {
				continue
			}
			o := mapRef[l.Authority().String()][potentialPath]
			if o != nil && !o.isFile {
				if loc, ok := o.i.(*Location); ok {
					return loc, nil
				}
			}
		}
	}
	return &Location{
		fileSystem: l.fileSystem,
		name:       str,
		exists:     false,
		authority:  l.Authority(),
	}, nil
}

// FileSystem returns the owning [FileSystem].
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile creates or opens a [vfs.File] relative to this location.
func (l *Location) NewFile(relFilePath string, _ ...vfsopt.NewFileOption) (vfs.File, error) {
	if relFilePath == "" {
		return nil, errors.New("mem: empty file name")
	}
	if err := utils.ValidateRelativeFilePath(relFilePath); err != nil {
		return nil, err
	}

	mapRef := l.fileSystem.fsMap
	relativeLocationPath := utils.EnsureTrailingSlash(path.Dir(path.Join(l.Path(), relFilePath)))
	if _, ok := mapRef[l.Authority().String()]; ok {
		fileList := mapRef[l.Authority().String()].filesHere(relativeLocationPath)
		for _, mf := range fileList {
			if mf.name == path.Base(relFilePath) {
				return deepCopyFile(mf), nil
			}
		}
	}

	pref := l.Path()
	nameStr := path.Join(pref, relFilePath)
	file := &File{name: path.Base(nameStr)}
	newLoc := *l
	newLoc.name = relativeLocationPath
	file.memFile = newMemFile(file, &newLoc)
	return file, nil
}

// DeleteFile removes a file relative to this location.
func (l *Location) DeleteFile(relFilePath string, _ ...vfsopt.DeleteOption) error {
	if err := utils.ValidateRelativeFilePath(relFilePath); err != nil {
		return err
	}
	vol := l.Authority().String()
	fullPath := path.Join(l.Path(), relFilePath)

	l.fileSystem.mu.Lock()
	defer l.fileSystem.mu.Unlock()
	mapRef := l.fileSystem.fsMap
	if _, ok := mapRef[vol]; ok {
		if thisObj, ok2 := mapRef[vol][fullPath]; ok2 {
			mf := thisObj.i.(*memFile)
			mf.exists = false
			thisObj.i = nil
			delete(mapRef[vol], fullPath)
			return nil
		}
	}

	return fmt.Errorf("mem: delete file: %w", os.ErrNotExist)
}

// URI returns the fully qualified location URI.
func (l *Location) URI() string {
	return formatLocationURI(l)
}

var _ vfs.Location = (*Location)(nil)
