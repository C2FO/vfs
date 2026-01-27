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

	"github.com/c2fo/vfs/v7"
	"github.com/c2fo/vfs/v7/backend"
	"github.com/c2fo/vfs/v7/backend/ftp/types"
	"github.com/c2fo/vfs/v7/options"
	"github.com/c2fo/vfs/v7/utils"
)

var tempFileNameGetter func(string) string
var now = time.Now

func init() {
	// this func is overridable for tests
	tempFileNameGetter = getTempFilename
}

// File implements vfs.File interface for FTP fs.
type File struct {
	location *Location
	path     string
	opts     []options.NewFileOption
	ctx      context.Context
	offset   int64
}

// Info Functions

// LastModified returns the LastModified property of ftp file.
func (f *File) LastModified() (*time.Time, error) {
	entry, err := f.stat(f.ctx)
	if err != nil {
		return nil, utils.WrapLastModifiedError(err)
	}
	return utils.Ptr(entry.Time), nil
}

func (f *File) stat(ctx context.Context) (*_ftp.Entry, error) {
	dc, err := f.location.fileSystem.DataConn(ctx, f.Location().Authority(), types.SingleOp, f)
	if err != nil {
		return nil, err
	}
	// check if MLST command is available - if so we'll want to grab file info
	// via MLST. otherwise we'll need to use LIST.
	if dc.IsTimePreciseInList() {
		entry, err := dc.GetEntry(f.Path())
		if err != nil {
			if strings.HasPrefix(err.Error(), strconv.Itoa(_ftp.StatusFileUnavailable)) {
				return nil, os.ErrNotExist
			}
			return nil, err
		}
		return entry, nil
	} else {
		entries, err := dc.List(f.Path())
		if err != nil {
			if strings.HasPrefix(err.Error(), strconv.Itoa(_ftp.StatusFileUnavailable)) {
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
	_, err := f.stat(f.ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// file does not exist
			return false, nil
		}
		// error calling stat
		return false, utils.WrapExistsError(err)
	}

	// file exists
	return true, nil
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	exists, err := f.Exists()
	if err != nil {
		return utils.WrapTouchError(err)
	}

	if !exists {
		_, err := f.Write([]byte{})
		if err != nil {
			return utils.WrapTouchError(err)
		}
		return f.Close()
	}

	// if a set time function is available use that to set last modified to now
	dc, err := f.location.fileSystem.DataConn(f.ctx, f.Location().Authority(), types.SingleOp, f)
	if err != nil {
		return utils.WrapTouchError(err)
	}
	if dc.IsSetTimeSupported() {
		return dc.SetTime(f.path, time.Now())
	}

	// doing move and move back to ensure last modified is updated
	newFile, err := f.Location().NewFile(tempFileNameGetter(f.Name()))
	if err != nil {
		return utils.WrapTouchError(err)
	}

	err = f.MoveToFile(newFile)
	if err != nil {
		return utils.WrapTouchError(err)
	}

	return newFile.MoveToFile(f)
}

func getTempFilename(origName string) string {
	return origName + strconv.FormatInt(now().UnixNano(), 10)
}

// Size returns the size of the remote file.
func (f *File) Size() (uint64, error) {
	entry, err := f.stat(f.ctx)
	if err != nil {
		return 0, utils.WrapSizeError(err)
	}
	return entry.Size, nil
}

// Location returns a vfs.Location at the location of the file. IE: if file is at
// ftp://someuser@host.com/here/is/the/file.txt the location points to ftp://someuser@host.com/here/is/the/
func (f *File) Location() vfs.Location {
	return f.location
}

// Move/Copy Operations

// MoveToFile puts the contents of File into the targetFile passed using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
// If the given location is also ftp AND for the same user and host, the ftp Rename method is used, otherwise
// we'll do an io.Copy to the destination file then delete source file.
func (f *File) MoveToFile(t vfs.File) error {
	// ftp rename if vfs is ftp and for the same user/host
	if f.Location().FileSystem().Scheme() == t.Location().FileSystem().Scheme() &&
		f.Location().Authority().UserInfo().Username() == t.Location().Authority().UserInfo().Username() &&
		f.Location().Authority().HostPortStr() == t.Location().Authority().HostPortStr() {
		// ensure destination exists before moving
		exists, err := t.Location().Exists()
		if err != nil {
			return utils.WrapMoveToFileError(err)
		}
		dc, err := f.location.fileSystem.DataConn(f.ctx, f.Location().Authority(), types.SingleOp, f)
		if err != nil {
			return utils.WrapMoveToFileError(err)
		}
		if !exists {
			// it doesn't matter which client we use since they are effectively the same
			err = dc.MakeDir(t.Location().Path())
			if err != nil {
				return utils.WrapMoveToFileError(fmt.Errorf("failed to create directory: %w", err))
			}
		}
		return dc.Rename(f.Path(), t.Path())
	}

	// otherwise do copy-delete
	if err := f.CopyToFile(t); err != nil {
		return utils.WrapMoveToFileError(err)
	}
	return f.Delete()
}

// MoveToLocation works by creating a new file on the target location then calling MoveToFile() on it.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
	}

	err = f.MoveToFile(newFile)
	if err != nil {
		return nil, utils.WrapMoveToLocationError(err)
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
				err = utils.WrapCopyToFileError(wErr)
			} else if rErr != nil {
				err = utils.WrapCopyToFileError(rErr)
			}
		}
	}()

	if err := backend.ValidateCopySeekPosition(f); err != nil {
		return utils.WrapCopyToFileError(err)
	}

	if f.Location().FileSystem().Scheme() == file.Location().FileSystem().Scheme() &&
		f.Location().Authority().UserInfo().Username() == file.Location().Authority().UserInfo().Username() &&
		f.Location().Authority().HostPortStr() == file.Location().Authority().HostPortStr() {
		// in the case that both files have the same authority we'll copy by writing a temporary
		// file to mem and then writing it back to the ftp server
		tempFile, err := f.createLocalTempFile()
		if err != nil {
			return utils.WrapCopyToFileError(err)
		}
		defer func() {
			_ = os.Remove(tempFile.Name())
		}()
		if err := utils.TouchCopyBuffered(tempFile, f, 0); err != nil {
			return utils.WrapCopyToFileError(err)
		}
		// validate seek is at 0,0 before doing copy
		_, err = tempFile.Seek(0, io.SeekStart)
		if err != nil {
			return utils.WrapCopyToFileError(fmt.Errorf("failed to determine current cursor offset: %w", err))
		}

		if err := f.Close(); err != nil {
			return utils.WrapCopyToFileError(err)
		}
		if err := utils.TouchCopyBuffered(file, tempFile, 0); err != nil {
			return utils.WrapCopyToFileError(err)
		}
		if err := tempFile.Close(); err != nil {
			return utils.WrapCopyToFileError(err)
		}

		return nil
	} else {
		if err := utils.TouchCopyBuffered(file, f, 0); err != nil {
			return utils.WrapCopyToFileError(err)
		}
		// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
		if cerr := file.Close(); cerr != nil {
			return utils.WrapCopyToFileError(cerr)
		}

		return nil
	}
}

