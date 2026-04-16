package v7adapter

import (
	"context"
	"iter"

	vfsv7 "github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/utils/authority"
	vfsv8 "github.com/c2fo/vfs/v8"
	vfsv8opt "github.com/c2fo/vfs/v8/options"
)

// Location wraps a v7 [vfsv7.Location] as a v8 [vfsv8.Location].
type Location struct {
	inner vfsv7.Location
}

// LocationFromV7 wraps loc. It returns nil if loc is nil.
func LocationFromV7(loc vfsv7.Location) vfsv8.Location {
	if loc == nil {
		return nil
	}
	return &Location{inner: loc}
}

// String delegates to the v7 location.
func (l *Location) String() string {
	return l.inner.String()
}

// URI returns the fully qualified URI for the location.
func (l *Location) URI() string {
	return l.inner.URI()
}

// Path returns the absolute location path.
func (l *Location) Path() string {
	return l.inner.Path()
}

// Authority returns the URI authority.
func (l *Location) Authority() authority.Authority {
	return l.inner.Authority()
}

// List implements [vfsv8.Lister] using v7 List, ListByPrefix, and filters.
func (l *Location) List(ctx context.Context, opts ...vfsv8.ListOption) iter.Seq2[vfsv8.Entry, error] {
	cfg := vfsv8.ApplyListOptions(vfsv8.ListConfig{}, opts...)

	return func(yield func(vfsv8.Entry, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(vfsv8.Entry{}, err)
			return
		}
		if cfg.Recursive {
			yield(vfsv8.Entry{}, vfsv8.ErrNotSupported)
			return
		}

		var names []string
		var err error
		if cfg.Prefix != "" {
			names, err = l.inner.ListByPrefix(cfg.Prefix)
		} else {
			names, err = l.inner.List()
		}
		if err != nil {
			yield(vfsv8.Entry{}, err)
			return
		}

		for _, name := range names {
			if err := ctx.Err(); err != nil {
				yield(vfsv8.Entry{}, err)
				return
			}
			if cfg.Matcher != nil && !cfg.Matcher(name) {
				continue
			}
			if !yield(vfsv8.Entry{Kind: vfsv8.EntryBlob, Name: name}, nil) {
				return
			}
		}
	}
}

// Exists delegates to the v7 location.
func (l *Location) Exists() (bool, error) {
	return l.inner.Exists()
}

// NewLocation delegates to the v7 location and wraps the result.
func (l *Location) NewLocation(rel string) (vfsv8.Location, error) {
	out, err := l.inner.NewLocation(rel)
	if err != nil {
		return nil, err
	}
	return LocationFromV7(out), nil
}

// NewFile delegates to the v7 location without forwarding v8 options to v7 yet.
func (l *Location) NewFile(rel string, _ ...vfsv8opt.NewFileOption) (vfsv8.File, error) {
	f, err := l.inner.NewFile(rel)
	if err != nil {
		return nil, err
	}
	return FileFromV7(f), nil
}

// DeleteFile delegates to the v7 location without forwarding v8 delete options yet.
func (l *Location) DeleteFile(rel string, _ ...vfsv8opt.DeleteOption) error {
	return l.inner.DeleteFile(rel)
}

// FileSystem returns the wrapped v8 file system.
func (l *Location) FileSystem() vfsv8.FileSystem {
	return FileSystemFromV7(l.inner.FileSystem())
}

var _ vfsv8.Location = (*Location)(nil)
