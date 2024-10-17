package sftp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/options"
	"github.com/c2fo/vfs/v6/utils"
)

// File implements vfs.File interface for SFTP fs.
type File struct {
	fileSystem *FileSystem
	Authority  utils.Authority
	path       string
	sftpfile   ReadWriteSeekCloser
	opener     fileOpener
	seekCalled bool
	readCalled bool
	flagsUsed  int
}

// this type allows for injecting a mock fileOpener function
type fileOpener func(c Client, p string, f int) (ReadWriteSeekCloser, error)

// Info Functions

// LastModified returns the LastModified property of sftp file.
func (f *File) LastModified() (*time.Time, error) {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return nil, err
	}
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()

	userinfo, err := client.Stat(f.Path())
	if err != nil {
		return nil, err
	}
	t := userinfo.ModTime()
	return &t, nil
}

// Name returns the path portion of the file's path property. IE: "file.txt" of "sftp://someuser@host.com/some/path/to/file.txt
func (f *File) Name() string {
	return path.Base(f.path)
}

// Path returns the directory portion of the file's path. IE: "path/to" of "sftp://someuser@host.com/some/path/to/file.txt
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.path)
}

// Exists returns a boolean of whether the file exists on the sftp server
func (f *File) Exists() (bool, error) {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return false, err
	}
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()

	_, err = client.Stat(f.Path())
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// Touch creates a zero-length file on the vfs.File if no File exists.  Update File's last modified timestamp.
// Returns error if unable to touch File.
func (f *File) Touch() error {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	exists, err := f.Exists()
	if err != nil {
		return err
	}

	if !exists {
		file, err := f.openFile(os.O_WRONLY | os.O_CREATE)
		if err != nil {
			return err
		}
		f.sftpfile = file
		return f.Close()
	}

	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}

	// set permissions if default permissions are set
	err = f.setPermissions(client, f.fileSystem.options)
	if err != nil {
		return err
	}

	// update last accessed and last modified times
	now := time.Now()
	return client.Chtimes(f.Path(), now, now)
}

// Size returns the size of the remote file.
func (f *File) Size() (uint64, error) {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return 0, err
	}
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()

	userinfo, err := client.Stat(f.Path())
	if err != nil {
		return 0, err
	}
	return uint64(userinfo.Size()), nil
}

// Location returns a vfs.Location at the location of the file. IE: if file is at
// sftp://someuser@host.com/here/is/the/file.txt the location points to sftp://someuser@host.com/here/is/the/
func (f *File) Location() vfs.Location {
	return &Location{
		fileSystem: f.fileSystem,
		path:       path.Dir(f.path),
		Authority:  f.Authority,
	}
}

// Move/Copy Operations

// MoveToFile puts the contents of File into the targetFile passed using File.CopyToFile.
// If the copy succeeds, the source file is deleted. Any errors from the copy or delete are
// returned.
// If the given location is also sftp AND for the same user and host, the sftp Rename method is used, otherwise
// we'll do an io.Copy to the destination file then delete source file.
func (f *File) MoveToFile(t vfs.File) error {
	// sftp rename if vfs is sftp and for the same user/host
	if f.fileSystem.Scheme() == t.Location().FileSystem().Scheme() &&
		f.Authority.UserInfo().Username() == t.(*File).Authority.UserInfo().Username() &&
		f.Authority.HostPortStr() == t.(*File).Authority.HostPortStr() {
		// ensure destination exists before moving
		exists, err := t.Location().Exists()
		if err != nil {
			return err
		}
		if !exists {
			// it doesn't matter which client we use since they are effectively the same
			client, err := f.fileSystem.Client(f.Authority)
			if err != nil {
				return err
			}
			// start timer once action is completed
			defer f.fileSystem.connTimerStart()

			err = client.MkdirAll(t.Location().Path())
			if err != nil {
				return err
			}
		}

		// check if file already exists in the destination and delete if exists
		exists, err = t.Exists()
		if err != nil {
			return err
		}
		if exists {
			err := t.Delete()
			if err != nil {
				return err
			}
		}

		return f.sftpRename(t.(*File))
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

	return newFile, f.MoveToFile(newFile)
}

// CopyToFile puts the contents of File into the targetFile passed.
func (f *File) CopyToFile(file vfs.File) (err error) {
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

	fileBufferSize := 0

	if opts, ok := f.Location().FileSystem().(*FileSystem).options.(Options); ok {
		fileBufferSize = opts.FileBufferSize
	}

	if err := utils.TouchCopyBuffered(file, f, fileBufferSize); err != nil {
		return err
	}

	// Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := file.Close(); cerr != nil {
		return cerr
	}

	return err
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
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()

	return client.Remove(f.Path())
}

// Close calls the underlying sftp.File Close, if opened, and clears the internal pointer
func (f *File) Close() error {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	f.seekCalled = false
	f.readCalled = false

	if f.sftpfile != nil {
		err := f.sftpfile.Close()
		if err != nil {
			return utils.WrapCloseError(err)
		}
		f.sftpfile = nil
	}

	// no op for unopened file
	return nil
}

// Read calls the underlying sftp.File Read.
func (f *File) Read(p []byte) (n int, err error) {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	sftpfile, err := f.openFile(os.O_RDONLY)
	if err != nil {
		return 0, utils.WrapReadError(err)
	}

	f.readCalled = true

	read, err := sftpfile.Read(p)
	if err != nil {
		// if we got io.EOF, we'll return the read and the EOF error
		// because io.Copy looks for EOF to determine if it's done
		// and doesn't support error wrapping
		if errors.Is(err, io.EOF) {
			return read, io.EOF
		}
		return read, utils.WrapReadError(err)
	}

	return read, nil
}

