package azure

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"strings"

	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfs "github.com/c2fo/vfs/v8"
	vfsopt "github.com/c2fo/vfs/v8/options"
)

var errLocationRequired = errors.New("azure.Location receiver pointer must be non-nil")

// Location is the azure implementation of [vfs.Location].
type Location struct {
	authority  authority.Authority
	path       string
	fileSystem *FileSystem
}

// String returns the URI.
func (l *Location) String() string {
	return l.URI()
}

// List lists blobs under this location (see [vfs.ListOption]).
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
			if !yield(vfs.Entry{Kind: vfs.EntryBlob, Name: name}, nil) {
				return
			}
		}
	}
}

func (l *Location) listNames(ctx context.Context, cfg vfs.ListConfig) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if cfg.Prefix != "" && strings.Contains(cfg.Prefix, "/") {
		listLoc, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(cfg.Prefix)))
		if err != nil {
			return nil, err
		}
		return listLocationByPrefix(listLoc.(*Location), path.Base(cfg.Prefix))
	}

	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	list, err := client.List(l)
	if err != nil {
		return nil, err
	}

	if len(list) == 0 {
		return []string{}, nil
	}

	var names []string
	for _, item := range list {
		names = append(names, path.Base(item))
	}

	if cfg.Prefix != "" {
		var filtered []string
		for _, item := range names {
			if strings.HasPrefix(item, cfg.Prefix) {
				filtered = append(filtered, path.Base(item))
			}
		}
		if len(filtered) == 0 {
			return []string{}, nil
		}
		return filtered, nil
	}

	return names, nil
}

func listLocationByPrefix(location *Location, prefix string) ([]string, error) {
	listing, err := listingBaseNames(location)
	if err != nil {
		return nil, err
	}

	var filtered []string
	for _, item := range listing {
		if strings.HasPrefix(item, prefix) {
			filtered = append(filtered, path.Base(item))
		}
	}

	if len(filtered) == 0 {
		return []string{}, nil
	}

	return filtered, nil
}

func listingBaseNames(l *Location) ([]string, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	list, err := client.List(l)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return []string{}, nil
	}
	ret := make([]string, len(list))
	for i, item := range list {
		ret[i] = path.Base(item)
	}
	return ret, nil
}

// Authority returns the authority for the Location.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the absolute path for the Location.
func (l *Location) Path() string {
	return utils.EnsureTrailingSlash(utils.EnsureLeadingSlash(l.path))
}

// Exists returns true if the container exists and is reachable.
func (l *Location) Exists() (bool, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return false, err
	}
	_, err = client.Properties(l.Authority().String(), "")
	if err != nil {
		return false, nil
	}
	return true, nil
}

// NewLocation creates a new location instance relative to the current location's path.
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if err := utils.ValidateRelativeLocationPath(relLocPath); err != nil {
		return nil, err
	}

	return &Location{
		fileSystem: l.fileSystem,
		path:       path.Join(l.path, relLocPath),
		authority:  l.Authority(),
	}, nil
}

// FileSystem returns the azure FileSystem instance.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile returns a new file instance at the given path, relative to the current location.
func (l *Location) NewFile(relFilePath string, opts ...vfsopt.NewFileOption) (vfs.File, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if err := utils.ValidateRelativeFilePath(relFilePath); err != nil {
		return nil, err
	}

	newLocation, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(relFilePath)))
	if err != nil {
		return nil, err
	}

	return &File{
		location: newLocation.(*Location),
		name:     path.Join(l.Path(), relFilePath),
		opts:     opts,
	}, nil
}

// DeleteFile deletes the file at the given path, relative to the current location.
func (l *Location) DeleteFile(relFilePath string, opts ...vfsopt.DeleteOption) error {
	file, err := l.NewFile(utils.RemoveLeadingSlash(relFilePath))
	if err != nil {
		return err
	}
	af, ok := file.(*File)
	if !ok {
		return fmt.Errorf("azure: unexpected file implementation %T", file)
	}
	return af.deleteObject(opts...)
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return fmt.Sprintf("%s://%s%s", l.FileSystem().Scheme(), l.Authority().String(), l.Path())
}

var _ vfs.Location = (*Location)(nil)
