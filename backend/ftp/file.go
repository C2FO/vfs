package ftp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

var dataConnGetterFunc func(context.Context, *File, types.OpenType) (types.DataConn, error)
var tempFileNameGetter func(string) string
var now = time.Now

func init() {
	// this func is overridable for tests
	dataConnGetterFunc = getDataConn
	tempFileNameGetter = getTempFilename
}

// File implements vfs.File interface for FTP fs.
type File struct {
	fileSystem *FileSystem
	authority  utils.Authority
	path       string
	dataconn   types.DataConn
	offset     int64
	resetConn  bool
}

// Info Functions

// LastModified returns the LastModified property of ftp file.
func (f *File) LastModified() (*time.Time, error) {

	entry, err := f.stat(context.TODO())
	if err != nil {
		return nil, err
	}
	t := entry.Time
	return &t, nil
}

func (f *File) stat(ctx context.Context) (*_ftp.Entry, error) {
	client, err := f.fileSystem.Client(ctx, f.authority)
	if err != nil {
		return nil, err
	}
	// check if MLSD command is availalbe - if so we'll want to grab file info
	// via MLST. otherwise we'll need to use LIST.
	if client.IsTimePreciseInList() {
		entry, err := client.GetEntry(f.Path())
		if err != nil {
			if strings.HasPrefix(err.Error(), fmt.Sprintf("%d", _ftp.StatusFileUnavailable)) {
				return nil, os.ErrNotExist
			}
			return nil, err
		}
		return entry, nil
	} else {
		entries, err := client.List(f.Path())
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			return nil, os.ErrNotExist
		}
		return entries[0], nil
	}
}

// Name returns the path portion of the file's path property. IE: "file.txt" of "ftp://someuser@host.com/some/path/to/file.txt
func (f *File) Name() string {
	return path.Base(f.path)
}

// Path return the directory portion of the file's path. IE: "path/to" of "ftp://someuser@host.com/some/path/to/file.txt
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.path)
}

// Exists returns a boolean of whether or not the file exists on the ftp server
func (f *File) Exists() (bool, error) {
	_, err := f.stat(context.TODO())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// file does not exist
			return false, nil
		}
		// error calling stat
		return false, err
	}

	// file exists
	return true, nil
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	exists, err := f.Exists()
	if err != nil {
		return err
	}

	if !exists {
		_, err := f.Write([]byte{})
		if err != nil {
			return err
		}
		return f.Close()
	}

	client, err := f.fileSystem.Client(context.TODO(), f.authority)
	if err != nil {
		return err
	}

	// if a set time function is available use that to set last modified to now
	if client.IsSetTimeSupported() {
		return client.SetTime(f.path, time.Now())
	}

	// doing move and move back to ensure last modified is updated
	newFile, err := f.Location().NewFile(tempFileNameGetter(f.Name()))
	if err != nil {
		return err
	}

	err = f.MoveToFile(newFile)
	if err != nil {
		return err
	}

	return newFile.MoveToFile(f)
}

func getTempFilename(origName string) string {
	return origName + strconv.FormatInt(now().UnixNano(), 10)
}

// Size returns the size of the remote file.
func (f *File) Size() (uint64, error) {

	entry, err := f.stat(context.TODO())
	if err != nil {
		return 0, err
	}
	return entry.Size, nil
}

// Location returns a vfs.Location at the location of the file. IE: if file is at
// ftp://someuser@host.com/here/is/the/file.txt the location points to ftp://someuser@host.com/here/is/the/
func (f *File) Location() vfs.Location {
	return &Location{
		fileSystem: f.fileSystem,
		path:       path.Dir(f.path),
		Authority:  f.authority,
	}
}

// Move/Copy Operations

