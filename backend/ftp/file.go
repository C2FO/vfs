package ftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	_ftp "github.com/jlaffaye/ftp"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/backend"
	"github.com/c2fo/vfs/v6/backend/ftp/types"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

var tempFileNameGetter func(string) string
var now = time.Now

func init() {
	// this func is overridable for tests
	tempFileNameGetter = getTempFilename
}

// File implements vfs.File interface for FTP fs.
type File struct {
	fileSystem *FileSystem
	authority  utils.Authority
	path       string
	offset     int64
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
	dc, err := f.fileSystem.DataConn(ctx, f.authority, types.SingleOp, f)
	if err != nil {
		return nil, err
	}
	// check if MLSD command is available - if so we'll want to grab file info
	// via MLST. otherwise we'll need to use LIST.
	if dc.IsTimePreciseInList() {
		entry, err := dc.GetEntry(f.Path())
		if err != nil {
			if strings.HasPrefix(err.Error(), fmt.Sprintf("%d", _ftp.StatusFileUnavailable)) {
				return nil, os.ErrNotExist
			}
			return nil, err
		}
		return entry, nil
	} else {
		entries, err := dc.List(f.Path())
		if err != nil {
			if strings.HasPrefix(err.Error(), fmt.Sprintf("%d", _ftp.StatusFileUnavailable)) {
				return nil, os.ErrNotExist
			}
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

// Path return the directory portion of the file's path. IE: "/path/to/" of "ftp://someuser@host.com/some/path/to/file.txt
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

	// if a set time function is available use that to set last modified to now
	dc, err := f.fileSystem.DataConn(context.TODO(), f.authority, types.SingleOp, f)
	if err != nil {
		return err
	}
	if dc.IsSetTimeSupported() {
		return dc.SetTime(f.path, time.Now())
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
// we'll do an io.Copy to the destination file then delete source file.
func (f *File) MoveToFile(t vfs.File) error {
	// ftp rename if vfs is ftp and for the same user/host
	if f.fileSystem.Scheme() == t.Location().FileSystem().Scheme() &&
		f.authority.UserInfo().Username() == t.(*File).authority.UserInfo().Username() &&
		f.authority.HostPortStr() == t.(*File).authority.HostPortStr() {

		// ensure destination exists before moving
		exists, err := t.Location().Exists()
		if err != nil {
			return err
		}
		dc, err := f.fileSystem.DataConn(context.TODO(), f.authority, types.SingleOp, f)
		if err != nil {
			return err
		}
		if !exists {
			// it doesn't matter which client we use since they are effectively the same
			err = dc.MakeDir(t.Location().Path())
			if err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}
		return dc.Rename(f.Path(), t.Path())
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
func (f *File) CopyToFile(file vfs.File) (err error) { //nolint:gocyclo
	// Close file (f) reader regardless of an error
	defer func() {
		// close writer
		wErr := file.Close()
		// close reader
		rErr := f.Close()
		//
		if err == nil {
			if wErr != nil {
				err = wErr
			} else if rErr != nil {
				err = rErr
			}
		}
	}()

	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return err
	}

	if f.fileSystem.Scheme() == file.Location().FileSystem().Scheme() &&
		f.authority.UserInfo().Username() == file.(*File).authority.UserInfo().Username() &&
		f.authority.HostPortStr() == file.(*File).authority.HostPortStr() {
		// in the case that both files have the same authority we'll copy by writing a temporary
		// file to mem and then writing it back to the ftp server
		tempFile, err := f.createLocalTempFile()
		if err != nil {
			return err
		}
		defer func() {
			_ = os.Remove(tempFile.Name())
		}()
		if err := utils.TouchCopyBuffered(tempFile, f, 0); err != nil {
			return err
		}
		// validate seek is at 0,0 before doing copy
		_, err = tempFile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to determine current cursor offset: %w", err)
		}

		if err := f.Close(); err != nil {
			return err
		}
		if err := utils.TouchCopyBuffered(file, tempFile, 0); err != nil {
			return err
		}
		if err := tempFile.Close(); err != nil {
			return err
		}

		return nil
	} else {
		if err := utils.TouchCopyBuffered(file, f, 0); err != nil {
			return err
		}
		// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
		if cerr := file.Close(); cerr != nil {
			return cerr
		}

		return err
	}

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
	dc, err := f.fileSystem.DataConn(context.TODO(), f.authority, types.SingleOp, f)
	if err != nil {
		return err
	}
	return dc.Delete(f.Path())
}

// Close calls the underlying ftp.Response Close, if opened, and clears the internal pointer
func (f *File) Close() error {
	if f.fileSystem.dataconn != nil {
		err := f.fileSystem.dataconn.Close()
		if err != nil {
			return utils.WrapCloseError(err)
		}
		f.fileSystem.resetConn = true
	}
	// no op for unopened file
	f.offset = 0
	return nil
}

// Read calls the underlying ftp.File Read.
func (f *File) Read(p []byte) (n int, err error) {
	dc, err := f.fileSystem.DataConn(context.TODO(), f.authority, types.OpenRead, f)
	if err != nil {
		return 0, utils.WrapReadError(err)
	}

	read, err := dc.Read(p)
	if err != nil {
		// if we got io.EOF, we'll return the read and the EOF error
		// because io.Copy looks for EOF to determine if it's done
		// and doesn't support error wrapping
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	f.offset += int64(read)

	return read, nil
}

// Seek calls the underlying ftp.File Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// ensure file exists before seeking
	exists, err := f.Exists()
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}
	if !exists {
		return 0, utils.WrapSeekError(os.ErrNotExist)
	}

	mode := types.OpenRead
	// no file open yet - assume read (will get reset to write on a subsequent write)
	if f.fileSystem.dataconn == nil {
		f.offset = offset
	} else {
		mode = f.fileSystem.dataconn.Mode()

		switch whence {
		case 0: // offset from the beginning of the file (position 0)
			f.offset = offset
		case 1: // offset relative to current position
			if f.offset < 0 {
				f.offset = 0
			}
			f.offset += offset

			// close dataconn so that it reset the offset on next reopen (in StorFrom or RetrFrom)
			err := f.fileSystem.dataconn.Close()
			if err != nil {
				return 0, utils.WrapSeekError(err)
			}
			f.fileSystem.resetConn = true
		case 2: // offset from end of the file
			sz, err := f.Size()
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return 0, utils.WrapSeekError(err)
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
			err = f.fileSystem.dataconn.Close()
			if err != nil {
				return 0, utils.WrapSeekError(err)
			}
			f.fileSystem.resetConn = true
		}
	}

	// now that f.offset has been adjusted and mode was captured, reinitialize file
	_, err = f.fileSystem.DataConn(context.TODO(), f.authority, mode, f)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	// return new offset from beginning of file
	return f.offset, nil
}

// Write calls the underlying ftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {
	dc, err := f.fileSystem.DataConn(context.TODO(), f.authority, types.OpenWrite, f)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	b, err := dc.Write(data)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	offset := int64(b)
	f.offset += offset

	return b, nil
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	loc := f.Location().(*Location)
	return utils.EncodeURI(
		f.fileSystem.Scheme(),
		loc.Authority.UserInfo().Username(),
		loc.Authority.HostPortStr(),
		f.Path(),
	)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}

func (f *File) createLocalTempFile() (*os.File, error) {
	// Create temp file
	return os.CreateTemp("", fmt.Sprintf("%s.%d", f.Name(), time.Now().UnixNano()))
}
