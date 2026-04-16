package sftp

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

var (
	errLocationRequired = errors.New("non-nil sftp.Location pointer receiver is required")
	errPathRequired     = errors.New("non-empty string relativePath is required")
)

// Location implements the vfs.Location interface specific to sftp fs.
type Location struct {
	fileSystem *FileSystem
	path       string
	authority  authority.Authority
}

// String returns the location URI.
func (l *Location) String() string {
	return l.URI()
}

// List calls SFTP ReadDir to list all files in the location's path (see [vfs.ListOption]).
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

		names, err := l.listNames(ctx, cfg)
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

func (l *Location) listNames(ctx context.Context, cfg vfs.ListConfig) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Prefix != "" {
		return l.listByPrefix(cfg.Prefix)
	}

	var filenames []string
	client, err := l.fileSystem.Client(l.Authority())
	if err != nil {
		return filenames, err
	}
	defer l.fileSystem.connTimerStart()

	fileinfos, err := client.ReadDir(l.Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return filenames, nil
		}
		return filenames, err
	}
	for _, fileinfo := range fileinfos {
		if !fileinfo.IsDir() {
			filenames = append(filenames, fileinfo.Name())
		}
	}

	return filenames, nil
}

// listByPrefix calls SFTP ReadDir with the location's path modified relatively by the prefix arg passed to the function.
func (l *Location) listByPrefix(prefix string) ([]string, error) {
	var filenames []string
	client, err := l.fileSystem.Client(l.Authority())
	if err != nil {
		return filenames, err
	}
	defer l.fileSystem.connTimerStart()

	fullpath := path.Join(l.Path(), prefix)
	baseprefix := ""
	r, _ := utf8.DecodeLastRuneInString(fullpath)
	if r != '/' {
		baseprefix = path.Base(fullpath)
	}
	fullpath = utils.EnsureTrailingSlash(path.Dir(fullpath))
	fileinfos, err := client.ReadDir(fullpath)
	if err != nil {
		return filenames, err
	}

	for _, fileinfo := range fileinfos {
		if !fileinfo.IsDir() {
			name := fileinfo.Name()
			if baseprefix != "" {
				if strings.HasPrefix(name, baseprefix) {
					filenames = append(filenames, name)
				}
			} else {
				filenames = append(filenames, name)
			}
		}
	}

	return filenames, nil
}

// Authority returns the Authority the location is contained in.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the path the location references in most SFTP calls.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.path))
}

// Exists returns true if the remote SFTP file exists.
func (l *Location) Exists() (bool, error) {
	client, err := l.fileSystem.Client(l.Authority())
	if err != nil {
		return false, err
	}
	defer l.fileSystem.connTimerStart()

	info, err := client.Stat(l.Path())
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if !info.IsDir() {
		return false, nil
	}

	return true, nil
}

// NewLocation makes a copy of the underlying Location with path extended by relativePath.
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
		path:       path.Join(l.path, relativePath),
		authority:  l.Authority(),
	}, nil
}

// NewFile uses the properties of the calling location to generate a vfs.File (backed by an sftp.File). The filePath
// argument is expected to be a relative path to the location's current path.
func (l *Location) NewFile(relFilePath string, opts ...vfsopt.NewFileOption) (vfs.File, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if relFilePath == "" {
		return nil, errPathRequired
	}

	err := utils.ValidateRelativeFilePath(relFilePath)
	if err != nil {
		return nil, err
	}

	newLocation, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(relFilePath)))
	if err != nil {
		return nil, err
	}

	return &File{
		location: newLocation.(*Location),
		path:     utils.EnsureLeadingSlash(path.Join(l.path, relFilePath)),
		opts:     opts,
	}, nil
}

// DeleteFile removes the file at fileName path.
func (l *Location) DeleteFile(fileName string, opts ...vfsopt.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}
	sf, ok := file.(*File)
	if !ok {
		return fmt.Errorf("sftp: unexpected file implementation %T", file)
	}
	return sf.deleteObject(opts...)
}

// FileSystem returns a vfs.fileSystem interface of the location's underlying fileSystem.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return utils.EncodeURI(l.FileSystem().Scheme(), l.Authority().UserInfo().Username(), l.Authority().HostPortStr(), l.Path())
}

var _ vfs.Location = (*Location)(nil)
