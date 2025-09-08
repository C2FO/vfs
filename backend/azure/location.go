package azure

import (
	"context"
	"errors"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/options/newfile"
	"github.com/c2fo/vfs/v7/options/newlocation"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

var errLocationRequired = errors.New("azure.Location receiver pointer must be non-nil")

// Location is the azure implementation of vfs.Location
type Location struct {
	authority  authority.Authority
	path       string
	fileSystem *FileSystem
	ctx        context.Context
}

// String returns the URI
func (l *Location) String() string {
	return l.URI()
}

// List returns a list of base names for the given location.
func (l *Location) List() ([]string, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}
	list, err := client.List(l.ctx, l)
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

// ListByPrefix returns a list of base names that contain the given prefix
func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	if strings.Contains(prefix, "/") {
		listLoc, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(prefix)))
		if err != nil {
			return nil, err
		}

		return listLocationByPrefix(listLoc.(*Location), path.Base(prefix))
	}

	return listLocationByPrefix(l, prefix)
}

func listLocationByPrefix(location *Location, prefix string) ([]string, error) {
	listing, err := location.List()
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

// ListByRegex returns a list of base names that match the given regular expression
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	listing, err := l.List()
	if err != nil {
		return nil, err
	}

	var filtered []string
	for _, item := range listing {
		if regex.MatchString(item) {
			filtered = append(filtered, path.Base(item))
		}
	}

	if len(filtered) == 0 {
		return []string{}, nil
	}

	return filtered, nil
}

// Volume returns the azure container.  Azure containers are equivalent to AWS Buckets
//
// Deprecated: Use Authority instead.
//
//	authStr := loc.Authority().String()
func (l *Location) Volume() string {
	return l.Authority().String()
}

// Authority returns the authority for the Location
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the absolute path for the Location
func (l *Location) Path() string {
	return utils.EnsureTrailingSlash(utils.EnsureLeadingSlash(l.path))
}

// Exists returns true if the file exists and false.  In the case of errors false is always returned along with
// the error
func (l *Location) Exists() (bool, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return false, err
	}
	_, err = client.Properties(l.ctx, l.Authority().String(), "")
	if err != nil {
		return false, nil
	}
	return true, nil
}

// NewLocation creates a new location instance relative to the current location's path.
func (l *Location) NewLocation(relLocPath string, opts ...options.NewLocationOption) (vfs.Location, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if err := utils.ValidateRelativeLocationPath(relLocPath); err != nil {
		return nil, err
	}

	ctx := l.ctx
	for _, o := range opts {
		switch o := o.(type) {
		case *newlocation.Context:
			ctx = context.Context(o)
		default:
		}
	}

	return &Location{
		fileSystem: l.fileSystem,
		path:       path.Join(l.path, relLocPath),
		authority:  l.Authority(),
		ctx:        ctx,
	}, nil
}

// ChangeDir changes the current location's path to the new, relative path.
//
// Deprecated: Use NewLocation instead:
//
//	loc, err := loc.NewLocation("../../")
func (l *Location) ChangeDir(relLocPath string) error {
	if l == nil {
		return errLocationRequired
	}

	err := utils.ValidateRelativeLocationPath(relLocPath)
	if err != nil {
		return err
	}

	newLoc, err := l.NewLocation(relLocPath)
	if err != nil {
		return err
	}
	*l = *newLoc.(*Location)

	return nil
}

// FileSystem returns the azure FileSystem instance
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile returns a new file instance at the given path, relative to the current location.
func (l *Location) NewFile(relFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
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

	ctx := l.ctx
	for _, o := range opts {
		switch o := o.(type) {
		case *newfile.Context:
			ctx = context.Context(o)
		default:
		}
	}

	return &File{
		location: newLocation.(*Location),
		name:     path.Join(l.Path(), relFilePath),
		opts:     opts,
		ctx:      ctx,
	}, nil
}

// DeleteFile deletes the file at the given path, relative to the current location.
func (l *Location) DeleteFile(relFilePath string, opts ...options.DeleteOption) error {
	file, err := l.NewFile(utils.RemoveLeadingSlash(relFilePath))
	if err != nil {
		return err
	}

	return file.Delete(opts...)
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return utils.GetLocationURI(l)
}