// CopyToLocation creates a copy of *File, using the file's current path as the new file's
// path at the given location.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {
	newFile, err := location.NewFile(f.Name())
	if err != nil {
		return nil, utils.WrapCopyToLocationError(err)
	}

	return newFile, f.CopyToFile(newFile)
}

// CRUD Operations

// Delete removes the remote file.  Error is returned, if any.
func (f *File) Delete(_ ...options.DeleteOption) error {
	dc, err := f.location.fileSystem.DataConn(f.ctx, f.Location().Authority(), types.SingleOp, f)
	if err != nil {
		return utils.WrapDeleteError(err)
	}
	return dc.Delete(f.Path())
}

// Close calls the underlying ftp.Response Close, if opened, and clears the internal pointer
func (f *File) Close() error {
	if f.location.fileSystem.dataconn != nil {
		err := f.location.fileSystem.dataconn.Close()
		if err != nil {
			return utils.WrapCloseError(err)
		}
		f.location.fileSystem.resetConn = true
	}
	// no op for unopened file
	f.offset = 0
	return nil
}

// Read calls the underlying ftp.File Read.
func (f *File) Read(p []byte) (n int, err error) {
	dc, err := f.location.fileSystem.DataConn(f.ctx, f.Location().Authority(), types.OpenRead, f)
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
	if f.location.fileSystem.dataconn == nil {
		f.offset = offset
	} else {
		mode = f.location.fileSystem.dataconn.Mode()

		switch whence {
		case 0: // offset from the beginning of the file (position 0)
			f.offset = offset
		case 1: // offset relative to current position
			if f.offset < 0 {
				f.offset = 0
			}
			f.offset += offset

			// close dataconn so that it reset the offset on next reopen (in StorFrom or RetrFrom)
			err := f.location.fileSystem.dataconn.Close()
			if err != nil {
				return 0, utils.WrapSeekError(err)
			}
			f.location.fileSystem.resetConn = true
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
			err = f.location.fileSystem.dataconn.Close()
			if err != nil {
				return 0, utils.WrapSeekError(err)
			}
			f.location.fileSystem.resetConn = true
		}
	}

	// now that f.offset has been adjusted and mode was captured, reinitialize file
	_, err = f.location.fileSystem.DataConn(f.ctx, f.Location().Authority(), mode, f)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	// return new offset from beginning of file
	return f.offset, nil
}

// Write calls the underlying ftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {
	dc, err := f.location.fileSystem.DataConn(f.ctx, f.Location().Authority(), types.OpenWrite, f)
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
	return utils.EncodeURI(
		f.location.fileSystem.Scheme(),
		f.location.Authority().UserInfo().Username(),
		f.location.Authority().HostPortStr(),
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
