package sftp

import (
	"io"
	"os"
	"path"
	"time"

	"github.com/c2fo/vfs/v6"
	"github.com/c2fo/vfs/v6/utils"
)

// File implements vfs.File interface for SFTP fs.
type File struct {
	fileSystem *FileSystem
	Authority  utils.Authority
	path       string
	sftpfile   ReadWriteSeekCloser
	opener     fileOpener
}

// this type allow for injecting a mock fileOpener function
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

// Path return the directory portion of the file's path. IE: "path/to" of "sftp://someuser@host.com/some/path/to/file.txt
func (f *File) Path() string {
	return utils.EnsureLeadingSlash(f.path)
}

// Exists returns a boolean of whether or not the file exists on the sftp server
func (f *File) Exists() (bool, error) {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return false, err
	}
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()

	_, err = client.Stat(f.Path())
	if err != nil && err == os.ErrNotExist {
		return false, nil
	} else if err != nil {
		return false, err
	}

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
		// restart timer once action is completed
		f.fileSystem.connTimerStop()
		defer f.fileSystem.connTimerStart()
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
	// start timer once action is completed
	defer f.fileSystem.connTimerStart()
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
// we'll do a an io.Copy to the destination file then delete source file.
func (f *File) MoveToFile(t vfs.File) error {
	// validate seek is at 0,0 before doing copy
	// TODO: Fix this later
	// if err := backend.ValidateCopySeekPosition(f); err != nil {
	//	  return err
	// }
	// sftp rename if vfs is sftp and for the same user/host
	if f.fileSystem.Scheme() == t.Location().FileSystem().Scheme() &&
		f.Authority.User == t.(*File).Authority.User &&
		f.Authority.Host == t.(*File).Authority.Host {
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
func (f *File) CopyToFile(file vfs.File) error {
	// validate seek is at 0,0 before doing copy
	// TODO: Fix this later
	// if err := backend.ValidateCopySeekPosition(f); err != nil {
	//  	return err
	// }

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
func (f *File) Delete() error {
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

	if f.sftpfile != nil {
		err := f.sftpfile.Close()
		if err != nil {
			return err
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
		return 0, err
	}

	return sftpfile.Read(p)
}

// Seek calls the underlying sftp.File Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	sftpfile, err := f.openFile(os.O_RDWR)
	if err != nil {
		return 0, err
	}

	return sftpfile.Seek(offset, whence)
}

// Write calls the underlying sftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {
	// restart timer once action is completed
	f.fileSystem.connTimerStop()
	defer f.fileSystem.connTimerStart()

	sftpfile, err := f.openFile(os.O_WRONLY | os.O_CREATE)
	if err != nil {
		return 0, err
	}

	return sftpfile.Write(data)
}

// URI returns the File's URI as a string.
func (f *File) URI() string {
	return utils.GetFileURI(f)
}

// String implement fmt.Stringer, returning the file's URI as the default string.
func (f *File) String() string {
	return f.URI()
}

/*
	Private helper functions
*/

// openFile wrapper allows us to inject a file opener (for mocking) vs the defaultOpenFile.
func (f *File) openFile(flag int) (ReadWriteSeekCloser, error) {
	if f.sftpfile != nil {
		return f.sftpfile, nil
	}

	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return nil, err
	}
	// normally we'd do a defer of fs connTimerStart() here but not necessary since we handle it in the openFile caller

	if flag&os.O_CREATE != 0 {
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

	file, err := opener(client, f.Path(), flag)
	if err != nil {
		return nil, err
	}

	f.sftpfile = file
	return file, nil
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
