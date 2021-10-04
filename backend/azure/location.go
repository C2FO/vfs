package azure

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

const errNilLocationReceiver = "azure.Location receiver pointer must be non-nil"

// Location is the azure implementation of vfs.Location
type Location struct {
	container  string
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
func (l *Location) Volume() string {
	return l.container
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
	_, err = client.Properties(l.ContainerURL(), "")
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
		container:  l.container,
		path:       path.Join(l.path, relLocPath),
	}, nil
}

// ChangeDir changes the current location's path to the new, relative path.
func (l *Location) ChangeDir(relLocPath string) error {
	if l == nil {
		return errors.New(errNilLocationReceiver)
	}

	err := utils.ValidateRelativeLocationPath(relLocPath)
	if err != nil {
		return err
	}

	l.path = path.Join(l.path, relLocPath)

	return nil
}

// FileSystem returns the azure FileSystem instance
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// NewFile returns a new file instance at the given path, relative to the current location.
func (l *Location) NewFile(relFilePath string) (vfs.File, error) {
	if l == nil {
		return nil, errors.New(errNilLocationReceiver)
	}

	if err := utils.ValidateRelativeFilePath(relFilePath); err != nil {
		return nil, err
	}

	return &File{
		name:       utils.EnsureLeadingSlash(path.Join(l.path, relFilePath)),
		container:  l.container,
		fileSystem: l.fileSystem,
	}, nil
}

// DeleteFile deletes the file at the given path, relative to the current location.
func (l *Location) DeleteFile(relFilePath string) error {
	file, err := l.NewFile(utils.RemoveLeadingSlash(relFilePath))
	if err != nil {
		return err
	}

	return file.Delete()
}

// URI returns a URI string for the azure location.
func (l *Location) URI() string {
	return fmt.Sprintf("%s://%s%s", l.fileSystem.Scheme(), utils.EnsureTrailingSlash(l.fileSystem.Host()),
		utils.EnsureTrailingSlash(path.Join(l.container, l.path)))
}

// ContainerURL returns the URL for the Azure Blob Storage container.
func (l *Location) ContainerURL() string {
	return fmt.Sprintf("%s://%s%s", l.fileSystem.Scheme(), utils.EnsureTrailingSlash(l.fileSystem.Host()),
		utils.EnsureTrailingSlash(l.container))
}
