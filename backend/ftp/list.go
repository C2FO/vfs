package ftp

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"iter"
	"net"
	"net/textproto"
	"path"
	"strconv"
	"strings"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/list"
	"github.com/c2fo/vfs/v7/options"
)

// ListEntries lists the location's contents as a stream of entries in byte order, implementing vfs.EntryLister. Use
// list.Entries to list a location with options and have every option applied.
//
// Each directory is read completely with one FTP listing when the listing reaches it, before any of its entries is
// yielded. The FTP connection is therefore free while the caller works on an entry, so a caller may read, write or
// delete files while it is listing, or stop listing and carry on. A FileSystem shares one FTP connection between
// listings and transfers, so a file opened for reading or writing must be closed before the listing advances to its
// next directory; a transfer left open across iterations is ended by the next directory read. Memory use is one
// directory per level of depth, and a listing that stops early reads no further directories.
//
// The size and modification time of each file come from the listing, so reading them makes no further request. An FTP
// server reports modification times to the minute when it does not support MLSD, and a time in the listing can then
// differ from File.LastModified by up to a minute. A symbolic link is listed as a file entry whose Type is
// fs.ModeSymlink, and is not followed. FTP listings carry no permission bits, so Mode reports only the type.
//
// A directory that cannot be read is yielded with its entry and an error, and the listing continues. FTP servers
// answer 550 both for a directory that does not exist and for one that cannot be entered, so a 550 is reported as
// matching fs.ErrNotExist, and a location that the server cannot enter lists nothing.
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

// readDir lists one directory of the location, holding the connection only for the listing.
func (l *Location) readDir(ctx context.Context, relDir string) ([]list.Child, error) {
	dc, err := l.fileSystem.DataConn(ctx, l.Authority(), types.SingleOp, nil)
	if err != nil {
		return nil, err
	}

	entries, err := dc.List(path.Join(l.Path(), relDir))
	if err != nil {
		if strings.HasPrefix(err.Error(), strconv.Itoa(_ftp.StatusFileUnavailable)) {
			return nil, fmt.Errorf("%w: %w", iofs.ErrNotExist, err)
		}
		if sessionLost(err) {
			// The control connection is gone; list.WalkDirs ends the listing instead of failing every remaining directory.
			return nil, fmt.Errorf("%w: %w", net.ErrClosed, err)
		}
		return nil, err
	}

	children := make([]list.Child, 0, len(entries))
	for _, e := range entries {
		switch e.Type {
		case _ftp.EntryTypeFolder:
			children = append(children, list.Child{Name: e.Name, IsDir: true})
		case _ftp.EntryTypeLink:
			children = append(children, list.Child{Name: e.Name, Options: []vfs.EntryOption{
				vfs.WithEntryMetadata(e.Size, e.Time),
				vfs.WithEntryMode(iofs.ModeSymlink),
				vfs.WithEntrySys(e),
			}})
		default:
			children = append(children, list.Child{Name: e.Name, Options: []vfs.EntryOption{
				vfs.WithEntryMetadata(e.Size, e.Time),
				vfs.WithEntryMode(0),
				vfs.WithEntrySys(e),
			}})
		}
	}
	return children, nil
}

// sessionLost reports whether err is the server closing the control connection or refusing the session, which would
// fail every further request the same way: service not available (421), data connection failures (425, 426) and the
// refusal of credentials (430, 434).
func sessionLost(err error) bool {
	var perr *textproto.Error
	if !errors.As(err, &perr) {
		return false
	}
	switch perr.Code {
	case _ftp.StatusNotAvailable, _ftp.StatusCanNotOpenDataConnection, _ftp.StatusTransfertAborted,
		_ftp.StatusInvalidCredentials, _ftp.StatusHostUnavailable:
		return true
	}
	return false
}