// MoveToFile puts the contents of File into the targetFile passed using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
// If the given location is also ftp AND for the same user and host, the ftp Rename method is used, otherwise
// we'll do a an io.Copy to the destination file then delete source file.
func (f *File) MoveToFile(t vfs.File) error {
	// ftp rename if vfs is ftp and for the same user/host
	if f.fileSystem.Scheme() == t.Location().FileSystem().Scheme() &&
		f.authority.UserInfo().Username() == t.(*File).authority.UserInfo().Username() &&
		f.authority.Host() == t.(*File).authority.Host() {

		client, err := f.fileSystem.Client(context.TODO(), f.authority)
		if err != nil {
			return err
		}

		// ensure destination exists before moving
		exists, err := t.Location().Exists()
		if err != nil {
			return err
		}

		if !exists {
			// it doesn't matter which client we use since they are effectively the same
			err = client.MakeDir(t.Location().Path())
			if err != nil {
				return err
			}
		}
		return client.Rename(f.Path(), t.Path())
	}

	// otherwise do copy-delete
	if err := f.CopyToFile(t); err != nil {
		return err
	}
	return f.Delete()
}

// MoveToLocation works by creating a new file on the target location then calling MoveToFile() on it.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}

	err = f.MoveToFile(newFile)
	if err != nil {
		return nil, err
	}
	return newFile, nil
}

// CopyToFile puts the contents of File into the targetFile passed.
func (f *File) CopyToFile(file vfs.File) error {

	if err := utils.TouchCopyBuffered(file, f, 0); err != nil {
		return err
	}
	// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := file.Close(); cerr != nil {
		return cerr
	}
	// Close file (f) reader
	return f.Close()
}

// CopyToLocation creates a copy of *File, using the file's current path as the new file's
// path at the given location.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, err
	}

	return newFile, f.CopyToFile(newFile)
}

// CRUD Operations

// Delete removes the remote file.  Error is returned, if any.
func (f *File) Delete(_ ...options.DeleteOption) error {
	client, err := f.fileSystem.Client(context.TODO(), f.authority)
	if err != nil {
		return err
	}
	return client.Delete(f.Path())
}

// Close calls the underlying ftp.Response Close, if opened, and clears the internal pointer
func (f *File) Close() error {
	if f.dataconn != nil {
		err := f.dataconn.Close()
		if err != nil {
			return err
		}
		f.resetConn = true
	}
	// no op for unopened file
	f.offset = 0
	return nil
}

// Read calls the underlying ftp.File Read.
func (f *File) Read(p []byte) (n int, err error) {
	dc, err := dataConnGetterFunc(context.TODO(), f, types.OpenRead)
	if err != nil {
		return 0, err
	}

	read, err := dc.Read(p)
	if err != nil {
		return read, err
	}

	f.offset += int64(read)

	return read, nil
}

// Seek calls the underlying ftp.File Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {

	mode := types.OpenRead
	// no file open yet - assume read (will get reset to write on a subsequent write)
	if f.dataconn == nil {
		f.offset = offset
	} else {
		mode = f.dataconn.Mode()

		switch whence {
		case 0: // offset from beginning of the file (position 0)
			f.offset = offset
		case 1: // offset relative to current position
			if f.offset < 0 {
				f.offset = 0
			}
			f.offset += offset

			// close dataconn so that it reset the offset on next reopen (in StorFrom or RetrFrom)
			err := f.dataconn.Close()
			if err != nil {
				return 0, err
			}
			f.resetConn = true
		case 2: // offset from end of the file
			sz, err := f.Size()
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return 0, err
				}
				// file doesn't exist, just use 0 as offset
				f.offset = 0
			} else {
				f.offset = int64(sz) - offset
				if f.offset < 0 {
					f.offset = 0
				}
			}

			// close dataconn so that it reset the offset on next reopen (in StorFrom or RetrFrom)
			err = f.dataconn.Close()
			if err != nil {
				return 0, err
			}
			f.resetConn = true
		}
	}

	// now that f.offset has been adjusted and mode was captured, reinitialize file
	_, err := dataConnGetterFunc(context.TODO(), f, mode)
	if err != nil {
		return 0, err
	}

	// return new offset from beginning of file
	return f.offset, nil
}

// Write calls the underlying ftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {

	dc, err := dataConnGetterFunc(context.TODO(), f, types.OpenWrite)
	if err != nil {
		return 0, err
	}

	b, err := dc.Write(data)
	if err != nil {
		return 0, err
	}

	offset := int64(b)
	f.offset += offset

	return b, nil
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}