// Seek calls the underlying sftp.File Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	sftpfile, err := f.openFile(os.O_RDONLY)
	if err != nil {
		return 0, utils.WrapSeekError(err)
	}

	f.seekCalled = true
	pos, err := sftpfile.Seek(offset, whence)
	if err != nil {
		return pos, utils.WrapSeekError(err)
	}

	return pos, nil
}

// Write calls the underlying sftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	// unless seek or read is called first, writes should replace a file (not edit)
	// writes should edit a file if seek or read is called first
	flags := os.O_WRONLY | os.O_CREATE
	if !f.readCalled && !f.seekCalled {
		flags |= os.O_TRUNC
	}

	sftpfile, err := f.openFile(flags)
	if err != nil {
		return 0, utils.WrapWriteError(err)
	}

	b, err := sftpfile.Write(data)
	if err != nil {
		return b, utils.WrapWriteError(err)
	}
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

/*
	Private helper functions
*/

// openFile wrapper allows us to inject a file opener (for mocking) vs the defaultOpenFile.
func (f *File) openFile(flags int) (ReadWriteSeekCloser, error) { //nolint:gocyclo // this function is complex by nature
	if f.sftpfile != nil {
		// this case shouldn't normally exist except when we've set our own ReadWriteSeekCloser in tests
		if f.flagsUsed == 0 && !f.readCalled && !f.seekCalled {
			return f.sftpfile, nil
		}

		needRw := false
		// check if the file is not already open in read-write mode
		if (f.flagsUsed & os.O_RDWR) == 0 {
			// check if currently open for read only but now need write
			if f.flagsUsed == os.O_RDONLY && (flags&(os.O_WRONLY|os.O_RDWR)) != 0 {
				needRw = true
			}

			// check if currently open for write only but now need read
			if (f.flagsUsed&(os.O_WRONLY|os.O_RDWR)) != 0 && flags == os.O_RDONLY {
				needRw = true
			}

			// set up new flags
			newFlags := os.O_RDWR
			if flags&os.O_CREATE != 0 {
				newFlags |= os.O_CREATE
			}
			if flags&os.O_TRUNC != 0 {
				newFlags |= os.O_TRUNC
			}

			flags = newFlags
		}

		// if we're trying to open a file for writing and it's already open for read, reopen it for read/write and
		// seek to current position
		if needRw {
			var pos int64

			// capture current position if file is open for read (only in edit mode)
			if f.readCalled || f.seekCalled {
				var err error
				// get current position
				pos, err = f.sftpfile.Seek(0, io.SeekCurrent)
				if err != nil {
					return nil, err
				}
			}

			// close file
			if err := f.sftpfile.Close(); err != nil {
				return nil, err
			}

			// reopen file for read/write
			file, err := f._open(flags)
			if err != nil {
				return nil, err
			}

			// seek to current position (only in edit mode)
			if f.readCalled || f.seekCalled {
				if _, err := file.Seek(pos, io.SeekStart); err != nil {
					return nil, err
				}
			}

			f.flagsUsed = flags
			f.sftpfile = file

		}
		return f.sftpfile, nil
	}

	file, err := f._open(flags)
	if err != nil {
		return nil, err
	}

	f.flagsUsed = flags
	f.sftpfile = file

	return file, nil
}

func (f *File) _open(flags int) (ReadWriteSeekCloser, error) {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return nil, err
	}
	// normally we'd do a defer of fs connTimerStart() here but not necessary since we handle it in the openFile caller

	if flags&os.O_CREATE != 0 {
		// vfs specifies that all implementations make dir path if it doesn't exist
		err = client.MkdirAll(path.Dir(f.path))
		if err != nil {
			return nil, err
		}
	}

	var opener fileOpener
	if f.opener != nil {
		opener = f.opener
	} else {
		opener = defaultOpenFile
	}

	rwsc, err := opener(client, f.Path(), flags)
	if err != nil {
		return nil, err
	}

	// chmod file if default permissions are set and opening for write
	if flags&os.O_WRONLY != 0 {
		err = f.setPermissions(client, f.fileSystem.options)
		if err != nil {
			return nil, err
		}
	}

	return rwsc, nil
}

// setPermissions sets the file permissions if they are set in the options
func (f *File) setPermissions(client Client, opts vfs.Options) error {
	if opts == nil {
		return nil
	}

	// ensure we're dealing with pointer to Options
	ptrOpts, ok := opts.(*Options)
	if !ok {
		p := opts.(Options)
		ptrOpts = &p
	}

	// if file permissions are not set, return early
	if ptrOpts.FilePermissions == nil {
		return nil
	}

	// get file mode
	perms, err := ptrOpts.GetFileMode()
	if err != nil {
		return fmt.Errorf("get file mode err: %w", err)
	}

	if err := client.Chmod(f.Path(), *perms); err != nil {
		return fmt.Errorf("chmod err: %w", err)
	}

	return nil
}

// defaultOpenFile uses sftp.Client to open a file and returns an sftp.File
func defaultOpenFile(c Client, p string, f int) (ReadWriteSeekCloser, error) {
	return c.OpenFile(p, f)
}

func (f *File) sftpRename(target *File) error {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()

	if err := client.Rename(f.Path(), target.Path()); err != nil {
		return err
	}
	return nil
}

// ReadWriteSeekCloser is a read write seek closer interface representing capabilities needed from std libs sftp File struct.
type ReadWriteSeekCloser interface {
	io.ReadWriteSeeker
	io.Closer
	// sftp.File also provides the following which we don't use (but could):
	//
	// io.WriterTo
	// io.ReaderFrom
	// func (f *File) Chmod(mode os.FileMode) error
	// func (f *File) Chown(uid, gid int) error
	// func (f *File) Name() string
	// func (f *File) Stat() (os.FileInfo, error)
	// func (f *File) Truncate(size int64) error
}
