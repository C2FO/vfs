package sqlfs

import (
	"context"
	"errors"
	"path"
	"regexp"
	"strings"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
	"github.com/c2fo/vfs/v7/utils/authority"
)

var errLocationRequired = errors.New("non-nil sqlfs.Location pointer is required")

// Location implements vfs.Location. Directories are virtual: a location is a namespace plus a path prefix.
type Location struct {
	fileSystem *FileSystem
	path       string
	authority  authority.Authority
}

var _ vfs.Location = (*Location)(nil)

// List returns the names of files directly within the location.
func (l *Location) List() ([]string, error) {
	fsys := l.fileSystem
	ctx, cancel := fsys.opCtx(context.Background())
	defer cancel()

	rows, err := fsys.q.QueryContext(ctx, fsys.qs.list, l.authority.String(), l.path)
	if err != nil {
		return nil, fsys.wrapErr("list "+l.URI(), err)
	}
	defer func() { _ = rows.Close() }()

	names := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fsys.wrapErr("list "+l.URI(), err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fsys.wrapErr("list "+l.URI(), err)
	}
	return names, nil
}

// ListByPrefix returns the names of files whose names begin with prefix. A prefix containing a slash is resolved
// relative to the location, so "sub/pre" lists files beginning with "pre" in the "sub" directory, and "sub/" lists
// every file in it.
//
// The prefix is matched case-sensitively in the application rather than with SQL LIKE, whose case sensitivity and
// wildcard handling differ between databases.
func (l *Location) ListByPrefix(prefix string) ([]string, error) {
	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		// Split by hand: path.Base drops a trailing slash, which would turn "sub/" into a prefix of "sub".
		sub, err := l.NewLocation(prefix[:i+1])
		if err != nil {
			return nil, err
		}
		return sub.ListByPrefix(prefix[i+1:])
	}
	return l.filter(func(n string) bool { return strings.HasPrefix(n, prefix) })
}

// ListByRegex returns the names of files whose names match regex.
func (l *Location) ListByRegex(regex *regexp.Regexp) ([]string, error) {
	return l.filter(regex.MatchString)
}

func (l *Location) filter(keep func(string) bool) ([]string, error) {
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, n := range all {
		if keep(n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// Volume returns the authority as a string.
//
// Deprecated: Use Authority instead.
func (l *Location) Volume() string { return l.Authority().String() }

// Authority returns the namespace authority.
func (l *Location) Authority() authority.Authority { return l.authority }

// Path returns the absolute location path, ending in a slash.
func (l *Location) Path() string { return l.path }

// Exists reports true if the database is reachable and the schema is valid. Directories are virtual, so an empty
// location exists.
func (l *Location) Exists() (bool, error) {
	if err := l.fileSystem.ValidateSchema(context.Background()); err != nil {
		return false, utils.WrapExistsError(err)
	}
	return true, nil
}

// NewLocation returns a location relative to this one.
func (l *Location) NewLocation(relLocPath string) (vfs.Location, error) {
	if l == nil {
		return nil, errLocationRequired
	}
	if relLocPath == "" {
		return nil, errPathRequired
	}
	if err := utils.ValidateRelativeLocationPath(relLocPath); err != nil {
		return nil, err
	}
	return &Location{
		fileSystem: l.fileSystem,
		path:       utils.EnsureTrailingSlash(path.Clean(path.Join(l.path, relLocPath))),
		authority:  l.authority,
	}, nil
}

// ChangeDir changes this location to the given relative path.
//
// Deprecated: Use NewLocation instead.
func (l *Location) ChangeDir(relLocPath string) error {
	newLoc, err := l.NewLocation(relLocPath)
	if err != nil {
		return err
	}
	*l = *newLoc.(*Location) //nolint:forcetypeassert // NewLocation always returns *Location
	return nil
}

// FileSystem returns the filesystem that owns the location.
func (l *Location) FileSystem() vfs.FileSystem { return l.fileSystem }

// NewFile returns a file relative to this location. No I/O is performed.
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
	abs := path.Clean(path.Join(l.path, relFilePath))
	loc, err := l.NewLocation(utils.EnsureTrailingSlash(path.Dir(relFilePath)))
	if err != nil {
		return nil, err
	}
	return &File{location: loc.(*Location), path: abs, opts: opts}, nil //nolint:forcetypeassert // always *Location
}

// DeleteFile deletes the named file relative to the location.
func (l *Location) DeleteFile(relFilePath string, opts ...options.DeleteOption) error {
	f, err := l.NewFile(relFilePath)
	if err != nil {
		return err
	}
	return f.Delete(opts...)
}

// URI returns the location URI, ending in a slash.
func (l *Location) URI() string { return utils.GetLocationURI(l) }

// String implements fmt.Stringer.
func (l *Location) String() string { return l.URI() }
