package sftp

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"net"
	"path"

	_sftp "github.com/pkg/sftp"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Each directory is read with one SFTP ReadDir when the listing reaches it, so memory use is one directory per level of
// depth, and a listing that stops early reads no further directories. The connection is taken for each directory read
// and released before the listing yields, so a caller that works slowly between entries does not hold the connection
// open or keep its idle timer from running. The size, modification time and mode of every file come from the directory
// read, so reading them makes no further request. A symbolic link is listed as a file entry whose Type is
// fs.ModeSymlink, and is not followed, so a link to a directory is not descended into. Entry.Sys returns the
// os.FileInfo the server reported.
//
// A directory that cannot be read is yielded with its entry and an error, and the listing continues; a permission
// failure matches fs.ErrPermission. A location that does not exist lists nothing.
func (l *Location) ListEntries(ctx context.Context, opts ...options.ListOption) iter.Seq2[vfs.Entry, error] {
	return func(yield func(vfs.Entry, error) bool) {
		q, err := list.ParseQuery(opts)
		if err != nil {
			yield(vfs.Entry{}, &iofs.PathError{Op: "list", Path: l.Path(), Err: err})
			return
		}
		list.WalkDirs(ctx, l, q, l.readDir, nil)(yield)
	}
}

// readDir reads one directory of the location, taking the connection only for the read.
func (l *Location) readDir(_ context.Context, relDir string) ([]list.Child, error) {
	client, err := l.fileSystem.Client(l.Authority())
	if err != nil {
		return nil, err
	}
	defer l.fileSystem.connTimerStart()

	infos, err := client.ReadDir(path.Join(l.Path(), relDir))
	if err != nil {
		if errors.Is(err, _sftp.ErrSSHFxConnectionLost) {
			// The session is gone; list.WalkDirs ends the listing instead of failing every remaining directory.
			return nil, fmt.Errorf("%w: %w", net.ErrClosed, err)
		}
		return nil, err
	}

	children := make([]list.Child, 0, len(infos))
	for _, info := range infos {
		if info.IsDir() {
			children = append(children, list.Child{Name: info.Name(), IsDir: true})
			continue
		}
		children = append(children, list.Child{Name: info.Name(), Options: []vfs.EntryOption{
			vfs.WithEntryMetadata(uint64(max(info.Size(), 0)), info.ModTime()),
			vfs.WithEntryMode(info.Mode()),
			vfs.WithEntrySys(info),
		}})
	}
	return children, nil
}
