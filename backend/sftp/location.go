package sftp

import (
	"errors"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
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

// List calls SFTP ReadDir to list all files in the location's path.
// If you have many thousands of files at the given location, this could become quite expensive.
func (l *Location) List() ([]string, error) {
	var filenames []string
	client, err := l.fileSystem.Client(l.Authority())
	if err != nil {
		return filenames, err
	}
	// start timer once action is completed
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

// ListByPrefix calls SFTP ReadDir with the location's path modified relatively by the prefix arg passed to the function.
func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	var filenames []string
	client, err := l.fileSystem.Client(l.Authority())
	if err != nil {
		return filenames, err
	}
	// start timer once action is completed
	defer l.fileSystem.connTimerStart()

	fullpath := path.Join(l.Path(), prefix)
	// check if last char is not /, aka is not a dir, get base of path
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

// ListByRegex retrieves the filenames of all the files at the location's current path, then filters out all those
// that don't match the given regex. The resource considerations of List() apply here as well.
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	filenames, err := l.List()
	if err != nil {
		return []string{}, err
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
//
// Deprecated: Use Authority instead.
//
//	authStr := loc.Authority().String()
func (l *Location) Volume() string {
	return l.Authority().String()
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
	// start timer once action is completed
	defer l.fileSystem.connTimerStart()

	info, err := client.Stat(l.Path())
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if !info.IsDir() {
		return false, err
	}

	return true, nil
}

// NewLocation makes a copy of the underlying Location, then modifies its path by calling ChangeDir with the
// relativePath argument, returning the resulting location.
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

// ChangeDir takes a relative path, and modifies the underlying Location's path. The caller is modified by this
// so the only return is any error. For this implementation there are no errors.
//
// Deprecated: Use NewLocation instead:
//
//	loc, err := loc.NewLocation("../../")
func (l *Location) ChangeDir(relativePath string) error {
	if l == nil {
		return errLocationRequired
	}

	if relativePath == "" {
		return errPathRequired
	}

	err := utils.ValidateRelativeLocationPath(relativePath)
	if err != nil {
		return err
	}

	newLoc, err := l.NewLocation(relativePath)
	if err != nil {
		return err
	}
	*l = *newLoc.(*Location)

	return nil
}

// NewFile uses the properties of the calling location to generate a vfs.File (backed by an sftp.File). The filePath
// argument is expected to be a relative path to the location's current path.
func (l *Location) NewFile(relFilePath string, opts ...options.NewFileOption) (vfs.File, error) {
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
func (l *Location) DeleteFile(fileName string, opts ...options.DeleteOption) error {
	file, err := l.NewFile(fileName)
	if err != nil {
		return err
	}

	return file.Delete(opts...)
}

// FileSystem returns a vfs.fileSystem interface of the location's underlying fileSystem.
func (l *Location) FileSystem() vfs.FileSystem {
	return l.fileSystem
}

// URI returns the Location's URI as a string.
func (l *Location) URI() string {
	return utils.EncodeURI(l.FileSystem().Scheme(), l.Authority().UserInfo().Username(), l.Authority().HostPortStr(), l.Path())
}

// String implement fmt.Stringer, returning the location's URI as the default string.
func (l *Location) String() string {
	return l.URI()
}
