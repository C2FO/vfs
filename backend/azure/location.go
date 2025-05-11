package azure

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

const errNilLocationReceiver = "azure.Location receiver pointer must be non-nil"

// Location is the azure implementation of vfs.Location
type Location struct {
	authority  authority.Authority
	path       string
	fileSystem *FileSystem
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
	list, err := client.List(l)
	if err != nil {
		return nil, err
	}

	var ret []string
	for _, item := range list {
		ret = append(ret, path.Base(item))
	}

	if len(ret) == 0 {
		return []string{}, nil
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
	_, err = client.Properties(l.Authority().String(), "")
	if err != nil {
		return false, nil
	}
	return true, nil
}

// NewLocation creates a new location instance relative to the current location's path.
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {
	if l == nil {
		return nil, errors.New(errNilLocationReceiver)
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

// ChangeDir changes the current location's path to the new, relative path.
//
// Deprecated: Use NewLocation instead:
//
//	loc, err := loc.NewLocation("../../")
func (l *Location) ChangeDir(relLocPath string) error {
	if l == nil {
		return errors.New(errNilLocationReceiver)
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
		return nil, errors.New(errNilLocationReceiver)
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

// Open opens the named file at this location.
// This implements the fs.FS interface from io/fs.
func (l *Location) Open(name string) (fs.File, error) {
	// fs.FS expects paths with no leading slash
	name = strings.TrimPrefix(name, "/")

	// For io/fs compliance, we need to validate that it doesn't contain "." or ".." elements
	if name == "." || name == ".." || strings.Contains(name, "/.") || strings.Contains(name, "./") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	// Create a standard vfs file using NewFile
	vfsFile, err := l.NewFile(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	// Check if the file exists, as fs.FS.Open requires the file to exist
	exists, err := vfsFile.Exists()
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if !exists {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return vfsFile, nil
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
