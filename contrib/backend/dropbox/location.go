package dropbox

import (
	"errors"
	"path"
	"regexp"
	"strings"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

var (
	errLocationRequired = errors.New("non-nil dropbox.Location pointer is required")
	errPathRequired     = errors.New("non-empty string for path is required")
)

// Location implements the vfs.Location interface for Dropbox.
type Location struct {
	fileSystem *FileSystem
	path       string
	authority  authority.Authority
}

// List returns a list of file names in the location.
func (l *Location) List() ([]string, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return nil, err
	}

	// Dropbox paths must not have trailing slash for ListFolder (except root)
	listPath := strings.TrimSuffix(l.path, "/")
	if listPath == "" {
		listPath = ""
	}

	var allFiles []string

	// Initial list request
	result, err := client.ListFolder(&files.ListFolderArg{
		Path: listPath,
	})
	if err != nil {
		// Check if it's a "path not found" error
		if isNotFoundError(err) {
			return []string{}, nil
		}
		return nil, err
	}

	// Collect file names from first batch (files only, not subdirectories)
	for _, entry := range result.Entries {
		if metadata, ok := entry.(*files.FileMetadata); ok {
			allFiles = append(allFiles, path.Base(metadata.PathDisplay))
		}
		// Skip subdirectories - List() should only return files in the current directory
	}

	// Continue pagination if needed
	for result.HasMore {
		result, err = client.ListFolderContinue(&files.ListFolderContinueArg{
			Cursor: result.Cursor,
		})
		if err != nil {
			return nil, err
		}

		for _, entry := range result.Entries {
			if metadata, ok := entry.(*files.FileMetadata); ok {
				allFiles = append(allFiles, path.Base(metadata.PathDisplay))
			}
			// Skip subdirectories
		}
	}

	return allFiles, nil
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

	var filtered []string
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

	var filtered []string
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

// Authority returns the authority for this location.
// For Dropbox, this is always empty as it uses a single namespace per token.
func (l *Location) Authority() authority.Authority {
	return l.authority
}

// Path returns the path of the location.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.path))
}

// Exists checks if the location exists.
// Note: Dropbox doesn't store empty folders, so this may return false for empty directories.
func (l *Location) Exists() (bool, error) {
	client, err := l.fileSystem.Client()
	if err != nil {
		return false, utils.WrapExistsError(err)
	}

	// Try to get metadata for the path
	checkPath := strings.TrimSuffix(l.path, "/")
	if checkPath == "" {
		// Root always exists
		return true, nil
	}

	_, err = client.GetMetadata(&files.GetMetadataArg{
		Path: checkPath,
	})

	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, utils.WrapExistsError(err)
	}

	return true, nil
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

// isNotFoundError checks if an error is a "path not found" error from Dropbox.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Dropbox returns errors with "path/not_found" or "not_found" in the message
	errStr := err.Error()
	return strings.Contains(errStr, "path/not_found") ||
		strings.Contains(errStr, "not_found") ||
		strings.Contains(errStr, "path_not_found")
}
