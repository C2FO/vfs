package os

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

var (
	errLocationRequired = errors.New("non-nil os.Location pointer is required")
	errPathRequired     = errors.New("non-empty string path is required")
)

// Location implements [vfs.Location] for the local OS file system.
type Location struct {
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
			if !yield(vfs.Entry{Kind: vfs.EntryFile, Name: name}, nil) {
				return
			}
		}
	}
}

func (l *Location) listNames(cfg vfs.ListConfig) ([]string, error) {
	var names []string
	var err error
	if cfg.Prefix != "" {
		names, err = l.listByPrefix(cfg.Prefix)
	} else {
		names, err = l.fileList(func(name string) bool { return true })
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func (l *Location) listByPrefix(prefix string) ([]string, error) {
	var loc vfs.Location
	var err error
	d := path.Dir(prefix)

	if d != "." && d != "/" {
		loc, err = l.NewLocation(utils.EnsureTrailingSlash(d))
		if err != nil {
			return nil, err
		}
		prefix = path.Base(prefix)
	} else {
		loc = l
	}

	return loc.(*Location).fileList(func(name string) bool {
		return strings.HasPrefix(name, prefix)
	})
}

type fileTest func(fileName string) bool

func (l *Location) fileList(testEval fileTest) ([]string, error) {
	files := make([]string, 0)
	exists, err := l.Exists()
	if err != nil {
		return files, err
	}

	if exists {
		entries, err := os.ReadDir(osLocationPath(l))
		if err != nil {
			return files, err
		}

		for _, info := range entries {
			if !info.IsDir() && testEval(info.Name()) {
				files = append(files, info.Name())
			}
		}
	}

	return files, nil
}

// NewFile uses the properties of the calling location to generate a [vfs.File]. The argument
// must be a relative path to the location's current path.
func (l *Location) NewFile(fileName string, _ ...vfsopt.NewFileOption) (vfs.File, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if fileName == "" {
		return nil, errPathRequired
	}

	fileName = filepath.ToSlash(fileName)
	if err := utils.ValidateRelativeFilePath(fileName); err != nil {
		return nil, err
	}
	fileName = utils.EnsureLeadingSlash(path.Clean(path.Join(l.name, fileName)))
	return l.fileSystem.NewFile(l.Authority().String(), fileName)
}

// DeleteFile removes the file with the given name relative to this location.
func (l *Location) DeleteFile(fileName string, _ ...vfsopt.DeleteOption) error {
	f, err := l.NewFile(fileName)
	if err != nil {
		return err
	}
	if err := os.Remove(toNativeOSPath(f.Path())); err != nil {
		return utils.WrapDeleteError(err)
	}
	return nil
}

// Authority returns the location authority.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the location path.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.name))
}

// Exists returns true if the location exists and the caller has permission to stat it.
func (l *Location) Exists() (bool, error) {
	_, err := os.Stat(osLocationPath(l))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return fmtLocationURI(l)
}

// NewLocation returns a new location by resolving relativePath against this location.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if relativePath == "" {
		return nil, errPathRequired
	}

	if err := utils.ValidateRelativeLocationPath(relativePath); err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: l.fileSystem,
		name:       path.Join(l.name, relativePath),
		authority:  l.Authority(),
	}, nil
}

// FileSystem returns the underlying [vfs.FileSystem].
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

func osLocationPath(l vfs.Location) string {
	return toNativeOSPath(l.Path())
}

func fmtLocationURI(l *Location) string {
	return fmt.Sprintf("%s://%s%s", l.FileSystem().Scheme(), l.Authority().String(), l.Path())
}

var _ vfs.Location = (*Location)(nil)
