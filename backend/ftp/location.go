package ftp

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

// Location implements the vfs.Location interface specific to ftp fs.
type Location struct {
	fileSystem *FileSystem
	path       string
	Authority  utils.Authority
}

// List calls FTP ReadDir to list all files in the location's path.
// If you have many thousands of files at the given location, this could become quite expensive.
func (l *Location) List() ([]string, error) {
	var filenames []string
	dc, err := l.fileSystem.DataConn(context.TODO(), l.Authority, types.SingleOp, nil)
	if err != nil {
		return filenames, err
	}

	entries, err := dc.List(l.Path())
	if err != nil {
		if strings.HasPrefix(err.Error(), fmt.Sprintf("%d", _ftp.StatusFileUnavailable)) {
			// in this case the directory does not exist
			return filenames, nil
		}
		return filenames, err
	}
	for _, entry := range entries {
		if entry.Type == _ftp.EntryTypeFile {
			filenames = append(filenames, entry.Name)
		}
	}

	return filenames, nil
}

// ListByPrefix calls FTP ReadDir with the location's path modified relatively by the prefix arg passed to the function.
//   - Returns ([]string{}, nil) in the case of a non-existent directory/prefix/location.
//   - "relative" prefixes are allowed, ie, listByPrefix from "/some/path/" with prefix "to/somepattern" is the same as
//     location "/some/path/to/" with prefix of "somepattern"
//   - If the user cares about the distinction between an empty location and a non-existent one, Location.Exists() should
//     be checked first.
func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	var filenames = make([]string, 0)

	// validate prefix
	if err := utils.ValidatePrefix(prefix); err != nil {
		return filenames, err
	}

	// get absolute prefix path (in case prefix contains relative prefix, ie, some/path/to/myprefix)
	fullpath := path.Join(l.Path(), prefix)

	// get prefix and location path after any relative pathing is resolved
	// For example, given:
	//   loc, _ := fs.NewLocation("user@host:21", "/some/path/")
	//   loc.ListByPrefix("subdir/prefix")
	// the location fullpath should resolve to be "/some/path/subdir/" while the prefix would be "prefix".
	baseprefix := ""
	if prefix == "." {
		// for prefix of ".", it is necessary to manually set baseprefix as "." and
		// add trailing slash since path.Join thinks that "." is a directory
		baseprefix = prefix
		fullpath = utils.EnsureTrailingSlash(fullpath)
	} else {
		// get baseprefix fix from the fullpath
		baseprefix = path.Base(fullpath)
		// get absolute dir path of fullpath
		fullpath = utils.EnsureTrailingSlash(path.Dir(fullpath))
	}

	// get dataconn
	dc, err := l.fileSystem.DataConn(context.TODO(), l.Authority, types.SingleOp, nil)
	if err != nil {
		return filenames, err
	}

	// list directory entries
	entries, err := dc.List(fullpath)
	if err != nil {
		// fullpath does not exist, is not an error here
		if strings.HasPrefix(err.Error(), fmt.Sprintf("%d", _ftp.StatusFileUnavailable)) {
			// in this case the directory does not exist
			return []string{}, nil
		}
		return filenames, err
	}

	for _, entry := range entries {
		// find entries that match prefix and are files
		if entry.Type == _ftp.EntryTypeFile && strings.HasPrefix(entry.Name, baseprefix) {
			filenames = append(filenames, entry.Name)
		}
	}

	return filenames, nil
}

// ListByRegex retrieves the filenames of all the files at the location's current path, then filters out all those
// that don't match the given regex. The resource considerations of List() apply here as well.
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	filenames, err := l.List()
	if err != nil {
		return nil, err
	}

	var filteredFilenames []string
	for _, filename := range filenames {
		if regex.MatchString(filename) {
			filteredFilenames = append(filteredFilenames, filename)
		}
	}
	return filteredFilenames, nil
}

// Volume returns the Authority the location is contained in.
func (l *Location) Volume() string {
	return l.Authority.String()
}

// Path returns the path the location references in most FTP calls.
func (l *Location) Path() string {
	return utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(l.path))
}

// Exists returns true if the remote FTP directory exists.
func (l *Location) Exists() (bool, error) {
	dc, err := l.fileSystem.DataConn(context.TODO(), l.Authority, types.SingleOp, nil)
	if err != nil {
		return false, err
	}

	locBasename := path.Base(l.Path())
	// get parent directory by removing the last part of the path
	parentDir := strings.TrimSuffix(l.Path(), locBasename+"/")

	entries, err := dc.List(parentDir)
	if err != nil {
		if strings.HasPrefix(err.Error(), fmt.Sprintf("%d", _ftp.StatusFileUnavailable)) {
			// in this case the directory does not exist
			return false, nil
		}
		return false, err
	}

	for i := range entries {
		if entries[i].Name == locBasename && entries[i].Type == _ftp.EntryTypeFolder {
			return true, nil
		}
	}

	return false, nil
}

// NewLocation makes a copy of the underlying Location, then modifies its path by calling ChangeDir with the
// relativePath argument, returning the resulting location. The only possible errors come from the call to
// ChangeDir, which, for the FTP implementation doesn't ever result in an error.
func (l *Location) NewLocation(relativePath string) (vfs.Location, error) {
	// make a copy of the original location first, then ChangeDir, leaving the original location as-is
	newLocation := &Location{}
	*newLocation = *l
	err := newLocation.ChangeDir(relativePath)
	if err != nil {
		return nil, err
	}
	return newLocation, nil
}

// ChangeDir takes a relative path, and modifies the underlying Location's path. The caller is modified by this
// so the only return is any error. For this implementation there are no errors.
func (l *Location) ChangeDir(relativePath string) error {
	err := utils.ValidateRelativeLocationPath(relativePath)
	if err != nil {
		return err
	}
	l.path = utils.EnsureLeadingSlash(utils.EnsureTrailingSlash(path.Join(l.path, relativePath)))
	return nil
}

// NewFile uses the properties of the calling location to generate a vfs.File (backed by an ftp.File). The filePath
// argument is expected to be a relative path to the location's current path.
func (l *Location) NewFile(filePath string, opts ...options.NewFileOption) (vfs.File, error) {
	err := utils.ValidateRelativeFilePath(filePath)
	if err != nil {
		return nil, err
	}
	newFile := &File{
		fileSystem: l.fileSystem,
		authority:  l.Authority,
		path:       utils.EnsureLeadingSlash(path.Join(l.path, filePath)),
		opts:       opts,
	}
	return newFile, nil
}

// DeleteFile removes the file at fileName path.
func (l *Location) DeleteFile(fileName string, _ ...options.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}

	return file.Delete()
}

// FileSystem returns a vfs.fileSystem interface of the location's underlying fileSystem.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return utils.EncodeURI(l.FileSystem().Scheme(), l.Authority.UserInfo().Username(), l.Authority.HostPortStr(), l.Path())
}

// String implement fmt.Stringer, returning the location's URI as the default string.
func (l *Location) String() string {
	return l.URI()
}
