package sftp

import (
	"os"
	"path"
	"time"

	"github.com/c2fo/vfs/v5"
	"github.com/c2fo/vfs/v5/utils"
	"github.com/pkg/sftp"
)

//File implements vfs.File interface for SFTP fs.
type File struct {
	fileSystem  *FileSystem
	Authority   utils.Authority
	path        string
	file        *sftp.File
}

// Info Functions

// LastModified returns the LastModified property of sftp file.
func (f *File) LastModified() (*time.Time, error) {

	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return nil, err
	}
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

	_, err = client.OpenFile(f.Path(), os.O_RDONLY)
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
		file, err := f.openFile()
		if err != nil {
			return err
		}
		f.file = file
		//TODO not sure if this works at all ************************** verify a file is written
		return f.Close()
	}

	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}
	now := time.Now()
	return client.Chtimes(f.Path(), now, now)
}

// Size returns the size of the remote file.
func (f *File) Size() (uint64, error) {

	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return 0, err
	}
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
func (f *File) MoveToFile(file vfs.File) error {
	// sftp rename if vfs is sftp and for the same user/host
	if tf, ok := file.(*File); ok &&
		f.Authority.User == tf.Authority.User &&
		f.Authority.Host == tf.Authority.Host {
		//ensure destination exists before moving
		exists, err := tf.Location().Exists()
		if err != nil {
			return err
		}
		if !exists {
			client, err := f.fileSystem.Client(f.Authority)
			if err != nil {
				return err
			}
			err = client.MkdirAll(tf.Location().Path())
			if err != nil {
				return err
			}
		}
		return f.sftpRename(tf)
	}

	//otherwise do copy-delete
	if err := f.CopyToFile(file); err != nil {
		return err
	}
	return f.Delete()
}

// MoveToLocation works by creating a newfile on the target location then calling MoveToFile() on it.
func (f *File) MoveToLocation(location vfs.Location) (vfs.File, error) {

	newFile, err := location.FileSystem().NewFile(location.Volume(), path.Join(location.Path(), f.Name()))
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

	if err := utils.TouchCopy(file, f); err != nil {
		return err
	}
	//Close target to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := file.Close(); cerr != nil {
		return cerr
	}
	//Close file (f) reader
	return f.Close()
}

// CopyToLocation creates a copy of *File, using the file's current path as the new file's
// path at the given location.
func (f *File) CopyToLocation(location vfs.Location) (vfs.File, error) {

	newFile, err := location.FileSystem().NewFile(location.Volume(), path.Join(location.Path(), f.Name()))
	if err != nil {
		return nil, err
	}

	if err := utils.TouchCopy(newFile, f); err != nil {
		return nil, err
	}
	//Close target file to flush and ensure that cursor isn't at the end of the file when the caller reopens for read
	if cerr := newFile.Close(); cerr != nil {
		return nil, cerr
	}
	//Close file (f) reader
	if cerr := f.Close(); cerr != nil {
		return nil, cerr
	}
	return newFile, nil
}

// CRUD Operations

// Delete removes the remote file.  Error is returned, if any.
func (f *File) Delete() error {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}
	return client.Remove(f.Path())
}

// Close calls the underlying sftp.File Close, if opened, and clears the internal pointer
func (f *File) Close() error {
	if f.file != nil {
		err := f.file.Close()
		if err != nil {
			return err
		}
		f.file = nil
	}
	//no op for unopened file
	return nil
}

// Read calls the underlying sftp.File Read.
func (f *File) Read(p []byte) (n int, err error) {

	//ensure destination exists before reading (because openFile will auto-vivify the File)
	exists, err := f.Exists()
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, os.ErrNotExist
	}

	file, err := f.openFile()
	if err != nil {
		return 0, err
	}
	return file.Read(p)
}

// Seek calls the underlying sftp.File Seek.
func (f *File) Seek(offset int64, whence int) (int64, error) {

	//ensure destination exists before seeking (because openFile() will auto-vivify the File)
	exists, err := f.Exists()
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, os.ErrNotExist
	}

	file, err := f.openFile()
	if err != nil {
		return 0, err
	}
	return file.Seek(offset, whence)
}

// Write calls the underlying sftp.File Write.
func (f *File) Write(data []byte) (res int, err error) {

	file, err := f.openFile()
	if err != nil {
		return 0, err
	}

	return file.Write(data)
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
func (f *File) openFile() (*sftp.File, error) {
	if f.file != nil {
		return f.file, nil
	}

	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return nil, err
	}

	//vfs specifies that all implementations make dir path if it doesn't exist
	err = client.MkdirAll(path.Dir(f.path))
	if err != nil {
		return nil, err
	}

	file, err := client.OpenFile(f.path, os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, err
	}
	f.file = file
	return file, nil
}

func (f *File) sftpRename(target *File) error {
	client, err := f.fileSystem.Client(f.Authority)
	if err != nil {
		return err
	}
	if err := client.Rename(f.Path(), target.Path()); err != nil {
		return err
	}
	return nil
}
