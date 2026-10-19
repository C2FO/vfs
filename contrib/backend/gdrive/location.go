package gdrive

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

var (
	errLocationRequired = errors.New("non-nil gdrive.Location pointer is required")
	errPathRequired     = errors.New("non-empty string for path is required")
)

// Location implements the vfs.Location interface for Google Drive.
type Location struct {
	fileSystem *FileSystem
	path       string
	authority  authority.Authority
}

// List returns a list of file names in the location.
func (l *Location) List() ([]string, error) {
	client, err := l.client()
	if err != nil {
		return nil, err
	}

	items, err := client.ListChildren(context.Background(), l.drivePath())
	if err != nil {
		if isMissing(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("gdrive: listing %s: %w", l.Path(), err)
	}

	files := []string{}
	for i := range items {
		if !items[i].IsFolder {
			files = append(files, items[i].Name)
		}
	}

	return files, nil
}

// ListByPrefix returns a list of file names that start with the given prefix.
// Supports relative paths, e.g., "subdir/prefix" will look in subdir for files with that prefix.
func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	// Check if prefix contains a path separator (relative path)
	if strings.Contains(prefix, "/") {
		// Split into directory path and file prefix
		dir := path.Dir(prefix)
		filePrefix := path.Base(prefix)

		// Create new location for subdirectory
		subLoc, err := l.NewLocation(dir + "/")
		if err != nil {
			return nil, err
		}

		// List files in subdirectory with the file prefix
		return subLoc.ListByPrefix(filePrefix)
	}

	// Simple case: no relative path, just filter current directory
	allFiles, err := l.List()
	if err != nil {
		return nil, err
	}

	filtered := []string{}
	for _, file := range allFiles {
		if strings.HasPrefix(file, prefix) {
			filtered = append(filtered, file)
		}
	}

	return filtered, nil
}

// ListByRegex returns a list of file names matching the given regex.
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	allFiles, err := l.List()
	if err != nil {
		return nil, err
	}

	filtered := []string{}
	for _, file := range allFiles {
		if regex.MatchString(file) {
			filtered = append(filtered, file)
		}
	}

	return filtered, nil
}

// Volume returns the authority as a string.
//
// Deprecated: Use Authority instead.
func (l *Location) Volume() string {
	return l.Authority().String()
}

// Authority returns the authority for this location: the shared drive ID, or empty for My Drive.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the path of the location.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.path))
}

// Exists checks if the location exists.
func (l *Location) Exists() (bool, error) {
	client, err := l.client()
	if err != nil {
		return false, err
	}

	checkPath := l.drivePath()
	if checkPath == "" {
		return true, nil
	}

	item, err := client.GetItem(context.Background(), checkPath)
	if err != nil {
		if isMissing(err) {
			return false, nil
		}
		return false, utils.WrapExistsError(err)
	}

	return item.IsFolder, nil
}

// NewLocation creates a new Location relative to the current one.
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
		authority:  l.authority,
	}, nil
}

// ChangeDir updates the location's path to the given relative path.
//
// Deprecated: Use NewLocation instead.
func (l *Location) ChangeDir(relativePath string) error {
	if l == nil {
		return errLocationRequired
	}

	if relativePath == "" {
		return errPathRequired
	}

	if err := utils.ValidateRelativeLocationPath(relativePath); err != nil {
		return err
	}

	newLoc, err := l.NewLocation(relativePath)
	if err != nil {
		return err
	}

	*l = *newLoc.(*Location)
	return nil
}

// NewFile creates a new File at the location.
func (l *Location) NewFile(relFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
	if l == nil {
		return nil, errLocationRequired
	}

	if relFilePath == "" {
		return nil, errPathRequired
	}

	if err := utils.ValidateRelativeFilePath(relFilePath); err != nil {
		return nil, err
	}

	newLocation, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(relFilePath)))
	if err != nil {
		return nil, err
	}

	newFile := &File{
		location: newLocation.(*Location),
		path:     path.Join(l.path, relFilePath),
		opts:     opts,
	}

	return newFile, nil
}

// DeleteFile deletes a file at the location.
func (l *Location) DeleteFile(fileName string, opts ...options.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}

	return file.Delete(opts...)
}

// FileSystem returns the underlying FileSystem.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// URI returns the location's URI.
func (l *Location) URI() string {
	return utils.GetLocationURI(l)
}

// String returns the location's URI as a string.
func (l *Location) String() string {
	return l.URI()
}

// client returns the Client for this location's drive.
func (l *Location) client() (Client, error) {
	return l.fileSystem.Client(l.authority.String())
}

// drivePath returns the location path in Client form: no leading or trailing slash, "" for the root.
func (l *Location) drivePath() string {
	return strings.Trim(l.path, "/")
}
